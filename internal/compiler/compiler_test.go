package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/integrity"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/workspace"
)

func TestCompileEnforcesPublishedSchemas(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{
			name: "record enum",
			mutate: func(input *Input) {
				input.Records.ToolExecutions[0].Status = "bogus"
			},
		},
		{
			name: "record date-time format",
			mutate: func(input *Input) {
				input.Records.Runs[0].CreatedAt = "not-a-time"
			},
		},
		{
			name: "record URI format",
			mutate: func(input *Input) {
				input.Records.Endpoints[0].CanonicalRouteURL = "http:///"
			},
		},
		{
			name: "candidate contract",
			mutate: func(input *Input) {
				input.Candidates = []json.RawMessage{json.RawMessage(`{"record_type":"arjun_candidate"}`)}
			},
		},
		{
			name: "manifest unique fixture cases",
			mutate: func(input *Input) {
				input.SourceFixtureCases = []string{"duplicate", "duplicate"}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := compilerFixture()
			test.mutate(&input)
			if _, err := Compile(input); err == nil {
				t.Fatal("Compile accepted data outside the published schema")
			}
		})
	}
}

func TestCompileDoesNotApplySchemaFormatsToRelationshipAttributes(t *testing.T) {
	input := compilerFixture()
	input.Records.Relationships = []model.Relationship{{
		SchemaVersion: model.SchemaVersion, RecordType: "relationship", ID: id("rel", "extension-attributes"), RunID: input.RunID,
		RelationshipType: "derived_from",
		From:             model.EntityRef{RecordType: "endpoint", ID: input.Records.Endpoints[0].ID},
		To:               model.EntityRef{RecordType: "observation", ID: input.Records.Observations[0].ID},
		EvidenceIDs:      []string{input.Records.Evidence[0].ID},
		Attributes:       map[string]any{"created_at": "source-label", "target_url": "opaque", "count": 1},
	}}
	if _, err := Compile(input); err != nil {
		t.Fatalf("Compile rejected schema-valid extension attributes: %v", err)
	}
}

func TestCompileRejectsDanglingCandidateProvenance(t *testing.T) {
	input := compilerFixture()
	row := map[string]any{
		"schema_version": "reconctx/v0", "record_type": "arjun_candidate", "candidate_policy_version": "arjun-candidate-policy/v0",
		"queue_digest": "sha256:" + strings.Repeat("0", 64), "candidate_id": "candidate_sha256_" + strings.Repeat("1", 64),
		"endpoint_id": input.Records.Endpoints[0].ID, "selected_url": input.Records.Endpoints[0].CanonicalRouteURL, "canonical_route_url": input.Records.Endpoints[0].CanonicalRouteURL,
		"method": "GET", "source_mode": "GET", "location": "query", "eligible": true, "included": true, "reason_codes": []string{"selected"},
		"rank_inputs":   map[string]any{"currently_observed_by_katana": true, "existing_query_name_evidence": false, "api_like_path": false, "independent_source_executions": 1, "no_static_extension": true, "supported_method_location": true},
		"rank_position": 1, "observation_ids": []string{input.Records.Observations[0].ID}, "evidence_ids": []string{input.Records.Evidence[0].ID},
		"source_execution_ids": []string{input.Records.ToolExecutions[0].ID}, "scope_decision": input.Records.Endpoints[0].Scope,
		"argv_redacted": []string{"<ARJUN>", "-u"}, "wordlist_sha256": "sha256:" + strings.Repeat("2", 64), "request_budget": 1, "max_targets": 25,
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	input.Candidates = []json.RawMessage{encoded}
	if _, err := Compile(input); err != nil {
		t.Fatalf("Compile rejected valid candidate provenance: %v", err)
	}

	row["endpoint_id"] = id("ep", "missing-candidate-endpoint")
	encoded, err = json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	input.Candidates[0] = encoded
	if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "unresolved endpoint") {
		t.Fatalf("Compile error = %v, want dangling candidate rejection", err)
	}
}

