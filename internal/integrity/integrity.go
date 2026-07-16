package integrity

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

const maxFileBytes = 128 << 20

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization|proxy-authorization|cookie|set-cookie)\s*:\s*\S+`),
	regexp.MustCompile(`(?i)(token|secret|password|passwd|api[_-]?key|access[_-]?key)\s*[=:]\s*["']?[^\s"']{6,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?:^|[\s"'=])/(?:home|Users)/[^/\s]+(?:/|\s|$)`),
	regexp.MustCompile(`(?i)(?:^|[\s"'=])[A-Z]:\\Users\\[^\\\s]+(?:\\|\s|$)`),
}

func ValidateRelativePath(name string) error {
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00\r\n") {
		return errors.New("unsafe relative path")
	}
	return nil
}

func ScanSecrets(data []byte) error {
	for _, pattern := range secretPatterns {
		if match := pattern.Find(data); match != nil {
			return fmt.Errorf("sensitive material matches %q", pattern.String())
		}
	}
	return nil
}

// ScanPrivatePaths rejects exact local filesystem paths before bytes cross the
// private-workspace/public-handoff boundary.
func ScanPrivatePaths(data []byte, values ...string) error {
	for _, value := range values {
		if value != "" && filepath.IsAbs(value) && bytes.Contains(data, []byte(value)) {
			return fmt.Errorf("sensitive local path is present")
		}
	}
	return nil
}

func Inventory(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsafe package entry %s", name)
		}
		if info.Mode().IsRegular() && info.Sys() != nil {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
				return fmt.Errorf("unsafe hardlink %s", name)
			}
		}
		if info.Mode().IsRegular() {
			relative, err := filepath.Rel(root, name)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if err := ValidateRelativePath(relative); err != nil {
				return fmt.Errorf("inventory %s: %w", relative, err)
			}
			files = append(files, relative)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func HashFile(root, name string) (string, int64, error) {
	if err := ValidateRelativePath(name); err != nil {
		return "", 0, err
	}
	full := filepath.Join(root, filepath.FromSlash(name))
	info, err := os.Lstat(full)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxFileBytes {
		return "", 0, errors.New("artifact is not a bounded regular file")
	}
	file, err := os.Open(full)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maxFileBytes+1))
	if err != nil || written != info.Size() {
		return "", 0, errors.Join(err, errors.New("artifact changed while hashing"))
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func Checksums(root string, names []string) ([]byte, error) {
	names = append([]string(nil), names...)
	sort.Strings(names)
	var output strings.Builder
	for _, name := range names {
		digest, _, err := HashFile(root, name)
		if err != nil {
			return nil, fmt.Errorf("checksum %s: %w", name, err)
		}
		fmt.Fprintf(&output, "%s  %s\n", digest, name)
	}
	return []byte(output.String()), nil
}

func VerifyChecksums(root string, manifest []byte) error {
	scanner := bufio.NewScanner(strings.NewReader(string(manifest)))
	for scanner.Scan() {
		line := scanner.Text()
		digest, name, ok := strings.Cut(line, "  ")
		if !ok || len(digest) != 64 || ValidateRelativePath(name) != nil {
			return errors.New("invalid checksum manifest")
		}
		actual, _, err := HashFile(root, name)
		if err != nil || actual != digest {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	return scanner.Err()
}
