package model

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRecordSetMergeCoalescesEntitiesAndRejectsConflicts(t *testing.T) {
	base := Endpoint{SchemaVersion: SchemaVersion, RecordType: "endpoint", ID: "ep_sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RunID: "run_test", CanonicalRouteURL: "https://example.test/", ObservationIDs: []string{"obs_1"}, EvidenceIDs: []string{"ev_1"}}
	records := RecordSet{Endpoints: []Endpoint{base}}
	incoming := base
	incoming.ObservationIDs, incoming.EvidenceIDs = []string{"obs_2"}, []string{"ev_2"}
	if err := records.Merge(RecordSet{Endpoints: []Endpoint{incoming}}); err != nil {
		t.Fatal(err)
	}
	if len(records.Endpoints) != 1 || len(records.Endpoints[0].ObservationIDs) != 2 || len(records.Endpoints[0].EvidenceIDs) != 2 {
		t.Fatalf("merged endpoint = %+v", records.Endpoints)
	}
	incoming.Path = "/different"
	if err := records.Merge(RecordSet{Endpoints: []Endpoint{incoming}}); err == nil {
		t.Fatal("conflicting endpoint was accepted")
	}
}

func TestRecordSetMergeCoalescesRelationshipEvidenceAndRejectsConflicts(t *testing.T) {
	base := Relationship{
		SchemaVersion:    SchemaVersion,
		RecordType:       "relationship",
		ID:               "rel_sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RunID:            "run_test",
		RelationshipType: "has_parameter",
		From:             EntityRef{RecordType: "endpoint", ID: "ep_1"},
		To:               EntityRef{RecordType: "parameter", ID: "param_1"},
		EvidenceIDs:      []string{"ev_2"},
		Attributes:       map[string]any{},
	}
	records := RecordSet{Relationships: []Relationship{base}}
	incoming := base
	incoming.EvidenceIDs = []string{"ev_1"}
	if err := records.Merge(RecordSet{Relationships: []Relationship{incoming}}); err != nil {
		t.Fatal(err)
	}
	if got := records.Relationships[0].EvidenceIDs; !slices.Equal(got, []string{"ev_1", "ev_2"}) {
		t.Fatalf("merged relationship evidence = %v", got)
	}
	incoming.To.ID = "param_2"
	if err := records.Merge(RecordSet{Relationships: []Relationship{incoming}}); err == nil {
		t.Fatal("conflicting relationship was accepted")
	}
}

func TestLocatorMarshalPreservesRequiredZeroValues(t *testing.T) {
	for _, test := range []struct {
		locator  Locator
		contains string
	}{
		{Locator{Kind: "byte_range", ByteEndExclusive: 1}, `"byte_start":0`},
		{Locator{Kind: "json_pointer"}, `"pointer":""`},
	} {
		raw, err := json.Marshal(test.locator)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), test.contains) {
			t.Fatalf("locator JSON %s lacks %s", raw, test.contains)
		}
	}
}
