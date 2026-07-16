package preflight

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// CaptureEnvironment returns the exact environment approved for later tool
// execution. Empty PATH components are removed because exec would interpret
// them as the mutable current directory.
func CaptureEnvironment(base, allowlist []string) ([]string, error) {
	result, err := FilterEnvironment(base, allowlist)
	if err != nil {
		return nil, err
	}
	for index, entry := range result {
		key, value, _ := strings.Cut(entry, "=")
		if key != "PATH" {
			continue
		}
		directories, err := approvedPathDirectories(value)
		if err != nil {
			return nil, err
		}
		result[index] = "PATH=" + strings.Join(directories, string(filepath.ListSeparator))
	}
	return result, nil
}

func FilterEnvironment(base, allowlist []string) ([]string, error) {
	values := make(map[string]string, len(base))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !validEnvironmentKey(key) || strings.ContainsRune(value, '\x00') {
			return nil, errors.New("invalid environment entry")
		}
		values[key] = value
	}
	keys := append([]string(nil), allowlist...)
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for index, key := range keys {
		if !validEnvironmentKey(key) || index > 0 && keys[index-1] == key {
			return nil, errors.New("invalid or duplicate environment allowlist key")
		}
		if value, ok := values[key]; ok {
			result = append(result, key+"="+value)
		}
	}
	return result, nil
}

func validEnvironmentKey(key string) bool {
	if key == "" {
		return false
	}
	for index, character := range key {
		if character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

func approvedPathDirectories(value string) ([]string, error) {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, directory := range filepath.SplitList(value) {
		if directory == "" {
			continue
		}
		if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
			return nil, errors.New("approved PATH entries must be absolute clean paths")
		}
		if !seen[directory] {
			seen[directory] = true
			result = append(result, directory)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("approved PATH has no absolute directories")
	}
	return result, nil
}
