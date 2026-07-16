package cli

import (
	"bufio"
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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/vtrpza/reconctx/internal/adapter"
	"github.com/vtrpza/reconctx/internal/app"
	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/candidate"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/integrity"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/runner"
	"github.com/vtrpza/reconctx/internal/scope"
	"github.com/vtrpza/reconctx/internal/workspace"
)

var errApprovalStopped = errors.New("operator did not approve the active phase")

type approvalPrompter interface {
	Prompt(phase, digest string, allowSkip bool) (model.ApprovalRecord, error)
}

type terminalPrompter struct {
	input  *bufio.Scanner
	output io.Writer
}

func newTerminalPrompter(input io.Reader, output io.Writer) *terminalPrompter {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 256), 4096)
	return &terminalPrompter{input: scanner, output: output}
}

func (prompt *terminalPrompter) Prompt(phase, digest string, allowSkip bool) (model.ApprovalRecord, error) {
	decisions := "approve|cancel"
	if allowSkip {
		decisions = "approve|skip|cancel"
	}
	if _, err := fmt.Fprintf(prompt.output, "%s approval required. Type exactly `<decision> %s` where decision is %s:\n", phase, digest, decisions); err != nil {
		return model.ApprovalRecord{}, err
	}
	if !prompt.input.Scan() {
		return model.ApprovalRecord{}, errors.Join(errApprovalStopped, prompt.input.Err())
	}
	fields := strings.Fields(prompt.input.Text())
	if len(fields) != 2 || fields[1] != digest || fields[0] != "approve" && fields[0] != "skip" && fields[0] != "cancel" || fields[0] == "skip" && !allowSkip {
		return model.ApprovalRecord{}, errApprovalStopped
	}
	if _, err := io.WriteString(prompt.output, "Operator label (private run evidence):\n"); err != nil {
		return model.ApprovalRecord{}, err
	}
	if !prompt.input.Scan() {
		return model.ApprovalRecord{}, errors.Join(errApprovalStopped, prompt.input.Err())
	}
	label := strings.TrimSpace(prompt.input.Text())
	if label == "" || len(label) > 128 || hasControl(label) {
		return model.ApprovalRecord{}, errors.New("operator label must be 1-128 printable characters")
	}
	return model.ApprovalRecord{
		Phase: phase, ApprovedDigest: digest, OperatorLabel: label, Decision: fields[0],
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func hasControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return true
		}
	}
	return false
}

type toolExecutor interface {
	Run(context.Context, runner.Request) (runner.Result, error)
}

type productionExecutor struct{}

func (productionExecutor) Run(ctx context.Context, request runner.Request) (runner.Result, error) {
	return runner.Run(ctx, request)
}

type executionOutcome struct {
	Request runner.Request
	Result  runner.Result
}

func runRun(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	handoff := flags.String("out", "", "handoff path inside the plan workspace")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "reconctx run: one plan artifact is required")
		return 2
	}
	artifact, err := loadPlanArtifact(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "reconctx run: %v\n", err)
		return 1
	}
	if !interactiveReader(stdin) {
		fmt.Fprintln(stderr, "reconctx run: an interactive terminal is required; v0 has no active non-interactive approval")
		return 3
	}
	output, err := handoffPrefix(artifact.Plan.WorkspaceRoot, artifact.Plan.RunID, *handoff)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx run: %v\n", err)
		return 2
	}
	root, err := workspace.Open(artifact.Plan.WorkspaceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx run: %v\n", err)
		return 1
	}
	defer root.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = executeNewRun(ctx, root, artifact, output, newTerminalPrompter(stdin, stdout), productionExecutor{}, stdout)
	switch {
	case err == nil:
		if _, writeErr := fmt.Fprintf(stdout, "handoff: %s\n", filepath.Join(artifact.Plan.WorkspaceRoot, filepath.FromSlash(output))); writeErr != nil {
			return 1
		}
		return 0
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(stderr, "reconctx run: interrupted; private evidence was preserved")
		return 130
	case errors.Is(err, errApprovalStopped):
		fmt.Fprintf(stderr, "reconctx run: %v\n", err)
		return 3
	default:
		fmt.Fprintf(stderr, "reconctx run: %v\n", err)
		return 1
	}
}

func interactiveReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func handoffPrefix(workspaceRoot, runID, requested string) (string, error) {
	if requested == "" {
		return path.Join("handoff", runID), nil
	}
	if !filepath.IsAbs(requested) {
		if err := integrity.ValidateRelativePath(filepath.ToSlash(requested)); err != nil {
			return "", err
		}
		return filepath.ToSlash(requested), nil
	}
	return workspaceRelative(workspaceRoot, requested)
}

