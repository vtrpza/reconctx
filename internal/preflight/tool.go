package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

type ToolIdentity struct {
	ResolvedPath string
	SHA256       string
	Mode         os.FileMode
	UID          uint32
	GID          uint32
	Device       uint64
	Inode        uint64
}

func ResolveTool(candidate string) (ToolIdentity, error) {
	if candidate == "" || strings.ContainsRune(candidate, '\x00') {
		return ToolIdentity{}, errors.New("tool path is empty or contains NUL")
	}
	if !filepath.IsAbs(candidate) {
		resolved, err := exec.LookPath(candidate)
		if err != nil {
			return ToolIdentity{}, err
		}
		candidate = resolved
	}
	candidate, err := filepath.Abs(candidate)
	if err != nil {
		return ToolIdentity{}, err
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return ToolIdentity{}, err
	}

	file, err := os.Open(resolved)
	if err != nil {
		return ToolIdentity{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ToolIdentity{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return ToolIdentity{}, errors.New("tool must be a regular executable file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ToolIdentity{}, errors.New("tool file identity unavailable")
	}
	if err := validateToolPath(resolved, stat); err != nil {
		return ToolIdentity{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ToolIdentity{}, err
	}
	return ToolIdentity{
		ResolvedPath: resolved,
		SHA256:       "sha256:" + hex.EncodeToString(hash.Sum(nil)),
		Mode:         info.Mode().Perm(),
		UID:          stat.Uid,
		GID:          stat.Gid,
		Device:       uint64(stat.Dev),
		Inode:        stat.Ino,
	}, nil
}

func validateToolPath(path string, opened *syscall.Stat_t) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("tool path contains symlink after resolution: %s", current)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.New("tool path identity unavailable")
		}
		if current == path && (stat.Dev != opened.Dev || stat.Ino != opened.Ino) {
			return errors.New("tool changed during preflight")
		}
		if stat.Uid != 0 && stat.Uid != uint32(os.Getuid()) {
			return fmt.Errorf("tool path has unexpected owner: %s", current)
		}
		writable := info.Mode().Perm()&0o022 != 0
		rootStickyDirectory := info.IsDir() && stat.Uid == 0 && info.Mode()&os.ModeSticky != 0
		if writable && !rootStickyDirectory {
			return fmt.Errorf("tool path is group/world writable: %s", current)
		}
		if current == filepath.Dir(current) {
			return nil
		}
	}
}
