package cli

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/integrity"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/workspace"
)

const workflowVersion = "reconctx-workflow/v0"

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type planArtifact struct {
	Plan       model.Plan `json:"plan"`
	PlanDigest string     `json:"plan_digest"`
}

type workflowState struct {
	WorkflowVersion string                `json:"workflow_version"`
	PlanDigest      string                `json:"plan_digest"`
	Records         model.RecordSet       `json:"records"`
	Queue           *model.CandidateQueue `json:"queue,omitempty"`
	Candidates      []json.RawMessage     `json:"candidate_decisions"`
	RawSources      map[string]string     `json:"raw_sources"`
	GeneratedAt     string                `json:"generated_at,omitempty"`
	Status          string                `json:"status,omitempty"`
}

func loadPlanArtifact(name string) (planArtifact, error) {
	raw, err := readRegularFile(name, 16<<20)
	if err != nil {
		return planArtifact{}, err
	}
	return decodePlanArtifact(raw)
}

func loadWorkspacePlan(root *workspace.Root, runID string) (planArtifact, error) {
	raw, err := root.ReadFile(path.Join("runs", runID, "plan.json"))
	if err != nil {
		return planArtifact{}, err
	}
	return decodePlanArtifact(raw)
}

func decodePlanArtifact(raw []byte) (planArtifact, error) {
	var artifact planArtifact
	if err := decodeCanonical(raw, &artifact); err != nil {
		return planArtifact{}, fmt.Errorf("decode plan artifact: %w", err)
	}
	if err := validatePlanArtifact(artifact); err != nil {
		return planArtifact{}, err
	}
	return artifact, nil
}

func validatePlanArtifact(artifact planArtifact) error {
	plan := artifact.Plan
	if plan.PlanVersion != "reconctx-plan/v0" || plan.SchemaVersion != model.SchemaVersion || plan.CanonicalizationPolicy != canonical.URLPolicyVersion {
		return errors.New("unsupported plan contract")
	}
	if !validRunID(plan.RunID) || !filepath.IsAbs(plan.WorkspaceRoot) || filepath.Clean(plan.WorkspaceRoot) != plan.WorkspaceRoot {
		return errors.New("invalid plan workspace or run ID")
	}
	if _, err := time.Parse(time.RFC3339Nano, plan.CreatedAt); err != nil {
		return errors.New("invalid plan creation time")
	}
	if integrity.ValidateRelativePath(plan.Inputs.ScopePath) != nil || !digestPattern.MatchString(plan.Inputs.ScopeSHA256) || !filepath.IsAbs(plan.Inputs.WordlistPath) || !digestPattern.MatchString(plan.Inputs.WordlistSHA256) || plan.Limits.ArjunMaxTargets <= 0 || plan.Limits.ArjunRequestBudget <= 0 {
		return errors.New("invalid plan inputs or limits")
	}
	if len(plan.Inputs.Seeds) == 0 || len(plan.Tools) < 3 {
		return errors.New("plan has no seeds or complete tool set")
	}
	seenTools := map[string]int{}
	for _, tool := range plan.Tools {
		seenTools[tool.Name]++
		if tool.ResolvedPath == "" || len(tool.Argv) == 0 || tool.Argv[0] != tool.ResolvedPath || tool.Version == "" || tool.Binary.SHA256 == "" || len(tool.OutputPaths) < 3 {
			return fmt.Errorf("invalid %s tool plan", tool.Name)
		}
		for _, output := range tool.OutputPaths {
			if integrity.ValidateRelativePath(output) != nil {
				return fmt.Errorf("invalid approved output path %q", output)
			}
		}
	}
	if seenTools["gau"] != 1 || seenTools["katana"] != len(plan.Inputs.Seeds) || seenTools["arjun"] != 1 || len(seenTools) != 3 {
		return errors.New("plan tool topology is invalid")
	}
	digest, err := approval.PlanDigest(plan)
	if err != nil {
		return err
	}
	if !digestPattern.MatchString(artifact.PlanDigest) || subtle.ConstantTimeCompare([]byte(digest), []byte(artifact.PlanDigest)) != 1 {
		return errors.New("plan digest does not match current behavior")
	}
	return nil
}

func validRunID(value string) bool {
	return strings.HasPrefix(value, "run_") && len(value) <= 132 && !strings.ContainsAny(value, "/\\\x00\r\n")
}

func decodeCanonical(raw []byte, destination any) error {
	canonicalJSON, err := canonical.Canonicalize(raw)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonicalJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func readRunState(root *workspace.Root, runID string) (model.Run, error) {
	raw, err := root.ReadFile(path.Join("state", runID+".json"))
	if err != nil {
		return model.Run{}, err
	}
	var run model.Run
	if err := decodeCanonical(raw, &run); err != nil {
		return model.Run{}, err
	}
	if run.ID != runID {
		return model.Run{}, errors.New("run state belongs to another run")
	}
	return run, nil
}

func writeRunState(root *workspace.Root, run model.Run) error {
	encoded, err := canonical.Marshal(run)
	if err != nil {
		return err
	}
	return root.ReplaceFile(path.Join("state", run.ID+".json"), append(encoded, '\n'))
}

func readWorkflow(root *workspace.Root, runID, planDigest string) (workflowState, error) {
	raw, err := root.ReadFile(path.Join("state", runID+"-workflow.json"))
	if err != nil {
		return workflowState{}, err
	}
	var state workflowState
	if err := decodeCanonical(raw, &state); err != nil {
		return workflowState{}, err
	}
	if state.WorkflowVersion != workflowVersion || state.PlanDigest != planDigest {
		return workflowState{}, errors.New("workflow metadata does not match the approved plan")
	}
	if state.RawSources == nil {
		state.RawSources = map[string]string{}
	}
	return state, nil
}

func writeWorkflow(root *workspace.Root, runID string, state workflowState) error {
	state.WorkflowVersion = workflowVersion
	if state.RawSources == nil {
		state.RawSources = map[string]string{}
	}
	encoded, err := canonical.Marshal(state)
	if err != nil {
		return err
	}
	return root.ReplaceFile(path.Join("state", runID+"-workflow.json"), append(encoded, '\n'))
}
