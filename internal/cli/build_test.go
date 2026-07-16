package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/candidate"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/integrity"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/workspace"
)

func TestBuildCompilesOfflineAndRejectsFinalizedDestination(t *testing.T) {
	workspacePath := buildFixture(t)
	args := []string{"build", "--run", "run_test", "--workspace", workspacePath}
	var stdout, stderr bytes.Buffer
	if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	output := filepath.Join(workspacePath, "handoff", "run_test")
	checksums, err := os.ReadFile(filepath.Join(output, "checksums.sha256"))
	if err != nil || integrity.VerifyChecksums(output, checksums) != nil {
		t.Fatalf("handoff checksums failed: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(output, "raw", "source.txt"))
	if err != nil || string(raw) != "sanitized evidence\n" {
		t.Fatalf("raw handoff = %q, %v", raw, err)
	}
	if got := stdout.String(); got != "handoff: "+output+"\n" {
		t.Fatalf("stdout = %q", got)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "finalized") {
		t.Fatalf("rebuilt finalized destination: code=%d, stderr=%q", code, stderr.String())
	}
}

func TestBuildAcceptsOnlyWorkspaceContainedOutput(t *testing.T) {
	workspacePath := buildFixture(t)
	inside := filepath.Join(workspacePath, "exports", "run_test")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"build", "--run", "run_test", "--workspace", workspacePath, "--out", inside}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("inside output: code=%d, stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(inside, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "handoff")
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"build", "--run", "run_test", "--workspace", workspacePath, "--out", outside}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("outside output: code=%d, stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside output was created: %v", err)
	}
}

func TestBuildRequiresAbsoluteWorkspace(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"build", "--run", "run_test", "--workspace", "relative"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "ABSOLUTE_PRIVATE_DIR") {
		t.Fatalf("relative workspace: code=%d stderr=%q", code, stderr.String())
	}
}

func TestBuildRejectsWorkflowDriftSecretsAndUnsafeSources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "incomplete metadata", mutate: func(t *testing.T, root string) {
			mutateBuildWorkflow(t, root, func(workflow *workflowState) { workflow.GeneratedAt = "" })
		}},
		{name: "plan digest drift", mutate: func(t *testing.T, root string) {
			mutateBuildWorkflow(t, root, func(workflow *workflowState) { workflow.PlanDigest = "sha256:" + strings.Repeat("f", 64) })
		}},
		{name: "unsafe raw source", mutate: func(t *testing.T, root string) {
			mutateBuildWorkflow(t, root, func(workflow *workflowState) { workflow.RawSources["raw/source.txt"] = "../outside" })
		}},
		{name: "missing raw source mapping", mutate: func(t *testing.T, root string) {
			mutateBuildWorkflow(t, root, func(workflow *workflowState) { workflow.RawSources = map[string]string{} })
		}},
		{name: "secret in raw source", mutate: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "runs", "run_test", "raw", "source.txt"), []byte("Authorization: Bearer private\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "private path in raw source", mutate: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "runs", "run_test", "raw", "source.txt"), []byte("tool output: "+root+"/runs/run_test\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspacePath := buildFixture(t)
			test.mutate(t, workspacePath)
			var stdout, stderr bytes.Buffer
			code := Run([]string{"build", "--run", "run_test", "--workspace", workspacePath}, strings.NewReader(""), &stdout, &stderr)
			if code != 1 || stderr.Len() == 0 {
				t.Fatalf("unsafe workflow: code=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
			}
			if _, err := os.Stat(filepath.Join(workspacePath, "handoff", "run_test")); !os.IsNotExist(err) {
				t.Fatalf("failed build created destination: %v", err)
			}
		})
	}
}

func TestValidateBuildWorkflowRequiresDecisionsForApprovedQueue(t *testing.T) {
	planDigest := "sha256:" + strings.Repeat("0", 64)
	queue := model.CandidateQueue{
		QueueVersion:  "reconctx-candidate-queue/v0",
		PolicyVersion: "candidate-policy/v0",
		PlanDigest:    planDigest,
		Candidates:    []model.Candidate{},
		Limits:        model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, RequestTimeoutSeconds: 1, ExecutionTimeoutSeconds: 2},
		MaxTargets:    0,
	}
	queueDigest, err := approval.QueueDigest(queue)
	if err != nil {
		t.Fatal(err)
	}
	run := model.Run{ID: "run_test", State: model.RunCompiling, PlanDigest: planDigest, QueueDigest: queueDigest}
	plan := planArtifact{Plan: model.Plan{RunID: "run_test"}, PlanDigest: planDigest}
	workflow := workflowState{
		PlanDigest:  planDigest,
		Records:     model.RecordSet{Runs: []model.RunRecord{{ID: "run_test", Status: "success"}}},
		Queue:       &queue,
		Candidates:  nil,
		GeneratedAt: "2026-07-16T10:00:01Z",
		Status:      "success",
	}
	if err := validateBuildWorkflow(run, plan, workflow); err == nil || !strings.Contains(err.Error(), "candidate metadata") {
		t.Fatalf("validateBuildWorkflow error = %v", err)
	}
}

