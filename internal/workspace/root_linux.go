//go:build linux

package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

const maxReadFileBytes = 16 << 20

type Root struct {
	mu   sync.RWMutex
	path string
	fd   int
	dev  uint64
	ino  uint64
}

func Open(name string) (*Root, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("workspace root: %w", err)
	}
	fd, err := openRootPath(absolute)
	if err != nil {
		return nil, fmt.Errorf("open workspace root: %w", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("stat workspace root: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("workspace root: %w", ErrSpecialFile)
	}
	if stat.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("workspace root permissions %o: %w", stat.Mode&0o777, ErrUnsafePermissions)
	}
	return &Root{path: absolute, fd: fd, dev: uint64(stat.Dev), ino: stat.Ino}, nil
}

func openRootPath(name string) (int, error) {
	fd, err := unix.Openat2(unix.AT_FDCWD, name, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.E2BIG) {
		return -1, ErrUnsupported
	}
	if errors.Is(err, unix.ELOOP) {
		return -1, ErrSymlink
	}
	return fd, err
}

func (root *Root) Close() error {
	root.mu.Lock()
	defer root.mu.Unlock()
	if root.fd < 0 {
		return nil
	}
	err := syscall.Close(root.fd)
	root.fd = -1
	return err
}

func (root *Root) MkdirAll(name string) error {
	root.mu.RLock()
	defer root.mu.RUnlock()
	fd, err := root.openDirectoryLocked(name, true)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	if err := syscall.Fsync(fd); err != nil {
		return fmt.Errorf("sync directory %q: %w", name, err)
	}
	return root.revalidateLocked()
}

func (root *Root) CreateRunDir(runID string) error {
	if validateManagedPath(runID) != nil || strings.Contains(runID, "/") || !strings.HasPrefix(runID, "run_") {
		return fmt.Errorf("run ID %q: %w", runID, ErrInvalidPath)
	}
	root.mu.RLock()
	defer root.mu.RUnlock()
	if err := root.revalidateLocked(); err != nil {
		return err
	}
	runsFD, err := root.openDirectoryLocked("runs", true)
	if err != nil {
		return err
	}
	defer syscall.Close(runsFD)
	if err := syscall.Mkdirat(runsFD, runID, 0o700); err != nil {
		if errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("run directory %q: %w", runID, ErrFinalized)
		}
		return fmt.Errorf("create run directory %q: %w", runID, err)
	}
	createdFD, err := syscall.Openat(runsFD, runID, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open new run directory %q: %w", runID, err)
	}
	if err := syscall.Fchmod(createdFD, 0o700); err != nil {
		_ = syscall.Close(createdFD)
		return fmt.Errorf("secure run directory %q: %w", runID, err)
	}
	if err := syscall.Fsync(createdFD); err != nil {
		_ = syscall.Close(createdFD)
		return fmt.Errorf("sync run directory %q: %w", runID, err)
	}
	_ = syscall.Close(createdFD)
	if err := syscall.Fsync(runsFD); err != nil {
		return fmt.Errorf("sync runs directory: %w", err)
	}
	return root.revalidateLocked()
}

func (root *Root) ReadFile(name string) ([]byte, error) {
	root.mu.RLock()
	defer root.mu.RUnlock()
	parentFD, base, err := root.openParentLocked(name, false)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(parentFD)
	fd, err := syscall.Openat(parentFD, base, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("read %q: %w", name, ErrSymlink)
		}
		return nil, fmt.Errorf("open %q: %w", name, err)
	}
	if err := validateRegularFD(fd); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("read %q: %w", name, err)
	}
	file := os.NewFile(uintptr(fd), base)
	content, readErr := io.ReadAll(io.LimitReader(file, maxReadFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read %q: %w", name, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %q: %w", name, closeErr)
	}
	if len(content) > maxReadFileBytes {
		return nil, fmt.Errorf("read %q: %w", name, ErrTooLarge)
	}
	if err := root.revalidateLocked(); err != nil {
		return nil, err
	}
	return content, nil
}

