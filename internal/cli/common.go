package cli

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/vtrpza/reconctx/internal/integrity"
)

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }
func (values *stringList) Set(value string) error {
	if value == "" {
		return errors.New("value cannot be empty")
	}
	*values = append(*values, value)
	return nil
}

func randomID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value[:]), nil
}

func readRegularFile(name string, limit int64) ([]byte, error) {
	fd, err := syscall.Open(name, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("open input: invalid file descriptor")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return nil, errors.New("input is not a bounded regular file")
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) != info.Size() {
		return nil, errors.New("input changed while reading")
	}
	return content, nil
}

func openWorkspace(name string) (string, bool, error) {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name || strings.ContainsRune(name, '\x00') {
		return "", false, errors.New("workspace must be an absolute clean path")
	}
	absolute := name
	created := false
	if _, err := os.Lstat(absolute); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(absolute, 0o700); err != nil {
			return "", false, err
		}
		created = true
	} else if err != nil {
		return "", false, err
	}
	return absolute, created, nil
}

func workspaceRelative(root, name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return "", err
	}
	relative = filepath.ToSlash(relative)
	if err := integrity.ValidateRelativePath(relative); err != nil {
		return "", fmt.Errorf("path must remain inside workspace: %w", err)
	}
	return relative, nil
}
