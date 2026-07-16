package compiler

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/schemas"
)

var (
	schemaOnce sync.Once
	schemaSet  map[string]*jsonschema.Resolved
	schemaErr  error
)

func validateSchema(name string, value any) error {
	schemaOnce.Do(loadSchemas)
	if schemaErr != nil {
		return schemaErr
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		return err
	}
	if err := schemaSet[name].Validate(instance); err != nil {
		return fmt.Errorf("%s validation: %w", name, err)
	}
	if err := validateFormats(name, instance); err != nil {
		return fmt.Errorf("%s validation: %w", name, err)
	}
	return nil
}

func validateRecordSchemas(records model.RecordSet) error {
	validate := func(kind, id string, record any) error {
		if err := validateSchema("record.schema.json", record); err != nil {
			return fmt.Errorf("%s record %q: %w", kind, id, err)
		}
		return nil
	}
	for _, record := range records.Runs {
		if err := validate("run", record.ID, record); err != nil {
			return err
		}
	}
	for _, record := range records.ToolExecutions {
		if err := validate("tool_execution", record.ID, record); err != nil {
			return err
		}
	}
	for _, record := range records.Assets {
		if err := validate("asset", record.ID, record); err != nil {
			return err
		}
	}
	for _, record := range records.Endpoints {
		if err := validate("endpoint", record.ID, record); err != nil {
			return err
		}
	}
	for _, record := range records.Parameters {
		if err := validate("parameter", record.ID, record); err != nil {
			return err
		}
	}
	for _, record := range records.Observations {
		if err := validate("observation", record.ID, record); err != nil {
			return err
		}
	}
	for _, record := range records.Evidence {
		if err := validate("evidence", record.ID, record); err != nil {
			return err
		}
	}
	for _, record := range records.Relationships {
		if err := validate("relationship", record.ID, record); err != nil {
			return err
		}
	}
	return nil
}

func loadSchemas() {
	schemaSet = make(map[string]*jsonschema.Resolved, 3)
	for _, name := range []string{"record.schema.json", "arjun-candidate.schema.json", "handoff-manifest.schema.json"} {
		document, err := loadSchema(name)
		if err != nil {
			schemaErr = err
			return
		}
		resolved, err := document.Resolve(&jsonschema.ResolveOptions{Loader: schemaLoader})
		if err != nil {
			schemaErr = fmt.Errorf("resolve %s: %w", name, err)
			return
		}
		schemaSet[name] = resolved
	}
}

func schemaLoader(uri *url.URL) (*jsonschema.Schema, error) {
	if uri.Scheme != "https" || uri.Host != "schemas.reconctx.dev" || path.Dir(uri.Path) != "/v0" {
		return nil, fmt.Errorf("unsupported schema reference %q", uri)
	}
	return loadSchema(path.Base(uri.Path))
}

func loadSchema(name string) (*jsonschema.Schema, error) {
	raw, err := schemas.V0.ReadFile("v0/" + name)
	if err != nil {
		return nil, err
	}
	var document jsonschema.Schema
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode schema %s: %w", name, err)
	}
	return &document, nil
}

func validateTimestamp(value string) error {
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return fmt.Errorf("invalid date-time %q", value)
	}
	return nil
}

func validateFormats(schemaName string, value any) error {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	timestamp := func(container map[string]any, fields ...string) error {
		for _, field := range fields {
			if text, ok := container[field].(string); ok {
				if err := validateTimestamp(text); err != nil {
					return err
				}
			}
		}
		return nil
	}
	httpURI := func(container map[string]any, fields ...string) error {
		for _, field := range fields {
			if text, ok := container[field].(string); ok {
				if err := validateAbsoluteHTTPURL(text); err != nil {
					return err
				}
			}
		}
		return nil
	}

	switch schemaName {
	case "handoff-manifest.schema.json":
		return timestamp(object, "generated_at")
	case "arjun-candidate.schema.json":
		return httpURI(object, "selected_url", "canonical_route_url")
	case "record.schema.json":
		switch object["record_type"] {
		case "run":
			if err := timestamp(object, "created_at", "finished_at"); err != nil {
				return err
			}
			if scope, ok := object["scope"].(map[string]any); ok {
				return timestamp(scope, "approved_at")
			}
		case "tool_execution":
			return timestamp(object, "started_at", "finished_at")
		case "endpoint":
			return httpURI(object, "canonical_route_url")
		case "observation":
			if err := timestamp(object, "observed_at"); err != nil {
				return err
			}
			details, ok := object["details"].(map[string]any)
			if !ok {
				return nil
			}
			switch object["observation_type"] {
			case "historical_url":
				return httpURI(details, "canonical_observation_url", "canonical_route_url")
			case "http_response":
				return httpURI(details, "canonical_observation_url")
			case "zero_result":
				return httpURI(details, "target_url")
			}
		}
	}
	return nil
}

func validateAbsoluteHTTPURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid HTTP URI %q", value)
	}
	return nil
}
