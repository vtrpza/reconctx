package preflight

import (
	"errors"
	"sort"
	"strings"
)

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