func executeNewRun(ctx context.Context, root *workspace.Root, artifact planArtifact, outputPrefix string, prompt approvalPrompter, executor toolExecutor, output io.Writer) error {
	workspaceArtifact, err := loadWorkspacePlan(root, artifact.Plan.RunID)
	if err != nil {
		return fmt.Errorf("load workspace plan: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(workspaceArtifact.PlanDigest), []byte(artifact.PlanDigest)) != 1 {
		return errors.New("supplied plan does not match the immutable workspace plan")
	}
	artifact = workspaceArtifact
	scopeDocument, evaluator, scopeConfig, err := loadApprovedScope(root, artifact.Plan)
	if err != nil {
		return err
	}
	if err := validateApprovedWordlist(artifact.Plan); err != nil {
		return err
	}
	run, err := readRunState(root, artifact.Plan.RunID)
	if err != nil {
		return err
	}
	switch run.State {
	case model.RunPlanned:
		run, err = app.AwaitCollectionApproval(run, artifact.Plan)
		if err != nil {
			return err
		}
		if err := writeRunState(root, run); err != nil {
			return err
		}
	case model.RunAwaitingCollectionApproval:
		if subtle.ConstantTimeCompare([]byte(run.PlanDigest), []byte(artifact.PlanDigest)) != 1 {
			return errors.New("persisted collection approval checkpoint does not match the immutable plan")
		}
	default:
		return fmt.Errorf("run is %q; use resume instead", run.State)
	}
	if _, err := io.WriteString(output, app.DisplayPlan(artifact.Plan, artifact.PlanDigest)); err != nil {
		return err
	}
	record, err := prompt.Prompt(approval.CollectionPhase, artifact.PlanDigest, false)
	if err != nil {
		return err
	}
	if record.Decision == "cancel" {
		run, err = app.CancelRun(run, record)
		if err == nil {
			err = writeRunState(root, run)
		}
		return errors.Join(errApprovalStopped, err)
	}
	run, err = app.StartCollection(run, artifact.Plan, record)
	if err != nil {
		return err
	}
	if err := writeRunState(root, run); err != nil {
		return err
	}

	initial, runErr := runInitialCollection(ctx, artifact.Plan, executor)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		run.State = model.RunFailed
		_ = writeRunState(root, run)
		return runErr
	}
	run, err = app.FinishCollection(run)
	if err != nil {
		return err
	}
	if err := writeRunState(root, run); err != nil {
		return err
	}
	workflow, err := normalizeInitial(root, artifact, scopeConfig, evaluator, record, initial)
	if err != nil {
		run.State = model.RunFailed
		_ = writeRunState(root, run)
		return err
	}
	if errors.Is(runErr, context.Canceled) {
		addRunGap(&workflow.Records, "run.interrupted_before_candidate_policy", "Collection was interrupted before the Arjun candidate phase; unfinished coverage remains unknown.")
		if err := compileInterrupted(root, artifact.Plan, &run, &workflow, outputPrefix); err != nil {
			return err
		}
		return context.Canceled
	}

	arjun := findTool(artifact.Plan, "arjun")
	candidates, err := candidate.Build(workflow.Records, candidate.Config{
		PlanDigest: artifact.PlanDigest, ArjunPath: arjun.ResolvedPath,
		WordlistPath: artifact.Plan.Inputs.WordlistPath, WordlistSHA256: artifact.Plan.Inputs.WordlistSHA256,
		NativeOutputRoot: filepath.Join(artifact.Plan.WorkspaceRoot, "runs", artifact.Plan.RunID, "executions", "arjun"),
		Limits:           arjun.Limits, MaxTargets: artifact.Plan.Limits.ArjunMaxTargets, RequestBudget: artifact.Plan.Limits.ArjunRequestBudget,
	})
	if err != nil {
		return err
	}
	workflow.Queue = &candidates.Queue
	workflow.Candidates, err = candidateMessages(candidates.Decisions)
	if err != nil {
		return err
	}
	if err := persistCandidates(root, artifact.Plan.RunID, candidates); err != nil {
		return err
	}
	if err := writeWorkflow(root, artifact.Plan.RunID, workflow); err != nil {
		return err
	}
	if err := validateApprovedWordlist(artifact.Plan); err != nil {
		return err
	}
	if err := validateWorkflowIntegrity(root, artifact, workflow); err != nil {
		return err
	}
	run, err = app.AwaitArjunApproval(run, artifact.Plan, scopeDocument, candidates.Queue)
	if err != nil {
		return err
	}
	if err := writeRunState(root, run); err != nil {
		return err
	}
	if err := displayCandidateQueue(output, candidates, artifact.Plan.Inputs.WordlistSHA256); err != nil {
		return err
	}
	recordB, err := prompt.Prompt(approval.ArjunPhase, candidates.QueueDigest, true)
	if err != nil {
		return err
	}
	switch recordB.Decision {
	case "cancel":
		run, err = app.CancelRun(run, recordB)
		if err == nil {
			err = writeRunState(root, run)
		}
		return errors.Join(errApprovalStopped, err)
	case "skip":
		run, err = app.SkipArjun(run, artifact.Plan, scopeDocument, candidates.Queue, recordB)
		if err == nil {
			run, err = app.CompileSkippedArjun(run)
		}
		if err != nil {
			return err
		}
		addRunGap(&workflow.Records, "arjun.skipped_by_operator", "The operator explicitly skipped parameter discovery.")
	case "approve":
		if err := validateApprovedWordlist(artifact.Plan); err != nil {
			return err
		}
		if err := validateWorkflowIntegrity(root, artifact, workflow); err != nil {
			return err
		}
		run, err = app.StartArjun(run, artifact.Plan, scopeDocument, candidates.Queue, recordB)
		if err != nil {
			return err
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

func loadApprovedScope(root *workspace.Root, plan model.Plan) ([]byte, *scope.Evaluator, scope.Config, error) {
	document, err := root.ReadFile(plan.Inputs.ScopePath)
	if err != nil {
		return nil, nil, scope.Config{}, fmt.Errorf("read approved scope: %w", err)
	}
	digest := sha256.Sum256(document)
	if subtle.ConstantTimeCompare([]byte("sha256:"+hex.EncodeToString(digest[:])), []byte(plan.Inputs.ScopeSHA256)) != 1 {
		return nil, nil, scope.Config{}, errors.New("scope document changed after planning")
	}
	config, err := scope.LoadYAML(strings.NewReader(string(document)))
	if err != nil {
		return nil, nil, scope.Config{}, err
	}
	evaluator, err := scope.NewEvaluator(config)
	return document, evaluator, config, err
}

func validateApprovedWordlist(plan model.Plan) error {
	content, err := readRegularFile(plan.Inputs.WordlistPath, maxWordlistDocument)
	if err != nil {
		return fmt.Errorf("read approved wordlist: %w", err)
	}
	digest := sha256.Sum256(content)
	if subtle.ConstantTimeCompare([]byte("sha256:"+hex.EncodeToString(digest[:])), []byte(plan.Inputs.WordlistSHA256)) != 1 {
		return errors.New("wordlist changed after planning")
	}
	return nil
}

func findTool(plan model.Plan, name string) model.ToolPlan {
	for _, tool := range plan.Tools {
		if tool.Name == name {
			return tool
		}
	}
	return model.ToolPlan{}
}

func runInitialCollection(ctx context.Context, plan model.Plan, executor toolExecutor) ([]executionOutcome, error) {
	outcomes := make([]executionOutcome, 0, len(plan.Tools)-1)
	for _, tool := range plan.Tools {
		if tool.Name == "arjun" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return outcomes, err
		}
		request, err := requestForTool(plan, tool, true)
		if err != nil {
			return outcomes, err
		}
		result, err := executor.Run(ctx, request)
		if err != nil {
			return outcomes, fmt.Errorf("execute %s: %w", tool.Name, err)
		}
		outcomes = append(outcomes, executionOutcome{Request: request, Result: result})
	}
	return outcomes, ctx.Err()
}

func requestForTool(plan model.Plan, tool model.ToolPlan, nativeRequired bool) (runner.Request, error) {
	if len(tool.OutputPaths) == 0 {
		return runner.Request{}, errors.New("tool has no approved outputs")
	}
	directory := path.Dir(tool.OutputPaths[0])
	native := make([]runner.NativeOutput, 0, len(tool.OutputPaths)-2)
	for _, output := range tool.OutputPaths {
		name := path.Base(output)
		if path.Dir(output) != directory {
			return runner.Request{}, errors.New("approved outputs do not share an execution directory")
		}
		if name != "stdout.raw" && name != "stderr.raw" {
			native = append(native, runner.NativeOutput{Path: name, Required: nativeRequired})
		}
	}
	return runner.Request{
		ExecutionID: path.Base(directory), WorkspaceRoot: plan.WorkspaceRoot,
		OutputDir: filepath.Join(plan.WorkspaceRoot, filepath.FromSlash(directory)), Tool: tool,
		Environment: append([]string(nil), plan.Environment...), EnvironmentAllowlist: append([]string(nil), plan.EnvironmentAllowlist...),
		NativeOutputs: native, Limits: runnerLimits(tool),
	}, nil
}

func runnerLimits(tool model.ToolPlan) runner.Limits {
	return runner.Limits{
		Timeout: time.Duration(tool.Limits.ExecutionTimeoutSeconds) * time.Second, GracePeriod: 2 * time.Second,
		MaxStdoutBytes: 16 << 20, MaxStderrBytes: 16 << 20, MaxNativeBytes: 16 << 20,
		MaxRecords: 100_000, MaxLineBytes: adapter.MaxLineBytes,
	}
}

func runArjun(ctx context.Context, plan model.Plan, queue model.CandidateQueue, executor toolExecutor) ([]executionOutcome, error) {
	base := findTool(plan, "arjun")
	outcomes := make([]executionOutcome, 0, len(queue.Candidates))
	for index, item := range queue.Candidates {
		if err := ctx.Err(); err != nil {
			return outcomes, err
		}
		directory := filepath.Dir(item.NativeOutputPath)
		relative, err := workspaceRelative(plan.WorkspaceRoot, directory)
		if err != nil {
			return outcomes, err
		}
		tool := base
		tool.Argv = append([]string(nil), item.Argv...)
		tool.Limits = queue.Limits
		tool.OutputPaths = []string{path.Join(relative, "stdout.raw"), path.Join(relative, "stderr.raw"), path.Join(relative, filepath.Base(item.NativeOutputPath))}
		request, err := requestForTool(plan, tool, false)
		if err != nil {
			return outcomes, err
		}
		request.ExecutionID = fmt.Sprintf("tx_arjun_%02d", index+1)
		request.StdoutIsResult = true
		result, err := executor.Run(ctx, request)
		if err != nil {
			return outcomes, fmt.Errorf("execute Arjun candidate %d: %w", index+1, err)
		}
		outcomes = append(outcomes, executionOutcome{Request: request, Result: result})
	}
	return outcomes, ctx.Err()
}

func candidateMessages(decisions []candidate.Decision) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(decisions))
	for index, decision := range decisions {
		encoded, err := canonical.Marshal(decision)
		if err != nil {
			return nil, err
		}
		result[index] = json.RawMessage(encoded)
	}
	return result, nil
}

