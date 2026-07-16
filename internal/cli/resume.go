package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/vtrpza/reconctx/internal/app"
	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/candidate"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/integrity"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/workspace"
)

var resumeCandidateIDPattern = regexp.MustCompile(`^candidate_sha256_[0-9a-f]{64}$`)

func runResume(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspacePath := flags.String("workspace", "", "absolute private workspace")
	outputPath := flags.String("out", "", "handoff path inside workspace")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || !validRunID(flags.Arg(0)) || !filepath.IsAbs(*workspacePath) || strings.ContainsAny(*workspacePath, "\x00\r\n") {
		fmt.Fprintln(stderr, "reconctx resume: --workspace ABSOLUTE_PRIVATE_DIR and one RUN_ID are required")
		return 2
	}
	runID := flags.Arg(0)
	workspaceName := filepath.Clean(*workspacePath)
	outputPrefix, err := buildOutputPrefix(workspaceName, *outputPath, runID)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx resume: %v\n", err)
		return 2
	}
	root, err := workspace.Open(workspaceName)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx resume: %v\n", err)
		return 1
	}
	defer root.Close()
	artifact, err := loadWorkspacePlan(root, runID)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx resume: load plan: %v\n", err)
		return 1
	}
	if artifact.Plan.RunID != runID || artifact.Plan.WorkspaceRoot != workspaceName {
		fmt.Fprintln(stderr, "reconctx resume: plan belongs to another run or workspace")
		return 1
	}
	run, err := readRunState(root, runID)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx resume: load run state: %v\n", err)
		return 1
	}
	if resumeNeedsApproval(run.State) && !interactiveReader(stdin) {
		fmt.Fprintln(stderr, "reconctx resume: an interactive terminal is required for a fresh active-phase approval")
		return 3
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = resumeWorkflow(ctx, root, artifact, run, outputPrefix, newTerminalPrompter(stdin, stdout), productionExecutor{}, stdout)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(stderr, "reconctx resume: interrupted; private evidence was preserved")
		return 130
	case errors.Is(err, errApprovalStopped):
		fmt.Fprintf(stderr, "reconctx resume: %v\n", err)
		return 3
	default:
		fmt.Fprintf(stderr, "reconctx resume: %v\n", err)
		return 1
	}
}

func resumeNeedsApproval(state model.RunState) bool {
	return state == model.RunPlanned || state == model.RunAwaitingCollectionApproval || state == model.RunAwaitingArjunApproval
}

func resumeWorkflow(ctx context.Context, root *workspace.Root, artifact planArtifact, run model.Run, outputPrefix string, prompt approvalPrompter, executor toolExecutor, output io.Writer) error {
	if root == nil || artifact.Plan.RunID == "" || run.ID != artifact.Plan.RunID {
		return errors.New("resume state does not match the immutable plan")
	}
	if run.State != model.RunPlanned && subtle.ConstantTimeCompare([]byte(run.PlanDigest), []byte(artifact.PlanDigest)) != 1 {
		return resumeRejected(run.State, errors.New("run state plan digest drifted"))
	}
	switch run.State {
	case model.RunPlanned:
		if run.PlanDigest != "" || run.QueueDigest != "" || len(run.Approvals) != 0 {
			return resumeRejected(run.State, errors.New("planned checkpoint contains unexpected approval state"))
		}
		if err := executeNewRun(ctx, root, artifact, outputPrefix, prompt, executor, output); err != nil {
			if errors.Is(err, errApprovalStopped) || errors.Is(err, context.Canceled) {
				return err
			}
			return resumeRejected(run.State, err)
		}
		return printHandoff(output, artifact.Plan.WorkspaceRoot, outputPrefix, false)
	case model.RunAwaitingCollectionApproval:
		if run.QueueDigest != "" || len(run.Approvals) != 0 {
			return resumeRejected(run.State, errors.New("collection checkpoint contains unexpected approval state"))
		}
		if err := executeNewRun(ctx, root, artifact, outputPrefix, prompt, executor, output); err != nil {
			if errors.Is(err, errApprovalStopped) || errors.Is(err, context.Canceled) {
				return err
			}
			return resumeRejected(run.State, err)
		}
		return printHandoff(output, artifact.Plan.WorkspaceRoot, outputPrefix, false)
	case model.RunAwaitingArjunApproval:
		if len(run.Approvals) != 1 || approval.VerifyDecision(run.Approvals[0], approval.CollectionPhase, artifact.PlanDigest, "approve") != nil {
			return resumeRejected(run.State, errors.New("Arjun checkpoint has no matching collection approval"))
		}
		if err := resumeAwaitingArjun(ctx, root, artifact, run, outputPrefix, prompt, executor, output); err != nil {
			return err
		}
		return printHandoff(output, artifact.Plan.WorkspaceRoot, outputPrefix, false)
	case model.RunCompiling:
		return resumeCompiling(root, artifact, run, outputPrefix, output)
	case model.RunSuccess:
		if err := verifyRootedHandoff(root, outputPrefix, run.ID); err != nil {
			return resumeRejected(run.State, fmt.Errorf("verify successful run handoff: %w", err))
		}
		return printHandoff(output, artifact.Plan.WorkspaceRoot, outputPrefix, true)
	case model.RunPreflightFailed, model.RunPartial, model.RunFailed, model.RunInterrupted, model.RunCancelled:
		return resumeRejected(run.State, errors.New("terminal run states are never retried implicitly"))
	case model.RunCollecting, model.RunNormalizingInitial, model.RunArjunSkipped, model.RunDiscoveringParameters, model.RunNormalizingFinal:
		return resumeRejected(run.State, errors.New("the persisted in-flight checkpoint is not safely resumable"))
	default:
		return resumeRejected(run.State, errors.New("unsupported run state"))
	}
}

