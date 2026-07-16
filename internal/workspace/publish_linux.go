//go:build linux

package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	maxTreeEntries = 1024
	maxTreeDepth   = 64
)

// PublishTree writes a complete tree to a unique sibling directory and makes
// it visible with one no-replace rename. An interrupted stage is never a
// partially published destination and cannot collide with a later retry.
func (root *Root) PublishTree(name string, files map[string][]byte) error {
	if err := validateManagedPath(name); err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("cannot publish an empty tree")
	}
	wantEntries := expectedTreeEntries(files)
	if wantEntries == nil {
		return ErrInvalidPath
	}
	if len(wantEntries) > maxTreeEntries {
		return fmt.Errorf("tree exceeds maximum entry count: %w", ErrTooLarge)
	}
	for fileName, content := range files {
		if strings.Count(fileName, "/") > maxTreeDepth {
			return fmt.Errorf("tree exceeds maximum depth: %w", ErrTooLarge)
		}
		if len(content) > MaxFileBytes {
			return fmt.Errorf("publish %q: %w", fileName, ErrTooLarge)
		}
	}

	root.mu.RLock()
	defer root.mu.RUnlock()
	parentFD, base, err := root.openParentLocked(name, true)
	if err != nil {
		return err
	}
	defer syscall.Close(parentFD)
	if err := destinationAbsent(parentFD, base); err != nil {
		return fmt.Errorf("publish %q: %w", name, err)
	}
	stage, err := stagingName()
	if err != nil {
		return err
	}
	if err := syscall.Mkdirat(parentFD, stage, 0o700); err != nil {
		return fmt.Errorf("create staging directory for %q: %w", name, err)
	}
	stageFD, err := syscall.Openat(parentFD, stage, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open staging directory for %q: %w", name, err)
	}
	defer syscall.Close(stageFD)
	if err := syscall.Fchmod(stageFD, 0o700); err != nil {
		return fmt.Errorf("secure staging directory for %q: %w", name, err)
	}

	names := make([]string, 0, len(files))
	for fileName := range files {
		names = append(names, fileName)
	}
	sort.Strings(names)
	for _, fileName := range names {
		if err := writeStagedFile(stageFD, fileName, files[fileName]); err != nil {
			return fmt.Errorf("stage %q: %w", fileName, err)
		}
	}
	gotEntries, err := listTreeFD(stageFD)
	if err != nil {
		return fmt.Errorf("verify staged tree: %w", err)
	}
	if !slices.Equal(gotEntries, wantEntries) {
		return errors.New("staged tree contains missing or unlisted entries")
	}
	for _, fileName := range names {
		content, err := readFileAt(stageFD, fileName)
		if err != nil {
			return fmt.Errorf("verify staged file %q: %w", fileName, err)
		}
		if !bytes.Equal(content, files[fileName]) {
			return fmt.Errorf("verify staged file %q: content changed", fileName)
		}
	}
	if err := syscall.Fsync(stageFD); err != nil {
		return fmt.Errorf("sync staging directory for %q: %w", name, err)
	}
	if err := renameNoReplace(parentFD, stage, base); err != nil {
		if errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("publish %q: %w", name, ErrFinalized)
		}
		return fmt.Errorf("publish staged tree %q: %w", name, err)
	}
	if err := syscall.Fsync(parentFD); err != nil {
		return fmt.Errorf("sync parent of %q: %w", name, err)
	}
	return root.revalidateLocked()
}

// ListTree returns every rooted entry below name. Directories have a trailing
// slash. Enumeration never follows links and rejects special or hardlinked
// files so callers can compare the result with an allowlist.
func (root *Root) ListTree(name string) ([]string, error) {
	root.mu.RLock()
	defer root.mu.RUnlock()
	fd, err := root.openDirectoryLocked(name, false)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(fd)
	entries, err := listTreeFD(fd)
	if err != nil {
		return nil, err
	}
	if err := root.revalidateLocked(); err != nil {
		return nil, err
	}
	return entries, nil
}

func stagingName() (string, error) {
	name, err := temporaryName()
	if err != nil {
		return "", err
	}
	return strings.Replace(name, ".reconctx-tmp-", ".reconctx-stage-", 1), nil
}

func destinationAbsent(parentFD int, base string) error {
	var stat unix.Stat_t
	err := unix.Fstatat(parentFD, base, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if err == nil {
		return ErrFinalized
	}
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	return err
}

func writeStagedFile(stageFD int, name string, data []byte) error {
	parentFD, base, err := openRelativeParent(stageFD, name, true)
	if err != nil {
		return err
	}
	defer syscall.Close(parentFD)
	fd, err := syscall.Openat(parentFD, base, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), base)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syscall.Fsync(parentFD)
}