func persistCandidates(root *workspace.Root, runID string, result candidate.Result) error {
	queue, err := canonical.Marshal(result.Queue)
	if err != nil {
		return err
	}
	if err := root.WriteFileExclusive(path.Join("runs", runID, "candidate-queue.json"), append(queue, '\n')); err != nil {
		return err
	}
	var lines []byte
	for _, decision := range result.Decisions {
		encoded, err := canonical.Marshal(decision)
		if err != nil {
			return err
		}
		lines = append(lines, encoded...)
		lines = append(lines, '\n')
	}
	return root.WriteFileExclusive(path.Join("runs", runID, "arjun-candidates.jsonl"), lines)
}

func displayCandidateQueue(output io.Writer, result candidate.Result, wordlistSHA256 string) error {
	queueJSON, err := canonical.Marshal(result.Queue)
	if err != nil {
		return fmt.Errorf("encode candidate queue for approval: %w", err)
	}
	included := len(result.Queue.Candidates)
	if _, err := fmt.Fprintf(output, "candidate_queue: version=%s policy=%s plan_digest=%s\nincluded: %d\nmax_targets: %d\nqueue_limits: rate=%d concurrency=%d parallelism=%d request_timeout=%ds execution_timeout=%ds\nwordlist_sha256: %s\nrequest_budget_each: %d\nqueue_digest: %s\ncanonical_queue_json_ascii: %s\n",
		strconv.QuoteToASCII(result.Queue.QueueVersion), strconv.QuoteToASCII(result.Queue.PolicyVersion), strconv.QuoteToASCII(result.Queue.PlanDigest),
		included, result.Queue.MaxTargets, result.Queue.Limits.RatePerSecond, result.Queue.Limits.Concurrency,
		result.Queue.Limits.Parallelism, result.Queue.Limits.RequestTimeoutSeconds, result.Queue.Limits.ExecutionTimeoutSeconds, safeApprovalValue(wordlistSHA256),
		firstRequestBudget(result.Queue), result.QueueDigest, strconv.QuoteToASCII(string(queueJSON))); err != nil {
		return err
	}
	for _, item := range result.Queue.Candidates {
		if _, err := fmt.Fprintf(output, "candidate: id=%s endpoint=%s url=%s method=%s source_mode=%s location=%s rank=%d\nrank_inputs: katana=%t query_evidence=%t api_like=%t independent_executions=%d no_static_extension=%t supported_method_location=%t\nprovenance: observations=%s evidence=%s source_executions=%s reasons=%s\nwordlist: path=%s sha256=%s request_budget=%d\nnative_output_path: %s\nscope: classification=%s rule=%s reason=%s\nargv_exact: %s\n",
			strconv.QuoteToASCII(item.ID), strconv.QuoteToASCII(item.EndpointID), strconv.QuoteToASCII(item.URL),
			strconv.QuoteToASCII(item.Method), strconv.QuoteToASCII(item.SourceMode), strconv.QuoteToASCII(item.Location), item.RankPosition,
			item.Rank.CurrentlyObservedByKatana, item.Rank.ExistingQueryNameEvidence, item.Rank.APILikePath,
			item.Rank.IndependentExecutions, item.Rank.NoStaticExtension, item.Rank.SupportedMethodLocation,
			displaySafeArgv(item.ObservationIDs), displaySafeArgv(item.EvidenceIDs), displaySafeArgv(item.SourceExecutionIDs), displaySafeArgv(item.ReasonCodes),
			strconv.QuoteToASCII(item.WordlistPath), safeApprovalValue(item.WordlistSHA256), item.RequestBudget,
			strconv.QuoteToASCII(item.NativeOutputPath), strconv.QuoteToASCII(item.Scope.Classification),
			strconv.QuoteToASCII(item.Scope.RuleID), strconv.QuoteToASCII(item.Scope.Reason), displaySafeArgv(item.Argv)); err != nil {
			return err
		}
	}
	return nil
}

