package scope

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"

	"github.com/vtrpza/reconctx/internal/canonical"
)

const maxScopeDocumentBytes = 1 << 20

// LoadJSON loads a bounded scope document and rejects ambiguous schema keys.
func LoadJSON(reader io.Reader) (Config, error) {
	raw, err := readScopeDocument(reader)
	if err != nil {
		return Config{}, err
	}
	canonicalJSON, err := canonical.Canonicalize(raw)
	if err != nil {
		return Config{}, fmt.Errorf("decode scope: %w", err)
	}
	if err := validateJSONKeys(canonicalJSON); err != nil {
		return Config{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonicalJSON))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode scope: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Config{}, err
	}
	if _, err := NewEvaluator(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateJSONKeys(raw []byte) error {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return fmt.Errorf("decode scope: %w", err)
	}
	if err := exactKeys(document, "scope", "mode", "roots", "external_policy"); err != nil {
		return err
	}
	var roots []map[string]json.RawMessage
	if value, ok := document["roots"]; ok {
		if err := json.Unmarshal(value, &roots); err != nil {
			return fmt.Errorf("decode scope roots: %w", err)
		}
		for index, root := range roots {
			if err := exactKeys(root, fmt.Sprintf("scope root %d", index+1), "id", "kind", "value"); err != nil {
				return err
			}
		}
	}
	return nil
}

func exactKeys(object map[string]json.RawMessage, label string, allowed ...string) error {
	known := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		known[key] = true
	}
	for key := range object {
		if !known[key] {
			return fmt.Errorf("decode %s: unknown field %q", label, key)
		}
	}
	return nil
}

// LoadYAML loads the operator-facing scope document and rejects unknown fields
// and multiple YAML documents before evaluating any URL.
func LoadYAML(reader io.Reader) (Config, error) {
	raw, err := readScopeDocument(reader)
	if err != nil {
		return Config{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode scope: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode scope: multiple YAML documents")
		}
		return Config{}, fmt.Errorf("decode scope trailing data: %w", err)
	}
	if _, err := NewEvaluator(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func readScopeDocument(reader io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxScopeDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read scope: %w", err)
	}
	if len(raw) > maxScopeDocumentBytes {
		return nil, fmt.Errorf("read scope: document exceeds %d bytes", maxScopeDocumentBytes)
	}
	return raw, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode scope: multiple JSON values")
		}
		return fmt.Errorf("decode scope trailing data: %w", err)
	}
	return nil
}