func TestCompileIsDeterministicChecksummedAndFailClosed(t *testing.T) {
	input := compilerFixture()
	first, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Compile(input)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("compile is not deterministic: %v", err)
	}
	if !strings.Contains(string(first["CONTEXT.md"]), "untrusted data") || len(first["normalized/agent-view.jsonl"]) == 0 {
		t.Fatal("compact front door is incomplete")
	}
	if !strings.Contains(string(first["manifest.json"]), `"source_fixture_cases":[]`) {
		t.Fatalf("manifest encoded an absent source fixture list as null: %s", first["manifest.json"])
	}
	rootPath := t.TempDir()
	if err := os.Chmod(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := Write(root, "handoff/run_test", first); err != nil {
		t.Fatal(err)
	}
	writtenRoot := filepath.Join(rootPath, "handoff", "run_test")
	manifest, err := os.ReadFile(filepath.Join(writtenRoot, "checksums.sha256"))
	if err != nil || integrity.VerifyChecksums(writtenRoot, manifest) != nil {
		t.Fatalf("published checksums failed: %v", err)
	}

	unsafe := compilerFixture()
	unsafe.RawFiles["raw/katana.jsonl"] = []byte("Authorization: Bearer private\n")
	if _, err := Compile(unsafe); err == nil {
		t.Fatal("compiler published a secret")
	}
	broken := compilerFixture()
	broken.Records.Endpoints[0].EvidenceIDs = []string{id("ev", "missing")}
	if _, err := Compile(broken); err == nil {
		t.Fatal("compiler accepted an unresolved Evidence ID")
	}
	orphan := compilerFixture()
	orphan.Records.Endpoints[0].ObservationIDs = []string{}
	if _, err := Compile(orphan); err == nil {
		t.Fatal("compiler accepted an endpoint without observations")
	}
	dangling := compilerFixture()
	dangling.Records.Parameters = []model.Parameter{{
		SchemaVersion: model.SchemaVersion, RecordType: "parameter", ID: id("param", "dangling"), RunID: dangling.RunID,
		EndpointID: dangling.Records.Endpoints[0].ID, Name: "q", Location: "query", DiscoveryKinds: []string{"observed_query"},
		ObservationIDs: []string{id("obs", "missing")}, EvidenceIDs: []string{dangling.Records.Evidence[0].ID},
	}}
	if _, err := Compile(dangling); err == nil {
		t.Fatal("compiler accepted a dangling Parameter observation reference")
	}
	diagnostic := compilerFixture()
	diagnostic.Records.ToolExecutions[0].Warnings = []model.Diagnostic{{
		Code: "fixture.warning", Message: "Fixture warning.", Severity: "warning", EvidenceIDs: []string{id("ev", "missing-diagnostic")},
	}}
	if _, err := Compile(diagnostic); err == nil {
		t.Fatal("compiler accepted a dangling diagnostic Evidence reference")
	}
	omittedExecution := compilerFixture()
	omittedExecution.Records.Runs[0].ToolExecutionIDs = []string{}
	if _, err := Compile(omittedExecution); err == nil {
		t.Fatal("compiler accepted a Run that omitted a ToolExecution")
	}
	statusDrift := compilerFixture()
	statusDrift.Status = "partial"
	if _, err := Compile(statusDrift); err == nil {
		t.Fatal("compiler accepted a manifest status that differed from the Run")
	}
	wrongOwner := compilerFixture()
	otherExecution := wrongOwner.Records.ToolExecutions[0]
	otherExecution.ID = "tx_other"
	wrongOwner.Records.ToolExecutions = append(wrongOwner.Records.ToolExecutions, otherExecution)
	wrongOwner.Records.Runs[0].ToolExecutionIDs = append(wrongOwner.Records.Runs[0].ToolExecutionIDs, otherExecution.ID)
	otherEvidence := wrongOwner.Records.Evidence[0]
	otherEvidence.ID = id("ev", "other-execution")
	otherEvidence.ToolExecutionID = otherExecution.ID
	wrongOwner.Records.Evidence = append(wrongOwner.Records.Evidence, otherEvidence)
	wrongOwner.Records.Observations[0].EvidenceIDs = []string{otherEvidence.ID}
	if _, err := Compile(wrongOwner); err == nil || !strings.Contains(err.Error(), "another tool execution") {
		t.Fatalf("Compile error = %v, want Observation Evidence ownership rejection", err)
	}
}