func safeApprovalValue(value string) string {
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return strconv.QuoteToASCII(value)
		}
	}
	return value
}

func firstRequestBudget(queue model.CandidateQueue) int {
	if len(queue.Candidates) == 0 {
		return 0
	}
	return queue.Candidates[0].RequestBudget
}

func displaySafeArgv(arguments []string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = strconv.QuoteToASCII(argument)
	}
	return strings.Join(quoted, " ")
}

type capturedArtifacts struct {
	sources    map[string]adapter.Source
	summaries  []model.ArtifactSummary
	rawSources map[string]string
}

func captureExecutionArtifacts(root *workspace.Root, outcome executionOutcome) (capturedArtifacts, error) {
	result := capturedArtifacts{sources: map[string]adapter.Source{}, rawSources: map[string]string{}}
	capturedPaths := make(map[string]bool, len(outcome.Result.Envelope.Artifacts))
	directory, err := workspaceRelative(outcome.Request.WorkspaceRoot, outcome.Request.OutputDir)
	if err != nil {
		return capturedArtifacts{}, err
	}
	for _, captured := range outcome.Result.Envelope.Artifacts {
		capturedPaths[captured.Path] = true
		privatePath := path.Join(directory, captured.Path)
		content, err := root.ReadFile(privatePath)
		if err != nil {
			return capturedArtifacts{}, err
		}
		digest := sha256.Sum256(content)
		digestHex := hex.EncodeToString(digest[:])
		if int64(len(content)) != captured.Size || captured.SHA256 != "sha256:"+digestHex {
			return capturedArtifacts{}, fmt.Errorf("captured artifact %s failed hash validation", privatePath)
		}
		if err := integrity.ScanSecrets(content); err != nil {
			return capturedArtifacts{}, fmt.Errorf("captured artifact %s cannot enter a public handoff: %w", privatePath, err)
		}
		privateValues := []string{outcome.Request.WorkspaceRoot, outcome.Request.OutputDir, outcome.Request.Tool.ResolvedPath}
		for _, argument := range outcome.Request.Tool.Argv {
			if filepath.IsAbs(argument) {
				privateValues = append(privateValues, argument)
			}
		}
		if err := integrity.ScanPrivatePaths(content, privateValues...); err != nil {
			return capturedArtifacts{}, fmt.Errorf("captured artifact %s cannot enter a public handoff: %w", privatePath, err)
		}
		publicPath := path.Join("raw", outcome.Request.ExecutionID, captured.Path)
		role, mediaType := artifactMetadata(outcome.Request.Tool.Name, captured.Role, captured.Path)
		artifact := model.Artifact{
			Role: role, Path: publicPath, SHA256: digestHex, SizeBytes: captured.Size,
			MediaType: mediaType, Sanitized: true,
		}
		result.sources[captured.Path] = adapter.Source{Reader: bytesReader(content), Artifact: artifact}
		sha, size := digestHex, captured.Size
		result.summaries = append(result.summaries, model.ArtifactSummary{
			Role: role, Path: publicPath, Present: true, SHA256: &sha, SizeBytes: &size, MediaType: mediaType,
		})
		result.rawSources[publicPath] = privatePath
	}
	for _, expected := range outcome.Request.NativeOutputs {
		if capturedPaths[expected.Path] {
			continue
		}
		role, mediaType := artifactMetadata(outcome.Request.Tool.Name, "native", expected.Path)
		result.summaries = append(result.summaries, model.ArtifactSummary{
			Role: role, Path: path.Join("raw", outcome.Request.ExecutionID, expected.Path), Present: false, MediaType: mediaType,
		})
	}
	return result, nil
}