func resumeRejected(state model.RunState, cause error) error {
	return fmt.Errorf("run state %q cannot be resumed safely: %w; a new reviewed plan is required", state, cause)
}

func resumeAwaitingArjun(ctx context.Context, root *workspace.Root, artifact planArtifact, run model.Run, outputPrefix string, prompt approvalPrompter, executor toolExecutor, output io.Writer) error {
	scopeDocument, evaluator, _, err := loadApprovedScope(root, artifact.Plan)
	if err != nil {
		return resumeRejected(run.State, err)
	}
	if err := validateApprovedWordlist(artifact.Plan); err != nil {
		return resumeRejected(run.State, err)
	}
	workflow, err := readWorkflow(root, run.ID, artifact.PlanDigest)
	if err != nil {
		return resumeRejected(run.State, fmt.Errorf("load workflow: %w", err))
	}
	candidates, err := loadCandidateCheckpoint(root, artifact.Plan, run, workflow)
	if err != nil {
		return resumeRejected(run.State, err)
	}
	if err := validateWorkflowIntegrity(root, artifact, workflow); err != nil {
		return resumeRejected(run.State, err)
	}
	run, err = app.AwaitArjunApproval(run, artifact.Plan, scopeDocument, candidates.Queue)
	if err != nil {
		return resumeRejected(model.RunAwaitingArjunApproval, err)
	}
	if err := displayCandidateQueue(output, candidates, artifact.Plan.Inputs.WordlistSHA256); err != nil {
		return err
	}
	record, err := prompt.Prompt(approval.ArjunPhase, candidates.QueueDigest, true)
	if err != nil {
		return err
	}
	switch record.Decision {
	case "cancel":
		run, err = app.CancelRun(run, record)
		if err == nil {
			err = writeRunState(root, run)
		}
		return errors.Join(errApprovalStopped, err)
	case "skip":
		run, err = app.SkipArjun(run, artifact.Plan, scopeDocument, candidates.Queue, record)
		if err == nil {
			run, err = app.CompileSkippedArjun(run)
		}
		if err != nil {
			return resumeRejected(model.RunAwaitingArjunApproval, err)
		}
		addRunGap(&workflow.Records, "arjun.skipped_by_operator", "The operator explicitly skipped parameter discovery.")
	case "approve":
		if err := validateApprovedWordlist(artifact.Plan); err != nil {
			return resumeRejected(model.RunAwaitingArjunApproval, err)
		}
		if err := validateWorkflowIntegrity(root, artifact, workflow); err != nil {
			return resumeRejected(model.RunAwaitingArjunApproval, err)
		}
		run, err = app.StartArjun(run, artifact.Plan, scopeDocument, candidates.Queue, record)
		if err != nil {
			return resumeRejected(model.RunAwaitingArjunApproval, err)
		}
		if err := writeRunState(root, run); err != nil {
			return err
		}
		outcomes, runErr := runArjun(ctx, artifact.Plan, candidates.Queue, executor)
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			run.State = model.RunFailed
			_ = writeRunState(root, run)
			return runErr
		}
		completedQueue := candidates.Queue
		completedQueue.Candidates = completedQueue.Candidates[:len(outcomes)]
		if err := normalizeArjun(root, evaluator, &workflow, outcomes, completedQueue); err != nil {
			return err
		}
		if errors.Is(runErr, context.Canceled) {
			addRunGap(&workflow.Records, "arjun.interrupted", "Parameter discovery was interrupted; unexecuted candidates and absence claims remain unknown.")
			if err := compileInterrupted(root, artifact.Plan, &run, &workflow, outputPrefix); err != nil {
				return err
			}
			return context.Canceled
		}
		run, err = app.FinishArjun(run)
		if err == nil {
			run, err = app.FinishFinalNormalization(run)
		}
		if err != nil {
			return err
		}
	default:
		return errApprovalStopped
	}
	return finishAndCompile(root, artifact.Plan, &run, &workflow, outputPrefix)
}

