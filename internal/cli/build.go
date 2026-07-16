package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/compiler"
	"github.com/vtrpza/reconctx/internal/integrity"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/workspace"
)

func runBuild(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	flags.SetOutput(stderr)
	runID := flags.String("run", "", "run ID")
	workspacePath := flags.String("workspace", "", "private workspace")
	outputPath := flags.String("out", "", "handoff path inside workspace")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || !validRunID(*runID) || !filepath.IsAbs(*workspacePath) || strings.ContainsAny(*workspacePath, "\x00\r\n") {
		fmt.Fprintln(stderr, "reconctx build: --run RUN_ID and --workspace ABSOLUTE_PRIVATE_DIR are required")
		return 2
	}
	workspaceName := filepath.Clean(*workspacePath)
	outputPrefix, err := buildOutputPrefix(workspaceName, *outputPath, *runID)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx build: %v\n", err)
		return 2
	}
	root, err := workspace.Open(workspaceName)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx build: %v\n", err)
		return 1
	}
	defer root.Close()
	plan, err := loadWorkspacePlan(root, *runID)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx build: load plan: %v\n", err)
		return 1
	}
	if plan.Plan.RunID != *runID || plan.Plan.WorkspaceRoot != workspaceName {
		fmt.Fprintln(stderr, "reconctx build: plan belongs to another run or workspace")
		return 1
	}
	run, err := readRunState(root, *runID)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx build: load run state: %v\n", err)
		return 1
	}
	workflow, err := readWorkflow(root, *runID, plan.PlanDigest)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx build: load workflow: %v\n", err)
		return 1
	}
	if err := validateBuildWorkflow(run, plan, workflow); err != nil {
		fmt.Fprintf(stderr, "reconctx build: %v\n", err)
		return 1
	}
	if err := validatePersistedCandidateCheckpoint(root, plan.Plan, run, workflow); err != nil {
		fmt.Fprintf(stderr, "reconctx build: validate candidate checkpoint: %v\n", err)
		return 1
	}
	if err := compileWorkflow(root, plan, workflow, outputPrefix); err != nil {
		fmt.Fprintf(stderr, "reconctx build: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "handoff: %s\n", filepath.Join(workspaceName, filepath.FromSlash(outputPrefix)))
	return 0
}

func validatePersistedCandidateCheckpoint(root *workspace.Root, plan model.Plan, run model.Run, workflow workflowState) error {
	if workflow.Queue == nil {
		return nil
	}
	_, err := loadCandidateCheckpoint(root, plan, run, workflow)
	return err
}

func buildOutputPrefix(workspaceName, outputPath, runID string) (string, error) {
	if outputPath == "" {
		return path.Join("handoff", runID), nil
	}
	if filepath.IsAbs(outputPath) {
		return workspaceRelative(workspaceName, outputPath)
	}
	outputPath = filepath.ToSlash(outputPath)
	if err := integrity.ValidateRelativePath(outputPath); err != nil {
		return "", fmt.Errorf("output path must remain inside workspace: %w", err)
	}
	return outputPath, nil
}

func validateBuildWorkflow(run model.Run, plan planArtifact, workflow workflowState) error {
	if run.PlanDigest == "" || run.PlanDigest != plan.PlanDigest {
		return errors.New("run state does not match the immutable plan")
	}
	switch run.State {
	case model.RunCompiling, model.RunSuccess, model.RunPartial, model.RunFailed, model.RunInterrupted:
	default:
		return fmt.Errorf("run state %q has no compilable workflow", run.State)
	}
	if workflow.GeneratedAt == "" || workflow.Status == "" || len(workflow.Records.Runs) != 1 {
		return errors.New("workflow metadata is incomplete")
	}
	if workflow.Records.Runs[0].ID != plan.Plan.RunID || workflow.Records.Runs[0].Status != workflow.Status {
		return errors.New("workflow run record is inconsistent")
	}
	if workflow.Queue != nil && workflow.Candidates == nil {
		return errors.New("workflow candidate metadata is incomplete")
	}
	if workflow.Queue == nil {
		if run.QueueDigest != "" || len(workflow.Candidates) != 0 {
			return errors.New("workflow queue metadata is incomplete")
		}
		return nil
	}
	if workflow.Queue.PlanDigest != plan.PlanDigest || run.QueueDigest == "" {
		return errors.New("workflow queue does not match the immutable plan")
	}
	digest, err := approval.QueueDigest(*workflow.Queue)
	if err != nil {
		return fmt.Errorf("validate workflow queue: %w", err)
	}
	if digest != run.QueueDigest {
		return errors.New("workflow queue drifted from the approved run state")
	}
	return nil
}

func compileWorkflow(root *workspace.Root, plan planArtifact, workflow workflowState, outputPrefix string) error {
	if root == nil {
		return errors.New("workspace is required")
	}
	rawFiles, rawPolicy, err := workflowRawFiles(root, plan, workflow)
	if err != nil {
		return err
	}
	bundle, err := compiler.Compile(compiler.Input{
		RunID:       plan.Plan.RunID,
		GeneratedAt: workflow.GeneratedAt,
		Status:      workflow.Status,
		RawPolicy:   rawPolicy,
		Records:     workflow.Records,
		Candidates:  workflow.Candidates,
		RawFiles:    rawFiles,
	})
	if err != nil {
		return fmt.Errorf("compile handoff: %w", err)
	}
	if err := compiler.Write(root, outputPrefix, bundle); err != nil {
		if errors.Is(err, workspace.ErrFinalized) || errors.Is(err, os.ErrExist) {
			return fmt.Errorf("handoff destination is finalized: %w", err)
		}
		return fmt.Errorf("write handoff: %w", err)
	}
	return nil
}

func validateWorkflowIntegrity(root *workspace.Root, plan planArtifact, workflow workflowState) error {
	rawFiles, rawPolicy, err := workflowRawFiles(root, plan, workflow)
	if err != nil {
		return err
	}
	if err := compiler.ValidateRecords(plan.Plan.RunID, workflow.Records, rawFiles, rawPolicy); err != nil {
		return fmt.Errorf("validate persisted evidence: %w", err)
	}
	return nil
}

func workflowRawFiles(root *workspace.Root, plan planArtifact, workflow workflowState) (map[string][]byte, string, error) {
	names := make([]string, 0, len(workflow.RawSources))
	for name := range workflow.RawSources {
		names = append(names, name)
	}
	sort.Strings(names)
	rawFiles := make(map[string][]byte, len(names))
	for _, name := range names {
		source := workflow.RawSources[name]
		if integrity.ValidateRelativePath(name) != nil || !strings.HasPrefix(name, "raw/") || integrity.ValidateRelativePath(source) != nil {
			return nil, "", fmt.Errorf("unsafe raw source mapping %q", name)
		}
		content, err := root.ReadFile(source)
		if err != nil {
			return nil, "", fmt.Errorf("read raw source %s: %w", source, err)
		}
		privateValues := []string{plan.Plan.WorkspaceRoot, plan.Plan.Inputs.WordlistPath}
		for _, tool := range plan.Plan.Tools {
			privateValues = append(privateValues, tool.ResolvedPath)
		}
		if err := integrity.ScanPrivatePaths(content, privateValues...); err != nil {
			return nil, "", fmt.Errorf("raw source %s contains private path data: %w", source, err)
		}
		rawFiles[name] = content
	}
	rawPolicy := "omitted"
	if len(rawFiles) != 0 || workflowRequiresEmbeddedRaw(workflow.Records) {
		rawPolicy = "embedded_sanitized"
	}
	return rawFiles, rawPolicy, nil
}

func workflowRequiresEmbeddedRaw(records model.RecordSet) bool {
	for _, execution := range records.ToolExecutions {
		for _, artifact := range execution.Artifacts {
			if artifact.Present {
				return true
			}
		}
	}
	for _, evidence := range records.Evidence {
		if evidence.RedactionStatus != "withheld" {
			return true
		}
	}
	return false
}
