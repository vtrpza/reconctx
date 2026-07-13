//go:build linux

package workspace

import "golang.org/x/sys/unix"

func renameNoReplace(dirFD int, oldName, newName string) error {
	return unix.Renameat2(dirFD, oldName, dirFD, newName, unix.RENAME_NOREPLACE)
}
