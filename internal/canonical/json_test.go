package canonical

import (
	"math"
	"strings"
	"testing"
)

func TestCanonicalJSONMapOrder(t *testing.T) {
	t.Parallel()

	first, err := Canonicalize([]byte(`{"z":1,"nested":{"b":2,"a":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Canonicalize([]byte(` { "nested": {"a":1,"b":2}, "z":1 } `))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("canonical forms differ:\n%s\n%s", first, second)
	}
	if got, want := string(first), `{"nested":{"a":1,"b":2},"z":1}`; got != want {
		t.Fatalf("canonical JSON = %s, want %s", got, want)
	}
}

func TestCanonicalJSONPreservesEmptyArrays(t *testing.T) {
	t.Parallel()

	got, err := Canonicalize([]byte(`{"null":null,"empty":[]}`))
	if err != nil || string(got) != `{"empty":[],"null":null}` {
		t.Fatalf("Canonicalize empty array = %s, %v", got, err)
	}
}

func TestCanonicalJSONFailsClosed(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`{"a":1,"a":2}`,
		`{"outer":{"a":1,"a":2}}`,
		`{"a":1} trailing`,
	} {
		if _, err := Canonicalize([]byte(input)); err == nil {
			t.Errorf("Canonicalize(%q) succeeded, want error", input)
		}
	}
	if _, err := Marshal(map[string]float64{"bad": math.NaN()}); err == nil {
		t.Error("Marshal(NaN) succeeded, want error")
	}
	if _, err := Marshal(map[string]any{"bad": func() {}}); err == nil {
		t.Error("Marshal(function) succeeded, want error")
	}
	for _, input := range [][]byte{
		{'"', 0xff, '"'},
		{'{', '"', 'k', 'e', 'y', '"', ':', '"', 0xff, '"', '}'},
		[]byte(`"\ud800"`),
		[]byte(`"\ud801"`),
	} {
		if _, err := Canonicalize(input); err == nil {
			t.Errorf("Canonicalize(%q) accepted invalid UTF-8", input)
		}
	}
	for _, value := range []any{
		map[string]string{"bad": string([]byte{0xff})},
		map[string]string{string([]byte{0xff}): "bad"},
	} {
		if _, err := Marshal(value); err == nil {
			t.Errorf("Marshal(%q) accepted invalid UTF-8", value)
		}
	}
	paired, err := Canonicalize([]byte(`"\ud83d\ude00"`))
	if err != nil || string(paired) != `"😀"` {
		t.Fatalf("valid surrogate pair = %q, %v", paired, err)
	}
	deep := []byte(strings.Repeat("[", 101) + "0" + strings.Repeat("]", 101))
	if _, err := Canonicalize(deep); err == nil {
		t.Fatal("Canonicalize accepted excessive nesting")
	}
}