func loadCandidateCheckpoint(root *workspace.Root, plan model.Plan, run model.Run, workflow workflowState) (candidate.Result, error) {
	if workflow.Queue == nil || workflow.Candidates == nil {
		return candidate.Result{}, errors.New("workflow candidate checkpoint is incomplete")
	}
	rawQueue, err := root.ReadFile(path.Join("runs", run.ID, "candidate-queue.json"))
	if err != nil {
		return candidate.Result{}, fmt.Errorf("read candidate queue: %w", err)
	}
	var queue model.CandidateQueue
	if err := decodeCanonical(rawQueue, &queue); err != nil {
		return candidate.Result{}, fmt.Errorf("decode candidate queue: %w", err)
	}
	if err := validateResumeQueue(plan, run.PlanDigest, queue); err != nil {
		return candidate.Result{}, err
	}
	queueJSON, err := canonical.Marshal(queue)
	if err != nil {
		return candidate.Result{}, err
	}
	workflowQueueJSON, err := canonical.Marshal(*workflow.Queue)
	if err != nil || !bytes.Equal(queueJSON, workflowQueueJSON) {
		return candidate.Result{}, errors.New("workflow queue differs from the immutable candidate queue")
	}
	digest, err := approval.QueueDigest(queue)
	if err != nil {
		return candidate.Result{}, fmt.Errorf("digest candidate queue: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(digest), []byte(run.QueueDigest)) != 1 {
		return candidate.Result{}, errors.New("candidate queue digest differs from run state")
	}

	rawDecisions, err := root.ReadFile(path.Join("runs", run.ID, "arjun-candidates.jsonl"))
	if err != nil {
		return candidate.Result{}, fmt.Errorf("read candidate decisions: %w", err)
	}
	decisions, err := decodeCandidateDecisions(rawDecisions, workflow.Candidates, digest)
	if err != nil {
		return candidate.Result{}, err
	}
	selected := 0
	seen := make(map[string]bool, len(decisions))
	for index := range decisions {
		decision := decisions[index]
		if seen[decision.CandidateID] {
			return candidate.Result{}, fmt.Errorf("candidate decision %d repeats an ID", index+1)
		}
		seen[decision.CandidateID] = true
		if !decision.Included {
			continue
		}
		if selected >= len(queue.Candidates) || !decisionMatchesQueue(decision, queue.Candidates[selected], queue.MaxTargets) {
			return candidate.Result{}, fmt.Errorf("included candidate decision %d differs from the ordered queue", index+1)
		}
		selected++
	}
	if selected != len(queue.Candidates) {
		return candidate.Result{}, errors.New("candidate decisions do not cover the ordered queue")
	}
	return candidate.Result{Queue: queue, QueueDigest: digest, Decisions: decisions}, nil
}

func validateResumeQueue(plan model.Plan, planDigest string, queue model.CandidateQueue) error {
	arjun := findTool(plan, "arjun")
	if queue.QueueVersion != "reconctx-candidate-queue/v0" || queue.PolicyVersion != candidate.PolicyVersion || subtle.ConstantTimeCompare([]byte(queue.PlanDigest), []byte(planDigest)) != 1 {
		return errors.New("candidate queue identity differs from the approved plan")
	}
	if arjun.ResolvedPath == "" || queue.MaxTargets != plan.Limits.ArjunMaxTargets || queue.Limits != arjun.Limits {
		return errors.New("candidate queue limits differ from the approved plan")
	}
	seen := make(map[string]bool, len(queue.Candidates))
	for index, item := range queue.Candidates {
		if !resumeCandidateIDPattern.MatchString(item.ID) || seen[item.ID] {
			return fmt.Errorf("candidate %d has an invalid or repeated ID", index+1)
		}
		seen[item.ID] = true
		expectedOutput := filepath.Join(plan.WorkspaceRoot, "runs", plan.RunID, "executions", "arjun", item.ID, "native-output.json")
		if item.RankPosition != index+1 || item.WordlistPath != plan.Inputs.WordlistPath || item.WordlistSHA256 != plan.Inputs.WordlistSHA256 || item.RequestBudget != plan.Limits.ArjunRequestBudget || item.NativeOutputPath != expectedOutput {
			return fmt.Errorf("candidate %d behavior differs from the approved policy", index+1)
		}
	}
	return nil
}

func decodeCandidateDecisions(raw []byte, workflowRows []json.RawMessage, queueDigest string) ([]candidate.Decision, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	decisions := make([]candidate.Decision, 0, len(workflowRows))
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 || len(decisions) >= len(workflowRows) {
			return nil, errors.New("candidate decision artifact differs from workflow")
		}
		var decision candidate.Decision
		if err := decodeCanonical(scanner.Bytes(), &decision); err != nil {
			return nil, fmt.Errorf("decode candidate decision %d: %w", len(decisions)+1, err)
		}
		line, err := canonical.Canonicalize(scanner.Bytes())
		if err != nil {
			return nil, err
		}
		workflowLine, err := canonical.Canonicalize(workflowRows[len(decisions)])
		if err != nil || !bytes.Equal(line, workflowLine) {
			return nil, fmt.Errorf("candidate decision %d differs from workflow", len(decisions)+1)
		}
		if decision.SchemaVersion != model.SchemaVersion || decision.RecordType != "arjun_candidate" || decision.PolicyVersion != candidate.PolicyVersion || subtle.ConstantTimeCompare([]byte(decision.QueueDigest), []byte(queueDigest)) != 1 {
			return nil, fmt.Errorf("candidate decision %d has invalid policy metadata", len(decisions)+1)
		}
		decisions = append(decisions, decision)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan candidate decisions: %w", err)
	}
	if len(decisions) != len(workflowRows) {
		return nil, errors.New("candidate decision artifact differs from workflow")
	}
	return decisions, nil
}