func TestBuildRejectsCandidateDecisionArtifactDrift(t *testing.T) {
	workspacePath := buildFixture(t)
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := loadWorkspacePlan(root, "run_test")
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, "run_test", plan.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	arjun := findTool(plan.Plan, "arjun")
	queue := model.CandidateQueue{
		QueueVersion:  "reconctx-candidate-queue/v0",
		PolicyVersion: candidate.PolicyVersion,
		PlanDigest:    plan.PlanDigest,
		Candidates:    []model.Candidate{},
		Limits:        arjun.Limits,
		MaxTargets:    plan.Plan.Limits.ArjunMaxTargets,
	}
	queueDigest, err := approval.QueueDigest(queue)
	if err != nil {
		t.Fatal(err)
	}
	workflow.Queue = &queue
	workflow.Candidates = make([]json.RawMessage, 0)
	if err := writeWorkflow(root, "run_test", workflow); err != nil {
		t.Fatal(err)
	}
	run, err := readRunState(root, "run_test")
	if err != nil {
		t.Fatal(err)
	}
	run.QueueDigest = queueDigest
	if err := writeRunState(root, run); err != nil {
		t.Fatal(err)
	}
	if err := persistCandidates(root, "run_test", candidate.Result{Queue: queue, QueueDigest: queueDigest, Decisions: []candidate.Decision{}}); err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	decisionPath := filepath.Join(workspacePath, "runs", "run_test", "arjun-candidates.jsonl")
	if err := os.WriteFile(decisionPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"build", "--run", "run_test", "--workspace", workspacePath}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "candidate decision artifact differs from workflow") {
		t.Fatalf("build with drift: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(workspacePath, "handoff", "run_test")); !os.IsNotExist(err) {
		t.Fatalf("failed build created destination: %v", err)
	}
}

