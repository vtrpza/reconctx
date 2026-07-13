package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/preflight"
)

func TestTransitionRequiresExactCollectionApproval(t *testing.T) {
	plan := stateTestPlan(t)
	run := model.Run{ID: "run_test", State: model.RunPlanned}
	awaiting, err := AwaitCollectionApproval(run, plan)
	if err != nil {
		t.Fatal(err)
	}
	if awaiting.State != model.RunAwaitingCollectionApproval || CanSchedule(awaiting) || len(awaiting.Approvals) != 0 {
		t.Fatalf("awaiting collection = %#v", awaiting)
	}
	record := stateDecision("collection", awaiting.PlanDigest, "approve")
	collecting, err := StartCollection(awaiting, plan, record)
	if err != nil {
		t.Fatal(err)
	}
	if collecting.State != model.RunCollecting || !CanSchedule(collecting) || len(collecting.Approvals) != 1 {
		t.Fatalf("collecting = %#v", collecting)
	}
	if err := os.WriteFile(plan.Tools[0].ResolvedPath, []byte("#!/bin/sh\nexit 0\n# changed\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := StartCollection(awaiting, plan, record); err == nil {
		t.Fatal("changed collection binary reused approval")
	}

	drifted := plan
	drifted.Tools = append([]model.ToolPlan(nil), plan.Tools...)
	drifted.Tools[0].Limits.RatePerSecond++
	if _, err := StartCollection(awaiting, drifted, record); err == nil {
		t.Fatal("plan drift reused collection approval")
	}
	drifted = plan
	drifted.RunID = "run_other"
	if _, err := StartCollection(awaiting, drifted, record); err == nil {
		t.Fatal("plan from another run reused collection approval")
	}
}

func TestTransitionRequiresFreshArjunApprovalAndSupportsSkip(t *testing.T) {
	plan := stateTestPlan(t)
	run, err := AwaitCollectionApproval(model.Run{ID: "run_test", State: model.RunPlanned}, plan)
	if err != nil {
		t.Fatal(err)
	}
	run, err = StartCollection(run, plan, stateDecision("collection", run.PlanDigest, "approve"))
	if err != nil {
		t.Fatal(err)
	}
	run, err = FinishCollection(run)
	if err != nil || run.State != model.RunNormalizingInitial || CanSchedule(run) {
		t.Fatalf("initial normalization = %#v, %v", run, err)
	}
	queue := stateTestQueue(t, run.PlanDigest, plan.Tools[1].ResolvedPath)
	scopeDocument := stateScopeDocument()
	awaiting, err := AwaitArjunApproval(run, plan, scopeDocument, queue)
	if err != nil {
		t.Fatal(err)
	}
	if awaiting.State != model.RunAwaitingArjunApproval || CanSchedule(awaiting) {
		t.Fatalf("awaiting Arjun = %#v", awaiting)
	}
	old := stateDecision("arjun", awaiting.QueueDigest, "approve")
	if err := os.WriteFile(plan.Tools[1].ResolvedPath, []byte("#!/bin/sh\nexit 0\n# changed\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := StartArjun(awaiting, plan, scopeDocument, queue, old); err == nil {
		t.Fatal("changed Arjun binary reused approval")
	}
	if err := os.WriteFile(plan.Tools[1].ResolvedPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(queue.Candidates[0].WordlistPath, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := StartArjun(awaiting, plan, scopeDocument, queue, old); err == nil {
		t.Fatal("changed wordlist reused Arjun approval")
	}
	if err := os.WriteFile(queue.Candidates[0].WordlistPath, []byte("id\nquery\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	edited := queue
	edited.Candidates = append([]model.Candidate(nil), queue.Candidates...)
	edited.Candidates[0].Argv = append([]string(nil), queue.Candidates[0].Argv...)
	edited.Limits.RatePerSecond = 1
	edited.Candidates[0].Argv[8] = "1"
	queue.Limits.RatePerSecond = 2
	awaiting, err = AwaitArjunApproval(awaiting, plan, scopeDocument, edited)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StartArjun(awaiting, plan, scopeDocument, edited, old); err == nil {
		t.Fatal("queue edit reused Arjun approval")
	}
	approved := stateDecision("arjun", awaiting.QueueDigest, "approve")
	discovering, err := StartArjun(awaiting, plan, scopeDocument, edited, approved)
	if err != nil {
		t.Fatal(err)
	}
	if discovering.State != model.RunDiscoveringParameters || !CanSchedule(discovering) {
		t.Fatalf("discovering = %#v", discovering)
	}
	normalizing, err := FinishArjun(discovering)
	if err != nil || normalizing.State != model.RunNormalizingFinal || CanSchedule(normalizing) {
		t.Fatalf("final normalization = %#v, %v", normalizing, err)
	}
	compiling, err := FinishFinalNormalization(normalizing)
	if err != nil || compiling.State != model.RunCompiling {
		t.Fatalf("finished normalization = %#v, %v", compiling, err)
	}
	if completed, err := CompleteRun(compiling); err != nil || completed.State != model.RunSuccess {
		t.Fatalf("completed Arjun = %#v, %v", completed, err)
	}

	awaiting, err = AwaitArjunApproval(run, plan, scopeDocument, edited)
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := SkipArjun(awaiting, plan, scopeDocument, edited, stateDecision("arjun", awaiting.QueueDigest, "skip"))
	if err != nil {
		t.Fatal(err)
	}
	if skipped.State != model.RunArjunSkipped || CanSchedule(skipped) || len(skipped.CoverageGaps) != 1 {
		t.Fatalf("skipped = %#v", skipped)
	}
	skipped, err = CompileSkippedArjun(skipped)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := CompleteRun(skipped)
	if err != nil || completed.State != model.RunSuccess {
		t.Fatalf("completed skip = %#v, %v", completed, err)
	}
}

func TestTransitionCancelStopsScheduling(t *testing.T) {
	plan := stateTestPlan(t)
	run, err := AwaitCollectionApproval(model.Run{ID: "run_test", State: model.RunPlanned}, plan)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := CancelRun(run, stateDecision("collection", run.PlanDigest, "cancel"))
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != model.RunCancelled || CanSchedule(cancelled) || len(cancelled.Approvals) != 1 {
		t.Fatalf("cancelled = %#v", cancelled)
	}
}

func TestTransitionRejectsOutOfScopePlanCeilingAndCommandDrift(t *testing.T) {
	plan := stateTestPlan(t)
	run, err := AwaitCollectionApproval(model.Run{ID: "run_test", State: model.RunPlanned}, plan)
	if err != nil {
		t.Fatal(err)
	}
	run, err = StartCollection(run, plan, stateDecision("collection", run.PlanDigest, "approve"))
	if err != nil {
		t.Fatal(err)
	}
	run, err = FinishCollection(run)
	if err != nil {
		t.Fatal(err)
	}
	queue := stateTestQueue(t, run.PlanDigest, plan.Tools[1].ResolvedPath)
	queue.Candidates[0].URL = "https://outside.test/"
	queue.Candidates[0].Argv[2] = queue.Candidates[0].URL
	if _, err := AwaitArjunApproval(run, plan, stateScopeDocument(), queue); err == nil {
		t.Fatal("out-of-scope candidate reached approval")
	}
	queue = stateTestQueue(t, run.PlanDigest, plan.Tools[1].ResolvedPath)
	queue.MaxTargets = plan.Limits.ArjunMaxTargets + 1
	if _, err := AwaitArjunApproval(run, plan, stateScopeDocument(), queue); err == nil {
		t.Fatal("queue exceeded plan candidate ceiling")
	}
	queue = stateTestQueue(t, run.PlanDigest, plan.Tools[1].ResolvedPath)
	queue.Limits.RatePerSecond = plan.Tools[1].Limits.RatePerSecond + 1
	if _, err := AwaitArjunApproval(run, plan, stateScopeDocument(), queue); err == nil {
		t.Fatal("queue exceeded approved Arjun intensity")
	}
	queue = stateTestQueue(t, run.PlanDigest, plan.Tools[1].ResolvedPath)
	queue.Candidates[0].Argv[2] = "https://outside.test/"
	if _, err := AwaitArjunApproval(run, plan, stateScopeDocument(), queue); err == nil {
		t.Fatal("command target differed from candidate URL")
	}
	queue = stateTestQueue(t, run.PlanDigest, plan.Tools[1].ResolvedPath)
	queue.Candidates[0].Argv = append(queue.Candidates[0].Argv, "-i", "/tmp/targets.txt")
	if _, err := AwaitArjunApproval(run, plan, stateScopeDocument(), queue); err == nil {
		t.Fatal("command imported unmodeled targets")
	}
}

func TestTransitionPermitsApprovedZeroTargetSkip(t *testing.T) {
	plan := stateTestPlan(t)
	plan.Limits.ArjunMaxTargets = 0
	run, err := AwaitCollectionApproval(model.Run{ID: "run_test", State: model.RunPlanned}, plan)
	if err != nil {
		t.Fatal(err)
	}
	run, err = StartCollection(run, plan, stateDecision("collection", run.PlanDigest, "approve"))
	if err != nil {
		t.Fatal(err)
	}
	run, err = FinishCollection(run)
	if err != nil {
		t.Fatal(err)
	}
	queue := stateTestQueue(t, run.PlanDigest, plan.Tools[1].ResolvedPath)
	queue.Candidates = nil
	queue.MaxTargets = 0
	run, err = AwaitArjunApproval(run, plan, stateScopeDocument(), queue)
	if err != nil {
		t.Fatal(err)
	}
	run, err = SkipArjun(run, plan, stateScopeDocument(), queue, stateDecision("arjun", run.QueueDigest, "skip"))
	if err != nil || run.State != model.RunArjunSkipped {
		t.Fatalf("zero-target skip = %#v, %v", run, err)
	}
}

func TestArjunCommandBindsLimitsAndJSONMode(t *testing.T) {
	limits := model.ToolLimits{RatePerSecond: 1, Concurrency: 2, Parallelism: 1, TimeoutSeconds: 9}
	candidate := model.Candidate{
		URL: "https://fixture.test/api", Method: "POST", Location: "json", WordlistPath: "/wordlists/params.txt",
		Argv: []string{"/tools/arjun", "-u", "https://fixture.test/api", "-m", "JSON", "-w", "/wordlists/params.txt", "--rate-limit", "1", "-t", "2", "-T", "9", "--headers", "Content-Type: application/json"},
	}
	if !validArjunCommand(candidate, "/tools/arjun", limits) {
		t.Fatal("valid JSON command rejected")
	}
	candidate.Argv[4] = "POST"
	if validArjunCommand(candidate, "/tools/arjun", limits) {
		t.Fatal("POST form mode accepted for JSON candidate")
	}
}

func TestWordlistFIFORejectedWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wordlist.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- verifyWordlist(model.Candidate{WordlistPath: path})
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("FIFO accepted as wordlist")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("wordlist validation blocked on FIFO")
	}
}

func stateDecision(phase, digest, decision string) model.ApprovalRecord {
	return model.ApprovalRecord{
		Phase:          phase,
		ApprovedDigest: digest,
		OperatorLabel:  "operator",
		Decision:       decision,
		CreatedAt:      "2026-07-13T13:00:00Z",
	}
}

func stateTestPlan(t *testing.T) model.Plan {
	t.Helper()
	toolDirectory := t.TempDir()
	if err := os.Chmod(toolDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	identities := make(map[string]preflight.ToolIdentity, 2)
	for _, name := range []string{"gau", "arjun"} {
		path := filepath.Join(toolDirectory, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		identity, err := preflight.ResolveTool(path)
		if err != nil {
			t.Fatal(err)
		}
		identities[name] = identity
	}
	binary := func(identity preflight.ToolIdentity) model.ToolBinary {
		return model.ToolBinary{SHA256: identity.SHA256, Mode: uint32(identity.Mode), UID: identity.UID, GID: identity.GID, Device: identity.Device, Inode: identity.Inode}
	}
	scopeHash := sha256.Sum256(stateScopeDocument())
	return model.Plan{
		PlanVersion:            "reconctx-plan/v0",
		RunID:                  "run_test",
		CreatedAt:              "2026-07-13T12:50:05Z",
		CanonicalizationPolicy: "url-canonicalization/v0",
		SchemaVersion:          "reconctx/v0",
		Inputs: model.PlanInputs{
			Target: "fixture.test", Seeds: []string{"https://fixture.test/"}, ScopePath: "scope.yaml",
			ScopeSHA256: "sha256:" + hex.EncodeToString(scopeHash[:]), Profile: "web-blackbox",
		},
		Tools: []model.ToolPlan{{
			Name: "gau", ResolvedPath: identities["gau"].ResolvedPath, Version: "2.2.4", ActivityClass: "passive_external",
			Binary: binary(identities["gau"]),
			Argv:   []string{identities["gau"].ResolvedPath, "fixture.test"}, Limits: model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, TimeoutSeconds: 45},
			OutputPaths: []string{"runs/run_test/gau/stdout.raw"},
		}, {
			Name: "arjun", ResolvedPath: identities["arjun"].ResolvedPath, Version: "2.2.7", ActivityClass: "active_approved",
			Binary: binary(identities["arjun"]),
			Argv:   []string{identities["arjun"].ResolvedPath, "--version"}, Limits: model.ToolLimits{RatePerSecond: 2, Concurrency: 1, Parallelism: 1, TimeoutSeconds: 15},
			OutputPaths: []string{"runs/run_test/arjun/stdout.raw"},
		}},
		Limits: model.PlanLimits{ArjunMaxTargets: 25}, EnvironmentAllowlist: []string{"LANG"}, WorkspaceRoot: "/work",
	}
}

func stateTestQueue(t *testing.T, planDigest, arjunPath string) model.CandidateQueue {
	t.Helper()
	wordlist := []byte("id\nquery\n")
	wordlistPath := filepath.Join(t.TempDir(), "params.txt")
	if err := os.WriteFile(wordlistPath, wordlist, 0o600); err != nil {
		t.Fatal(err)
	}
	wordlistHash := sha256.Sum256(wordlist)
	return model.CandidateQueue{
		QueueVersion: "reconctx-candidate-queue/v0", PlanDigest: planDigest,
		Candidates: []model.Candidate{{
			URL: "https://fixture.test/search", Method: "GET", Location: "query", WordlistPath: wordlistPath,
			WordlistSHA256: "sha256:" + hex.EncodeToString(wordlistHash[:]),
			Argv:           []string{arjunPath, "-u", "https://fixture.test/search", "-m", "GET", "-w", wordlistPath, "--rate-limit", "2", "-t", "1", "-T", "15"}, RequestBudget: 100,
			Scope: model.CandidateScope{Classification: "in_scope", RuleID: "fixture", Reason: "origin allowlist root matched"},
		}},
		Limits: model.ToolLimits{RatePerSecond: 2, Concurrency: 1, Parallelism: 1, TimeoutSeconds: 15}, MaxTargets: 25,
	}
}

func stateScopeDocument() []byte {
	return []byte(`{"mode":"allowlist","roots":[{"id":"fixture","kind":"origin","value":"https://fixture.test"}],"external_policy":"reject"}`)
}
