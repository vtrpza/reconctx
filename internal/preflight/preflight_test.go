package preflight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
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

func TestPreflightCapturesCanonicalPATH(t *testing.T) {
	base := []string{"PATH=/first::/second:/first", "LANG=C.UTF-8", "HOME=/private"}
	got, err := CaptureEnvironment(base, []string{"PATH", "LANG"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"LANG=C.UTF-8", "PATH=/first:/second"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("captured environment = %#v, want %#v", got, want)
	}
	if _, err := CaptureEnvironment([]string{"PATH=relative:/bin"}, []string{"PATH"}); err == nil {
		t.Fatal("relative PATH entry was approved")
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
		{"arjun", "v2.2.7\n", "2.2.7"},
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

func TestPreflightReadsBoundArjunMetadataWithoutStartingEntrypoint(t *testing.T) {
	prefix := t.TempDir()
	if err := os.Chmod(prefix, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(prefix, "bin")
	metadata := filepath.Join(prefix, "lib", "python3.13", "site-packages", "arjun-2.2.7.dist-info")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(prefix, "started")
	interpreter := filepath.Join(bin, "python3")
	if err := os.WriteFile(interpreter, []byte(fmt.Sprintf("#!/bin/sh\nprintf started > %q\n", sentinel)), 0o700); err != nil {
		t.Fatal(err)
	}
	entrypoint := filepath.Join(bin, "arjun")
	wrapper := fmt.Sprintf("#!%s\nfrom arjun.__main__ import main\nmain()\n", interpreter)
	if err := os.WriteFile(entrypoint, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "METADATA"), []byte("Metadata-Version: 2.1\nName: arjun\nVersion: 2.2.7\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := InspectTool(context.Background(), "arjun", entrypoint, []string{"PATH=" + bin}, []string{"PATH"})
	if err != nil || result.Version != "2.2.7" {
		t.Fatalf("InspectTool(arjun) = %#v, %v", result, err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("Arjun entrypoint was started during preflight: %v", err)
	}
}

func TestPreflightUsesApprovedPATHAndDoesNotStartWrapper(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(directory, "started")
	tool := filepath.Join(directory, "gau")
	content := fmt.Sprintf("#!/bin/sh\n# reconctx-tool-metadata/v0 name=gau version=2.2.4\nprintf started > %q\n", sentinel)
	if err := os.WriteFile(tool, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := InspectTool(context.Background(), "gau", "gau", []string{"PATH=" + directory}, []string{"PATH"})
	if err != nil || result.Identity.ResolvedPath != tool {
		t.Fatalf("InspectTool(gau) = %#v, %v", result, err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("version wrapper was started during preflight: %v", err)
	}
	if _, err := InspectTool(context.Background(), "gau", "./gau", []string{"PATH=" + directory}, []string{"PATH"}); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("relative path with separator was accepted: %v", err)
	}
}

func TestPreflightValidatesGoBuildMetadata(t *testing.T) {
	tests := []struct{ name, main, module, version, want string }{
		{"gau", "github.com/lc/gau/v2/cmd/gau", "github.com/lc/gau/v2", "v2.2.4", "2.2.4"},
		{"katana", "github.com/projectdiscovery/katana/cmd/katana", "github.com/projectdiscovery/katana", "v1.6.1", "v1.6.1"},
	}
	for _, test := range tests {
		info := &debug.BuildInfo{Path: test.main, Main: debug.Module{Path: test.module, Version: test.version}}
		got, err := validateGoBuildInfo(test.name, info)
		if err != nil || got != test.want {
			t.Errorf("validateGoBuildInfo(%s) = %q, %v", test.name, got, err)
		}
	}
	if _, err := validateGoBuildInfo("gau", &debug.BuildInfo{Path: "example.invalid/main", Main: debug.Module{Path: "github.com/lc/gau/v2", Version: "v2.2.4"}}); err == nil || !strings.Contains(err.Error(), "main package") {
		t.Fatalf("wrong Go main package was accepted: %v", err)
	}
	if _, err := validateGoBuildInfo("gau", &debug.BuildInfo{Path: goMainPackages["gau"], Main: debug.Module{Path: "example.invalid/module", Version: "v2.2.4"}}); err == nil || !strings.Contains(err.Error(), "main module") {
		t.Fatalf("wrong Go main module was accepted: %v", err)
	}
}

func TestPreflightReadsToolMetadataWithoutExecutingCommands(t *testing.T) {
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
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n# reconctx-tool-metadata/v0 name=gau version=9.9.9\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectTool(context.Background(), "gau", tool, os.Environ(), []string{"PATH"}); err == nil {
		t.Fatal("InspectTool accepted an unsupported version")
	}
}
