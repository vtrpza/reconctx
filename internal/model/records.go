package model

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

const SchemaVersion = "reconctx/v0"

type ScopeDecision struct {
	Classification string  `json:"classification"`
	RuleID         *string `json:"rule_id"`
	Reason         string  `json:"reason"`
}

type EntityRef struct {
	RecordType string `json:"record_type"`
	ID         string `json:"id"`
}

type Artifact struct {
	Role      string `json:"role"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	MediaType string `json:"media_type"`
	Sanitized bool   `json:"sanitized"`
}

type ArtifactSummary struct {
	Role            string  `json:"role"`
	Path            string  `json:"path"`
	Present         bool    `json:"present"`
	SHA256          *string `json:"sha256"`
	SizeBytes       *int64  `json:"size_bytes"`
	MediaType       string  `json:"media_type"`
	DuplicateOfRole *string `json:"duplicate_of_role,omitempty"`
}

type Locator struct {
	Kind             string `json:"kind"`
	LineStart        int    `json:"line_start,omitempty"`
	LineEnd          int    `json:"line_end,omitempty"`
	Pointer          string `json:"pointer,omitempty"`
	ByteStart        int64  `json:"byte_start,omitempty"`
	ByteEndExclusive int64  `json:"byte_end_exclusive,omitempty"`
}

func (locator Locator) MarshalJSON() ([]byte, error) {
	switch locator.Kind {
	case "whole_artifact":
		if locator.LineStart != 0 || locator.LineEnd != 0 || locator.Pointer != "" || locator.ByteStart != 0 || locator.ByteEndExclusive != 0 {
			return nil, fmt.Errorf("whole_artifact locator has range fields")
		}
		return json.Marshal(struct {
			Kind string `json:"kind"`
		}{locator.Kind})
	case "line_range":
		if locator.LineStart < 1 || locator.LineEnd < locator.LineStart || locator.Pointer != "" || locator.ByteStart != 0 || locator.ByteEndExclusive != 0 {
			return nil, fmt.Errorf("invalid line_range locator")
		}
		return json.Marshal(struct {
			Kind      string `json:"kind"`
			LineStart int    `json:"line_start"`
			LineEnd   int    `json:"line_end"`
		}{locator.Kind, locator.LineStart, locator.LineEnd})
	case "json_pointer":
		if !strings.HasPrefix(locator.Pointer, "/") && locator.Pointer != "" || locator.LineStart != 0 || locator.LineEnd != 0 || locator.ByteStart != 0 || locator.ByteEndExclusive != 0 {
			return nil, fmt.Errorf("invalid json_pointer locator")
		}
		return json.Marshal(struct {
			Kind    string `json:"kind"`
			Pointer string `json:"pointer"`
		}{locator.Kind, locator.Pointer})
	case "byte_range":
		if locator.ByteStart < 0 || locator.ByteEndExclusive <= locator.ByteStart || locator.LineStart != 0 || locator.LineEnd != 0 || locator.Pointer != "" {
			return nil, fmt.Errorf("invalid byte_range locator")
		}
		return json.Marshal(struct {
			Kind             string `json:"kind"`
			ByteStart        int64  `json:"byte_start"`
			ByteEndExclusive int64  `json:"byte_end_exclusive"`
		}{locator.Kind, locator.ByteStart, locator.ByteEndExclusive})
	default:
		return nil, fmt.Errorf("unsupported locator kind %q", locator.Kind)
	}
}

type Diagnostic struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Severity    string   `json:"severity"`
	Provider    *string  `json:"provider"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type ProviderStatus struct {
	Provider    string   `json:"provider"`
	Status      string   `json:"status"`
	RecordCount *int     `json:"record_count"`
	ErrorCode   *string  `json:"error_code"`
	Message     *string  `json:"message"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type RunScopeRoot struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type RunScope struct {
	Mode             string         `json:"mode"`
	Roots            []RunScopeRoot `json:"roots"`
	ExternalPolicy   string         `json:"external_policy"`
	ApprovedBy       string         `json:"approved_by"`
	ApprovedAt       string         `json:"approved_at"`
	AuthorizationRef *string        `json:"authorization_ref,omitempty"`
}

type RunRecord struct {
	SchemaVersion          string       `json:"schema_version"`
	RecordType             string       `json:"record_type"`
	ID                     string       `json:"id"`
	CreatedAt              string       `json:"created_at"`
	FinishedAt             *string      `json:"finished_at,omitempty"`
	Status                 string       `json:"status"`
	CanonicalizationPolicy string       `json:"canonicalization_policy"`
	Scope                  RunScope     `json:"scope"`
	ToolExecutionIDs       []string     `json:"tool_execution_ids"`
	Warnings               []Diagnostic `json:"warnings"`
	Gaps                   []Diagnostic `json:"gaps"`
}

type ToolIdentity struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	ResolvedPath string `json:"resolved_path"`
}

type ToolExecution struct {
	SchemaVersion  string            `json:"schema_version"`
	RecordType     string            `json:"record_type"`
	ID             string            `json:"id"`
	RunID          string            `json:"run_id"`
	Tool           ToolIdentity      `json:"tool"`
	AdapterVersion string            `json:"adapter_version"`
	AuthContextID  *string           `json:"auth_context_id"`
	ActivityClass  string            `json:"activity_class"`
	ApprovalPhase  string            `json:"approval_phase"`
	ArgvRedacted   []string          `json:"argv_redacted"`
	StartedAt      *string           `json:"started_at"`
	FinishedAt     *string           `json:"finished_at"`
	DurationMS     *int64            `json:"duration_ms"`
	ExitCode       *int              `json:"exit_code"`
	Status         string            `json:"status"`
	Coverage       string            `json:"coverage"`
	Artifacts      []ArtifactSummary `json:"artifacts"`
	ProviderStatus []ProviderStatus  `json:"provider_status"`
	Warnings       []Diagnostic      `json:"warnings"`
	Gaps           []Diagnostic      `json:"gaps"`
}

type Asset struct {
	SchemaVersion  string        `json:"schema_version"`
	RecordType     string        `json:"record_type"`
	ID             string        `json:"id"`
	RunID          string        `json:"run_id"`
	AssetKind      string        `json:"asset_kind"`
	CanonicalValue string        `json:"canonical_value"`
	DisplayValue   string        `json:"display_value"`
	Scope          ScopeDecision `json:"scope_decision"`
	ObservationIDs []string      `json:"observation_ids"`
	EvidenceIDs    []string      `json:"evidence_ids"`
}

type Endpoint struct {
	SchemaVersion     string        `json:"schema_version"`
	RecordType        string        `json:"record_type"`
	ID                string        `json:"id"`
	RunID             string        `json:"run_id"`
	OriginAssetID     string        `json:"origin_asset_id"`
	CanonicalRouteURL string        `json:"canonical_route_url"`
	Scheme            string        `json:"scheme"`
	Host              string        `json:"host"`
	Port              *int          `json:"port"`
	Path              string        `json:"path"`
	Method            *string       `json:"method"`
	MethodKnown       bool          `json:"method_known"`
	RouteTemplate     *string       `json:"route_template"`
	Scope             ScopeDecision `json:"scope_decision"`
	ObservationIDs    []string      `json:"observation_ids"`
	EvidenceIDs       []string      `json:"evidence_ids"`
}

type Parameter struct {
	SchemaVersion  string   `json:"schema_version"`
	RecordType     string   `json:"record_type"`
	ID             string   `json:"id"`
	RunID          string   `json:"run_id"`
	EndpointID     string   `json:"endpoint_id"`
	Name           string   `json:"name"`
	Location       string   `json:"location"`
	DiscoveryKinds []string `json:"discovery_kinds"`
	ObservationIDs []string `json:"observation_ids"`
	EvidenceIDs    []string `json:"evidence_ids"`
}

type Observation struct {
	SchemaVersion   string        `json:"schema_version"`
	RecordType      string        `json:"record_type"`
	ID              string        `json:"id"`
	RunID           string        `json:"run_id"`
	ToolExecutionID string        `json:"tool_execution_id"`
	AuthContextID   *string       `json:"auth_context_id"`
	ObservationType string        `json:"observation_type"`
	SemanticState   string        `json:"semantic_state"`
	Subject         EntityRef     `json:"subject"`
	Scope           ScopeDecision `json:"scope_decision"`
	ObservedAt      *string       `json:"observed_at"`
	EvidenceIDs     []string      `json:"evidence_ids"`
	Details         any           `json:"details"`
}

type HistoricalDetails struct {
	URLRaw                  string      `json:"url_raw"`
	CanonicalObservationURL string      `json:"canonical_observation_url"`
	CanonicalRouteURL       string      `json:"canonical_route_url"`
	QueryPairs              []QueryPair `json:"query_pairs"`
	ProviderSet             []string    `json:"provider_set"`
	CurrentReachability     string      `json:"current_reachability"`
}

type QueryPair struct {
	Index     int     `json:"index"`
	RawName   string  `json:"raw_name"`
	RawValue  *string `json:"raw_value"`
	Name      string  `json:"name"`
	Value     *string `json:"value"`
	HasEquals bool    `json:"has_equals"`
}

type HTTPDetails struct {
	RequestURLRaw           string  `json:"request_url_raw"`
	CanonicalObservationURL string  `json:"canonical_observation_url"`
	Method                  string  `json:"method"`
	StatusCode              int     `json:"status_code"`
	ContentLength           *int64  `json:"content_length"`
	ContentType             *string `json:"content_type"`
}

type ParameterDetails struct {
	ParameterName   string  `json:"parameter_name"`
	Location        string  `json:"location"`
	SourceMode      *string `json:"source_mode"`
	DetectionBasis  *string `json:"detection_basis"`
	AcceptanceState string  `json:"acceptance_state"`
}

type ZeroDetails struct {
	ResultKind string  `json:"result_kind"`
	TargetURL  *string `json:"target_url"`
	Message    string  `json:"message"`
}

type WarningDetails struct {
	Code     string  `json:"code"`
	Message  string  `json:"message"`
	Provider *string `json:"provider"`
}

type Evidence struct {
	SchemaVersion   string        `json:"schema_version"`
	RecordType      string        `json:"record_type"`
	ID              string        `json:"id"`
	RunID           string        `json:"run_id"`
	ToolExecutionID string        `json:"tool_execution_id"`
	Artifact        Artifact      `json:"artifact"`
	Locator         Locator       `json:"locator"`
	ExcerptRedacted *string       `json:"excerpt_redacted,omitempty"`
	RedactionStatus string        `json:"redaction_status"`
	Scope           ScopeDecision `json:"scope_decision"`
}

type Relationship struct {
	SchemaVersion    string         `json:"schema_version"`
	RecordType       string         `json:"record_type"`
	ID               string         `json:"id"`
	RunID            string         `json:"run_id"`
	RelationshipType string         `json:"relationship_type"`
	From             EntityRef      `json:"from_ref"`
	To               EntityRef      `json:"to_ref"`
	EvidenceIDs      []string       `json:"evidence_ids"`
	Attributes       map[string]any `json:"attributes"`
}

type RecordSet struct {
	Runs           []RunRecord     `json:"runs,omitempty"`
	ToolExecutions []ToolExecution `json:"tool_executions,omitempty"`
	Assets         []Asset         `json:"assets,omitempty"`
	Endpoints      []Endpoint      `json:"endpoints,omitempty"`
	Parameters     []Parameter     `json:"parameters,omitempty"`
	Observations   []Observation   `json:"observations,omitempty"`
	Evidence       []Evidence      `json:"evidence,omitempty"`
	Relationships  []Relationship  `json:"relationships,omitempty"`
}

// Merge coalesces deterministic entities and rejects conflicting records with
// the same ID. Occurrence records remain distinct.
func (records *RecordSet) Merge(other RecordSet) error {
	mergedRuns, err := mergeExact(records.Runs, other.Runs, func(record RunRecord) string { return record.ID })
	if err != nil {
		return err
	}
	records.Runs = mergedRuns
	mergedExecutions, err := mergeExact(records.ToolExecutions, other.ToolExecutions, func(record ToolExecution) string { return record.ID })
	if err != nil {
		return err
	}
	records.ToolExecutions = mergedExecutions
	for _, candidate := range other.Assets {
		index := slices.IndexFunc(records.Assets, func(record Asset) bool { return record.ID == candidate.ID })
		if index < 0 {
			records.Assets = append(records.Assets, candidate)
			continue
		}
		existing, incoming := records.Assets[index], candidate
		existing.ObservationIDs, incoming.ObservationIDs = nil, nil
		existing.EvidenceIDs, incoming.EvidenceIDs = nil, nil
		if !reflect.DeepEqual(existing, incoming) {
			return fmt.Errorf("conflicting asset %s", candidate.ID)
		}
		records.Assets[index].ObservationIDs = union(records.Assets[index].ObservationIDs, candidate.ObservationIDs)
		records.Assets[index].EvidenceIDs = union(records.Assets[index].EvidenceIDs, candidate.EvidenceIDs)
	}
	for _, candidate := range other.Endpoints {
		index := slices.IndexFunc(records.Endpoints, func(record Endpoint) bool { return record.ID == candidate.ID })
		if index < 0 {
			records.Endpoints = append(records.Endpoints, candidate)
			continue
		}
		existing, incoming := records.Endpoints[index], candidate
		existing.ObservationIDs, incoming.ObservationIDs = nil, nil
		existing.EvidenceIDs, incoming.EvidenceIDs = nil, nil
		if !reflect.DeepEqual(existing, incoming) {
			return fmt.Errorf("conflicting endpoint %s", candidate.ID)
		}
		records.Endpoints[index].ObservationIDs = union(records.Endpoints[index].ObservationIDs, candidate.ObservationIDs)
		records.Endpoints[index].EvidenceIDs = union(records.Endpoints[index].EvidenceIDs, candidate.EvidenceIDs)
	}
	for _, candidate := range other.Parameters {
		index := slices.IndexFunc(records.Parameters, func(record Parameter) bool { return record.ID == candidate.ID })
		if index < 0 {
			records.Parameters = append(records.Parameters, candidate)
			continue
		}
		existing, incoming := records.Parameters[index], candidate
		existing.DiscoveryKinds, incoming.DiscoveryKinds = nil, nil
		existing.ObservationIDs, incoming.ObservationIDs = nil, nil
		existing.EvidenceIDs, incoming.EvidenceIDs = nil, nil
		if !reflect.DeepEqual(existing, incoming) {
			return fmt.Errorf("conflicting parameter %s", candidate.ID)
		}
		records.Parameters[index].DiscoveryKinds = union(records.Parameters[index].DiscoveryKinds, candidate.DiscoveryKinds)
		records.Parameters[index].ObservationIDs = union(records.Parameters[index].ObservationIDs, candidate.ObservationIDs)
		records.Parameters[index].EvidenceIDs = union(records.Parameters[index].EvidenceIDs, candidate.EvidenceIDs)
	}
	mergedObservations, err := mergeExact(records.Observations, other.Observations, func(record Observation) string { return record.ID })
	if err != nil {
		return err
	}
	records.Observations = mergedObservations
	mergedEvidence, err := mergeExact(records.Evidence, other.Evidence, func(record Evidence) string { return record.ID })
	if err != nil {
		return err
	}
	records.Evidence = mergedEvidence
	mergedRelationships, err := mergeExact(records.Relationships, other.Relationships, func(record Relationship) string { return record.ID })
	if err != nil {
		return err
	}
	records.Relationships = mergedRelationships
	records.Sort()
	return nil
}

func mergeExact[T any](existing, incoming []T, id func(T) string) ([]T, error) {
	for _, candidate := range incoming {
		index := slices.IndexFunc(existing, func(record T) bool { return id(record) == id(candidate) })
		if index < 0 {
			existing = append(existing, candidate)
		} else if !reflect.DeepEqual(existing[index], candidate) {
			return nil, fmt.Errorf("conflicting record %s", id(candidate))
		}
	}
	return existing, nil
}

func union(existing, incoming []string) []string {
	for _, value := range incoming {
		if !slices.Contains(existing, value) {
			existing = append(existing, value)
		}
	}
	slices.Sort(existing)
	return existing
}

func (records *RecordSet) Sort() {
	slices.SortFunc(records.Runs, func(a, b RunRecord) int { return compareID(a.ID, b.ID) })
	slices.SortFunc(records.ToolExecutions, func(a, b ToolExecution) int { return compareID(a.ID, b.ID) })
	slices.SortFunc(records.Assets, func(a, b Asset) int { return compareID(a.ID, b.ID) })
	slices.SortFunc(records.Endpoints, func(a, b Endpoint) int { return compareID(a.ID, b.ID) })
	slices.SortFunc(records.Parameters, func(a, b Parameter) int { return compareID(a.ID, b.ID) })
	slices.SortFunc(records.Observations, func(a, b Observation) int { return compareID(a.ID, b.ID) })
	slices.SortFunc(records.Evidence, func(a, b Evidence) int { return compareID(a.ID, b.ID) })
	slices.SortFunc(records.Relationships, func(a, b Relationship) int { return compareID(a.ID, b.ID) })
}

func compareID(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
