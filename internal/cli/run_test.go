package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/adapter"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/integrity"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/runner"
	"github.com/vtrpza/reconctx/internal/workspace"
)

func TestRunRequiresTTYBeforeAnyActiveExecution(t *testing.T) {
	workspacePath, planPath := plannedWorkflowFixture(t)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", planPath}, strings.NewReader("approve anything\noperator\n"), &stdout, &stderr); code != 3 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	entries, err := os.ReadDir(filepath.Join(workspacePath, "runs"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected run directories: %v, %v", entries, err)
	}
	artifact, err := loadPlanArtifact(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspacePath, "runs", artifact.Plan.RunID, "executions")); !os.IsNotExist(err) {
		t.Fatalf("non-TTY run created active execution state: %v", err)
	}
}

func TestTerminalPrompterRejectsUnicodeFormatControls(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	prompt := newTerminalPrompter(strings.NewReader("approve "+digest+"\noperator\u202elabel\n"), io.Discard)
	if _, err := prompt.Prompt("collection", digest, false); err == nil || !strings.Contains(err.Error(), "printable") {
		t.Fatalf("format-control label error = %v", err)
	}
}

func TestRequestForToolUsesApprovedEnvironmentSnapshot(t *testing.T) {
	tool := model.ToolPlan{
		Name: "gau", ResolvedPath: "/tools/gau", Argv: []string{"/tools/gau"},
		Limits:      model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, TimeoutSeconds: 10},
		OutputPaths: []string{"runs/run_test/executions/tx_gau/stdout.raw", "runs/run_test/executions/tx_gau/stderr.raw", "runs/run_test/executions/tx_gau/native-output.txt"},
	}
	plan := model.Plan{
		WorkspaceRoot: "/private/work", EnvironmentAllowlist: []string{"LANG", "PATH"},
		Environment: []string{"LANG=C.UTF-8", "PATH=/approved/bin:/usr/bin"},
	}
	request, err := requestForTool(plan, tool, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(request.Environment, plan.Environment) || slices.Equal(request.Environment, os.Environ()) {
		t.Fatalf("runner environment = %#v, want approved snapshot %#v", request.Environment, plan.Environment)
	}
	plan.Environment[0] = "LANG=drifted"
	if request.Environment[0] != "LANG=C.UTF-8" {
		t.Fatal("runner request aliases mutable plan environment")
	}
}

