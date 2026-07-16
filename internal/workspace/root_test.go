//go:build linux

package workspace

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/vtrpza/reconctx/internal/integrity"
)

func TestRootRejectsTraversalAndSymlinks(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	root := openTestRoot(t, directory)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "file"), filepath.Join(directory, "final")); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"/absolute", "../escape", "safe/../../escape", "link/file", "final"} {
		if err := root.WriteFileExclusive(name, []byte("unsafe")); err == nil {
			t.Errorf("WriteFileExclusive(%q) succeeded", name)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "file")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file status = %v, want not exist", err)
	}
}

func TestRootRejectsSpecialFilesAndUnsafeHardlinks(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := syscall.Mkfifo(filepath.Join(directory, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	names := []string{"pipe", "original", "hardlink"}
	socketFD, socketErr := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if socketErr == nil {
		t.Cleanup(func() { _ = syscall.Close(socketFD) })
		socketErr = syscall.Bind(socketFD, &syscall.SockaddrUnix{Name: filepath.Join(directory, "socket")})
	}
	if socketErr == nil {
		names = append(names, "socket")
	} else if errors.Is(socketErr, syscall.EPERM) {
		t.Log("Unix socket fixture unavailable in sandbox: operation not permitted")
	} else {
		t.Fatal(socketErr)
	}
	if err := os.WriteFile(filepath.Join(directory, "original"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(directory, "original"), filepath.Join(directory, "hardlink")); err != nil {
		t.Fatal(err)
	}

	for _, name := range names {
		if _, err := root.ReadFile(name); err == nil {
			t.Errorf("ReadFile(%q) succeeded", name)
		}
	}
}

func TestRootCreatesPrivateUniqueRunDirectories(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := root.CreateRunDir("run_test"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(directory, "runs", "run_test"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("run directory mode = %o, want 700", got)
	}
	if err := root.CreateRunDir("run_test"); err == nil {
		t.Fatal("duplicate run directory creation succeeded")
	}
	if err := root.CreateRunDir("../escape"); err == nil {
		t.Fatal("traversing run ID succeeded")
	}
}

func TestOpenRejectsNonPrivateRoot(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory); !errors.Is(err, ErrUnsafePermissions) {
		t.Fatalf("Open error = %v, want ErrUnsafePermissions", err)
	}
}

func TestOpenRejectsAncestorSymlink(t *testing.T) {
	t.Parallel()
	realParent := t.TempDir()
	workspace := filepath.Join(realParent, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	linkParent := filepath.Join(t.TempDir(), "linked-parent")
	if err := os.Symlink(realParent, linkParent); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(linkParent, "workspace")); err == nil {
		t.Fatal("Open accepted a workspace beneath an ancestor symlink")
	}
}

func TestReadFileRejectsOversizeInput(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := openTestRoot(t, directory)
	file, err := os.OpenFile(filepath.Join(directory, "large"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxFileBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := root.ReadFile("large"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ReadFile error = %v, want ErrTooLarge", err)
	}
}

func TestRootFailsIfTrustedDirectoryIdentityChanges(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	directory := filepath.Join(parent, "workspace")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	root := openTestRoot(t, directory)
	if err := os.Rename(directory, directory+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := root.MkdirAll("runs"); !errors.Is(err, ErrRootChanged) {
		t.Fatalf("MkdirAll error = %v, want ErrRootChanged", err)
	}
}

func TestRootFailsIfPermissionsBecomePublic(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := openTestRoot(t, directory)
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := root.MkdirAll("runs"); !errors.Is(err, ErrUnsafePermissions) {
		t.Fatalf("MkdirAll error = %v, want ErrUnsafePermissions", err)
	}
}

func FuzzManagedIntegrityPaths(f *testing.F) {
	for _, seed := range []string{
		"runs/run_test/native-output.json",
		"normalized/arjun-candidates.jsonl",
		"..",
		"../escape",
		"/absolute",
		"a//b",
		"a\\b",
		"line\nbreak",
		"a/../../b",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 4<<10 {
			return
		}
		managedErr, managedAgain := validateManagedPath(name), validateManagedPath(name)
		integrityErr, integrityAgain := integrity.ValidateRelativePath(name), integrity.ValidateRelativePath(name)
		if !samePathError(managedErr, managedAgain) || !samePathError(integrityErr, integrityAgain) {
			t.Fatalf("path validation was non-deterministic for %q", name)
		}
		if managedErr == nil {
			if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\\x00") {
				t.Fatalf("managed validator accepted unsafe path %q", name)
			}
			for _, component := range strings.Split(name, "/") {
				if component == "" || component == "." || component == ".." {
					t.Fatalf("managed validator accepted unsafe component in %q", name)
				}
			}
		}
		if integrityErr == nil && managedErr != nil {
			t.Fatalf("public-integrity path was not a valid managed path: %q", name)
		}
	})
}

func samePathError(first, second error) bool {
	return first == nil && second == nil || first != nil && second != nil && first.Error() == second.Error()
}

func openTestRoot(t *testing.T, directory string) *Root {
	t.Helper()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return root
}
