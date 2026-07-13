package preflight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPreflightResolvesAndIdentifiesTool(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "gau-real")
	content := []byte("#!/bin/sh\nprintf 'gau version 2.2.4\\n'\n")
	if err := os.WriteFile(target, content, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "gau")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	identity, err := ResolveTool(link)
	if err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256(content)
	if identity.ResolvedPath != target {
		t.Fatalf("resolved path = %q, want %q", identity.ResolvedPath, target)
	}
	if identity.SHA256 != "sha256:"+hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("sha256 = %q", identity.SHA256)
	}
	if identity.Mode != 0o700 || identity.Inode == 0 || identity.Device == 0 {
		t.Fatalf("incomplete identity: %#v", identity)
	}
}

func TestPreflightRejectsWritableParent(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "unsafe")
	if err := os.Mkdir(parent, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o777); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(parent, "katana")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveTool(tool); err == nil {
		t.Fatal("ResolveTool accepted a tool below a group/world-writable parent")
	}
}

func TestPreflightRejectsWritableTool(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(directory, "arjun")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o722); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveTool(tool); err == nil {
		t.Fatal("ResolveTool accepted a group/world-writable tool")
	}
}

func TestPreflightFiltersEnvironmentByExplicitAllowlist(t *testing.T) {
	base := []string{
		"PATH=/usr/bin:/bin",
		"LANG=C.UTF-8",
		"PYTHONPATH=/untrusted",
		"LD_PRELOAD=/untrusted.so",
		"HTTP_PROXY=http://untrusted",
		"HOME=/untrusted-home",
	}
	got, err := FilterEnvironment(base, []string{"LANG", "PATH"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"LANG=C.UTF-8", "PATH=/usr/bin:/bin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %#v, want %#v", got, want)
	}
	explicit, err := FilterEnvironment(base, []string{"HTTP_PROXY"})
	if err != nil || !reflect.DeepEqual(explicit, []string{"HTTP_PROXY=http://untrusted"}) {
		t.Fatalf("explicit modeled proxy = %#v, %v", explicit, err)
	}
}

func TestPreflightParsesAnchoredToolVersions(t *testing.T) {
	tests := []struct {
		tool   string
		output string
		want   string
	}{
		{"gau", "gau version 2.2.4\n", "2.2.4"},
		{"katana", "v1.6.1\n", "v1.6.1"},
		{"arjun", "arjun=2.2.7\npython=3.13.5\n", "2.2.7"},
	}
	for _, test := range tests {
		got, err := ParseVersion(test.tool, test.output)
		if err != nil || got != test.want {
			t.Errorf("ParseVersion(%q) = %q, %v; want %q", test.tool, got, err, test.want)
		}
	}
	if got, err := ParseVersion("arjun", "usage: arjun -u http://127.0.0.1/\n"); err == nil {
		t.Fatalf("Arjun help false positive = %q", got)
	}
}

func TestPreflightProbesOnlyVersionCommands(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for tool, want := range map[string]string{"gau": "2.2.4", "katana": "v1.6.1", "arjun": "2.2.7"} {
		source, err := filepath.Abs(filepath.Join("..", "..", "integration", "faketools", tool))
		if err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, tool)
		if err := os.WriteFile(path, content, 0o700); err != nil {
			t.Fatal(err)
		}
		result, err := InspectTool(context.Background(), tool, path, os.Environ(), []string{"LANG", "PATH"})
		if err != nil {
			t.Fatalf("InspectTool(%s): %v", tool, err)
		}
		if result.Version != want || result.Identity.ResolvedPath != path {
			t.Errorf("InspectTool(%s) = %#v, want version %q and path %q", tool, result, want, path)
		}
	}
}

func TestPreflightRejectsUnsupportedVersion(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(directory, "gau")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nprintf 'gau version 9.9.9\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectTool(context.Background(), "gau", tool, os.Environ(), []string{"PATH"}); err == nil {
		t.Fatal("InspectTool accepted an unsupported version")
	}
}
