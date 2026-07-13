package approval

import (
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/model"
)

func TestApprovalQueueDigestChangesWithBehavior(t *testing.T) {
	base := testQueue()
	baseDigest, err := QueueDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*model.CandidateQueue){
		"remove": func(q *model.CandidateQueue) { q.Candidates = nil },
		"add":    func(q *model.CandidateQueue) { q.Candidates = append(q.Candidates, q.Candidates[0]) },
		"method": func(q *model.CandidateQueue) {
			q.Candidates[0].Method = "POST"
			q.Candidates[0].Location = "form"
		},
		"location": func(q *model.CandidateQueue) {
			q.Candidates[0].Method = "POST"
			q.Candidates[0].Location = "json"
		},
		"wordlist": func(q *model.CandidateQueue) { q.Candidates[0].WordlistSHA256 = "sha256:" + strings.Repeat("b", 64) },
		"argv":     func(q *model.CandidateQueue) { q.Candidates[0].Argv = append(q.Candidates[0].Argv, "--stable") },
		"limits":   func(q *model.CandidateQueue) { q.Limits.RatePerSecond++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			queue := testQueue()
			mutate(&queue)
			digest, err := QueueDigest(queue)
			if err != nil {
				t.Fatal(err)
			}
			if digest == baseDigest {
				t.Fatal("queue digest did not change")
			}
		})
	}
}

func TestApprovalRecordsAreVerifiedAndAppendedWithSupersession(t *testing.T) {
	digest, err := QueueDigest(testQueue())
	if err != nil {
		t.Fatal(err)
	}
	first := model.ApprovalRecord{
		Phase:          "arjun",
		ApprovedDigest: digest,
		OperatorLabel:  "operator",
		Decision:       "approve",
		CreatedAt:      "2026-07-13T13:00:00Z",
	}
	if err := VerifyDecision(first, "arjun", digest, "approve"); err != nil {
		t.Fatal(err)
	}
	stale := first
	stale.ApprovedDigest = "sha256:" + strings.Repeat("f", 64)
	if err := VerifyDecision(stale, "arjun", digest, "approve"); err == nil {
		t.Fatal("stale approval accepted")
	}
	records, err := AppendRecord(nil, first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.CreatedAt = "2026-07-13T13:01:00Z"
	records, err = AppendRecord(records, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0] != first || records[1].Supersedes != first.ApprovedDigest {
		t.Fatalf("approval audit = %#v", records)
	}
}

func TestApprovalQueueDigestRejectsInvalidBehavior(t *testing.T) {
	tests := map[string]func(*model.CandidateQueue){
		"version":  func(q *model.CandidateQueue) { q.QueueVersion = "other/v0" },
		"plan":     func(q *model.CandidateQueue) { q.PlanDigest = "sha256:bad" },
		"limit":    func(q *model.CandidateQueue) { q.Limits.TimeoutSeconds = 0 },
		"too many": func(q *model.CandidateQueue) { q.MaxTargets = 0 },
		"URL":      func(q *model.CandidateQueue) { q.Candidates[0].URL += "?query=1" },
		"method":   func(q *model.CandidateQueue) { q.Candidates[0].Method = "DELETE" },
		"wordlist": func(q *model.CandidateQueue) { q.Candidates[0].WordlistSHA256 = "sha256:bad" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			queue := testQueue()
			mutate(&queue)
			if _, err := QueueDigest(queue); err == nil {
				t.Fatal("QueueDigest accepted invalid behavior")
			}
		})
	}
}

func testQueue() model.CandidateQueue {
	return model.CandidateQueue{
		QueueVersion: "reconctx-candidate-queue/v0",
		PlanDigest:   "sha256:" + strings.Repeat("a", 64),
		Candidates: []model.Candidate{{
			URL:            "https://fixture.test/search",
			Method:         "GET",
			Location:       "query",
			WordlistPath:   "/wordlists/params.txt",
			WordlistSHA256: "sha256:" + strings.Repeat("c", 64),
			Argv:           []string{"/tools/arjun", "-u", "https://fixture.test/search", "-m", "GET", "-w", "/wordlists/params.txt"},
			RequestBudget:  100,
			Scope:          model.CandidateScope{Classification: "in_scope", RuleID: "fixture", Reason: "origin allowlist root matched"},
		}},
		Limits:     model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, TimeoutSeconds: 15},
		MaxTargets: 25,
	}
}