func bytesReader(content []byte) io.Reader { return strings.NewReader(string(content)) }

func artifactMetadata(tool, runnerRole, name string) (string, string) {
	role, mediaType := runnerRole, "text/plain"
	if runnerRole == "native" {
		role = "native_output"
		switch tool {
		case "katana":
			mediaType = "application/x-ndjson"
		case "arjun":
			mediaType = "application/json"
		}
	}
	if name == "stdout.raw" {
		role = "stdout"
	} else if name == "stderr.raw" {
		role = "stderr"
	}
	return role, mediaType
}

func normalizeInitial(root *workspace.Root, artifact planArtifact, config scope.Config, evaluator *scope.Evaluator, approvalRecord model.ApprovalRecord, outcomes []executionOutcome) (workflowState, error) {
	workflow := workflowState{PlanDigest: artifact.PlanDigest, RawSources: map[string]string{}, Candidates: []json.RawMessage{}}
	for _, outcome := range outcomes {
		records, raw, err := normalizeOutcome(root, evaluator, outcome, nil)
		if err != nil {
			return workflowState{}, err
		}
		if err := workflow.Records.Merge(records); err != nil {
			return workflowState{}, err
		}
		if err := mergeRawSources(workflow.RawSources, raw); err != nil {
			return workflowState{}, err
		}
	}
	roots := make([]model.RunScopeRoot, len(config.Roots))
	for index, item := range config.Roots {
		roots[index] = model.RunScopeRoot{Kind: item.Kind, Value: item.Value}
	}
	runRecord := model.RunRecord{
		SchemaVersion: model.SchemaVersion, RecordType: "run", ID: artifact.Plan.RunID,
		CreatedAt: artifact.Plan.CreatedAt, Status: "running", CanonicalizationPolicy: canonical.URLPolicyVersion,
		Scope:            model.RunScope{Mode: config.Mode, Roots: roots, ExternalPolicy: config.ExternalPolicy, ApprovedBy: "operator", ApprovedAt: approvalRecord.CreatedAt},
		ToolExecutionIDs: []string{}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{},
	}
	for _, execution := range workflow.Records.ToolExecutions {
		runRecord.ToolExecutionIDs = append(runRecord.ToolExecutionIDs, execution.ID)
		runRecord.Warnings = append(runRecord.Warnings, execution.Warnings...)
		runRecord.Gaps = append(runRecord.Gaps, execution.Gaps...)
	}
	slices.Sort(runRecord.ToolExecutionIDs)
	workflow.Records.Runs = []model.RunRecord{runRecord}
	workflow.Records.Sort()
	return workflow, nil
}

