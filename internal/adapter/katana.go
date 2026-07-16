package adapter

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
)

const KatanaAdapterVersion = "katana-adapter/v0"

type KatanaOptions struct {
	Context
	ExitCode    *int
	Interrupted bool
	TimedOut    bool
}

type katanaRecord struct {
	Timestamp string `json:"timestamp"`
	Request   struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
	} `json:"request"`
	Response struct {
		StatusCode    *int              `json:"status_code"`
		Headers       map[string]string `json:"headers"`
		ContentLength *int64            `json:"content_length"`
	} `json:"response"`
}

func ParseKatana(native Source, options KatanaOptions) (Result, error) {
	if err := validateContext(options.Context); err != nil {
		return Result{}, err
	}
	raw, err := readSource(native)
	if err != nil {
		return Result{}, err
	}
	nativeLines, err := lines(raw)
	if err != nil {
		return Result{}, err
	}
	builder := newRecordBuilder(options.Context)
	result := Result{Status: "success", Coverage: "complete", ProviderStatus: []model.ProviderStatus{}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{}}
	valid, malformed := 0, 0
	for index, rawLine := range nativeLines {
		if len(strings.TrimSpace(string(rawLine))) == 0 {
			continue
		}
		lineNumber := index + 1
		canonicalJSON, parseErr := canonical.Canonicalize(rawLine)
		var source katanaRecord
		if parseErr == nil {
			parseErr = json.Unmarshal(canonicalJSON, &source)
		}
		decision := model.ScopeDecision{Classification: "unknown", Reason: "record URL is unavailable or invalid"}
		if source.Request.Endpoint != "" {
			decision = builder.decision(source.Request.Endpoint)
		}
		evidence := builder.addEvidence(native.Artifact, model.Locator{Kind: "line_range", LineStart: lineNumber, LineEnd: lineNumber}, decision)
		if parseErr == nil {
			parseErr = validateKatanaRecord(source)
		}
		if parseErr != nil {
			malformed++
			result.Gaps = append(result.Gaps, diagnostic("katana.malformed_record", fmt.Sprintf("line %d: %v", lineNumber, parseErr), "warning", evidence.ID))
			continue
		}
		method, parseErr := canonical.NormalizeSourceMethod(&source.Request.Method, "katana")
		if parseErr != nil {
			malformed++
			result.Gaps = append(result.Gaps, diagnostic("katana.invalid_method", fmt.Sprintf("line %d: %v", lineNumber, parseErr), "warning", evidence.ID))
			continue
		}
		endpoint, canonicalURL, parseErr := builder.addEndpoint(method.HTTPMethod, source.Request.Endpoint)
		if parseErr != nil {
			malformed++
			result.Gaps = append(result.Gaps, diagnostic("katana.invalid_url", fmt.Sprintf("line %d: %v", lineNumber, parseErr), "warning", evidence.ID))
			continue
		}
		valid++
		observedAt := source.Timestamp
		contentType := header(source.Response.Headers, "Content-Type")
		builder.addObservation(
			"http_response", "observed", endpointEntity{endpoint}, &observedAt, []string{evidence.ID},
			model.HTTPDetails{
				RequestURLRaw: source.Request.Endpoint, CanonicalObservationURL: canonicalURL.CanonicalObservationURL,
				Method: *method.HTTPMethod, StatusCode: *source.Response.StatusCode, ContentLength: source.Response.ContentLength, ContentType: contentType,
			},
			endpoint.Scope,
		)
		for _, pair := range canonicalURL.QueryPairs {
			if pair.Name == "" {
				continue
			}
			parameter, parameterErr := builder.addParameter(endpoint, pair.Name, "query", "observed_query")
			if parameterErr != nil {
				return Result{}, parameterErr
			}
			sourceMode, basis := "katana_url_query", "observed in requested URL"
			builder.addObservation(
				"parameter_discovery", "observed", parameterEntity{parameter}, &observedAt, []string{evidence.ID},
				model.ParameterDetails{ParameterName: pair.Name, Location: "query", SourceMode: &sourceMode, DetectionBasis: &basis, AcceptanceState: "unknown"},
				endpoint.Scope,
			)
		}
	}

	switch {
	case options.Interrupted:
		result.Status, result.Coverage = "interrupted", "partial"
	case options.TimedOut:
		result.Status = "timed_out"
		if valid > 0 {
			result.Coverage = "partial"
		} else {
			result.Coverage = "unknown"
		}
	case options.ExitCode == nil:
		result.Status, result.Coverage = "failed", "unknown"
		result.Gaps = append(result.Gaps, diagnostic("katana.exit_unknown", "Katana exit status is unavailable.", "error"))
	case *options.ExitCode != 0 && valid > 0:
		result.Status, result.Coverage = "partial", "partial"
	case *options.ExitCode != 0:
		result.Status, result.Coverage = "failed", "unknown"
	case malformed > 0 && valid > 0:
		result.Status, result.Coverage = "partial", "partial"
	case malformed > 0:
		result.Status, result.Coverage = "unsupported_format", "unknown"
	case valid == 0:
		result.Status, result.Coverage = "success_zero", "zero"
	}
	result.Records = builder.finish()
	return result, nil
}

func validateKatanaRecord(record katanaRecord) error {
	if record.Timestamp == "" {
		return fmt.Errorf("timestamp is missing")
	}
	if _, err := time.Parse(time.RFC3339Nano, record.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	if err := requireNonEmpty(record.Request.Endpoint, "request endpoint"); err != nil {
		return err
	}
	if err := requireNonEmpty(record.Request.Method, "request method"); err != nil {
		return err
	}
	if record.Response.StatusCode == nil || *record.Response.StatusCode < 100 || *record.Response.StatusCode > 599 {
		return fmt.Errorf("invalid or missing status code")
	}
	if record.Response.ContentLength == nil || *record.Response.ContentLength < 0 {
		return fmt.Errorf("invalid or missing content length")
	}
	if record.Response.Headers == nil {
		return fmt.Errorf("response headers are missing")
	}
	return nil
}

func header(headers map[string]string, name string) *string {
	if value, ok := headers[name]; ok {
		return &value
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if strings.EqualFold(key, name) {
			value := headers[key]
			result := value
			return &result
		}
	}
	return nil
}
