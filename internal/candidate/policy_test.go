package candidate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/adapter"
	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/scope"
)

func TestBuildRanksCapsExplainsAndBindsArjunQueue(t *testing.T) {
	inScope := scopeDecision("in_scope")
	outOfScope := model.ScopeDecision{Classification: "out_of_scope", Reason: "no allowlist root matched"}
	api := testEndpoint(t, "GET", "https://fixture.test/api/search", inScope, []string{observationID('1'), observationID('2')}, []string{evidenceID('1')})
	page := testEndpoint(t, "GET", "https://fixture.test/page", inScope, []string{observationID('3')}, []string{evidenceID('3')})
	static := testEndpoint(t, "GET", "https://fixture.test/app.js", inScope, []string{observationID('4')}, []string{evidenceID('4')})
	historical := testEndpoint(t, "", "https://fixture.test/old", inScope, []string{observationID('5')}, []string{evidenceID('5')})
	outside := testEndpoint(t, "GET", "https://outside.test/search", outOfScope, []string{observationID('6')}, []string{evidenceID('6')})
	admin := testEndpoint(t, "GET", "https://fixture.test/admin/users", inScope, []string{observationID('7')}, []string{evidenceID('7')})
	fragment := testEndpoint(t, "GET", "https://fixture.test/fragment", inScope, []string{observationID('8')}, []string{evidenceID('8')})
	records := model.RecordSet{
		ToolExecutions: []model.ToolExecution{
			{ID: "tx_katana", Tool: model.ToolIdentity{Name: "katana"}},
			{ID: "tx_gau", Tool: model.ToolIdentity{Name: "gau"}},
		},
		Endpoints: []model.Endpoint{page, static, historical, outside, api, admin, fragment, page},
		Parameters: []model.Parameter{{
			ID: "param_sha256_" + strings.Repeat("9", 64), EndpointID: api.ID, Location: "query",
			ObservationIDs: []string{observationID('9')}, EvidenceIDs: []string{evidenceID('9')},
		}},
		Observations: []model.Observation{
			httpObservation(api, observationID('1'), evidenceID('1'), "tx_katana", api.CanonicalRouteURL, nil),
			historicalObservation(api, observationID('2'), evidenceID('2'), "tx_gau", api.CanonicalRouteURL+"?q=1"),
			httpObservation(page, observationID('3'), evidenceID('3'), "tx_katana", page.CanonicalRouteURL, nil),
			httpObservation(static, observationID('4'), evidenceID('4'), "tx_katana", static.CanonicalRouteURL, stringPointer("application/javascript")),
			historicalObservation(historical, observationID('5'), evidenceID('5'), "tx_gau", historical.CanonicalRouteURL),
			httpObservation(outside, observationID('6'), evidenceID('6'), "tx_katana", outside.CanonicalRouteURL, nil),
			httpObservation(admin, observationID('7'), evidenceID('7'), "tx_katana", admin.CanonicalRouteURL, nil),
			httpObservation(fragment, observationID('8'), evidenceID('8'), "tx_katana", fragment.CanonicalRouteURL+"#section", nil),
			{ID: observationID('9'), ToolExecutionID: "tx_gau", ObservationType: "parameter_discovery", SemanticState: "historical", Subject: model.EntityRef{RecordType: "parameter", ID: "param_sha256_" + strings.Repeat("9", 64)}, EvidenceIDs: []string{evidenceID('9')}},
		},
	}
	config := testConfig()
	result, err := Build(records, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Queue.Candidates) != 1 || result.Queue.Candidates[0].EndpointID != api.ID {
		t.Fatalf("selected queue = %#v", result.Queue.Candidates)
	}
	selected := result.Queue.Candidates[0]
	if selected.RankPosition != 1 || !selected.Rank.CurrentlyObservedByKatana || !selected.Rank.ExistingQueryNameEvidence || !selected.Rank.APILikePath || selected.Rank.IndependentExecutions != 2 {
		t.Fatalf("selected rank = %#v", selected.Rank)
	}
	if len(selected.Argv) < 2 || selected.Argv[len(selected.Argv)-2] != "-oJ" || selected.Argv[len(selected.Argv)-1] != selected.NativeOutputPath || !strings.HasPrefix(selected.NativeOutputPath, config.NativeOutputRoot+"/") {
		t.Fatalf("selected argv/output = %#v / %q", selected.Argv, selected.NativeOutputPath)
	}
	if result.QueueDigest == "" || result.Decisions[0].QueueDigest != result.QueueDigest || result.Decisions[0].ArgvRedacted[0] != "<ARJUN>" || slices.Contains(result.Decisions[0].ArgvRedacted, config.WordlistPath) {
		t.Fatalf("public decision binding/redaction = %#v", result.Decisions[0])
	}
	assertReason(t, result.Decisions, static.CanonicalRouteURL, "static_extension")
	assertReason(t, result.Decisions, static.CanonicalRouteURL, "static_mime")
	assertReason(t, result.Decisions, historical.CanonicalRouteURL, "historical_only")
	assertReason(t, result.Decisions, historical.CanonicalRouteURL, "unsupported_method_location")
	assertReason(t, result.Decisions, outside.CanonicalRouteURL, "out_of_scope")
	assertReason(t, result.Decisions, admin.CanonicalRouteURL, "excluded_path")
	assertReason(t, result.Decisions, fragment.CanonicalRouteURL, "fragment_only")
	assertReason(t, result.Decisions, page.CanonicalRouteURL, "max_targets_overflow")
	assertReason(t, result.Decisions, page.CanonicalRouteURL, "canonical_duplicate")

	reversed := records
	reversed.Endpoints = slices.Clone(records.Endpoints)
	reversed.Observations = slices.Clone(records.Observations)
	slices.Reverse(reversed.Endpoints)
	slices.Reverse(reversed.Observations)
	again, err := Build(reversed, config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, again) {
		t.Fatal("candidate policy depended on input order")
	}

	drifted := result.Queue
	drifted.Candidates = slices.Clone(result.Queue.Candidates)
	drifted.Candidates[0].EvidenceIDs = append(slices.Clone(drifted.Candidates[0].EvidenceIDs), evidenceID('a'))
	digest, err := approval.QueueDigest(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if digest == result.QueueDigest {
		t.Fatal("evidence drift did not change exact queue digest")
	}
	drifted = result.Queue
	drifted.Candidates = slices.Clone(result.Queue.Candidates)
	drifted.Candidates[0].Rank.APILikePath = false
	digest, err = approval.QueueDigest(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if digest == result.QueueDigest {
		t.Fatal("rank drift did not change exact queue digest")
	}
}

func TestBuildMapsSupportedPOSTLocationsAndRejectsUnsafeConfig(t *testing.T) {
	endpoint := testEndpoint(t, "POST", "https://fixture.test/api/items", scopeDecision("in_scope"), []string{observationID('1')}, []string{evidenceID('1')})
	records := model.RecordSet{
		ToolExecutions: []model.ToolExecution{{ID: "tx_katana", Tool: model.ToolIdentity{Name: "katana"}}},
		Endpoints:      []model.Endpoint{endpoint},
		Parameters: []model.Parameter{
			{ID: "param_sha256_" + strings.Repeat("1", 64), EndpointID: endpoint.ID, Location: "form"},
			{ID: "param_sha256_" + strings.Repeat("2", 64), EndpointID: endpoint.ID, Location: "json"},
		},
		Observations: []model.Observation{httpObservation(endpoint, observationID('1'), evidenceID('1'), "tx_katana", endpoint.CanonicalRouteURL, nil)},
	}
	config := testConfig()
	config.MaxTargets = 2
	result, err := Build(records, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Queue.Candidates) != 2 {
		t.Fatalf("POST candidates = %#v", result.Queue.Candidates)
	}
	modes := []string{result.Queue.Candidates[0].SourceMode, result.Queue.Candidates[1].SourceMode}
	slices.Sort(modes)
	if !slices.Equal(modes, []string{"JSON", "POST"}) {
		t.Fatalf("POST source modes = %v", modes)
	}
	config.NativeOutputRoot = "relative"
	if _, err := Build(records, config); err == nil {
		t.Fatal("relative native output root accepted")
	}
}

func TestBuildEncodesMissingProvenanceCollectionsAsArrays(t *testing.T) {
	endpoint := testEndpoint(t, "GET", "https://fixture.test/api/missing", scopeDecision("in_scope"), nil, nil)
	result, err := Build(model.RecordSet{Endpoints: []model.Endpoint{endpoint}}, testConfig())
	if err != nil || len(result.Decisions) != 1 {
		t.Fatalf("Build missing provenance = %#v, %v", result.Decisions, err)
	}
	encoded, err := canonical.Marshal(result.Decisions[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`"observation_ids":null`, `"evidence_ids":null`, `"source_execution_ids":null`} {
		if bytes.Contains(encoded, []byte(invalid)) {
			t.Fatalf("candidate contains schema-invalid %s: %s", invalid, encoded)
		}
	}
}

func TestBuildUsesKatanaFixtureWithoutSchedulingStaticAssets(t *testing.T) {
	raw, err := os.ReadFile("../../fixtures/cases/katana/v1.6.1/KAT-NORMAL-MINIMAL/native-output.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := scope.NewEvaluator(scope.Config{
		Mode: "allowlist", ExternalPolicy: "reject",
		Roots: []scope.Root{{ID: "loopback", Kind: "origin", Value: "http://127.0.0.1:18080"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	exitCode := 0
	parsed, err := adapter.ParseKatana(adapter.Source{
		Reader:   bytes.NewReader(raw),
		Artifact: model.Artifact{Role: "native_output", Path: "raw/katana/native-output.jsonl", SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(raw)), MediaType: "application/x-ndjson", Sanitized: true},
	}, adapter.KatanaOptions{Context: adapter.Context{RunID: "run_fixture", ToolExecutionID: "tx_katana", Scope: evaluator}, ExitCode: &exitCode})
	if err != nil {
		t.Fatal(err)
	}
	config := testConfig()
	config.MaxTargets = 25
	result, err := Build(parsed.Records, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Decisions) != 6 || len(result.Queue.Candidates) != 5 {
		t.Fatalf("fixture candidates: %d decisions, %d selected", len(result.Decisions), len(result.Queue.Candidates))
	}
	if result.Queue.Candidates[0].URL != "http://127.0.0.1:18080/api/users" || !result.Queue.Candidates[0].Rank.ExistingQueryNameEvidence || !result.Queue.Candidates[0].Rank.APILikePath {
		t.Fatalf("fixture top rank = %#v", result.Queue.Candidates[0])
	}
	assertReason(t, result.Decisions, "http://127.0.0.1:18080/static/app.js", "static_extension")
}

func testConfig() Config {
	return Config{
		PlanDigest: "sha256:" + strings.Repeat("a", 64), ArjunPath: "/tools/arjun",
		WordlistPath: "/wordlists/params.txt", WordlistSHA256: "sha256:" + strings.Repeat("b", 64),
		NativeOutputRoot: "/private/runs/run_test/arjun", Limits: model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, RequestTimeoutSeconds: 15, ExecutionTimeoutSeconds: 7200},
		MaxTargets: 1, RequestBudget: 100, ExcludedPathPrefixes: []string{"/admin"},
	}
}

func testEndpoint(t *testing.T, method, rawURL string, scope model.ScopeDecision, observations, evidence []string) model.Endpoint {
	t.Helper()
	value, err := canonical.CanonicalizeURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	var methodPointer *string
	if method != "" {
		methodPointer = &method
	}
	id, err := canonical.EndpointID(methodPointer, value.CanonicalRouteURL)
	if err != nil {
		t.Fatal(err)
	}
	return model.Endpoint{ID: id, CanonicalRouteURL: value.CanonicalRouteURL, Path: value.Path, Method: methodPointer, MethodKnown: methodPointer != nil, Scope: scope, ObservationIDs: observations, EvidenceIDs: evidence}
}

func httpObservation(endpoint model.Endpoint, id, evidence, execution, rawURL string, contentType *string) model.Observation {
	return model.Observation{
		ID: id, ToolExecutionID: execution, ObservationType: "http_response", SemanticState: "observed",
		Subject: model.EntityRef{RecordType: "endpoint", ID: endpoint.ID}, EvidenceIDs: []string{evidence},
		Details: model.HTTPDetails{RequestURLRaw: rawURL, CanonicalObservationURL: rawURL, Method: "GET", StatusCode: 200, ContentType: contentType},
	}
}

func historicalObservation(endpoint model.Endpoint, id, evidence, execution, rawURL string) model.Observation {
	return model.Observation{
		ID: id, ToolExecutionID: execution, ObservationType: "historical_url", SemanticState: "historical",
		Subject: model.EntityRef{RecordType: "endpoint", ID: endpoint.ID}, EvidenceIDs: []string{evidence},
		Details: model.HistoricalDetails{URLRaw: rawURL, CanonicalRouteURL: endpoint.CanonicalRouteURL, CurrentReachability: "unknown"},
	}
}

func scopeDecision(classification string) model.ScopeDecision {
	rule := "fixture"
	return model.ScopeDecision{Classification: classification, RuleID: &rule, Reason: "origin allowlist root matched"}
}

func assertReason(t *testing.T, decisions []Decision, route, reason string) {
	t.Helper()
	for _, decision := range decisions {
		if decision.CanonicalRouteURL == route && slices.Contains(decision.ReasonCodes, reason) {
			return
		}
	}
	t.Fatalf("%s missing reason %s in %#v", route, reason, decisions)
}

func observationID(character byte) string {
	return "obs_sha256_" + strings.Repeat(string(character), 64)
}
func evidenceID(character byte) string   { return "ev_sha256_" + strings.Repeat(string(character), 64) }
func stringPointer(value string) *string { return &value }