func buildFixture(t *testing.T) string {
	t.Helper()
	workspacePath := t.TempDir()
	if err := os.Chmod(workspacePath, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	const runID = "run_test"
	if err := root.CreateRunDir(runID); err != nil {
		t.Fatal(err)
	}
	plan := buildTestPlan(workspacePath, runID)
	digest, err := approval.PlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	artifact := planArtifact{Plan: plan, PlanDigest: digest}
	encoded, err := canonical.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFileExclusive(path.Join("runs", runID, "plan.json"), append(encoded, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := writeRunState(root, model.Run{ID: runID, State: model.RunCompiling, PlanDigest: digest}); err != nil {
		t.Fatal(err)
	}
	sourceContent := []byte("sanitized evidence\n")
	sourceDigest := sha256.Sum256(sourceContent)
	sourceSHA, sourceSize := hex.EncodeToString(sourceDigest[:]), int64(len(sourceContent))
	observationID, evidenceID := "obs_sha256_"+sourceSHA, "ev_sha256_"+sourceSHA
	source := path.Join("runs", runID, "raw", "source.txt")
	if err := root.WriteFileExclusive(source, sourceContent); err != nil {
		t.Fatal(err)
	}
	finished := "2026-07-16T10:00:01Z"
	duration, exitCode := int64(1), 0
	rule := "fixture"
	scope := model.ScopeDecision{Classification: "in_scope", RuleID: &rule, Reason: "fixture scope"}
	workflow := workflowState{
		PlanDigest: digest,
		Records: model.RecordSet{
			Runs: []model.RunRecord{{
				SchemaVersion: model.SchemaVersion, RecordType: "run", ID: runID,
				CreatedAt: plan.CreatedAt, FinishedAt: &finished, Status: "success",
				CanonicalizationPolicy: canonical.URLPolicyVersion,
				Scope:                  model.RunScope{Mode: "allowlist", Roots: []model.RunScopeRoot{{Kind: "origin", Value: "https://fixture.test"}}, ExternalPolicy: "reject", ApprovedBy: "operator", ApprovedAt: plan.CreatedAt},
				ToolExecutionIDs:       []string{"tx_test"}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{},
			}},
			ToolExecutions: []model.ToolExecution{{
				SchemaVersion: model.SchemaVersion, RecordType: "tool_execution", ID: "tx_test", RunID: runID,
				Tool: model.ToolIdentity{Name: "unknown", Version: "fixture", ResolvedPath: "/unavailable/tool"}, AdapterVersion: "unknown-adapter/v0",
				ActivityClass: "offline", ApprovalPhase: "offline_import", ArgvRedacted: []string{"fixture"}, StartedAt: &plan.CreatedAt, FinishedAt: &finished,
				DurationMS: &duration, ExitCode: &exitCode, Status: "success_zero", Coverage: "zero",
				Artifacts:      []model.ArtifactSummary{{Role: "stdout", Path: "raw/source.txt", Present: true, SHA256: &sourceSHA, SizeBytes: &sourceSize, MediaType: "text/plain"}},
				ProviderStatus: []model.ProviderStatus{}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{},
			}},
			Observations: []model.Observation{{
				SchemaVersion: model.SchemaVersion, RecordType: "observation", ID: observationID, RunID: runID, ToolExecutionID: "tx_test",
				ObservationType: "zero_result", SemanticState: "observed", Subject: model.EntityRef{RecordType: "tool_execution", ID: "tx_test"},
				Scope: scope, ObservedAt: &finished, EvidenceIDs: []string{evidenceID},
				Details: model.ZeroDetails{ResultKind: "provider_query", Message: "The fixture emitted no result rows."},
			}},
			Evidence: []model.Evidence{{
				SchemaVersion: model.SchemaVersion, RecordType: "evidence", ID: evidenceID, RunID: runID, ToolExecutionID: "tx_test",
				Artifact: model.Artifact{Role: "stdout", Path: "raw/source.txt", SHA256: sourceSHA, SizeBytes: sourceSize, MediaType: "text/plain", Sanitized: true},
				Locator:  model.Locator{Kind: "line_range", LineStart: 1, LineEnd: 1}, RedactionStatus: "not_needed", Scope: scope,
			}},
		},
		Candidates:  make([]json.RawMessage, 0),
		RawSources:  map[string]string{"raw/source.txt": source},
		GeneratedAt: finished,
		Status:      "success",
	}
	if err := writeWorkflow(root, runID, workflow); err != nil {
		t.Fatal(err)
	}
	return workspacePath
}

func buildTestPlan(workspacePath, runID string) model.Plan {
	digest := "sha256:" + strings.Repeat("0", 64)
	tools := make([]model.ToolPlan, 0, 3)
	for index, name := range []string{"gau", "katana", "arjun"} {
		resolved := "/unavailable/" + name
		directory := path.Join("runs", runID, "executions", "tx_"+name)
		tools = append(tools, model.ToolPlan{
			Name: name, ResolvedPath: resolved, Version: "fixture", ActivityClass: "fixture",
			Binary: model.ToolBinary{SHA256: digest, Mode: 0o700, Device: 1, Inode: uint64(index + 1)},
			Argv:   []string{resolved}, Limits: model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, RequestTimeoutSeconds: 1, ExecutionTimeoutSeconds: 2},
			OutputPaths: []string{path.Join(directory, "stdout.raw"), path.Join(directory, "stderr.raw"), path.Join(directory, "native-output.json")},
		})
	}
	return model.Plan{
		PlanVersion: "reconctx-plan/v0", RunID: runID, CreatedAt: "2026-07-16T10:00:00Z",
		Inputs:                 model.PlanInputs{Target: "fixture.test", Seeds: []string{"https://fixture.test/"}, ScopePath: path.Join("runs", runID, "scope.yaml"), ScopeSHA256: digest, Profile: "web-blackbox", WordlistPath: "/unavailable/params.txt", WordlistSHA256: digest},
		CanonicalizationPolicy: canonical.URLPolicyVersion, SchemaVersion: model.SchemaVersion,
		Tools: tools, Limits: model.PlanLimits{ArjunMaxTargets: 25, ArjunRequestBudget: 2},
		EnvironmentAllowlist: []string{"LANG"}, WorkspaceRoot: workspacePath,
	}
}

func mutateBuildWorkflow(t *testing.T, workspacePath string, mutate func(*workflowState)) {
	t.Helper()
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	plan, err := loadWorkspacePlan(root, "run_test")
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, "run_test", plan.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&workflow)
	if err := writeWorkflow(root, "run_test", workflow); err != nil {
		t.Fatal(err)
	}
}
