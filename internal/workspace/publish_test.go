//go:build linux

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

func TestPublishTreeIsAtomicFinalAndIgnoresStaleStages(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := root.MkdirAll("handoff/.reconctx-stage-stale"); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"CONTEXT.md":               []byte("context\n"),
		"normalized/records.jsonl": []byte("{}\n"),
	}
	if err := root.PublishTree("handoff/run_test", files); err != nil {
		t.Fatal(err)
	}
	want := []string{"CONTEXT.md", "normalized/", "normalized/records.jsonl"}
	if got, err := root.ListTree("handoff/run_test"); err != nil || !slices.Equal(got, want) {
		t.Fatalf("published entries = %v, %v", got, err)
	}
	if err := root.PublishTree("handoff/run_test", files); !errors.Is(err, ErrFinalized) {
		t.Fatalf("second publish error = %v, want ErrFinalized", err)
	}
}

func TestPublishTreeRejectsPreexistingPartialDestination(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := root.WriteFileExclusive("handoff/run_test/partial", []byte("partial")); err != nil {
		t.Fatal(err)
	}
	if err := root.PublishTree("handoff/run_test", map[string][]byte{"complete": []byte("complete")}); !errors.Is(err, ErrFinalized) {
		t.Fatalf("publish error = %v, want ErrFinalized", err)
	}
	content, err := root.ReadFile("handoff/run_test/partial")
	if err != nil || string(content) != "partial" {
		t.Fatalf("partial destination changed: %q, %v", content, err)
	}
}

func TestListTreeExposesExtrasAndRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()
	t.Run("extra", func(t *testing.T) {
		directory := t.TempDir()
		root := openTestRoot(t, directory)
		if err := root.PublishTree("handoff/run_test", map[string][]byte{"listed": []byte("ok")}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "handoff", "run_test", "extra"), []byte("extra"), 0o600); err != nil {
			t.Fatal(err)
		}
		entries, err := root.ListTree("handoff/run_test")
		if err != nil || !slices.Contains(entries, "extra") {
			t.Fatalf("entries = %v, %v", entries, err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		root := openTestRoot(t, directory)
		if err := root.MkdirAll("handoff/run_test"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("outside", filepath.Join(directory, "handoff", "run_test", "link")); err != nil {
			t.Fatal(err)
		}
		if _, err := root.ListTree("handoff/run_test"); !errors.Is(err, ErrSymlink) {
			t.Fatalf("ListTree error = %v, want ErrSymlink", err)
		}
	})
	t.Run("special", func(t *testing.T) {
		directory := t.TempDir()
		root := openTestRoot(t, directory)
		if err := root.MkdirAll("handoff/run_test"); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(filepath.Join(directory, "handoff", "run_test", "pipe"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := root.ListTree("handoff/run_test"); !errors.Is(err, ErrSpecialFile) {
			t.Fatalf("ListTree error = %v, want ErrSpecialFile", err)
		}
	})
}

func TestListTreeIsDepthBounded(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := openTestRoot(t, directory)
	deep := "deep/" + strings.Repeat("d/", maxTreeDepth+1) + "file"
	if err := root.WriteFileExclusive(strings.TrimSuffix(deep, "/"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := root.ListTree("deep"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ListTree error = %v, want ErrTooLarge", err)
	}
}

func TestPublishTreeRejectsEntryOverflowBeforeStaging(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := openTestRoot(t, directory)
	files := make(map[string][]byte, maxTreeEntries+1)
	for index := 0; index <= maxTreeEntries; index++ {
		files[fmt.Sprintf("file-%04d", index)] = []byte("x")
	}
	if err := root.PublishTree("handoff/run_test", files); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("PublishTree error = %v, want ErrTooLarge", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "handoff")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("entry overflow created staging state: %v", err)
	}
}

func TestListTreeFDDoesNotReuseCallerOffset(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "file"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if _, err := opened.ReadDir(-1); err != nil {
		t.Fatal(err)
	}
	if entries, err := listTreeFD(int(opened.Fd())); err != nil || !slices.Equal(entries, []string{"file"}) {
		t.Fatalf("entries = %v, %v", entries, err)
	}
}
