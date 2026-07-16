package preflight

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const (
	maxVersionOutputBytes = 64 << 10
	maxMetadataEntries    = 4096
)

var versionPatterns = map[string]*regexp.Regexp{
	"gau":    regexp.MustCompile(`(?mi)^gau(?: version)?\s+v?([0-9]+\.[0-9]+\.[0-9]+)\s*$`),
	"katana": regexp.MustCompile(`(?mi)^(?:\[INF\]\s*)?(?:Current Version:\s*)?(v[0-9]+\.[0-9]+\.[0-9]+)\s*$`),
	"arjun":  regexp.MustCompile(`(?mi)^(?:arjun(?:=|(?: version)?\s+)|v)([0-9]+\.[0-9]+\.[0-9]+)\s*$`),
}

var supportedVersions = map[string]string{
	"gau":    "2.2.4",
	"katana": "v1.6.1",
	"arjun":  "2.2.7",
}

var goMainPackages = map[string]string{
	"gau":    "github.com/lc/gau/v2/cmd/gau",
	"katana": "github.com/projectdiscovery/katana/cmd/katana",
}

var goMainModules = map[string]string{
	"gau":    "github.com/lc/gau/v2",
	"katana": "github.com/projectdiscovery/katana",
}

var wrapperMetadataPattern = regexp.MustCompile(`(?m)^# reconctx-tool-metadata/v0 name=([a-z]+) version=(v?[0-9]+\.[0-9]+\.[0-9]+)$`)
var arjunImportPattern = regexp.MustCompile(`(?m)^\s*(?:from\s+arjun(?:[.\s]|$)|import\s+arjun(?:[.\s,]|$))`)

type ToolResult struct {
	Identity ToolIdentity
	Version  string
}

func ParseVersion(tool, output string) (string, error) {
	pattern, ok := versionPatterns[tool]
	if !ok {
		return "", fmt.Errorf("unsupported tool %q", tool)
	}
	match := pattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", fmt.Errorf("%s version not found in anchored runtime output", tool)
	}
	return match[1], nil
}

// InspectTool reads version metadata only. It never starts the candidate, so a
// plan cannot send traffic or leave descendants behind during preflight.
func InspectTool(ctx context.Context, name, candidate string, baseEnvironment, allowlist []string) (ToolResult, error) {
	if _, supported := supportedVersions[name]; !supported {
		return ToolResult{}, fmt.Errorf("unsupported tool %q", name)
	}
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	environment, err := FilterEnvironment(baseEnvironment, allowlist)
	if err != nil {
		return ToolResult{}, err
	}
	identity, err := resolveToolWithEnvironment(candidate, environment)
	if err != nil {
		return ToolResult{}, err
	}

	version, found, err := metadataVersion(name, identity.ResolvedPath)
	if err != nil {
		return ToolResult{}, err
	}
	if !found {
		return ToolResult{}, fmt.Errorf("%s has no supported, non-executing version metadata", name)
	}
	if version != supportedVersions[name] {
		return ToolResult{}, fmt.Errorf("unsupported %s version %q", name, version)
	}
	return ToolResult{Identity: identity, Version: version}, nil
}

func resolveToolWithEnvironment(candidate string, environment []string) (ToolIdentity, error) {
	if filepath.IsAbs(candidate) {
		return ResolveTool(candidate)
	}
	if strings.ContainsRune(candidate, filepath.Separator) {
		return ToolIdentity{}, errors.New("tool path with a separator must be absolute")
	}
	pathValue := ""
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		if key == "PATH" {
			pathValue = value
			break
		}
	}
	if pathValue == "" {
		return ToolIdentity{}, errors.New("relative tool name requires an approved non-empty PATH")
	}
	directories, err := approvedPathDirectories(pathValue)
	if err != nil {
		return ToolIdentity{}, err
	}
	for _, directory := range directories {
		path := filepath.Join(directory, candidate)
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) || err == nil && (info.IsDir() || info.Mode().Perm()&0o111 == 0) {
			continue
		}
		if err != nil {
			return ToolIdentity{}, err
		}
		return ResolveTool(path)
	}
	return ToolIdentity{}, fmt.Errorf("tool %q was not found in the approved PATH", candidate)
}

func metadataVersion(name, executable string) (string, bool, error) {
	if name == "gau" || name == "katana" {
		version, found, err := goBuildVersion(name, executable)
		if found || err != nil {
			return version, found, err
		}
	}
	if name == "arjun" {
		version, found, err := arjunPackageVersion(executable)
		if found || err != nil {
			return version, found, err
		}
	}
	return embeddedWrapperVersion(name, executable)
}

func goBuildVersion(name, executable string) (string, bool, error) {
	info, err := buildinfo.ReadFile(executable)
	if err != nil {
		return "", false, nil
	}
	version, err := validateGoBuildInfo(name, info)
	return version, true, err
}

