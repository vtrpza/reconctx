//go:build linux

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicExclusiveWriteIsPrivateAndFinal(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := root.WriteFileExclusive("runs/run_test/metadata.json", []byte("first")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "runs", "run_test", "metadata.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first" {
		t.Fatalf("content = %q", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %o, want 600", got)
	}
	if err := root.WriteFileExclusive("runs/run_test/metadata.json", []byte("second")); !errors.Is(err, ErrFinalized) {
		t.Fatalf("second write error = %v, want ErrFinalized", err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first" {
		t.Fatalf("finalized content changed to %q", content)
	}
	assertNoTemporaryFiles(t, filepath.Dir(path))
}

func TestAtomicReplaceUpdatesRegularMetadata(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := root.ReplaceFile("state/run.json", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := root.ReplaceFile("state/run.json", []byte("two")); err != nil {
		t.Fatal(err)
	}
	content, err := root.ReadFile("state/run.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "two" {
		t.Fatalf("content = %q, want two", content)
	}
	assertNoTemporaryFiles(t, filepath.Join(directory, "state"))
}

func TestAtomicReplaceRejectsSymlinkAndHardlinkDestinations(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := root.MkdirAll("state"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "state", "symlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "state", "linked"), []byte("linked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(directory, "state", "linked"), filepath.Join(directory, "state", "linked-copy")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"state/symlink", "state/linked"} {
		if err := root.ReplaceFile(name, []byte("new")); err == nil {
			t.Errorf("ReplaceFile(%q) succeeded", name)
		}
	}
	content, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "outside" {
		t.Fatalf("outside content changed to %q", content)
	}
}

func TestReplaceFileCannotOverwriteFinalizedArtifact(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := root.WriteFileExclusive("raw/evidence.jsonl", []byte("final")); err != nil {
		t.Fatal(err)
	}
	if err := root.ReplaceFile("raw/evidence.jsonl", []byte("changed")); !errors.Is(err, ErrFinalized) {
		t.Fatalf("ReplaceFile error = %v, want ErrFinalized", err)
	}
	content, err := root.ReadFile("raw/evidence.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "final" {
		t.Fatalf("finalized artifact changed to %q", content)
	}
}

func TestAtomicWritesRejectUnreadableSize(t *testing.T) {
	t.Parallel()
	root := openTestRoot(t, t.TempDir())
	tooLarge := make([]byte, MaxFileBytes+1)
	if err := root.WriteFileExclusive("raw/large", tooLarge); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("exclusive write error = %v, want ErrTooLarge", err)
	}
	if err := root.ReplaceFile("state/large", tooLarge); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("replace error = %v, want ErrTooLarge", err)
	}
}

func assertNoTemporaryFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".reconctx-tmp-") {
			t.Errorf("temporary file remains: %s", entry.Name())
		}
	}
}
