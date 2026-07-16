package adapter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/scope"
)

const fixtures = "../../fixtures/cases"

func TestGAUFixtures(t *testing.T) {
	exitZero := 0
	context := testContext(t, "tx_fixture_gau", []scope.Root{{Kind: "host", Value: "fixture.test"}, {Kind: "host", Value: "finance.fixture.test"}})
	caseRoot := filepath.Join(fixtures, "gau/2.2.4/GAU-APEX-SUBS-TEXT")
	stderr := fixtureSource(t, filepath.Join(caseRoot, "stderr.sanitized.log"), "raw/gau/stderr.raw", "stderr", "text/plain")
	result, err := ParseGAU(
		fixtureSource(t, filepath.Join(caseRoot, "native-output.txt"), "raw/gau/native-output.txt", "native_output", "text/plain"),
		GAUOptions{Context: context, Providers: []string{"urlscan", "otx"}, ExitCode: &exitZero, Stderr: &stderr},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || result.Coverage != "complete" {
		t.Fatalf("GAU status = %s/%s", result.Status, result.Coverage)
	}
	if got := []int{len(result.Records.Assets), len(result.Records.Endpoints), len(result.Records.Observations), len(result.Records.Evidence)}; !slices.Equal(got, []int{2, 2, 3, 6}) {
		t.Fatalf("GAU record counts = %v", got)
	}
	for _, provider := range result.ProviderStatus {
		if provider.Status != "success" || len(provider.EvidenceIDs) == 0 {
			t.Fatalf("GAU provider lifecycle = %+v", result.ProviderStatus)
		}
	}
	for _, endpoint := range result.Records.Endpoints {
		if endpoint.Method != nil || endpoint.MethodKnown {
			t.Fatalf("GAU invented method for %s", endpoint.CanonicalRouteURL)
		}
	}
	if result.Records.Observations[0].ID == result.Records.Observations[1].ID || result.Records.Observations[1].ID == result.Records.Observations[2].ID {
		t.Fatal("GAU collapsed duplicate raw occurrences")
	}
	if !hasDiagnostic(result.Warnings, "gau.config_defaulted") {
		t.Fatal("GAU config warning was not classified")
	}

	jsonRoot := filepath.Join(fixtures, "gau/2.2.4/GAU-JSON-EXTENSIONLESS-DROP")
	jsonResult, err := ParseGAU(
		fixtureSource(t, filepath.Join(jsonRoot, "native-output.json"), "raw/gau/native-output.json", "native_output", "application/x-ndjson"),
		GAUOptions{Context: context, Format: "json", Providers: []string{"otx", "urlscan"}, ExitCode: &exitZero},
	)
	if err != nil {
		t.Fatal(err)
	}
	if jsonResult.Status != "unsupported_format" || jsonResult.Coverage != "unknown" || !hasDiagnostic(jsonResult.Gaps, "gau.json_unsupported") {
		t.Fatalf("GAU JSON regression classified as %+v", jsonResult)
	}

	errorLog := source([]byte(strings.Join([]string{
		`time="[TIMESTAMP]" level=info msg="fetching fixture.test" page=0 provider=otx`,
		`time="[TIMESTAMP]" level=warning msg="fixture.test - failed to fetch provider results" provider=otx`,
		`time="[TIMESTAMP]" level=info msg="fetching fixture.test" page=0 provider=urlscan`,
	}, "\n")+"\n"), "raw/gau/stderr.raw", "stderr", "text/plain")
	providerFailure, err := ParseGAU(
		fixtureSource(t, filepath.Join(caseRoot, "native-output.txt"), "raw/gau/native-output.txt", "native_output", "text/plain"),
		GAUOptions{Context: context, Providers: []string{"otx", "urlscan"}, ExitCode: &exitZero, Stderr: &errorLog},
	)
	if err != nil {
		t.Fatal(err)
	}
	if providerFailure.Status != "partial" || providerFailure.ProviderStatus[0].Status != "error" || !hasDiagnostic(providerFailure.Gaps, "gau.provider_error") {
		t.Fatalf("GAU provider error = %s, providers=%+v, gaps=%+v", providerFailure.Status, providerFailure.ProviderStatus, providerFailure.Gaps)
	}
	if providerFailure.ProviderStatus[1].Status != "success" {
		t.Fatalf("unaffected GAU provider = %+v", providerFailure.ProviderStatus[1])
	}

	configErrorLog := source([]byte(strings.Join([]string{
		`time="[TIMESTAMP]" level=warning msg="error reading config: invalid syntax, using default config"`,
		`time="[TIMESTAMP]" level=info msg="fetching fixture.test" page=0 provider=otx`,
		`time="[TIMESTAMP]" level=info msg="fetching fixture.test" page=0 provider=urlscan`,
	}, "\n")+"\n"), "raw/gau/stderr.raw", "stderr", "text/plain")
	configFallback, err := ParseGAU(
		fixtureSource(t, filepath.Join(caseRoot, "native-output.txt"), "raw/gau/native-output.txt", "native_output", "text/plain"),
		GAUOptions{Context: context, Providers: []string{"otx", "urlscan"}, ExitCode: &exitZero, Stderr: &configErrorLog},
	)
	if err != nil {
		t.Fatal(err)
	}
	if configFallback.Status != "partial" || !hasDiagnostic(configFallback.Gaps, "gau.config_error") {
		t.Fatalf("unexpected GAU config fallback = %s/%s, gaps=%+v", configFallback.Status, configFallback.Coverage, configFallback.Gaps)
	}
}

func TestGAURequiresProviderLifecycleEvidence(t *testing.T) {
	exitZero := 0
	context := testContext(t, "tx_fixture_gau_lifecycle", []scope.Root{{Kind: "host", Value: "fixture.test"}, {Kind: "host", Value: "finance.fixture.test"}})
	tests := []struct {
		name   string
		stderr *Source
	}{
		{name: "missing stderr"},
		{name: "empty stderr", stderr: func() *Source {
			value := source([]byte{}, "raw/gau/stderr.raw", "stderr", "text/plain")
			return &value
		}()},
		{name: "missing selected provider", stderr: func() *Source {
			value := source([]byte(`time="[TIMESTAMP]" level=info msg="fetching fixture.test" page=0 provider=otx`+"\n"), "raw/gau/stderr.raw", "stderr", "text/plain")
			return &value
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			native := fixtureSource(t, filepath.Join(fixtures, "gau/2.2.4/GAU-APEX-SUBS-TEXT/native-output.txt"), "raw/gau/native-output.txt", "native_output", "text/plain")
			result, err := ParseGAU(native, GAUOptions{Context: context, Providers: []string{"otx", "urlscan"}, ExitCode: &exitZero, Stderr: test.stderr})
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "partial" || !hasDiagnostic(result.Gaps, "gau.provider_state_unknown") {
				t.Fatalf("GAU lifecycle gap = %s/%s providers=%+v gaps=%+v", result.Status, result.Coverage, result.ProviderStatus, result.Gaps)
			}
			if test.name == "missing selected provider" && (result.ProviderStatus[0].Status != "success" || result.ProviderStatus[1].Status != "unknown") {
				t.Fatalf("GAU provider states = %+v", result.ProviderStatus)
			}
		})
	}

	t.Run("incomplete capture", func(t *testing.T) {
		stderr := source([]byte(strings.Join([]string{
			`time="[TIMESTAMP]" level=info msg="fetching fixture.test" page=0 provider=otx`,
			`time="[TIMESTAMP]" level=info msg="fetching fixture.test" page=0 provider=urlscan`,
		}, "\n")+"\n"), "raw/gau/stderr.raw", "stderr", "text/plain")
		native := fixtureSource(t, filepath.Join(fixtures, "gau/2.2.4/GAU-APEX-SUBS-TEXT/native-output.txt"), "raw/gau/native-output.txt", "native_output", "text/plain")
		result, err := ParseGAU(native, GAUOptions{Context: context, Providers: []string{"otx", "urlscan"}, ExitCode: &exitZero, Stderr: &stderr, Incomplete: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "partial" || result.ProviderStatus[0].Status != "unknown" || result.ProviderStatus[1].Status != "unknown" {
			t.Fatalf("incomplete GAU lifecycle = %s providers=%+v", result.Status, result.ProviderStatus)
		}
	})
}

func TestKatanaFixtures(t *testing.T) {
	exitZero, exitInterrupted := 0, 124
	context := testContext(t, "tx_fixture_katana", []scope.Root{{Kind: "host", Value: "127.0.0.1"}})
	normal := fixtureSource(t, filepath.Join(fixtures, "katana/v1.6.1/KAT-NORMAL-MINIMAL/native-output.jsonl"), "raw/katana/native-output.jsonl", "native_output", "application/x-ndjson")
	result, err := ParseKatana(normal, KatanaOptions{Context: context, ExitCode: &exitZero})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || result.Coverage != "complete" {
		t.Fatalf("Katana status = %s/%s", result.Status, result.Coverage)
	}
	if got := []int{len(result.Records.Endpoints), len(result.Records.Parameters), len(result.Records.Observations), len(result.Records.Evidence)}; !slices.Equal(got, []int{6, 2, 8, 6}) {
		t.Fatalf("Katana record counts = %v", got)
	}
	for _, parameter := range result.Records.Parameters {
		if parameter.Location != "query" || !slices.Equal(parameter.DiscoveryKinds, []string{"observed_query"}) {
			t.Fatalf("Katana parameter = %+v", parameter)
		}
	}

	interrupted := fixtureSource(t, filepath.Join(fixtures, "katana/v1.6.1/KAT-INTERRUPTED-LOOPBACK/native-output.jsonl"), "raw/katana-interrupted/native-output.jsonl", "native_output", "application/x-ndjson")
	partial, err := ParseKatana(interrupted, KatanaOptions{Context: context, ExitCode: &exitInterrupted, Interrupted: true, TimedOut: true})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Status != "interrupted" || partial.Coverage != "partial" || len(partial.Records.Observations) != 3 {
		t.Fatalf("interrupted Katana = %s/%s, observations=%d", partial.Status, partial.Coverage, len(partial.Records.Observations))
	}
}

func TestKatanaKeepsValidSiblings(t *testing.T) {
	exitZero := 0
	context := testContext(t, "tx_katana_malformed", []scope.Root{{Kind: "host", Value: "127.0.0.1"}})
	valid, err := os.ReadFile(filepath.Join(fixtures, "katana/v1.6.1/KAT-INTERRUPTED-LOOPBACK/native-output.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	raw := append(append([]byte{}, valid...), []byte("{malformed}\n")...)
	result, err := ParseKatana(source(raw, "raw/katana/native-output.jsonl", "native_output", "application/x-ndjson"), KatanaOptions{Context: context, ExitCode: &exitZero})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || len(result.Records.Observations) != 3 || !hasDiagnostic(result.Gaps, "katana.malformed_record") {
		t.Fatalf("malformed sibling handling = %s, observations=%d, gaps=%+v", result.Status, len(result.Records.Observations), result.Gaps)
	}
}

func TestArjunFoundFixtures(t *testing.T) {
	exitZero := 0
	cases := []struct {
		caseID, rawDir, target, method, wantHTTP, wantLocation string
		wantParams                                             []string
	}{
		{"ARJUN-GET-FOUND", "arjun-get", "http://127.0.0.1:18080/api/search", "GET", "GET", "query", []string{"q"}},
		{"ARJUN-POST-FORM-FOUND", "arjun-post-form", "http://127.0.0.1:18080/api/update", "POST", "POST", "form", []string{"id", "name"}},
		{"ARJUN-JSON-FOUND", "arjun-json", "http://127.0.0.1:18080/api/json", "JSON", "POST", "json", []string{"filter", "id"}},
	}
	for _, test := range cases {
		t.Run(test.caseID, func(t *testing.T) {
			root := filepath.Join(fixtures, "arjun/2.2.7", test.caseID)
			native := fixtureSource(t, filepath.Join(root, "native-output.json"), "raw/"+test.rawDir+"/native-output.json", "native_output", "application/json")
			stdout := fixtureSource(t, filepath.Join(root, "stdout.sanitized.log"), "raw/"+test.rawDir+"/stdout.raw", "stdout", "text/plain")
			result, err := ParseArjun(ArjunOptions{
				Context: testContext(t, "tx_fixture_"+stringsForID(test.rawDir), []scope.Root{{Kind: "host", Value: "127.0.0.1"}}),
				Native:  &native, Stdout: &stdout, TargetURL: test.target, SourceMethod: test.method, ExitCode: &exitZero,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "success" || result.Coverage != "complete" {
				t.Fatalf("Arjun status = %s/%s", result.Status, result.Coverage)
			}
			if len(result.Records.Endpoints) != 1 || result.Records.Endpoints[0].Method == nil || *result.Records.Endpoints[0].Method != test.wantHTTP {
				t.Fatalf("Arjun endpoint = %+v", result.Records.Endpoints)
			}
			got := make([]string, len(result.Records.Parameters))
			for index, parameter := range result.Records.Parameters {
				got[index] = parameter.Name
				if parameter.Location != test.wantLocation {
					t.Fatalf("parameter location = %q", parameter.Location)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, test.wantParams) {
				t.Fatalf("Arjun params = %v", got)
			}
		})
	}
}

func TestArjunAbsentNativeFixtures(t *testing.T) {
	exitZero, exitInterrupted, exitFailure := 0, 124, 1
	context := testContext(t, "tx_fixture_arjun_absent", []scope.Root{{Kind: "host", Value: "127.0.0.1"}})

	zeroRoot := filepath.Join(fixtures, "arjun/2.2.7/ARJUN-ZERO")
	zeroStdout := fixtureSource(t, filepath.Join(zeroRoot, "stdout.sanitized.log"), "raw/arjun-zero/stdout.raw", "stdout", "text/plain")
	zero, err := ParseArjun(ArjunOptions{Context: context, Stdout: &zeroStdout, TargetURL: "http://127.0.0.1:18080/api/no-params", SourceMethod: "GET", ExitCode: &exitZero})
	if err != nil {
		t.Fatal(err)
	}
	if zero.Status != "success_zero" || zero.Coverage != "zero" || len(zero.Records.Observations) != 1 || zero.Records.Observations[0].ObservationType != "zero_result" {
		t.Fatalf("Arjun zero = %s/%s, records=%+v", zero.Status, zero.Coverage, zero.Records)
	}

	interruptedRoot := filepath.Join(fixtures, "arjun/2.2.7/ARJUN-INTERRUPTED-LOOPBACK")
	interruptedStdout := fixtureSource(t, filepath.Join(interruptedRoot, "stdout.sanitized.log"), "raw/arjun-interrupted/stdout.raw", "stdout", "text/plain")
	interrupted, err := ParseArjun(ArjunOptions{Context: context, Stdout: &interruptedStdout, TargetURL: "http://127.0.0.1:18080/api/search", SourceMethod: "GET", ExitCode: &exitInterrupted, Interrupted: true, TimedOut: true})
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != "interrupted" || interrupted.Coverage != "partial" || len(interrupted.Records.Observations) != 0 || len(interrupted.Records.Evidence) != 1 {
		t.Fatalf("Arjun interrupted = %s/%s, records=%+v", interrupted.Status, interrupted.Coverage, interrupted.Records)
	}

	timeoutRoot := filepath.Join(fixtures, "arjun/2.2.7/ARJUN-REQUEST-TIMEOUT-LOOPBACK")
	timeoutStdout := fixtureSource(t, filepath.Join(timeoutRoot, "stdout.sanitized.log"), "raw/arjun-timeout/stdout.raw", "stdout", "text/plain")
	timeoutStderr := fixtureSource(t, filepath.Join(timeoutRoot, "stderr.sanitized.log"), "raw/arjun-timeout/stderr.raw", "stderr", "text/plain")
	failed, err := ParseArjun(ArjunOptions{Context: context, Stdout: &timeoutStdout, Stderr: &timeoutStderr, TargetURL: "http://127.0.0.1:18080/slow?delay_ms=5000", SourceMethod: "GET", ExitCode: &exitFailure})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" || failed.Coverage != "unknown" || len(failed.Records.Observations) != 0 || !hasDiagnostic(failed.Gaps, "arjun.tool_error") {
		t.Fatalf("Arjun timeout failure = %s/%s, gaps=%+v", failed.Status, failed.Coverage, failed.Gaps)
	}
}

func TestArjunMalformedTargetsDoNotCreateOrphanEntities(t *testing.T) {
	exitZero := 0
	context := testContext(t, "tx_fixture_arjun_malformed", []scope.Root{{Kind: "host", Value: "127.0.0.1"}})
	for _, test := range []struct {
		name, raw, target string
	}{
		{"unexpected target", `{"http://127.0.0.1:18080/api/other":{"method":"GET","params":["q"]}}`, "http://127.0.0.1:18080/api/search"},
		{"empty parameters", `{"http://127.0.0.1:18080/api/search":{"method":"GET","params":[]}}`, "http://127.0.0.1:18080/api/search"},
		{"all invalid parameters", `{"http://127.0.0.1:18080/api/search":{"method":"GET","params":[""]}}`, "http://127.0.0.1:18080/api/search"},
	} {
		t.Run(test.name, func(t *testing.T) {
			native := source([]byte(test.raw), "raw/arjun-malformed/native-output.json", "native_output", "application/json")
			result, err := ParseArjun(ArjunOptions{Context: context, Native: &native, TargetURL: test.target, SourceMethod: "GET", ExitCode: &exitZero})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Records.Assets) != 0 || len(result.Records.Endpoints) != 0 || len(result.Records.Parameters) != 0 || len(result.Records.Observations) != 0 || len(result.Records.Evidence) == 0 {
				t.Fatalf("malformed Arjun result created orphan entities: %#v", result.Records)
			}
		})
	}
}

func TestArtifactBoundsAndDigest(t *testing.T) {
	exitZero := 0
	context := testContext(t, "tx_fixture_bounds", []scope.Root{{Kind: "host", Value: "fixture.test"}})
	tooLarge := bytes.Repeat([]byte("x"), MaxArtifactBytes+1)
	if _, err := ParseGAU(source(tooLarge, "raw/gau.txt", "native_output", "text/plain"), GAUOptions{Context: context, Providers: []string{"otx"}, ExitCode: &exitZero}); err == nil {
		t.Fatal("oversized artifact was accepted")
	}
	bad := source([]byte("https://fixture.test/\n"), "raw/gau.txt", "native_output", "text/plain")
	bad.Artifact.SHA256 = stringsForID("wrong")
	if _, err := ParseGAU(bad, GAUOptions{Context: context, Providers: []string{"otx"}, ExitCode: &exitZero}); err == nil {
		t.Fatal("artifact hash mismatch was accepted")
	}
}

func FuzzNativeParsers(f *testing.F) {
	f.Add(uint8(0), []byte("https://fixture.test/search?q=seed\n"))
	f.Add(uint8(1), []byte(`{"timestamp":"2026-07-12T21:58:26Z","request":{"method":"GET","endpoint":"http://127.0.0.1:18080/api/search"},"response":{"status_code":200,"headers":{"Content-Type":"application/json"},"content_length":2}}`+"\n"))
	f.Add(uint8(2), []byte(`{"http://127.0.0.1:18080/api/search":{"method":"GET","params":["q"]}}`))
	for selector := uint8(0); selector < 3; selector++ {
		f.Add(selector, []byte("{malformed}\n"))
	}
	evaluator, err := scope.NewEvaluator(scope.Config{Mode: "allowlist", ExternalPolicy: "record_only", Roots: []scope.Root{{Kind: "host", Value: "fixture.test"}, {Kind: "host", Value: "127.0.0.1"}}})
	if err != nil {
		f.Fatal(err)
	}
	context := Context{RunID: "run_fuzz", ToolExecutionID: "tx_fuzz", Scope: evaluator}
	f.Fuzz(func(t *testing.T, selector uint8, raw []byte) {
		if len(raw) > 64<<10 {
			return
		}
		first, firstErr := parseFuzzInput(selector, raw, context)
		second, secondErr := parseFuzzInput(selector, raw, context)
		if (firstErr == nil) != (secondErr == nil) || firstErr != nil && firstErr.Error() != secondErr.Error() || !reflect.DeepEqual(first, second) {
			t.Fatalf("parser %d was non-deterministic", selector%3)
		}
	})
}

func parseFuzzInput(selector uint8, raw []byte, context Context) (Result, error) {
	exitZero := 0
	switch selector % 3 {
	case 0:
		return ParseGAU(source(raw, "raw/gau/native-output.txt", "native_output", "text/plain"), GAUOptions{Context: context, Providers: []string{"otx"}, ExitCode: &exitZero})
	case 1:
		return ParseKatana(source(raw, "raw/katana/native-output.jsonl", "native_output", "application/x-ndjson"), KatanaOptions{Context: context, ExitCode: &exitZero})
	default:
		native := source(raw, "raw/arjun/native-output.json", "native_output", "application/json")
		return ParseArjun(ArjunOptions{Context: context, Native: &native, ExitCode: &exitZero})
	}
}

func testContext(t *testing.T, txID string, roots []scope.Root) Context {
	t.Helper()
	evaluator, err := scope.NewEvaluator(scope.Config{Mode: "allowlist", ExternalPolicy: "record_only", Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	return Context{RunID: "run_fixture_web_blackbox_v0", ToolExecutionID: txID, Scope: evaluator}
}

func fixtureSource(t *testing.T, filename, artifactPath, role, mediaType string) Source {
	t.Helper()
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return source(raw, artifactPath, role, mediaType)
}

func source(raw []byte, artifactPath, role, mediaType string) Source {
	digest := sha256.Sum256(raw)
	return Source{
		Reader:   bytes.NewReader(raw),
		Artifact: model.Artifact{Role: role, Path: artifactPath, SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(raw)), MediaType: mediaType, Sanitized: true},
	}
}

func hasDiagnostic(diagnostics []model.Diagnostic, code string) bool {
	return slices.ContainsFunc(diagnostics, func(diagnostic model.Diagnostic) bool { return diagnostic.Code == code })
}

func stringsForID(value string) string {
	result := make([]byte, 64)
	for index := range result {
		result[index] = "0123456789abcdef"[(index+len(value))%16]
	}
	return string(result)
}
