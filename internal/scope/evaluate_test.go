package scope

import (
	"bytes"
	"strings"
	"testing"
)

func TestEvaluatorClassifiesCanonicalScope(t *testing.T) {
	t.Parallel()

	evaluator, err := NewEvaluator(Config{
		Mode:           "allowlist",
		ExternalPolicy: "reject",
		Roots: []Root{
			{ID: "origin_fixture", Kind: "origin", Value: "http://127.0.0.1:18080"},
			{ID: "host_fixture", Kind: "host", Value: "fixture.test"},
			{ID: "prefix_api", Kind: "url_prefix", Value: "https://api.example.test/v1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		url            string
		classification Classification
		active         bool
	}{
		{"origin", "http://127.0.0.1:18080/anything", InScope, true},
		{"host", "https://fixture.test/path", InScope, true},
		{"prefix", "https://api.example.test/v1/users", InScope, true},
		{"prefix safe unreserved escape", "https://api.example.test/v1/%7Euser", InScope, true},
		{"prefix encoded separator", "https://api.example.test/v1/%2e%2e%2fadmin", OutOfScope, false},
		{"prefix double-encoded separator", "https://api.example.test/v1/%252e%252e%252fadmin", OutOfScope, false},
		{"prefix boundary", "https://api.example.test/v10/users", OutOfScope, false},
		{"outside", "https://outside.example/", OutOfScope, false},
		{"invalid", "not a URL", Unknown, false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			decision := evaluator.EvaluateURL(test.url)
			if decision.Classification != test.classification {
				t.Fatalf("classification = %q, want %q (%s)", decision.Classification, test.classification, decision.Reason)
			}
			if got := decision.AllowedForActive(); got != test.active {
				t.Fatalf("AllowedForActive = %t, want %t", got, test.active)
			}
		})
	}
}

func TestLoadJSONIsStrict(t *testing.T) {
	t.Parallel()

	config, err := LoadJSON(strings.NewReader(`{
		"mode":"allowlist",
		"roots":[{"id":"loopback","kind":"origin","value":"http://127.0.0.1:18080"}],
		"external_policy":"reject"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Roots) != 1 || config.Roots[0].ID != "loopback" {
		t.Fatalf("loaded config = %#v", config)
	}
	invalid := []string{
		`{"mode":"allowlist","mode":"allowlist","roots":[],"external_policy":"reject"}`,
		`{"mode":"allowlist","Mode":"allowlist","roots":[],"external_policy":"reject"}`,
		`{"mode":"allowlist","roots":[],"external_policy":"reject","unknown":true}`,
		`{"mode":"allowlist","roots":[{"kind":"host","kind":"host","value":"fixture.test"}],"external_policy":"reject"}`,
		`{"mode":"allowlist","roots":[{"Kind":"host","value":"fixture.test"}],"external_policy":"reject"}`,
		`{"mode":"allowlist","roots":[{"kind":"host","value":"fixture.test","unknown":true}],"external_policy":"reject"}`,
	}
	for _, document := range invalid {
		if _, err := LoadJSON(strings.NewReader(document)); err == nil {
			t.Errorf("LoadJSON accepted ambiguous schema: %s", document)
		}
	}
	if _, err := LoadJSON(bytes.NewReader(make([]byte, maxScopeDocumentBytes+1))); err == nil {
		t.Fatal("LoadJSON accepted an oversized document")
	}
}

func TestEvaluatorSupportsIPv6HostRoots(t *testing.T) {
	t.Parallel()
	evaluator, err := NewEvaluator(Config{Mode: "allowlist", ExternalPolicy: "reject", Roots: []Root{{Kind: "host", Value: "2001:0db8::1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if decision := evaluator.EvaluateURL("https://[2001:db8::1]/path"); decision.Classification != InScope {
		t.Fatalf("classification = %q (%s)", decision.Classification, decision.Reason)
	}
}

func TestEvaluatorRejectsScopeRootsThatNormalizeWider(t *testing.T) {
	t.Parallel()
	for _, root := range []Root{
		{Kind: "origin", Value: "https://example.test/a/.."},
		{Kind: "url_prefix", Value: "https://example.test/a/.."},
		{Kind: "url_prefix", Value: "https://example.test/%2e%2e/"},
		{Kind: "url_prefix", Value: "https://example.test/public/%2fprivate"},
	} {
		if _, err := NewEvaluator(Config{Mode: "allowlist", ExternalPolicy: "reject", Roots: []Root{root}}); err == nil {
			t.Errorf("NewEvaluator accepted widening root %#v", root)
		}
	}
}

func TestEvaluatorRejectsDuplicateRuleIDs(t *testing.T) {
	t.Parallel()
	for _, roots := range [][]Root{
		{{ID: "duplicate", Kind: "host", Value: "a.example"}, {ID: "duplicate", Kind: "host", Value: "b.example"}},
		{{ID: "scope_root_2", Kind: "host", Value: "a.example"}, {Kind: "host", Value: "b.example"}},
	} {
		if _, err := NewEvaluator(Config{Mode: "allowlist", ExternalPolicy: "reject", Roots: roots}); err == nil {
			t.Fatalf("NewEvaluator accepted duplicate IDs: %#v", roots)
		}
	}
}

func TestLoadYAMLIsStrict(t *testing.T) {
	t.Parallel()

	config, err := LoadYAML(strings.NewReader("mode: allowlist\nroots:\n  - id: loopback\n    kind: origin\n    value: http://127.0.0.1:18080\nexternal_policy: reject\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Roots) != 1 || config.Roots[0].ID != "loopback" {
		t.Fatalf("loaded config = %#v", config)
	}
	if _, err := LoadYAML(strings.NewReader("mode: allowlist\nroots: []\nexternal_policy: reject\nunknown: true\n")); err == nil {
		t.Fatal("LoadYAML accepted an unknown field")
	}
	if _, err := LoadYAML(strings.NewReader("mode: allowlist\nroots: []\nexternal_policy: reject\n---\n{}\n")); err == nil {
		t.Fatal("LoadYAML accepted multiple documents")
	}
	if _, err := LoadYAML(bytes.NewReader(make([]byte, maxScopeDocumentBytes+1))); err == nil {
		t.Fatal("LoadYAML accepted an oversized document")
	}
}