func normalizeOutcome(root *workspace.Root, evaluator *scope.Evaluator, outcome executionOutcome, arjunCandidate *model.Candidate) (model.RecordSet, map[string]string, error) {
	captured, err := captureExecutionArtifacts(root, outcome)
	if err != nil {
		return model.RecordSet{}, nil, err
	}
	envelope, tool := outcome.Result.Envelope, outcome.Request.Tool
	exitCode := envelope.ExitCode
	adapterContext := adapter.Context{ToolExecutionID: outcome.Request.ExecutionID, Scope: evaluator}
	// Output paths are runs/<run-id>/executions/...; bind the adapter to the
	// approved run ID rather than trusting process output.
	components := strings.Split(tool.OutputPaths[0], "/")
	if len(components) < 2 || components[0] != "runs" || !validRunID(components[1]) {
		return model.RecordSet{}, nil, errors.New("approved output path does not identify a run")
	}
	adapterContext.RunID = components[1]
	var parsed adapter.Result
	switch tool.Name {
	case "gau":
		native, ok := captured.sources["native-output.txt"]
		if !ok {
			parsed = missingNativeResult("gau.missing_native", "GAU did not produce its required native output.")
			break
		}
		var stderr *adapter.Source
		if source, ok := captured.sources["stderr.raw"]; ok {
			copy := source
			stderr = &copy
		}
		parsed, err = adapter.ParseGAU(native, adapter.GAUOptions{
			Context: adapterContext, Format: "text", Providers: []string{"otx", "urlscan"}, ExitCode: &exitCode, Stderr: stderr,
			Incomplete: envelope.Status != runner.StatusSuccess || envelope.Reason != "" || envelope.Truncated,
		})
	case "katana":
		native, ok := captured.sources["native-output.jsonl"]
		if !ok {
			parsed = missingNativeResult("katana.missing_native", "Katana did not produce its required native output.")
			break
		}
		parsed, err = adapter.ParseKatana(native, adapter.KatanaOptions{Context: adapterContext, ExitCode: &exitCode, Interrupted: envelope.Status == runner.StatusInterrupted, TimedOut: envelope.TimedOut})
	case "arjun":
		if arjunCandidate == nil {
			return model.RecordSet{}, nil, errors.New("Arjun result has no approved candidate")
		}
		options := adapter.ArjunOptions{Context: adapterContext, TargetURL: arjunCandidate.URL, SourceMethod: arjunCandidate.SourceMode, ExitCode: &exitCode, Interrupted: envelope.Status == runner.StatusInterrupted, TimedOut: envelope.TimedOut}
		observedAt := envelope.FinishedAt
		options.ObservedAt = &observedAt
		if source, ok := captured.sources["native-output.json"]; ok {
			copy := source
			options.Native = &copy
		}
		if source, ok := captured.sources["stdout.raw"]; ok {
			copy := source
			options.Stdout = &copy
		}
		if source, ok := captured.sources["stderr.raw"]; ok {
			copy := source
			options.Stderr = &copy
		}
		parsed, err = adapter.ParseArjun(options)
	default:
		return model.RecordSet{}, nil, fmt.Errorf("unsupported planned tool %q", tool.Name)
	}
	if err != nil {
		return model.RecordSet{}, nil, err
	}
	applyRunnerSemantics(&parsed, envelope)
	duration := envelope.DurationMillis
	started, finished := envelope.StartedAt, envelope.FinishedAt
	execution := model.ToolExecution{
		SchemaVersion: model.SchemaVersion, RecordType: "tool_execution", ID: outcome.Request.ExecutionID, RunID: adapterContext.RunID,
		Tool: model.ToolIdentity{Name: tool.Name, Version: tool.Version, ResolvedPath: "<" + strings.ToUpper(tool.Name) + ">"}, AdapterVersion: adapterVersion(tool.Name),
		ActivityClass: tool.ActivityClass, ApprovalPhase: approvalPhase(tool.Name), ArgvRedacted: publicArgv(tool.Name, envelope.Argv),
		StartedAt: &started, FinishedAt: &finished, DurationMS: &duration, ExitCode: &exitCode,
		Status: parsed.Status, Coverage: parsed.Coverage, Artifacts: captured.summaries,
		ProviderStatus: parsed.ProviderStatus, Warnings: parsed.Warnings, Gaps: parsed.Gaps,
	}
	if err := parsed.Records.Merge(model.RecordSet{ToolExecutions: []model.ToolExecution{execution}}); err != nil {
		return model.RecordSet{}, nil, err
	}
	return parsed.Records, captured.rawSources, nil
}

