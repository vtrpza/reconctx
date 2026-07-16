package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/app"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/runner"
	"github.com/vtrpza/reconctx/internal/workspace"
)

func TestResumeRequiresTTYBeforeFreshApproval(t *testing.T) {
	workspacePath, planPath := plannedWorkflowFixture(t)
	artifact, err := loadPlanArtifact(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"resume", "--workspace", workspacePath, artifact.Plan.RunID}, strings.NewReader("approve ignored\noperator\n"), &stdout, &stderr)
	if code != 3 || !strings.Contains(stderr.String(), "interactive terminal") {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(workspacePath, "runs", artifact.Plan.RunID, "executions")); !os.IsNotExist(err) {
		t.Fatalf("non-TTY resume created active execution state: %v", err)
	}
}

func TestResumePlannedAndCollectionApprovalCheckpoints(t *testing.T) {
	for _, state := range []model.RunState{model.RunPlanned, model.RunAwaitingCollectionApproval} {
		t.Run(string(state), func(t *testing.T) {
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
			run, err := readRunState(root, artifact.Plan.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if state == model.RunAwaitingCollectionApproval {
				run, err = app.AwaitCollectionApproval(run, artifact.Plan)
				if err != nil {
					t.Fatal(err)
				}
				if err := writeRunState(root, run); err != nil {
					t.Fatal(err)
				}
			}
			executor := &fixtureExecutor{}
			prompt := &checkpointPrompter{t: t, executor: executor}
			var output bytes.Buffer
			if err := resumeWorkflow(context.Background(), root, artifact, run, filepath.ToSlash(filepath.Join("handoff", artifact.Plan.RunID)), prompt, executor, &output); err != nil {
				t.Fatal(err)
			}
			resumed, err := readRunState(root, run.ID)
			if err != nil || resumed.State != model.RunSuccess || executor.calls != 3 || prompt.phases != 2 || !strings.Contains(output.String(), "handoff: "+filepath.Join(workspacePath, "handoff", run.ID)) {
				t.Fatalf("resumed = %#v, calls = %d, prompts = %d, output = %q, err = %v", resumed, executor.calls, prompt.phases, output.String(), err)
			}
		})
	}
}

func TestResumeArjunCheckpointDecisions(t *testing.T) {
	for _, decision := range []string{"approve", "skip", "cancel"} {
		t.Run(decision, func(t *testing.T) {
			workspacePath, artifact, executor := awaitingArjunFixture(t)
			root, err := workspace.Open(workspacePath)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			run, err := readRunState(root, artifact.Plan.RunID)
			if err != nil {
				t.Fatal(err)
			}
			prompt := &resumeDecisionPrompter{t: t, executor: executor, decision: decision, digest: run.QueueDigest}
			err = resumeWorkflow(context.Background(), root, artifact, run, filepath.ToSlash(filepath.Join("handoff", run.ID)), prompt, executor, io.Discard)
			resumed, stateErr := readRunState(root, run.ID)
			if stateErr != nil || prompt.calls != 1 {
				t.Fatalf("state error = %v, prompts = %d", stateErr, prompt.calls)
			}
			switch decision {
			case "approve":
				if err != nil || resumed.State != model.RunSuccess || executor.calls != 3 {
					t.Fatalf("approve: state=%s calls=%d err=%v", resumed.State, executor.calls, err)
				}
			case "skip":
				if err != nil || resumed.State != model.RunPartial || executor.calls != 2 || !slices.Contains(resumed.CoverageGaps, "arjun_skipped_by_operator") {
					t.Fatalf("skip: state=%s calls=%d gaps=%v err=%v", resumed.State, executor.calls, resumed.CoverageGaps, err)
				}
			case "cancel":
				if !errors.Is(err, errApprovalStopped) || resumed.State != model.RunCancelled || executor.calls != 2 {
					t.Fatalf("cancel: state=%s calls=%d err=%v", resumed.State, executor.calls, err)
				}
			}
		})
	}
}

func TestResumeArjunRejectsCheckpointDriftBeforePromptOrLaunch(t *testing.T) {
	workspacePath, artifact, executor := awaitingArjunFixture(t)
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, run.ID, artifact.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	workflow.Queue.MaxTargets--
	if err := writeWorkflow(root, run.ID, workflow); err != nil {
		t.Fatal(err)
	}
	prompt := &unexpectedPrompter{}
	err = resumeWorkflow(context.Background(), root, artifact, run, filepath.ToSlash(filepath.Join("handoff", run.ID)), prompt, executor, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "new reviewed plan is required") || prompt.calls != 0 || executor.calls != 2 {
		t.Fatalf("drift result: err=%v prompts=%d launches=%d", err, prompt.calls, executor.calls)
	}
}

func TestResumeRejectsChangedApprovedWordlistBeforePromptOrLaunch(t *testing.T) {
	workspacePath, artifact, executor := awaitingArjunFixture(t)
	if err := os.WriteFile(artifact.Plan.Inputs.WordlistPath, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil {
		t.Fatal(err)
	}
	prompt := &unexpectedPrompter{}
	err = resumeWorkflow(context.Background(), root, artifact, run, filepath.ToSlash(filepath.Join("handoff", run.ID)), prompt, executor, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "wordlist changed") || prompt.calls != 0 || executor.calls != 2 {
		t.Fatalf("resume result: err=%v prompts=%d launches=%d", err, prompt.calls, executor.calls)
	}
}

func TestResumeRejectsDeletedRawSourceMappingsBeforePromptOrLaunch(t *testing.T) {
	workspacePath, artifact, executor := awaitingArjunFixture(t)
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, run.ID, artifact.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	workflow.RawSources = map[string]string{}
	if err := writeWorkflow(root, run.ID, workflow); err != nil {
		t.Fatal(err)
	}
	prompt := &unexpectedPrompter{}
	err = resumeWorkflow(context.Background(), root, artifact, run, filepath.ToSlash(filepath.Join("handoff", run.ID)), prompt, executor, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not embedded") || prompt.calls != 0 || executor.calls != 2 {
		t.Fatalf("resume result: err=%v prompts=%d launches=%d", err, prompt.calls, executor.calls)
	}
}

func TestResumeRejectsMutatedRawSourceBeforeAtomicPublication(t *testing.T) {
	workspacePath, artifact, executor := awaitingArjunFixture(t)
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, run.ID, artifact.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	var source string
	for _, candidate := range workflow.RawSources {
		source = candidate
		break
	}
	if source == "" {
		t.Fatal("checkpoint has no raw source to mutate")
	}
	content, err := root.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, filepath.FromSlash(source)), append(content, []byte("mutated\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt := &unexpectedPrompter{}
	outputPrefix := filepath.ToSlash(filepath.Join("handoff", run.ID))
	err = resumeWorkflow(context.Background(), root, artifact, run, outputPrefix, prompt, executor, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "mismatch") || prompt.calls != 0 || executor.calls != 2 {
		t.Fatalf("resume result: err=%v prompts=%d launches=%d", err, prompt.calls, executor.calls)
	}
	checkpoint, stateErr := readRunState(root, run.ID)
	if stateErr != nil || checkpoint.State != model.RunAwaitingArjunApproval {
		t.Fatalf("checkpoint state = %#v, %v", checkpoint, stateErr)
	}
	if _, statErr := os.Stat(filepath.Join(workspacePath, filepath.FromSlash(outputPrefix))); !os.IsNotExist(statErr) {
		t.Fatalf("integrity failure published a handoff: %v", statErr)
	}
}

func TestResumeRevalidatesRawSourceAfterApprovalBeforeLaunch(t *testing.T) {
	workspacePath, artifact, executor := awaitingArjunFixture(t)
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := readWorkflow(root, run.ID, artifact.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	var source string
	for _, candidate := range workflow.RawSources {
		source = candidate
		break
	}
	content, err := root.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	prompt := &mutatingResumePrompter{
		t: t, executor: executor, digest: run.QueueDigest,
		path: filepath.Join(workspacePath, filepath.FromSlash(source)), content: append(content, []byte("mutated during approval\n")...),
	}
	err = resumeWorkflow(context.Background(), root, artifact, run, filepath.ToSlash(filepath.Join("handoff", run.ID)), prompt, executor, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "mismatch") || prompt.calls != 1 || executor.calls != 2 {
		t.Fatalf("resume result: err=%v prompts=%d launches=%d", err, prompt.calls, executor.calls)
	}
}

func TestResumeRevalidatesWordlistAfterApprovalBeforeLaunch(t *testing.T) {
	workspacePath, artifact, executor := awaitingArjunFixture(t)
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil {
		t.Fatal(err)
	}
	prompt := &mutatingResumePrompter{
		t: t, executor: executor, digest: run.QueueDigest,
		path: artifact.Plan.Inputs.WordlistPath, content: []byte("changed during approval\n"),
	}
	err = resumeWorkflow(context.Background(), root, artifact, run, filepath.ToSlash(filepath.Join("handoff", run.ID)), prompt, executor, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "wordlist changed") || prompt.calls != 1 || executor.calls != 2 {
		t.Fatalf("resume result: err=%v prompts=%d launches=%d", err, prompt.calls, executor.calls)
	}
}

func TestResumeCompilesOfflineAndVerifiesSuccessfulNoOp(t *testing.T) {
	workspacePath := buildFixture(t)
	args := []string{"resume", "--workspace", workspacePath, "run_test"}
	var stdout, stderr bytes.Buffer
	if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("offline compile: code=%d stderr=%q", code, stderr.String())
	}
	root, err := workspace.Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	run, err := readRunState(root, "run_test")
	if err != nil || run.State != model.RunSuccess {
		t.Fatalf("compiled run = %#v, %v", run, err)
	}
	// Simulate a crash after a finalized compile but before the success state write.
	run.State = model.RunCompiling
	if err := writeRunState(root, run); err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("finalized compile recovery: code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "handoff verified") {
		t.Fatalf("success no-op: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "handoff", "run_test", "CONTEXT.md"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "checksum mismatch") {
		t.Fatalf("tampered success: code=%d stderr=%q", code, stderr.String())
	}
}

func TestResumeRejectsUnlistedOrPartialPublishedDestination(t *testing.T) {
	t.Run("stale unpublished stage", func(t *testing.T) {
		workspacePath := buildFixture(t)
		stale := filepath.Join(workspacePath, "handoff", ".reconctx-stage-crash")
		if err := os.MkdirAll(stale, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stale, "partial"), []byte("partial\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := Run([]string{"resume", "--workspace", workspacePath, "run_test"}, strings.NewReader(""), &stdout, &stderr)
		if code != 0 {
			t.Fatalf("resume with stale stage: code=%d stderr=%q", code, stderr.String())
		}
		if _, err := os.Stat(filepath.Join(workspacePath, "handoff", "run_test", "checksums.sha256")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unlisted extra", func(t *testing.T) {
		workspacePath := buildFixture(t)
		args := []string{"resume", "--workspace", workspacePath, "run_test"}
		var stdout, stderr bytes.Buffer
		if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 {
			t.Fatalf("initial compile: code=%d stderr=%q", code, stderr.String())
		}
		extra := filepath.Join(workspacePath, "handoff", "run_test", "extra")
		if err := os.WriteFile(extra, []byte("unlisted\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		stdout.Reset()
		stderr.Reset()
		if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "unlisted") {
			t.Fatalf("extra entry: code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("checksum omission", func(t *testing.T) {
		workspacePath := buildFixture(t)
		args := []string{"resume", "--workspace", workspacePath, "run_test"}
		var stdout, stderr bytes.Buffer
		if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 {
			t.Fatalf("initial compile: code=%d stderr=%q", code, stderr.String())
		}
		output := filepath.Join(workspacePath, "handoff", "run_test")
		checksumPath := filepath.Join(output, "checksums.sha256")
		checksums, err := os.ReadFile(checksumPath)
		if err != nil {
			t.Fatal(err)
		}
		var reduced strings.Builder
		for _, line := range strings.Split(string(checksums), "\n") {
			if line != "" && !strings.HasSuffix(line, "  README.md") {
				reduced.WriteString(line + "\n")
			}
		}
		if err := os.WriteFile(checksumPath, []byte(reduced.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(output, "README.md")); err != nil {
			t.Fatal(err)
		}
		stdout.Reset()
		stderr.Reset()
		if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "inventory") {
			t.Fatalf("checksum omission: code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("preexisting partial", func(t *testing.T) {
		workspacePath := buildFixture(t)
		partial := filepath.Join(workspacePath, "handoff", "run_test", "partial")
		if err := os.MkdirAll(filepath.Dir(partial), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(partial, []byte("partial\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := Run([]string{"resume", "--workspace", workspacePath, "run_test"}, strings.NewReader(""), &stdout, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "finalized") {
			t.Fatalf("partial destination: code=%d stderr=%q", code, stderr.String())
		}
		content, err := os.ReadFile(partial)
		if err != nil || string(content) != "partial\n" {
			t.Fatalf("partial destination changed: %q, %v", content, err)
		}
	})
}

func TestResumeFailsClosedForTerminalAndInFlightStates(t *testing.T) {
	for _, state := range []model.RunState{model.RunFailed, model.RunCollecting} {
		t.Run(string(state), func(t *testing.T) {
			workspacePath := buildFixture(t)
			root, err := workspace.Open(workspacePath)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			artifact, err := loadWorkspacePlan(root, "run_test")
			if err != nil {
				t.Fatal(err)
			}
			run, err := readRunState(root, "run_test")
			if err != nil {
				t.Fatal(err)
			}
			run.State = state
			if err := writeRunState(root, run); err != nil {
				t.Fatal(err)
			}
			prompt := &unexpectedPrompter{}
			executor := &countingExecutor{}
			err = resumeWorkflow(context.Background(), root, artifact, run, "handoff/run_test", prompt, executor, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "new reviewed plan is required") || prompt.calls != 0 || executor.calls != 0 {
				t.Fatalf("state %s: err=%v prompts=%d launches=%d", state, err, prompt.calls, executor.calls)
			}
		})
	}
}

func TestResumeRequiresAbsoluteWorkspace(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"resume", "--workspace", "relative", "run_test"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
}

func TestResumeRejectsOutputOutsideWorkspace(t *testing.T) {
	workspacePath := buildFixture(t)
	outside := filepath.Join(t.TempDir(), "handoff")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"resume", "--workspace", workspacePath, "--out", outside, "run_test"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("unsafe output was created: %v", err)
	}
}

func awaitingArjunFixture(t *testing.T) (string, planArtifact, *fixtureExecutor) {
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
	executor := &fixtureExecutor{}
	prompt := &pauseAtArjunPrompter{t: t, executor: executor}
	err = executeNewRun(context.Background(), root, artifact, filepath.ToSlash(filepath.Join("handoff", artifact.Plan.RunID)), prompt, executor, io.Discard)
	if !errors.Is(err, errApprovalStopped) || executor.calls != 2 {
		t.Fatalf("prepare Arjun checkpoint: calls=%d err=%v", executor.calls, err)
	}
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil || run.State != model.RunAwaitingArjunApproval {
		t.Fatalf("Arjun checkpoint = %#v, %v", run, err)
	}
	return workspacePath, artifact, executor
}

type pauseAtArjunPrompter struct {
	t        *testing.T
	executor *fixtureExecutor
}

func (prompt *pauseAtArjunPrompter) Prompt(phase, digest string, allowSkip bool) (model.ApprovalRecord, error) {
	prompt.t.Helper()
	if allowSkip {
		if prompt.executor.calls != 2 {
			prompt.t.Fatalf("Arjun checkpoint observed %d launches", prompt.executor.calls)
		}
		return model.ApprovalRecord{}, errApprovalStopped
	}
	if prompt.executor.calls != 0 {
		prompt.t.Fatalf("collection prompt observed %d launches", prompt.executor.calls)
	}
	return model.ApprovalRecord{Phase: phase, ApprovedDigest: digest, OperatorLabel: "fixture-operator", Decision: "approve", CreatedAt: "2026-07-16T10:00:01Z"}, nil
}

type resumeDecisionPrompter struct {
	t        *testing.T
	executor *fixtureExecutor
	decision string
	digest   string
	calls    int
}

type mutatingResumePrompter struct {
	t        *testing.T
	executor *fixtureExecutor
	digest   string
	path     string
	content  []byte
	calls    int
}

func (prompt *mutatingResumePrompter) Prompt(phase, digest string, allowSkip bool) (model.ApprovalRecord, error) {
	prompt.t.Helper()
	prompt.calls++
	if !allowSkip || prompt.executor.calls != 2 || digest != prompt.digest {
		prompt.t.Fatalf("unsafe resumed prompt: allowSkip=%v launches=%d digest=%q want=%q", allowSkip, prompt.executor.calls, digest, prompt.digest)
	}
	if err := os.WriteFile(prompt.path, prompt.content, 0o600); err != nil {
		prompt.t.Fatal(err)
	}
	return model.ApprovalRecord{Phase: phase, ApprovedDigest: digest, OperatorLabel: "fixture-operator", Decision: "approve", CreatedAt: "2026-07-16T10:00:02Z"}, nil
}

func (prompt *resumeDecisionPrompter) Prompt(phase, digest string, allowSkip bool) (model.ApprovalRecord, error) {
	prompt.t.Helper()
	prompt.calls++
	if !allowSkip || prompt.executor.calls != 2 || digest != prompt.digest {
		prompt.t.Fatalf("unsafe resumed prompt: allowSkip=%v launches=%d digest=%q want=%q", allowSkip, prompt.executor.calls, digest, prompt.digest)
	}
	return model.ApprovalRecord{Phase: phase, ApprovedDigest: digest, OperatorLabel: "fixture-operator", Decision: prompt.decision, CreatedAt: "2026-07-16T10:00:02Z"}, nil
}

type unexpectedPrompter struct{ calls int }

func (prompt *unexpectedPrompter) Prompt(string, string, bool) (model.ApprovalRecord, error) {
	prompt.calls++
	return model.ApprovalRecord{}, errors.New("unexpected prompt")
}

type countingExecutor struct{ calls int }

func (executor *countingExecutor) Run(context.Context, runner.Request) (runner.Result, error) {
	executor.calls++
	return runner.Result{}, fmt.Errorf("unexpected execution")
}