func TestCompileRevalidatesRawBytesAndLocators(t *testing.T) {
	t.Run("post-normalization mutation", func(t *testing.T) {
		input := compilerFixture()
		input.RawFiles["raw/katana.jsonl"] = append(input.RawFiles["raw/katana.jsonl"], []byte("mutated\n")...)
		if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "mismatch") {
			t.Fatalf("Compile error = %v, want raw integrity mismatch", err)
		}
	})
	t.Run("artifact summary drift", func(t *testing.T) {
		input := compilerFixture()
		wrong := strings.Repeat("0", sha256.Size*2)
		input.Records.ToolExecutions[0].Artifacts[0].SHA256 = &wrong
		if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "tool execution") {
			t.Fatalf("Compile error = %v, want summary integrity rejection", err)
		}
	})
	t.Run("Evidence metadata drift", func(t *testing.T) {
		input := compilerFixture()
		input.Records.Evidence[0].Artifact.SizeBytes--
		if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "evidence") {
			t.Fatalf("Compile error = %v, want Evidence integrity rejection", err)
		}
	})

	for _, test := range []struct {
		name    string
		locator model.Locator
	}{
		{name: "line past end", locator: model.Locator{Kind: "line_range", LineStart: 1, LineEnd: 2}},
		{name: "missing JSON pointer", locator: model.Locator{Kind: "json_pointer", Pointer: "/request/missing"}},
		{name: "invalid JSON pointer escape", locator: model.Locator{Kind: "json_pointer", Pointer: "/request/~2"}},
		{name: "byte past end", locator: model.Locator{Kind: "byte_range", ByteStart: 0, ByteEndExclusive: 1 << 20}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := compilerFixture()
			input.Records.Evidence[0].Locator = test.locator
			if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "locator") {
				t.Fatalf("Compile error = %v, want locator rejection", err)
			}
		})
	}
}

func TestCompileRejectsFilesLargerThanWorkspaceReadLimit(t *testing.T) {
	input := compilerFixture()
	content := make([]byte, workspace.MaxFileBytes+1)
	digest := sha256.Sum256(content)
	digestHex, size := hex.EncodeToString(digest[:]), int64(len(content))
	input.RawFiles["raw/katana.jsonl"] = content
	input.Records.ToolExecutions[0].Artifacts[0].SHA256 = &digestHex
	input.Records.ToolExecutions[0].Artifacts[0].SizeBytes = &size
	input.Records.Evidence[0].Artifact.SHA256 = digestHex
	input.Records.Evidence[0].Artifact.SizeBytes = size
	input.Records.Evidence[0].Locator = model.Locator{Kind: "whole_artifact"}
	if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "read limit") {
		t.Fatalf("Compile error = %v, want managed read-limit rejection", err)
	}
}