func readFileAt(rootFD int, name string) ([]byte, error) {
	parentFD, base, err := openRelativeParent(rootFD, name, false)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(parentFD)
	fd, err := syscall.Openat(parentFD, base, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, ErrSymlink
		}
		return nil, err
	}
	if err := validateRegularFD(fd); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	file := os.NewFile(uintptr(fd), base)
	content, readErr := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(content) > MaxFileBytes {
		return nil, ErrTooLarge
	}
	return content, nil
}

func openRelativeParent(rootFD int, name string, create bool) (int, string, error) {
	if err := validateManagedPath(name); err != nil {
		return -1, "", err
	}
	directory, base := path.Split(name)
	directory = strings.TrimSuffix(directory, "/")
	fd, err := openRelativeDirectory(rootFD, directory, create)
	return fd, base, err
}

func openRelativeDirectory(rootFD int, name string, create bool) (int, error) {
	current, err := syscall.Dup(rootFD)
	if err != nil {
		return -1, err
	}
	syscall.CloseOnExec(current)
	if name == "" {
		return current, nil
	}
	for _, component := range strings.Split(name, "/") {
		next, openErr := syscall.Openat(current, component, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if openErr != nil && create && errors.Is(openErr, syscall.ENOENT) {
			created := false
			if err := syscall.Mkdirat(current, component, 0o700); err == nil {
				created = true
			} else if !errors.Is(err, syscall.EEXIST) {
				_ = syscall.Close(current)
				return -1, err
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
				return -1, ErrSymlink
			}
			return -1, openErr
		}
		if err := validateDirectoryFD(next); err != nil {
			_ = syscall.Close(next)
			return -1, err
		}
		current = next
	}
	return current, nil
}

func listTreeFD(rootFD int) ([]string, error) {
	var entries []string
	if err := walkTreeFD(rootFD, "", 0, &entries); err != nil {
		return nil, err
	}
	sort.Strings(entries)
	return entries, nil
}

func walkTreeFD(directoryFD int, prefix string, depth int, output *[]string) error {
	if depth > maxTreeDepth {
		return fmt.Errorf("tree exceeds maximum depth: %w", ErrTooLarge)
	}
	duplicate, err := syscall.Openat(directoryFD, ".", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(duplicate), prefix)
	defer file.Close()
	for {
		entries, readErr := file.ReadDir(64)
		for _, entry := range entries {
			if len(*output) >= maxTreeEntries {
				return fmt.Errorf("tree exceeds maximum entry count: %w", ErrTooLarge)
			}
			name := entry.Name()
			if name == "." || name == ".." || strings.Contains(name, "/") {
				return ErrInvalidPath
			}
			relative := name
			if prefix != "" {
				relative = prefix + "/" + name
			}
			var stat unix.Stat_t
			if err := unix.Fstatat(directoryFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return err
			}
			switch stat.Mode & unix.S_IFMT {
			case unix.S_IFDIR:
				child, err := syscall.Openat(directoryFD, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
				if err != nil {
					if errors.Is(err, syscall.ELOOP) {
						return ErrSymlink
					}
					return err
				}
				if err := validateDirectoryFD(child); err != nil {
					_ = syscall.Close(child)
					return err
				}
				*output = append(*output, relative+"/")
				err = walkTreeFD(child, relative, depth+1, output)
				_ = syscall.Close(child)
				if err != nil {
					return err
				}
			case unix.S_IFREG:
				fd, err := syscall.Openat(directoryFD, name, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
				if err != nil {
					return err
				}
				err = validateRegularFD(fd)
				_ = syscall.Close(fd)
				if err != nil {
					return err
				}
				if stat.Size > MaxFileBytes {
					return ErrTooLarge
				}
				*output = append(*output, relative)
			case unix.S_IFLNK:
				return ErrSymlink
			default:
				return ErrSpecialFile
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func expectedTreeEntries(files map[string][]byte) []string {
	set := make(map[string]bool, len(files)*2)
	for name := range files {
		if validateManagedPath(name) != nil {
			return nil
		}
		set[name] = true
		for directory := path.Dir(name); directory != "."; directory = path.Dir(directory) {
			set[directory+"/"] = true
		}
	}
	entries := make([]string, 0, len(set))
	for entry := range set {
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	return entries
}
