package workspace

import (
	"errors"
	"strings"
)

var (
	ErrInvalidPath       = errors.New("invalid managed path")
	ErrSymlink           = errors.New("managed path contains a symbolic link")
	ErrSpecialFile       = errors.New("managed path is not a regular file or directory")
	ErrUnsafeHardlink    = errors.New("managed file has multiple hard links")
	ErrUnsafePermissions = errors.New("managed path permissions are not private")
	ErrRootChanged       = errors.New("trusted workspace root identity changed")
	ErrFinalized         = errors.New("artifact is already finalized")
	ErrTooLarge          = errors.New("managed file exceeds the read limit")
	ErrUnsupported       = errors.New("safe rooted workspace operations are unsupported on this platform")
)

// MaxFileBytes is the largest managed file that can be read back and verified.
const MaxFileBytes = 16 << 20

// WriteFileExclusive atomically publishes a new finalized file and refuses to
// replace any existing directory entry.
func (root *Root) WriteFileExclusive(name string, data []byte) error {
	if len(data) > MaxFileBytes {
		return ErrTooLarge
	}
	return root.atomicWrite(name, data, false)
}

// ReplaceFile atomically replaces a regular, single-link metadata file. It
// never follows the destination when validating or replacing it.
func (root *Root) ReplaceFile(name string, data []byte) error {
	if !strings.HasPrefix(name, "state/") {
		return ErrFinalized
	}
	if len(data) > MaxFileBytes {
		return ErrTooLarge
	}
	return root.atomicWrite(name, data, true)
}