func decisionMatchesQueue(decision candidate.Decision, queued model.Candidate, maxTargets int) bool {
	if !decision.Eligible || decision.Method == nil || decision.SourceMode == nil || decision.RankPosition == nil || decision.Scope.RuleID == nil {
		return false
	}
	return decision.CandidateID == queued.ID && decision.EndpointID == queued.EndpointID &&
		decision.SelectedURL == queued.URL && decision.CanonicalRouteURL == queued.URL &&
		*decision.Method == queued.Method && *decision.SourceMode == queued.SourceMode && decision.Location == queued.Location &&
		*decision.RankPosition == queued.RankPosition && decision.Rank == queued.Rank &&
		slices.Equal(decision.ObservationIDs, queued.ObservationIDs) && slices.Equal(decision.EvidenceIDs, queued.EvidenceIDs) && slices.Equal(decision.SourceExecutionIDs, queued.SourceExecutionIDs) &&
		slices.Equal(decision.ReasonCodes, queued.ReasonCodes) && decision.Scope.Classification == queued.Scope.Classification && *decision.Scope.RuleID == queued.Scope.RuleID && decision.Scope.Reason == queued.Scope.Reason &&
		slices.Equal(decision.ArgvRedacted, publicArgv("arjun", queued.Argv)) && decision.WordlistSHA256 == queued.WordlistSHA256 && decision.RequestBudget == queued.RequestBudget && decision.MaxTargets == maxTargets
}

func resumeCompiling(root *workspace.Root, artifact planArtifact, run model.Run, outputPrefix string, output io.Writer) error {
	workflow, err := readWorkflow(root, run.ID, artifact.PlanDigest)
	if err != nil {
		return resumeRejected(run.State, fmt.Errorf("load compiling workflow: %w", err))
	}
	if err := validateBuildWorkflow(run, artifact, workflow); err != nil {
		return resumeRejected(run.State, err)
	}
	if err := validatePersistedCandidateCheckpoint(root, artifact.Plan, run, workflow); err != nil {
		return resumeRejected(run.State, fmt.Errorf("validate candidate checkpoint: %w", err))
	}
	_, checksumErr := root.ReadFile(path.Join(outputPrefix, "checksums.sha256"))
	switch {
	case checksumErr == nil:
		if err := verifyRootedHandoff(root, outputPrefix, run.ID); err != nil {
			return fmt.Errorf("existing handoff is not a valid completed compile: %w", err)
		}
	case errors.Is(checksumErr, os.ErrNotExist):
		if err := compileWorkflow(root, artifact, workflow, outputPrefix); err != nil {
			return err
		}
		if err := verifyRootedHandoff(root, outputPrefix, run.ID); err != nil {
			return fmt.Errorf("verify compiled handoff: %w", err)
		}
	default:
		return fmt.Errorf("inspect handoff destination: %w", checksumErr)
	}
	completed, err := completedRunState(run, workflow)
	if err != nil {
		return err
	}
	if err := writeRunState(root, completed); err != nil {
		return err
	}
	return printHandoff(output, artifact.Plan.WorkspaceRoot, outputPrefix, false)
}

