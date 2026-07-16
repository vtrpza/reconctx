package integrity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIntegrityChecksumsAndTrustBoundaries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Checksums(root, []string{"a.txt"})
	if err != nil || VerifyChecksums(root, manifest) != nil {
		t.Fatalf("checksums = %q, %v", manifest, err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if VerifyChecksums(root, manifest) == nil {
		t.Fatal("tampered artifact verified")
	}
	for _, name := range []string{"", "..", "../escape", "/absolute", "a\\b", "line\nbreak"} {
		if ValidateRelativePath(name) == nil {
			t.Fatalf("unsafe path accepted: %q", name)
		}
	}
	if ScanSecrets([]byte("Authorization: Bearer private")) == nil || ScanSecrets([]byte("ordinary target text")) != nil {
		t.Fatal("secret boundary mismatch")
	}
	if ScanSecrets([]byte("tool path: /home/alice/private/bin")) == nil {
		t.Fatal("private home path passed the public boundary")
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Inventory(root); err == nil {
		t.Fatal("symlink entered package inventory")
	}
}

func TestIntegrityRejectsPrivatePathDisclosure(t *testing.T) {
	private := filepath.Join(t.TempDir(), "runs", "run_test")
	if err := ScanPrivatePaths([]byte("tool wrote "+private+"/native.json\n"), private); err == nil {
		t.Fatal("private workspace path was accepted")
	}
	if err := ScanPrivatePaths([]byte("http://fixture.test/path\n"), private); err != nil {
		t.Fatalf("public target data was rejected: %v", err)
	}
}