func (root *Root) atomicWrite(name string, data []byte, replace bool) error {
	root.mu.RLock()
	defer root.mu.RUnlock()
	parentFD, base, err := root.openParentLocked(name, true)
	if err != nil {
		return err
	}
	defer syscall.Close(parentFD)
	if replace {
		if err := validateReplaceDestination(parentFD, base); err != nil {
			return fmt.Errorf("replace %q: %w", name, err)
		}
	}

	temporary, err := temporaryName()
	if err != nil {
		return err
	}
	fd, err := syscall.Openat(parentFD, temporary, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", name, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = syscall.Unlinkat(parentFD, temporary)
		}
	}()
	file := os.NewFile(uintptr(fd), temporary)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary file for %q: %w", name, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure temporary file for %q: %w", name, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync temporary file for %q: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file for %q: %w", name, err)
	}
	if replace {
		err = syscall.Renameat(parentFD, temporary, parentFD, base)
	} else {
		err = renameNoReplace(parentFD, temporary, base)
	}
	if err != nil {
		if !replace && errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("publish %q: %w", name, ErrFinalized)
		}
		return fmt.Errorf("publish %q: %w", name, err)
	}
	cleanup = false
	if err := syscall.Fsync(parentFD); err != nil {
		return fmt.Errorf("sync parent of %q: %w", name, err)
	}
	return root.revalidateLocked()
}

func (root *Root) openParentLocked(name string, create bool) (int, string, error) {
	if err := validateManagedPath(name); err != nil {
		return -1, "", err
	}
	directory, base := path.Split(name)
	directory = strings.TrimSuffix(directory, "/")
	if directory == "" {
		fd, err := syscall.Dup(root.fd)
		if err != nil {
			return -1, "", err
		}
		syscall.CloseOnExec(fd)
		if err := root.revalidateLocked(); err != nil {
			_ = syscall.Close(fd)
			return -1, "", err
		}
		return fd, base, nil
	}
	fd, err := root.openDirectoryLocked(directory, create)
	return fd, base, err
}

func (root *Root) openDirectoryLocked(name string, create bool) (int, error) {
	if err := validateManagedPath(name); err != nil {
		return -1, err
	}
	if err := root.revalidateLocked(); err != nil {
		return -1, err
	}
	current, err := syscall.Dup(root.fd)
	if err != nil {
		return -1, fmt.Errorf("duplicate workspace root descriptor: %w", err)
	}
	syscall.CloseOnExec(current)
	for _, component := range strings.Split(name, "/") {
		next, openErr := syscall.Openat(current, component, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if openErr != nil && create && errors.Is(openErr, syscall.ENOENT) {
			created := false
			if err := syscall.Mkdirat(current, component, 0o700); err == nil {
				created = true
			} else if !errors.Is(err, syscall.EEXIST) {
				_ = syscall.Close(current)
				return -1, fmt.Errorf("create directory component %q: %w", component, err)
			}
			next, openErr = syscall.Openat(current, component, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
			if openErr == nil && created {
				openErr = syscall.Fchmod(next, 0o700)
				if openErr == nil {
					openErr = syscall.Fsync(current)
				}
			}
		}
		_ = syscall.Close(current)
		if openErr != nil {
			if next >= 0 {
				_ = syscall.Close(next)
			}
			if errors.Is(openErr, syscall.ELOOP) || errors.Is(openErr, syscall.ENOTDIR) {
				return -1, fmt.Errorf("directory component %q: %w", component, ErrSymlink)
			}
			return -1, fmt.Errorf("open directory component %q: %w", component, openErr)
		}
		if err := validateDirectoryFD(next); err != nil {
			_ = syscall.Close(next)
			return -1, fmt.Errorf("directory component %q: %w", component, err)
		}
		current = next
	}
	return current, nil
}

func (root *Root) revalidateLocked() error {
	if root.fd < 0 {
		return os.ErrClosed
	}
	fd, err := openRootPath(root.path)
	if err != nil {
		return ErrRootChanged
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || uint64(stat.Dev) != root.dev || stat.Ino != root.ino {
		return ErrRootChanged
	}
	if stat.Mode&0o077 != 0 {
		return ErrUnsafePermissions
	}
	return nil
}

func validateManagedPath(name string) error {
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\\x00") {
		return fmt.Errorf("managed path %q: %w", name, ErrInvalidPath)
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("managed path %q: %w", name, ErrInvalidPath)
		}
	}
	return nil
}

func validateDirectoryFD(fd int) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return ErrSpecialFile
	}
	if stat.Mode&0o077 != 0 {
		return fmt.Errorf("directory permissions %o: %w", stat.Mode&0o777, ErrUnsafePermissions)
	}
	return nil
}

func validateRegularFD(fd int) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		return ErrSpecialFile
	}
	if stat.Nlink != 1 {
		return ErrUnsafeHardlink
	}
	return nil
}

func validateReplaceDestination(parentFD int, base string) error {
	fd, err := syscall.Openat(parentFD, base, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return ErrSymlink
		}
		return err
	}
	defer syscall.Close(fd)
	return validateRegularFD(fd)
}

func temporaryName() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate temporary file name: %w", err)
	}
	return ".reconctx-tmp-" + hex.EncodeToString(random[:]), nil
}