func verifyRootedHandoff(root *workspace.Root, prefix, runID string) error {
	manifest, err := root.ReadFile(path.Join(prefix, "checksums.sha256"))
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	seen := map[string]bool{}
	for scanner.Scan() {
		digest, name, ok := strings.Cut(scanner.Text(), "  ")
		if !ok || len(digest) != sha256.Size*2 || digest != strings.ToLower(digest) || integrity.ValidateRelativePath(name) != nil || seen[name] {
			return errors.New("invalid checksum manifest")
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return errors.New("invalid checksum manifest")
		}
		content, err := root.ReadFile(path.Join(prefix, name))
		if err != nil {
			return fmt.Errorf("read checksummed file %s: %w", name, err)
		}
		actual := sha256.Sum256(content)
		actualHex := hex.EncodeToString(actual[:])
		if subtle.ConstantTimeCompare([]byte(actualHex), []byte(digest)) != 1 {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
		seen[name] = true
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, required := range []string{"CONTEXT.md", "manifest.json", "normalized/records.jsonl"} {
		if !seen[required] {
			return fmt.Errorf("checksum manifest omits %s", required)
		}
	}
	rawManifest, err := root.ReadFile(path.Join(prefix, "manifest.json"))
	if err != nil {
		return err
	}
	var identity struct {
		SchemaVersion string `json:"schema_version"`
		ManifestType  string `json:"manifest_type"`
		RunID         string `json:"run_id"`
		Files         []struct {
			Path      string `json:"path"`
			SHA256    string `json:"sha256"`
			SizeBytes int64  `json:"size_bytes"`
		} `json:"files"`
	}
	if err := json.Unmarshal(rawManifest, &identity); err != nil || identity.SchemaVersion != model.SchemaVersion || identity.ManifestType != "reconctx_handoff" || identity.RunID != runID {
		return errors.New("handoff manifest belongs to another contract or run")
	}
	manifestNames := make(map[string]bool, len(identity.Files)+1)
	for _, file := range identity.Files {
		if integrity.ValidateRelativePath(file.Path) != nil || file.Path == "manifest.json" || file.Path == "checksums.sha256" || manifestNames[file.Path] || !seen[file.Path] {
			return errors.New("handoff manifest inventory differs from checksums")
		}
		content, err := root.ReadFile(path.Join(prefix, file.Path))
		if err != nil {
			return fmt.Errorf("read manifest file %s: %w", file.Path, err)
		}
		digest := sha256.Sum256(content)
		if file.SizeBytes != int64(len(content)) || subtle.ConstantTimeCompare([]byte(file.SHA256), []byte(hex.EncodeToString(digest[:]))) != 1 {
			return fmt.Errorf("manifest inventory mismatch for %s", file.Path)
		}
		manifestNames[file.Path] = true
	}
	manifestNames["manifest.json"] = true
	if len(manifestNames) != len(seen) {
		return errors.New("handoff checksum inventory differs from manifest")
	}
	for name := range seen {
		if !manifestNames[name] {
			return errors.New("handoff checksum inventory differs from manifest")
		}
	}
	expectedEntries := map[string]bool{"checksums.sha256": true}
	for name := range seen {
		expectedEntries[name] = true
		for directory := path.Dir(name); directory != "."; directory = path.Dir(directory) {
			expectedEntries[directory+"/"] = true
		}
	}
	want := make([]string, 0, len(expectedEntries))
	for name := range expectedEntries {
		want = append(want, name)
	}
	slices.Sort(want)
	got, err := root.ListTree(prefix)
	if err != nil {
		return fmt.Errorf("enumerate handoff: %w", err)
	}
	if !slices.Equal(got, want) {
		return errors.New("handoff contains missing or unlisted entries")
	}
	return nil
}

func printHandoff(output io.Writer, workspaceName, outputPrefix string, verified bool) error {
	label := "handoff"
	if verified {
		label = "handoff verified"
	}
	_, err := fmt.Fprintf(output, "%s: %s\n", label, filepath.Join(workspaceName, filepath.FromSlash(outputPrefix)))
	return err
}