func TestWorkflowBindsBothApprovalsAndCompilesFakeEvidence(t *testing.T) {
	workspacePath, planPath := plannedWorkflowFixture(t)
	artifact, err := loadPlanArtifact(planPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	executor := &fixtureExecutor{}
	prompt := &checkpointPrompter{executor: executor, t: t}
	var output bytes.Buffer
	if err := executeNewRun(context.Background(), root, artifact, filepath.ToSlash(filepath.Join("handoff", artifact.Plan.RunID)), prompt, executor, &output); err != nil {
		t.Fatal(err)
	}
	if executor.calls != 3 || prompt.phases != 2 {
		t.Fatalf("calls = %d, approvals = %d", executor.calls, prompt.phases)
	}
	if !strings.Contains(output.String(), "wordlist_sha256: "+artifact.Plan.Inputs.WordlistSHA256) {
		t.Fatalf("Approval B omitted the wordlist hash:\n%s", output.String())
	}
	workflow, err := readWorkflow(root, artifact.Plan.RunID, artifact.PlanDigest)
	if err != nil || workflow.Queue == nil || len(workflow.Queue.Candidates) == 0 {
		t.Fatalf("load approved queue: %#v, %v", workflow.Queue, err)
	}
	queueJSON, err := canonical.Marshal(*workflow.Queue)
	if err != nil {
		t.Fatal(err)
	}
	queued := workflow.Queue.Candidates[0]
	for _, field := range []string{
		"candidate_queue: version=\"reconctx-candidate-queue/v0\" policy=\"arjun-candidate-policy/v0\" plan_digest=\"" + artifact.PlanDigest + "\"",
		fmt.Sprintf("queue_limits: rate=%d concurrency=%d parallelism=%d timeout=%ds", workflow.Queue.Limits.RatePerSecond, workflow.Queue.Limits.Concurrency, workflow.Queue.Limits.Parallelism, workflow.Queue.Limits.TimeoutSeconds),
		"canonical_queue_json_ascii: " + strconv.QuoteToASCII(string(queueJSON)),
		"wordlist: path=" + strconv.QuoteToASCII(queued.WordlistPath) + " sha256=" + queued.WordlistSHA256,
		"native_output_path: " + strconv.QuoteToASCII(queued.NativeOutputPath),
		"scope: classification=" + strconv.QuoteToASCII(queued.Scope.Classification) + " rule=" + strconv.QuoteToASCII(queued.Scope.RuleID),
		"argv_exact: " + displaySafeArgv(queued.Argv),
	} {
		if !strings.Contains(output.String(), field) {
			t.Errorf("Approval B omitted exact queue field %q:\n%s", field, output.String())
		}
	}
	if strings.Contains(output.String(), "<WORDLIST>") || strings.Contains(output.String(), "<NATIVE_OUTPUT>") {
		t.Fatalf("Approval B displayed redacted behavior instead of the exact queue:\n%s", output.String())
	}
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil || run.State != model.RunSuccess || len(run.Approvals) != 2 || run.Approvals[0].ApprovedDigest != artifact.PlanDigest || run.Approvals[1].ApprovedDigest == artifact.PlanDigest {
		t.Fatalf("run state = %#v, %v", run, err)
	}
	handoff := filepath.Join(workspacePath, "handoff", artifact.Plan.RunID)
	checksums, err := os.ReadFile(filepath.Join(handoff, "checksums.sha256"))
	if err != nil || integrity.VerifyChecksums(handoff, checksums) != nil {
		t.Fatalf("handoff checksums failed: %v", err)
	}
	records, err := os.ReadFile(filepath.Join(handoff, "normalized", "records.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(records, []byte(`"record_type":"parameter"`)) || bytes.Contains(records, []byte(workspacePath)) {
		t.Fatalf("handoff records are incomplete or leak private paths:\n%s", records)
	}
	for _, invalid := range []string{`"provider_status":null`, `"warnings":null`, `"gaps":null`, `"evidence_ids":null`} {
		if bytes.Contains(records, []byte(invalid)) {
			t.Fatalf("handoff records contain schema-invalid %s:\n%s", invalid, records)
		}
	}
}

func TestExecuteNewRunUsesImmutableWorkspacePlanMetadata(t *testing.T) {
	workspacePath, planPath := plannedWorkflowFixture(t)
	artifact, err := loadPlanArtifact(planPath)
	if err != nil {
		t.Fatal(err)
	}
	wantCreatedAt := artifact.Plan.CreatedAt
	artifact.Plan.CreatedAt = "2000-01-01T00:00:00Z"

	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	executor := &fixtureExecutor{}
	prompt := &checkpointPrompter{executor: executor, t: t}
	if err := executeNewRun(context.Background(), root, artifact, filepath.ToSlash(filepath.Join("handoff", artifact.Plan.RunID)), prompt, executor, io.Discard); err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, artifact.Plan.RunID, artifact.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	if got := workflow.Records.Runs[0].CreatedAt; got != wantCreatedAt {
		t.Fatalf("run created_at = %q, want immutable workspace value %q", got, wantCreatedAt)
	}
}

func TestWorkflowExplicitArjunSkipRecordsPartialCoverage(t *testing.T) {
	workspacePath, planPath := plannedWorkflowFixture(t)
	artifact, err := loadPlanArtifact(planPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	executor := &fixtureExecutor{}
	prompt := &checkpointPrompter{executor: executor, t: t, secondDecision: "skip"}
	if err := executeNewRun(context.Background(), root, artifact, filepath.ToSlash(filepath.Join("handoff", artifact.Plan.RunID)), prompt, executor, io.Discard); err != nil {
		t.Fatal(err)
	}
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil || run.State != model.RunPartial || !slices.Contains(run.CoverageGaps, "arjun_skipped_by_operator") || executor.calls != 2 {
		t.Fatalf("skip state = %#v, calls = %d, err = %v", run, executor.calls, err)
	}
	manifest, err := os.ReadFile(filepath.Join(workspacePath, "handoff", artifact.Plan.RunID, "manifest.json"))
	if err != nil || !bytes.Contains(manifest, []byte(`"status":"partial"`)) {
		t.Fatalf("skip manifest = %s, %v", manifest, err)
	}
}

func TestWorkflowInterruptionPreservesPartialHandoff(t *testing.T) {
	workspacePath, planPath := plannedWorkflowFixture(t)
	artifact, err := loadPlanArtifact(planPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx, cancel := context.WithCancel(context.Background())
	base := &fixtureExecutor{}
	executor := interruptingExecutor{fixture: base, cancel: cancel}
	prompt := &checkpointPrompter{executor: base, t: t}
	err = executeNewRun(ctx, root, artifact, filepath.ToSlash(filepath.Join("handoff", artifact.Plan.RunID)), prompt, executor, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted workflow error = %v", err)
	}
	run, stateErr := readRunState(root, artifact.Plan.RunID)
	if stateErr != nil || run.State != model.RunInterrupted || base.calls != 1 || prompt.phases != 1 {
		t.Fatalf("interrupted state = %#v, calls = %d, approvals = %d, err = %v", run, base.calls, prompt.phases, stateErr)
	}
	manifest, readErr := os.ReadFile(filepath.Join(workspacePath, "handoff", artifact.Plan.RunID, "manifest.json"))
	if readErr != nil || !bytes.Contains(manifest, []byte(`"status":"partial"`)) {
		t.Fatalf("interrupted manifest = %s, %v", manifest, readErr)
	}
}

func TestApplyRunnerSemanticsNeverUpgradesIncompleteEnvelope(t *testing.T) {
	for _, test := range []struct {
		name, adapterStatus, adapterCoverage, status, reason, wantStatus, wantCoverage, wantGap string
	}{
		{name: "partial", adapterStatus: "success", adapterCoverage: "complete", status: runner.StatusPartial, reason: "output_limit", wantStatus: "partial", wantCoverage: "partial", wantGap: "runner.output_limit"},
		{name: "partial zero", adapterStatus: "success_zero", adapterCoverage: "zero", status: runner.StatusPartial, reason: "output_limit", wantStatus: "partial", wantCoverage: "partial", wantGap: "runner.output_limit"},
		{name: "failed", adapterStatus: "success", adapterCoverage: "complete", status: runner.StatusFailed, reason: "native_artifact_invalid", wantStatus: "failed", wantCoverage: "unknown", wantGap: "runner.native_artifact_invalid"},
		{name: "descendant leak reason", adapterStatus: "success", adapterCoverage: "complete", status: runner.StatusSuccess, reason: "descendant_leak", wantStatus: "partial", wantCoverage: "partial", wantGap: "runner.descendant_leak"},
		{name: "preserve adapter failure", adapterStatus: "failed", adapterCoverage: "unknown", status: runner.StatusPartial, reason: "exit_nonzero", wantStatus: "failed", wantCoverage: "unknown", wantGap: "runner.exit_nonzero"},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed := adapter.Result{Status: test.adapterStatus, Coverage: test.adapterCoverage}
			applyRunnerSemantics(&parsed, runner.ArtifactEnvelope{Status: test.status, Reason: test.reason})
			if parsed.Status != test.wantStatus || parsed.Coverage != test.wantCoverage || len(parsed.Gaps) != 1 || parsed.Gaps[0].Code != test.wantGap {
				t.Fatalf("result = %#v", parsed)
			}
		})
	}
}

func TestWorkflowPropagatesRunnerDescendantLeak(t *testing.T) {
	executor := &fixtureExecutor{responses: map[string]fixtureResponse{
		"gau": {nativeName: "native-output.txt", native: []byte("http://127.0.0.1:18080/api?existing=1\n"), status: runner.StatusPartial, reason: "descendant_leak"},
	}}
	workspacePath, artifact, run, workflow := completedFixtureWorkflow(t, executor)
	assertExecution(t, workflow, "gau", "partial", "runner.descendant_leak")
	assertPartialWorkflow(t, workspacePath, artifact.Plan.RunID, run, workflow)
}

func TestWorkflowPartialSuccessMatrix(t *testing.T) {
	t.Run("GAU fails and Katana succeeds", func(t *testing.T) {
		executor := &fixtureExecutor{responses: map[string]fixtureResponse{
			"gau": {nativeName: "native-output.txt", stderr: []byte("provider request failed\n"), status: runner.StatusFailed, reason: "exit_nonzero", exitCode: 1},
		}}
		workspacePath, artifact, run, workflow := completedFixtureWorkflow(t, executor)
		assertExecution(t, workflow, "gau", "failed", "runner.exit_nonzero")
		assertExecution(t, workflow, "katana", "success", "")
		assertPartialWorkflow(t, workspacePath, artifact.Plan.RunID, run, workflow)
	})

	t.Run("Katana fails and GAU succeeds", func(t *testing.T) {
		executor := &fixtureExecutor{responses: map[string]fixtureResponse{
			"katana": {nativeName: "native-output.jsonl", stderr: []byte("crawl failed\n"), status: runner.StatusFailed, reason: "exit_nonzero", exitCode: 1},
		}}
		workspacePath, artifact, run, workflow := completedFixtureWorkflow(t, executor)
		assertExecution(t, workflow, "gau", "success", "")
		assertExecution(t, workflow, "katana", "failed", "runner.exit_nonzero")
		assertPartialWorkflow(t, workspacePath, artifact.Plan.RunID, run, workflow)
	})

	t.Run("Arjun explicit zero", func(t *testing.T) {
		executor := &fixtureExecutor{responses: map[string]fixtureResponse{
			"arjun": {stdout: repositoryFixture(t, "fixtures/cases/arjun/2.2.7/ARJUN-ZERO/stdout.sanitized.log"), status: runner.StatusSuccess},
		}}
		workspacePath, artifact, run, workflow := completedFixtureWorkflow(t, executor)
		assertExecution(t, workflow, "arjun", "success_zero", "")
		absentPath := "raw/tx_arjun_01/native-output.json"
		for _, execution := range workflow.Records.ToolExecutions {
			if execution.Tool.Name != "arjun" {
				continue
			}
			if len(execution.Artifacts) != 3 {
				t.Fatalf("Arjun artifacts = %#v", execution.Artifacts)
			}
			absent := execution.Artifacts[2]
			if absent.Role != "native_output" || absent.Path != absentPath || absent.Present || absent.SHA256 != nil || absent.SizeBytes != nil || absent.MediaType != "application/json" {
				t.Fatalf("absent native output summary = %#v", absent)
			}
		}
		if _, exists := workflow.RawSources[absentPath]; exists {
			t.Fatalf("absent native output has a raw source: %s", absentPath)
		}
		if run.State != model.RunSuccess || workflow.Status != "success" {
			t.Fatalf("run=%s workflow=%s", run.State, workflow.Status)
		}
		manifest, err := os.ReadFile(filepath.Join(workspacePath, "handoff", artifact.Plan.RunID, "manifest.json"))
		if err != nil || !bytes.Contains(manifest, []byte(`"status":"success"`)) {
			t.Fatalf("manifest=%s err=%v", manifest, err)
		}
		if _, err := os.Stat(filepath.Join(workspacePath, "handoff", artifact.Plan.RunID, filepath.FromSlash(absentPath))); !os.IsNotExist(err) {
			t.Fatalf("absent native output was embedded: %v", err)
		}
	})

	t.Run("Arjun target error", func(t *testing.T) {
		executor := &fixtureExecutor{responses: map[string]fixtureResponse{
			"arjun": {
				stdout: repositoryFixture(t, "fixtures/cases/arjun/2.2.7/ARJUN-REQUEST-TIMEOUT-LOOPBACK/stdout.sanitized.log"),
				stderr: repositoryFixture(t, "fixtures/cases/arjun/2.2.7/ARJUN-REQUEST-TIMEOUT-LOOPBACK/stderr.sanitized.log"),
				status: runner.StatusPartial, reason: "exit_nonzero", exitCode: 1,
			},
		}}
		workspacePath, artifact, run, workflow := completedFixtureWorkflow(t, executor)
		assertExecution(t, workflow, "arjun", "failed", "arjun.tool_error")
		assertExecution(t, workflow, "arjun", "failed", "runner.exit_nonzero")
		assertPartialWorkflow(t, workspacePath, artifact.Plan.RunID, run, workflow)
	})
}

func TestFinishWithoutValidObservationsIsPartial(t *testing.T) {
	workspacePath := buildFixture(t)
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	plan, err := loadWorkspacePlan(root, "run_test")
	if err != nil {
		t.Fatal(err)
	}
	run, err := readRunState(root, "run_test")
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, "run_test", plan.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	workflow.Records.Observations = nil
	workflow.Records.Evidence = nil
	if err := finishAndCompile(root, plan.Plan, &run, &workflow, "handoff/run_test"); err != nil {
		t.Fatal(err)
	}
	if run.State != model.RunPartial || workflow.Status != "partial" {
		t.Fatalf("run=%s workflow=%s, want partial", run.State, workflow.Status)
	}
	found := false
	for _, gap := range workflow.Records.Runs[0].Gaps {
		found = found || gap.Code == "run.no_valid_observations"
	}
	if !found {
		t.Fatalf("run gaps = %#v", workflow.Records.Runs[0].Gaps)
	}
	manifest, err := os.ReadFile(filepath.Join(workspacePath, "handoff", "run_test", "manifest.json"))
	if err != nil || !bytes.Contains(manifest, []byte(`"status":"partial"`)) {
		t.Fatalf("manifest=%s err=%v", manifest, err)
	}
}

func TestAddRunGapIsIdempotent(t *testing.T) {
	records := model.RecordSet{Runs: []model.RunRecord{{Gaps: []model.Diagnostic{}}}}
	addRunGap(&records, "run.no_valid_observations", "No valid observations.")
	addRunGap(&records, "run.no_valid_observations", "No valid observations.")
	if len(records.Runs[0].Gaps) != 1 {
		t.Fatalf("gaps = %#v", records.Runs[0].Gaps)
	}
}

type checkpointPrompter struct {
	t              *testing.T
	executor       *fixtureExecutor
	phases         int
	secondDecision string
}

func (prompt *checkpointPrompter) Prompt(phase, digest string, allowSkip bool) (model.ApprovalRecord, error) {
	prompt.t.Helper()
	wantCalls := 0
	if allowSkip {
		wantCalls = 2
	}
	if prompt.executor.calls != wantCalls {
		prompt.t.Fatalf("%s prompt observed %d launches, want %d", phase, prompt.executor.calls, wantCalls)
	}
	prompt.phases++
	decision := "approve"
	if allowSkip && prompt.secondDecision != "" {
		decision = prompt.secondDecision
	}
	return model.ApprovalRecord{Phase: phase, ApprovedDigest: digest, OperatorLabel: "fixture-operator", Decision: decision, CreatedAt: fmt.Sprintf("2026-07-16T10:00:0%dZ", prompt.phases)}, nil
}

type fixtureResponse struct {
	nativeName     string
	native, stdout []byte
	stderr         []byte
	status, reason string
	exitCode       int
}

type fixtureExecutor struct {
	calls     int
	responses map[string]fixtureResponse
}

type interruptingExecutor struct {
	fixture *fixtureExecutor
	cancel  context.CancelFunc
}

func (executor interruptingExecutor) Run(ctx context.Context, request runner.Request) (runner.Result, error) {
	result, err := executor.fixture.Run(ctx, request)
	executor.cancel()
	return result, err
}

func (executor *fixtureExecutor) Run(_ context.Context, request runner.Request) (runner.Result, error) {
	executor.calls++
	if err := os.MkdirAll(request.OutputDir, 0o700); err != nil {
		return runner.Result{}, err
	}
	if err := os.Chmod(request.OutputDir, 0o700); err != nil {
		return runner.Result{}, err
	}
	response, configured := executor.responses[request.Tool.Name]
	if !configured {
		response = defaultFixtureResponse(request)
	}
	if response.status == "" {
		response.status = runner.StatusSuccess
	}
	artifacts := make([]runner.Artifact, 0, 3)
	for _, item := range []struct {
		name, role string
		content    []byte
	}{{"stdout.raw", "stdout", response.stdout}, {"stderr.raw", "stderr", response.stderr}, {response.nativeName, "native", response.native}} {
		if item.name == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(request.OutputDir, item.name), item.content, 0o600); err != nil {
			return runner.Result{}, err
		}
		digest := sha256.Sum256(item.content)
		artifacts = append(artifacts, runner.Artifact{Role: item.role, Path: item.name, SHA256: "sha256:" + hex.EncodeToString(digest[:]), Size: int64(len(item.content))})
	}
	return runner.Result{Envelope: runner.ArtifactEnvelope{
		ExecutionID: request.ExecutionID, ToolName: request.Tool.Name, ToolPath: request.Tool.ResolvedPath,
		ToolVersion: request.Tool.Version, ActivityClass: request.Tool.ActivityClass, Argv: append([]string(nil), request.Tool.Argv...),
		StartedAt: "2026-07-16T10:00:00Z", FinishedAt: "2026-07-16T10:00:01Z", DurationMillis: 1000,
		Status: response.status, Reason: response.reason, ExitCode: response.exitCode, Artifacts: artifacts,
	}}, nil
}

func defaultFixtureResponse(request runner.Request) fixtureResponse {
	switch request.Tool.Name {
	case "gau":
		return fixtureResponse{
			nativeName: "native-output.txt",
			native:     []byte("http://127.0.0.1:18080/api?existing=1\n"),
			stderr: []byte(
				"time=\"2026-07-16T10:00:00Z\" level=warning msg=\"error reading config: Config file .reconctx-gau-config-absent not found, using default config\"\n" +
					"time=\"2026-07-16T10:00:00Z\" level=info msg=\"fetching 127.0.0.1\" page=0 provider=otx\n" +
					"time=\"2026-07-16T10:00:00Z\" level=info msg=\"fetching 127.0.0.1\" page=0 provider=urlscan\n",
			),
			status: runner.StatusSuccess,
		}
	case "katana":
		return fixtureResponse{nativeName: "native-output.jsonl", native: []byte("{\"timestamp\":\"2026-07-16T10:00:00Z\",\"request\":{\"endpoint\":\"http://127.0.0.1:18080/api?existing=1\",\"method\":\"GET\"},\"response\":{\"status_code\":200,\"headers\":{\"Content-Type\":\"application/json\"},\"content_length\":2}}\n"), status: runner.StatusSuccess}
	case "arjun":
		target := argumentAfter(request.Tool.Argv, "-u")
		native, _ := json.Marshal(map[string]any{target: map[string]any{"headers": map[string]string{}, "method": "GET", "params": []string{"id"}}})
		return fixtureResponse{nativeName: "native-output.json", native: append(native, '\n'), stdout: []byte("Processing chunks: 100%\n"), status: runner.StatusSuccess}
	default:
		return fixtureResponse{status: runner.StatusFailed, reason: "unexpected_tool", exitCode: 1}
	}
}

func completedFixtureWorkflow(t *testing.T, executor *fixtureExecutor) (string, planArtifact, model.Run, workflowState) {
	t.Helper()
	workspacePath, planPath := plannedWorkflowFixture(t)
	artifact, err := loadPlanArtifact(planPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	prompt := &checkpointPrompter{executor: executor, t: t}
	if err := executeNewRun(context.Background(), root, artifact, filepath.ToSlash(filepath.Join("handoff", artifact.Plan.RunID)), prompt, executor, io.Discard); err != nil {
		t.Fatal(err)
	}
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, artifact.Plan.RunID, artifact.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	return workspacePath, artifact, run, workflow
}

func assertExecution(t *testing.T, workflow workflowState, tool, status, gap string) {
	t.Helper()
	for _, execution := range workflow.Records.ToolExecutions {
		if execution.Tool.Name != tool {
			continue
		}
		if execution.Status != status {
			t.Fatalf("%s status=%s want=%s", tool, execution.Status, status)
		}
		if gap == "" {
			return
		}
		for _, diagnostic := range execution.Gaps {
			if diagnostic.Code == gap {
				return
			}
		}
		t.Fatalf("%s gaps=%#v want=%s", tool, execution.Gaps, gap)
	}
	t.Fatalf("missing %s execution", tool)
}

func assertPartialWorkflow(t *testing.T, workspacePath, runID string, run model.Run, workflow workflowState) {
	t.Helper()
	if run.State != model.RunPartial || workflow.Status != "partial" {
		t.Fatalf("run=%s workflow=%s", run.State, workflow.Status)
	}
	manifest, err := os.ReadFile(filepath.Join(workspacePath, "handoff", runID, "manifest.json"))
	if err != nil || !bytes.Contains(manifest, []byte(`"status":"partial"`)) {
		t.Fatalf("manifest=%s err=%v", manifest, err)
	}
}

func repositoryFixture(t *testing.T, name string) []byte {
	t.Helper()
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func argumentAfter(arguments []string, flag string) string {
	for index := range len(arguments) - 1 {
		if arguments[index] == flag {
			return arguments[index+1]
		}
	}
	return ""
}

func plannedWorkflowFixture(t *testing.T) (string, string) {
	t.Helper()
	workspacePath := t.TempDir()
	if err := os.Chmod(workspacePath, 0o700); err != nil {
		t.Fatal(err)
	}
	inputs := t.TempDir()
	scopePath := filepath.Join(inputs, "scope.yaml")
	wordlistPath := filepath.Join(inputs, "params.txt")
	if err := os.WriteFile(scopePath, []byte("mode: allowlist\nroots:\n  - id: loopback\n    kind: origin\n    value: http://127.0.0.1:18080\nexternal_policy: reject\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wordlistPath, []byte("id\nexisting\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	toolParent := t.TempDir()
	if err := os.Chmod(toolParent, 0o700); err != nil {
		t.Fatal(err)
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, name := range []string{"gau", "katana", "arjun"} {
		content, err := os.ReadFile(filepath.Join(repository, "integration", "faketools", name))
		if err != nil {
			t.Fatal(err)
		}
		paths[name] = filepath.Join(toolParent, name)
		if err := os.WriteFile(paths[name], content, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	planPath := filepath.Join(workspacePath, "approved-plan.json")
	args := []string{"plan", "--target", "127.0.0.1", "--seed", "http://127.0.0.1:18080/", "--scope", scopePath, "--wordlist", wordlistPath, "--workspace", workspacePath, "--out", planPath, "--gau-path", paths["gau"], "--katana-path", paths["katana"], "--arjun-path", paths["arjun"]}
	var stdout, stderr bytes.Buffer
	if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("plan exit = %d, stderr = %q", code, stderr.String())
	}
	return workspacePath, planPath
}