func compilerFixture() Input {
	content := []byte("{\"request\":{\"method\":\"GET\",\"endpoint\":\"http://127.0.0.1/\"},\"response\":{\"status_code\":200}}\n")
	digest := sha256.Sum256(content)
	digestHex, size := hex.EncodeToString(digest[:]), int64(len(content))
	runID, txID := "run_test", "tx_test"
	assetID, endpointID, observationID, evidenceID := id("asset", "origin"), id("ep", "endpoint"), id("obs", "observation"), id("ev", "evidence")
	rule := "loopback"
	scope := model.ScopeDecision{Classification: "in_scope", RuleID: &rule, Reason: "matched origin rule loopback"}
	method, finished, observed := "GET", "2026-07-16T10:00:01Z", "2026-07-16T10:00:00Z"
	exitCode := 0
	return Input{
		RunID: runID, GeneratedAt: finished, Status: "success", RawPolicy: "embedded_sanitized",
		RawFiles: map[string][]byte{"raw/katana.jsonl": content},
		Records: model.RecordSet{
			Runs: []model.RunRecord{{
				SchemaVersion: model.SchemaVersion, RecordType: "run", ID: runID, CreatedAt: observed, FinishedAt: &finished,
				Status: "success", CanonicalizationPolicy: "url-canonicalization/v0",
				Scope:            model.RunScope{Mode: "allowlist", Roots: []model.RunScopeRoot{{Kind: "origin", Value: "http://127.0.0.1"}}, ExternalPolicy: "reject", ApprovedBy: "operator", ApprovedAt: observed},
				ToolExecutionIDs: []string{txID}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{},
			}},
			ToolExecutions: []model.ToolExecution{{
				SchemaVersion: model.SchemaVersion, RecordType: "tool_execution", ID: txID, RunID: runID,
				Tool: model.ToolIdentity{Name: "katana", Version: "v1.6.1", ResolvedPath: "/usr/bin/katana"}, AdapterVersion: "katana-adapter/v0",
				ActivityClass: "active_local", ApprovalPhase: "initial_recon", ArgvRedacted: []string{"/usr/bin/katana"}, StartedAt: &observed, FinishedAt: &finished,
				ExitCode: &exitCode, Status: "success", Coverage: "complete", Artifacts: []model.ArtifactSummary{{Role: "native_output", Path: "raw/katana.jsonl", Present: true, SHA256: &digestHex, SizeBytes: &size, MediaType: "application/x-ndjson"}}, ProviderStatus: []model.ProviderStatus{}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{},
			}},
			Assets:       []model.Asset{{SchemaVersion: model.SchemaVersion, RecordType: "asset", ID: assetID, RunID: runID, AssetKind: "origin", CanonicalValue: "http://127.0.0.1", DisplayValue: "http://127.0.0.1", Scope: scope, ObservationIDs: []string{observationID}, EvidenceIDs: []string{evidenceID}}},
			Endpoints:    []model.Endpoint{{SchemaVersion: model.SchemaVersion, RecordType: "endpoint", ID: endpointID, RunID: runID, OriginAssetID: assetID, CanonicalRouteURL: "http://127.0.0.1/", Scheme: "http", Host: "127.0.0.1", Path: "/", Method: &method, MethodKnown: true, Scope: scope, ObservationIDs: []string{observationID}, EvidenceIDs: []string{evidenceID}}},
			Observations: []model.Observation{{SchemaVersion: model.SchemaVersion, RecordType: "observation", ID: observationID, RunID: runID, ToolExecutionID: txID, ObservationType: "http_response", SemanticState: "observed", Subject: model.EntityRef{RecordType: "endpoint", ID: endpointID}, Scope: scope, ObservedAt: &observed, EvidenceIDs: []string{evidenceID}, Details: model.HTTPDetails{RequestURLRaw: "http://127.0.0.1/", CanonicalObservationURL: "http://127.0.0.1/", Method: method, StatusCode: 200}}},
			Evidence:     []model.Evidence{{SchemaVersion: model.SchemaVersion, RecordType: "evidence", ID: evidenceID, RunID: runID, ToolExecutionID: txID, Artifact: model.Artifact{Role: "native_output", Path: "raw/katana.jsonl", SHA256: digestHex, SizeBytes: size, MediaType: "application/x-ndjson", Sanitized: true}, Locator: model.Locator{Kind: "line_range", LineStart: 1, LineEnd: 1}, RedactionStatus: "not_needed", Scope: scope}},
		},
	}
}

func id(prefix, value string) string {
	digest := sha256.Sum256([]byte(value))
	return prefix + "_sha256_" + hex.EncodeToString(digest[:])
}