func applyRunnerSemantics(parsed *adapter.Result, envelope runner.ArtifactEnvelope) {
	status := envelope.Status
	if status == runner.StatusSuccess && envelope.Reason != "" {
		status = runner.StatusFailed
		if envelope.Reason == "descendant_leak" {
			status = runner.StatusPartial
		}
	}
	if envelope.Truncated && status == runner.StatusSuccess {
		status = runner.StatusPartial
	}
	if status == runner.StatusSuccess {
		return
	}
	code, message := "runner.status_invalid", "The bounded runner returned an invalid status."
	if status == runner.StatusPartial || status == runner.StatusFailed || status == runner.StatusInterrupted {
		code, message = "runner."+status, "The bounded runner did not complete successfully."
	}
	if envelope.Reason != "" {
		code = "runner." + envelope.Reason
		message = "The bounded runner reported " + envelope.Reason + "."
	} else if envelope.Truncated {
		code, message = "runner.output_limit", "The bounded runner truncated at least one artifact."
	}
	parsed.Gaps = append(parsed.Gaps, model.Diagnostic{Code: code, Message: message, Severity: "error", EvidenceIDs: []string{}})
	if parsed.Status != "success" && parsed.Status != "success_zero" {
		return
	}
	switch status {
	case runner.StatusPartial:
		parsed.Status, parsed.Coverage = "partial", "partial"
	case runner.StatusInterrupted:
		parsed.Status, parsed.Coverage = "interrupted", "partial"
	default:
		parsed.Status, parsed.Coverage = "failed", "unknown"
	}
}

func missingNativeResult(code, message string) adapter.Result {
	return adapter.Result{Status: "failed", Coverage: "unknown", Records: model.RecordSet{}, ProviderStatus: []model.ProviderStatus{}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{{Code: code, Message: message, Severity: "error", EvidenceIDs: []string{}}}}
}

func adapterVersion(tool string) string {
	switch tool {
	case "gau":
		return adapter.GAUAdapterVersion
	case "katana":
		return adapter.KatanaAdapterVersion
	case "arjun":
		return adapter.ArjunAdapterVersion
	default:
		return "unknown-adapter/v0"
	}
}

func approvalPhase(tool string) string {
	if tool == "arjun" {
		return "parameter_discovery"
	}
	return "initial_recon"
}

func publicArgv(tool string, arguments []string) []string {
	result := append([]string(nil), arguments...)
	if len(result) > 0 {
		result[0] = "<" + strings.ToUpper(tool) + ">"
	}
	for index := 0; index+1 < len(result); index++ {
		if result[index] == "-w" {
			result[index+1] = "<WORDLIST>"
		} else if result[index] == "-oJ" {
			result[index+1] = "<NATIVE_OUTPUT>"
		}
	}
	return result
}

func mergeRawSources(destination, source map[string]string) error {
	for public, private := range source {
		if previous, exists := destination[public]; exists && previous != private {
			return fmt.Errorf("conflicting raw artifact mapping %s", public)
		}
		destination[public] = private
	}
	return nil
}

func normalizeArjun(root *workspace.Root, evaluator *scope.Evaluator, workflow *workflowState, outcomes []executionOutcome, queue model.CandidateQueue) error {
	if len(outcomes) != len(queue.Candidates) {
		return errors.New("Arjun outcomes do not match the approved candidate queue")
	}
	for index, outcome := range outcomes {
		records, raw, err := normalizeOutcome(root, evaluator, outcome, &queue.Candidates[index])
		if err != nil {
			return err
		}
		if err := workflow.Records.Merge(records); err != nil {
			return err
		}
		if err := mergeRawSources(workflow.RawSources, raw); err != nil {
			return err
		}
	}
	refreshRunSummary(&workflow.Records)
	return nil
}

