package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/scope"
)

const (
	MaxArtifactBytes = 16 << 20
	MaxLineBytes     = 1 << 20
)

var (
	runIDPattern = regexp.MustCompile(`^run_[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	txIDPattern  = regexp.MustCompile(`^tx_[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	shaPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Source struct {
	Reader   io.Reader
	Artifact model.Artifact
}

type Context struct {
	RunID           string
	ToolExecutionID string
	AuthContextID   *string
	Scope           *scope.Evaluator
}

type Result struct {
	Status         string
	Coverage       string
	Records        model.RecordSet
	ProviderStatus []model.ProviderStatus
	Warnings       []model.Diagnostic
	Gaps           []model.Diagnostic
}

func validateContext(context Context) error {
	if !runIDPattern.MatchString(context.RunID) {
		return fmt.Errorf("invalid run ID %q", context.RunID)
	}
	if !txIDPattern.MatchString(context.ToolExecutionID) {
		return fmt.Errorf("invalid tool execution ID %q", context.ToolExecutionID)
	}
	return nil
}

func readSource(source Source) ([]byte, error) {
	if source.Reader == nil {
		return nil, errors.New("artifact reader is nil")
	}
	if err := validateArtifact(source.Artifact); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(source.Reader, MaxArtifactBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read artifact %q: %w", source.Artifact.Path, err)
	}
	if len(raw) > MaxArtifactBytes {
		return nil, fmt.Errorf("artifact %q exceeds %d bytes", source.Artifact.Path, MaxArtifactBytes)
	}
	if int64(len(raw)) != source.Artifact.SizeBytes {
		return nil, fmt.Errorf("artifact %q size mismatch", source.Artifact.Path)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != source.Artifact.SHA256 {
		return nil, fmt.Errorf("artifact %q SHA-256 mismatch", source.Artifact.Path)
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("artifact %q is not valid UTF-8", source.Artifact.Path)
	}
	return raw, nil
}

func validateArtifact(artifact model.Artifact) error {
	if artifact.Path == "" || strings.Contains(artifact.Path, `\`) || path.IsAbs(artifact.Path) || path.Clean(artifact.Path) != artifact.Path || artifact.Path == "." || strings.HasPrefix(artifact.Path, "../") {
		return fmt.Errorf("invalid artifact path %q", artifact.Path)
	}
	if !shaPattern.MatchString(artifact.SHA256) {
		return fmt.Errorf("invalid SHA-256 for artifact %q", artifact.Path)
	}
	if artifact.SizeBytes < 0 {
		return fmt.Errorf("invalid size for artifact %q", artifact.Path)
	}
	if artifact.Role == "" || artifact.MediaType == "" {
		return fmt.Errorf("artifact %q is missing role or media type", artifact.Path)
	}
	return nil
}

func lines(raw []byte) ([][]byte, error) {
	parts := strings.Split(string(raw), "\n")
	result := make([][]byte, len(parts))
	for index, part := range parts {
		part = strings.TrimSuffix(part, "\r")
		if len(part) > MaxLineBytes {
			return nil, fmt.Errorf("line %d exceeds %d bytes", index+1, MaxLineBytes)
		}
		result[index] = []byte(part)
	}
	return result, nil
}

func diagnostic(code, message, severity string, evidenceIDs ...string) model.Diagnostic {
	if evidenceIDs == nil {
		evidenceIDs = []string{}
	}
	return model.Diagnostic{Code: code, Message: message, Severity: severity, EvidenceIDs: evidenceIDs}
}
