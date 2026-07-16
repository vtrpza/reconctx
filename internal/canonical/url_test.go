package canonical

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestURLDifferentialHelper(t *testing.T) {
	if os.Getenv("RECONCTX_DIFFERENTIAL_HELPER") != "1" {
		t.Skip("invoked only by the Python differential gate")
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var input string
		if err := json.Unmarshal(scanner.Bytes(), &input); err != nil {
			t.Fatal(err)
		}
		value, err := CanonicalizeURL(input)
		response := struct {
			Value *URL `json:"value,omitempty"`
			Error bool `json:"error"`
		}{Error: err != nil}
		if err == nil {
			response.Value = &value
		}
		if err := encoder.Encode(response); err != nil {
			t.Fatal(err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestURLCompatibilityVectors(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonicalization", "v0", "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		PolicyVersion string `json:"policy_version"`
		URLCases      []struct {
			ID       string         `json:"id"`
			Input    string         `json:"input"`
			Expected map[string]any `json:"expected"`
			Error    string         `json:"error"`
		} `json:"url_cases"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.PolicyVersion != URLPolicyVersion {
		t.Fatalf("policy version = %q", vectors.PolicyVersion)
	}

	for _, test := range vectors.URLCases {
		test := test
		t.Run(test.ID, func(t *testing.T) {
			actual, err := CanonicalizeURL(test.Input)
			if test.Error != "" {
				if err == nil {
					t.Fatalf("CanonicalizeURL(%q) succeeded, want %s", test.Input, test.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(fields, test.Expected) {
				t.Errorf("%s: result = %#v, want %#v", test.ID, fields, test.Expected)
			}
		})
	}
}

func TestURLQueryAndIdentitySemantics(t *testing.T) {
	t.Parallel()

	result, err := CanonicalizeURL("HTTP://Example.COM:80/users?id=1&id=2&debug&empty=#fragment")
	if err != nil {
		t.Fatal(err)
	}
	wantPairs := []QueryPair{
		{Index: 0, RawName: "id", RawValue: stringPointer("1"), Name: "id", Value: stringPointer("1"), HasEquals: true},
		{Index: 1, RawName: "id", RawValue: stringPointer("2"), Name: "id", Value: stringPointer("2"), HasEquals: true},
		{Index: 2, RawName: "debug", Name: "debug", HasEquals: false},
		{Index: 3, RawName: "empty", RawValue: stringPointer(""), Name: "empty", Value: stringPointer(""), HasEquals: true},
	}
	if !reflect.DeepEqual(result.QueryPairs, wantPairs) {
		t.Fatalf("query pairs = %#v, want %#v", result.QueryPairs, wantPairs)
	}
	get := "get"
	known, err := EndpointID(&get, result.CanonicalRouteURL)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := EndpointID(nil, result.CanonicalRouteURL)
	if err != nil {
		t.Fatal(err)
	}
	if known == unknown {
		t.Fatal("known and unknown method endpoint IDs are equal")
	}
	literalStar := "*"
	star, err := EndpointID(&literalStar, result.CanonicalRouteURL)
	if err != nil {
		t.Fatal(err)
	}
	if star == unknown {
		t.Fatal("literal * method collided with unknown method")
	}
	jsonID, err := ParameterID(known, "json", "id")
	if err != nil {
		t.Fatal(err)
	}
	jsonUpperID, err := ParameterID(known, "json", "ID")
	if err != nil {
		t.Fatal(err)
	}
	formID, err := ParameterID(known, "form", "id")
	if err != nil {
		t.Fatal(err)
	}
	if jsonID == jsonUpperID {
		t.Fatal("parameter identity ignored name case")
	}
	if jsonID == formID {
		t.Fatal("parameter identity ignored location")
	}
	if _, err := ParameterID(known, "json", string([]byte{0xff})); err == nil {
		t.Fatal("ParameterID accepted invalid UTF-8")
	}
}

func TestNormalizeArjunJSONMode(t *testing.T) {
	t.Parallel()

	label := "JSON"
	got, err := NormalizeSourceMethod(&label, "arjun")
	if err != nil {
		t.Fatal(err)
	}
	want := SourceMethod{SourceLabel: &label, HTTPMethod: stringPointer("POST"), MethodKnown: true, BodyKind: "json", ParameterLocation: "json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeSourceMethod = %#v, want %#v", got, want)
	}
}

func TestURLRejectsAmbiguousInputs(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`https:\\example.com\path`,
		"https://example.com/\x1f",
		"http://127.1/",
		"http://2130706433/",
		"http://[example.test]/",
		"http://[127.0.0.1]/",
		"https://exa_mple.com/",
	} {
		if _, err := CanonicalizeURL(raw); err == nil {
			t.Errorf("CanonicalizeURL(%q) succeeded", raw)
		}
	}
}

func TestURLStripsAllTrailingHostDots(t *testing.T) {
	t.Parallel()
	result, err := CanonicalizeURL("https://Example.COM../path")
	if err != nil {
		t.Fatal(err)
	}
	if result.Host != "example.com" {
		t.Fatalf("host = %q, want example.com", result.Host)
	}
}

func FuzzCanonicalURL(f *testing.F) {
	for _, seed := range []string{
		"https://example.com/",
		"HTTP://Example.COM:80/users?id=1&id=2#fragment",
		"https://[2001:db8::1]:443/a/../b",
		"https://bücher.example/資料?q=%E2%9C%93",
		`https:\\example.com\path`,
		"http://2130706433/",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64<<10 {
			return
		}
		first, firstErr := CanonicalizeURL(raw)
		second, secondErr := CanonicalizeURL(raw)
		if (firstErr == nil) != (secondErr == nil) || firstErr != nil && firstErr.Error() != secondErr.Error() || !reflect.DeepEqual(first, second) {
			t.Fatalf("non-deterministic result for %q", raw)
		}
		if firstErr != nil {
			return
		}
		if strings.ContainsAny(first.CanonicalRouteURL, "?#") || strings.Contains(first.CanonicalObservationURL, "#") {
			t.Fatalf("canonical URLs retained query/fragment boundary: %#v", first)
		}
		roundTrip, err := CanonicalizeURL(first.CanonicalObservationURL)
		if err != nil || roundTrip.CanonicalRouteURL != first.CanonicalRouteURL || roundTrip.CanonicalObservationURL != first.CanonicalObservationURL {
			t.Fatalf("canonical URL did not round-trip: %#v, %v", first, err)
		}
	})
}

func stringPointer(value string) *string { return &value }