func refreshRunSummary(records *model.RecordSet) {
	if len(records.Runs) != 1 {
		return
	}
	run := &records.Runs[0]
	run.ToolExecutionIDs = run.ToolExecutionIDs[:0]
	run.Warnings = run.Warnings[:0]
	run.Gaps = run.Gaps[:0]
	for _, execution := range records.ToolExecutions {
		run.ToolExecutionIDs = append(run.ToolExecutionIDs, execution.ID)
		run.Warnings = append(run.Warnings, execution.Warnings...)
		run.Gaps = append(run.Gaps, execution.Gaps...)
	}
	slices.Sort(run.ToolExecutionIDs)
}

func addRunGap(records *model.RecordSet, code, message string) {
	if len(records.Runs) != 1 {
		return
	}
	for _, gap := range records.Runs[0].Gaps {
		if gap.Code == code {
			return
		}
	}
	records.Runs[0].Gaps = append(records.Runs[0].Gaps, model.Diagnostic{Code: code, Message: message, Severity: "warning", EvidenceIDs: []string{}})
}

func finishAndCompile(root *workspace.Root, plan model.Plan, run *model.Run, workflow *workflowState, outputPrefix string) error {
	if len(workflow.Records.Runs) != 1 {
		return errors.New("normalized workflow has no unique run record")
	}
	if err := validatePersistedCandidateCheckpoint(root, plan, *run, *workflow); err != nil {
		return fmt.Errorf("validate candidate checkpoint: %w", err)
	}
	finished := time.Now().UTC().Format(time.RFC3339Nano)
	record := &workflow.Records.Runs[0]
	record.FinishedAt = &finished
	record.Status = "success"
	for _, execution := range workflow.Records.ToolExecutions {
		if execution.Status != "success" && execution.Status != "success_zero" {
			record.Status = "partial"
			break
		}
	}
	if len(workflow.Records.Observations) == 0 {
		record.Status = "partial"
		addRunGap(&workflow.Records, "run.no_valid_observations", "The completed tools produced no valid observations; the handoff contains no discovery facts.")
	}
	if slices.Contains(run.CoverageGaps, "arjun_skipped_by_operator") {
		record.Status = "partial"
	}
	workflow.GeneratedAt = finished
	workflow.Status = record.Status
	workflow.Records.Sort()
	if err := writeWorkflow(root, plan.RunID, *workflow); err != nil {
		return err
	}
	if run.State != model.RunCompiling {
		return fmt.Errorf("cannot compile run from %q", run.State)
	}
	if err := writeRunState(root, *run); err != nil {
		return err
	}
	if err := compileWorkflow(root, planArtifact{Plan: plan, PlanDigest: workflow.PlanDigest}, *workflow, outputPrefix); err != nil {
		run.State = model.RunFailed
		_ = writeRunState(root, *run)
		return err
	}
	completed, err := completedRunState(*run, *workflow)
	if err != nil {
		return err
	}
	*run = completed
	return writeRunState(root, *run)
}

func completedRunState(run model.Run, workflow workflowState) (model.Run, error) {
	completed, err := app.CompleteRun(run)
	if err != nil {
		return run, err
	}
	switch workflow.Status {
	case "success":
	case "partial":
		completed.State = model.RunPartial
	case "failed":
		completed.State = model.RunFailed
	default:
		return run, fmt.Errorf("invalid completed workflow status %q", workflow.Status)
	}
	for _, execution := range workflow.Records.ToolExecutions {
		if execution.Status != "success" && execution.Status != "success_zero" {
			completed.State = model.RunPartial
			break
		}
	}
	if len(workflow.Records.Observations) == 0 {
		completed.State = model.RunPartial
	}
	return completed, nil
}

func compileInterrupted(root *workspace.Root, plan model.Plan, run *model.Run, workflow *workflowState, outputPrefix string) error {
	if len(workflow.Records.Runs) != 1 {
		return errors.New("interrupted workflow has no unique run record")
	}
	if err := validatePersistedCandidateCheckpoint(root, plan, *run, *workflow); err != nil {
		return fmt.Errorf("validate candidate checkpoint: %w", err)
	}
	finished := time.Now().UTC().Format(time.RFC3339Nano)
	workflow.Records.Runs[0].FinishedAt = &finished
	workflow.Records.Runs[0].Status = "partial"
	workflow.GeneratedAt = finished
	workflow.Status = "partial"
	workflow.Records.Sort()
	run.State = model.RunInterrupted
	if err := writeWorkflow(root, plan.RunID, *workflow); err != nil {
		return err
	}
	if err := writeRunState(root, *run); err != nil {
		return err
	}
	return compileWorkflow(root, planArtifact{Plan: plan, PlanDigest: workflow.PlanDigest}, *workflow, outputPrefix)
}