func validateGoBuildInfo(name string, info *buildinfo.BuildInfo) (string, error) {
	if info.Path != goMainPackages[name] {
		return "", fmt.Errorf("%s Go main package is %q", name, info.Path)
	}
	if info.Main.Path != goMainModules[name] {
		return "", fmt.Errorf("%s Go main module is %q", name, info.Main.Path)
	}
	version := info.Main.Version
	if version == "" || version == "(devel)" {
		return "", fmt.Errorf("%s Go build metadata has no released module version", name)
	}
	if name == "gau" {
		version = strings.TrimPrefix(version, "v")
	}
	return version, nil
}

func arjunPackageVersion(executable string) (string, bool, error) {
	wrapper, err := readBoundedRegular(executable, maxVersionOutputBytes)
	if err != nil {
		return "", false, err
	}
	if !arjunImportPattern.Match(wrapper) {
		return "", false, nil
	}
	line, _, _ := strings.Cut(string(wrapper), "\n")
	if !strings.HasPrefix(line, "#!") {
		return "", true, errors.New("Arjun entrypoint has no bound interpreter")
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) != 1 || !filepath.IsAbs(fields[0]) || filepath.Base(fields[0]) == "env" {
		return "", true, errors.New("Arjun entrypoint interpreter must be one absolute path")
	}
	entryPrefix, interpreterPrefix := filepath.Dir(filepath.Dir(executable)), filepath.Dir(filepath.Dir(fields[0]))
	if filepath.Base(filepath.Dir(executable)) != "bin" || entryPrefix != interpreterPrefix {
		return "", true, errors.New("Arjun entrypoint and interpreter are not in the same environment")
	}
	if _, err := ResolveTool(fields[0]); err != nil {
		return "", true, fmt.Errorf("Arjun interpreter: %w", err)
	}

	metadata, err := findDistributionMetadata(entryPrefix, "arjun")
	if err != nil {
		return "", true, err
	}
	if metadata == "" {
		return "", true, errors.New("Arjun distribution metadata was not found beside its entrypoint environment")
	}
	content, err := readBoundedRegular(metadata, maxVersionOutputBytes)
	if err != nil {
		return "", true, err
	}
	name, version := metadataHeader(content, "Name"), metadataHeader(content, "Version")
	if !strings.EqualFold(name, "arjun") || version == "" {
		return "", true, errors.New("Arjun distribution metadata has no exact name and version")
	}
	return version, true, nil
}

func findDistributionMetadata(prefix, packageName string) (string, error) {
	var matches []string
	seen := make(map[string]bool)
	for _, library := range []string{"lib", "lib64"} {
		versions, err := readDirBounded(filepath.Join(prefix, library))
		if err != nil {
			return "", err
		}
		for _, version := range versions {
			if !version.IsDir() || !strings.HasPrefix(version.Name(), "python") {
				continue
			}
			for _, packages := range []string{"site-packages", "dist-packages"} {
				directory := filepath.Join(prefix, library, version.Name(), packages)
				entries, err := readDirBounded(directory)
				if err != nil {
					return "", err
				}
				for _, entry := range entries {
					name := strings.ToLower(entry.Name())
					if entry.IsDir() && strings.HasPrefix(name, packageName+"-") && strings.HasSuffix(name, ".dist-info") {
						metadata := filepath.Join(directory, entry.Name(), "METADATA")
						resolved, err := filepath.EvalSymlinks(metadata)
						if err != nil {
							return "", err
						}
						if !seen[resolved] {
							seen[resolved] = true
							matches = append(matches, metadata)
						}
					}
				}
			}
		}
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple %s distributions found in the entrypoint environment", packageName)
	}
	if len(matches) == 0 {
		return "", nil
	}
	return matches[0], nil
}

func readDirBounded(directory string) ([]os.DirEntry, error) {
	file, err := os.Open(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(maxMetadataEntries + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > maxMetadataEntries {
		return nil, fmt.Errorf("metadata directory %q exceeds entry limit", directory)
	}
	return entries, nil
}

func metadataHeader(content []byte, key string) string {
	for _, line := range strings.Split(string(content), "\n") {
		if line == "" {
			break
		}
		if before, after, ok := strings.Cut(line, ":"); ok && strings.EqualFold(before, key) {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func embeddedWrapperVersion(name, executable string) (string, bool, error) {
	content, err := readBoundedRegular(executable, maxVersionOutputBytes)
	if err != nil {
		return "", false, err
	}
	match := wrapperMetadataPattern.FindSubmatch(content)
	if len(match) != 3 || string(match[1]) != name {
		return "", false, nil
	}
	version := string(match[2])
	if name == "gau" || name == "arjun" {
		version = strings.TrimPrefix(version, "v")
	}
	return version, true, nil
}

func readBoundedRegular(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("version metadata is not a bounded regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("version metadata identity is unavailable")
	}
	if err := validateToolPath(path, stat); err != nil {
		return nil, fmt.Errorf("unsafe version metadata: %w", err)
	}
	return io.ReadAll(io.LimitReader(file, limit+1))
}
