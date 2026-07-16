package model

import (
	"encoding/json"
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
