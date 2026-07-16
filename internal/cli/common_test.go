//go:build linux

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadRegularFileIsBoundedAndRejectsSpecialFiles(t *testing.T) {
	directory := t.TempDir()
	regular := filepath.Join(directory, "input")
	content := []byte("bounded\n")
	if err := os.WriteFile(regular, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readRegularFile(regular, int64(len(content))); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("exact limit: content=%q err=%v", got, err)
	}
	if _, err := readRegularFile(regular, int64(len(content)-1)); err == nil {
		t.Fatal("oversized regular file accepted")
	}

	symlink := filepath.Join(directory, "symlink")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegularFile(symlink, int64(len(content))); err == nil {
		t.Fatal("symbolic link accepted")
	}

	fifo := filepath.Join(directory, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := readRegularFile(fifo, int64(len(content)))
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO read blocked")
	}
}
