package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"

	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/preflight"
	"github.com/vtrpza/reconctx/internal/scope"
)

func AwaitCollectionApproval(run model.Run, plan model.Plan) (model.Run, error) {
	if run.State != model.RunPlanned || run.ID == "" || run.ID != plan.RunID {
		return run, errors.New("run cannot await collection approval")
	}
	digest, err := approval.PlanDigest(plan)
	if err != nil {
		return run, err
	}
	run.PlanDigest = digest
	run.State = model.RunAwaitingCollectionApproval
	return run, nil
}

func StartCollection(run model.Run, current model.Plan, record model.ApprovalRecord) (model.Run, error) {
	if run.State != model.RunAwaitingCollectionApproval {
		return run, errors.New("run is not awaiting collection approval")
	}
	if err := validateCurrentPlan(run, current); err != nil {
		return run, err
	}
	if err := approval.VerifyDecision(record, approval.CollectionPhase, run.PlanDigest, "approve"); err != nil {
		return run, err
	}
	var err error
	run.Approvals, err = approval.AppendRecord(run.Approvals, record)
	if err != nil {
		return run, err
	}
	run.State = model.RunCollecting
	return run, nil
}

func FinishCollection(run model.Run) (model.Run, error) {
	if run.State != model.RunCollecting {
		return run, errors.New("run is not collecting")
	}
	run.State = model.RunNormalizingInitial
	return run, nil
}

func AwaitArjunApproval(run model.Run, plan model.Plan, scopeDocument []byte, queue model.CandidateQueue) (model.Run, error) {
	if run.State != model.RunNormalizingInitial && run.State != model.RunAwaitingArjunApproval {
		return run, errors.New("run cannot await Arjun approval")
	}
	digest, err := validateArjunContext(run, plan, scopeDocument, queue)
	if err != nil {
		return run, err
	}
	run.QueueDigest = digest
	run.State = model.RunAwaitingArjunApproval
	return run, nil
}

func StartArjun(run model.Run, plan model.Plan, scopeDocument []byte, queue model.CandidateQueue, record model.ApprovalRecord) (model.Run, error) {
	if err := verifyArjunTransition(run, plan, scopeDocument, queue, record, "approve"); err != nil {
		return run, err
	}
	var err error
	run.Approvals, err = approval.AppendRecord(run.Approvals, record)
	if err != nil {
		return run, err
	}
	run.State = model.RunDiscoveringParameters
	return run, nil
}

func FinishArjun(run model.Run) (model.Run, error) {
	if run.State != model.RunDiscoveringParameters {
		return run, errors.New("run is not discovering parameters")
	}
	run.State = model.RunNormalizingFinal
	return run, nil
}

func FinishFinalNormalization(run model.Run) (model.Run, error) {
	if run.State != model.RunNormalizingFinal {
		return run, errors.New("run is not normalizing final artifacts")
	}
	run.State = model.RunCompiling
	return run, nil
}

func SkipArjun(run model.Run, plan model.Plan, scopeDocument []byte, queue model.CandidateQueue, record model.ApprovalRecord) (model.Run, error) {
	if err := verifyArjunTransition(run, plan, scopeDocument, queue, record, "skip"); err != nil {
		return run, err
	}
	var err error
	run.Approvals, err = approval.AppendRecord(run.Approvals, record)
	if err != nil {
		return run, err
	}
	run.CoverageGaps = append(append([]string(nil), run.CoverageGaps...), "arjun_skipped_by_operator")
	run.State = model.RunArjunSkipped
	return run, nil
}

func CompileSkippedArjun(run model.Run) (model.Run, error) {
	if run.State != model.RunArjunSkipped {
		return run, errors.New("run did not skip Arjun")
	}
	run.State = model.RunCompiling
	return run, nil
}

func CancelRun(run model.Run, record model.ApprovalRecord) (model.Run, error) {
	phase, digest := "", ""
	switch run.State {
	case model.RunAwaitingCollectionApproval:
		phase, digest = approval.CollectionPhase, run.PlanDigest
	case model.RunAwaitingArjunApproval:
		phase, digest = approval.ArjunPhase, run.QueueDigest
	default:
		return run, errors.New("run cannot be cancelled from current state")
	}
	if err := approval.VerifyDecision(record, phase, digest, "cancel"); err != nil {
		return run, err
	}
	var err error
	run.Approvals, err = approval.AppendRecord(run.Approvals, record)
	if err != nil {
		return run, err
	}
	run.State = model.RunCancelled
	return run, nil
}

func CompleteRun(run model.Run) (model.Run, error) {
	if run.State != model.RunCompiling {
		return run, fmt.Errorf("cannot complete run from %q", run.State)
	}
	run.State = model.RunSuccess
	return run, nil
}

func CanSchedule(run model.Run) bool {
	return run.State == model.RunCollecting || run.State == model.RunDiscoveringParameters
}

func validateCurrentPlan(run model.Run, plan model.Plan) error {
	if plan.RunID != run.ID {
		return errors.New("current plan belongs to another run")
	}
	digest, err := approval.PlanDigest(plan)
	if err != nil {
		return err
	}
	if digest != run.PlanDigest {
		return errors.New("plan drift invalidated approval")
	}
	for _, tool := range plan.Tools {
		result, err := preflight.InspectTool(context.Background(), tool.Name, tool.ResolvedPath, plan.Environment, plan.EnvironmentAllowlist)
		if err != nil {
			return fmt.Errorf("revalidate tool %s: %w", tool.Name, err)
		}
		identity := result.Identity
		if result.Version != tool.Version {
			return fmt.Errorf("tool %s version metadata changed after approval", tool.Name)
		}
		if identity.ResolvedPath != tool.ResolvedPath || identity.SHA256 != tool.Binary.SHA256 || uint32(identity.Mode) != tool.Binary.Mode || identity.UID != tool.Binary.UID || identity.GID != tool.Binary.GID || identity.Device != tool.Binary.Device || identity.Inode != tool.Binary.Inode {
			return fmt.Errorf("tool %s identity changed after approval", tool.Name)
		}
	}
	return nil
}

func verifyArjunTransition(run model.Run, plan model.Plan, scopeDocument []byte, queue model.CandidateQueue, record model.ApprovalRecord, decision string) error {
	if run.State != model.RunAwaitingArjunApproval {
		return errors.New("run is not awaiting Arjun approval")
	}
	digest, err := validateArjunContext(run, plan, scopeDocument, queue)
	if err != nil {
		return err
	}
	if digest != run.QueueDigest {
		return errors.New("candidate queue drift invalidated approval")
	}
	return approval.VerifyDecision(record, approval.ArjunPhase, digest, decision)
}

func validateArjunContext(run model.Run, plan model.Plan, scopeDocument []byte, queue model.CandidateQueue) (string, error) {
	if err := validateCurrentPlan(run, plan); err != nil {
		return "", err
	}
	if queue.PolicyVersion != "arjun-candidate-policy/v0" || queue.PlanDigest != run.PlanDigest || queue.MaxTargets > plan.Limits.ArjunMaxTargets || len(queue.Candidates) > plan.Limits.ArjunMaxTargets {
		return "", errors.New("candidate queue exceeds or differs from approved plan")
	}
	digest, err := approval.QueueDigest(queue)
	if err != nil {
		return "", err
	}
	scopeHash := sha256.Sum256(scopeDocument)
	if "sha256:"+hex.EncodeToString(scopeHash[:]) != plan.Inputs.ScopeSHA256 {
		return "", errors.New("scope document does not match approved plan")
	}
	config, err := scope.LoadYAML(bytes.NewReader(scopeDocument))
	if err != nil {
		return "", err
	}
	evaluator, err := scope.NewEvaluator(config)
	if err != nil {
		return "", err
	}
	arjunPath := ""
	var arjunLimits model.ToolLimits
	for _, tool := range plan.Tools {
		if tool.Name == "arjun" {
			arjunPath = tool.ResolvedPath
			arjunLimits = tool.Limits
			break
		}
	}
	if arjunPath != "" && (queue.Limits.RatePerSecond > arjunLimits.RatePerSecond || queue.Limits.Concurrency > arjunLimits.Concurrency || queue.Limits.Parallelism > arjunLimits.Parallelism || queue.Limits.TimeoutSeconds > arjunLimits.TimeoutSeconds) {
		return "", errors.New("candidate queue exceeds approved Arjun limits")
	}
	for index, candidate := range queue.Candidates {
		if candidate.ID == "" || candidate.EndpointID == "" || len(candidate.ObservationIDs) == 0 || len(candidate.EvidenceIDs) == 0 || len(candidate.SourceExecutionIDs) == 0 || !slices.Equal(candidate.ReasonCodes, []string{"selected"}) || candidate.RankPosition <= 0 || !candidate.Rank.SupportedMethodLocation {
			return "", fmt.Errorf("candidate %d policy metadata is incomplete", index+1)
		}
		if arjunPath == "" || !validArjunCommand(candidate, arjunPath, queue.Limits) {
			return "", fmt.Errorf("candidate %d command is not bound to approved Arjun", index+1)
		}
		if candidate.WordlistPath != plan.Inputs.WordlistPath || candidate.WordlistSHA256 != plan.Inputs.WordlistSHA256 || candidate.RequestBudget <= 0 || candidate.RequestBudget > plan.Limits.ArjunRequestBudget {
			return "", fmt.Errorf("candidate %d wordlist or request budget exceeds the approved plan", index+1)
		}
		if err := verifyWordlist(candidate); err != nil {
			return "", fmt.Errorf("candidate %d: %w", index+1, err)
		}
		actual := evaluator.EvaluateURL(candidate.URL)
		ruleID := ""
		if actual.RuleID != nil {
			ruleID = *actual.RuleID
		}
		if !actual.AllowedForActive() || candidate.Scope.Classification != string(actual.Classification) || candidate.Scope.RuleID != ruleID || candidate.Scope.Reason != actual.Reason {
			return "", fmt.Errorf("candidate %d scope decision is not approved", index+1)
		}
	}
	return digest, nil
}

func validArjunCommand(candidate model.Candidate, arjunPath string, limits model.ToolLimits) bool {
	switch candidate.Location {
	case "query":
		if candidate.Method != "GET" || candidate.SourceMode != "GET" {
			return false
		}
	case "form":
		if candidate.Method != "POST" || candidate.SourceMode != "POST" {
			return false
		}
	case "json":
		if candidate.Method != "POST" || candidate.SourceMode != "JSON" {
			return false
		}
	default:
		return false
	}
	expected := []string{
		arjunPath, "-u", candidate.URL, "-m", candidate.SourceMode, "-w", candidate.WordlistPath,
		"--rate-limit", strconv.Itoa(limits.RatePerSecond), "-t", strconv.Itoa(limits.Concurrency), "-T", strconv.Itoa(limits.TimeoutSeconds),
	}
	if candidate.Location == "form" {
		expected = append(expected, "--headers", "Content-Type: application/x-www-form-urlencoded")
	} else if candidate.Location == "json" {
		expected = append(expected, "--headers", "Content-Type: application/json")
	}
	if !filepath.IsAbs(candidate.NativeOutputPath) {
		return false
	}
	expected = append(expected, "-oJ", candidate.NativeOutputPath)
	return slices.Equal(candidate.Argv, expected)
}

func verifyWordlist(candidate model.Candidate) error {
	fd, err := syscall.Open(candidate.WordlistPath, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open wordlist: %w", err)
	}
	file := os.NewFile(uintptr(fd), candidate.WordlistPath)
	if file == nil {
		syscall.Close(fd)
		return errors.New("open wordlist: invalid file descriptor")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("wordlist is not a regular file")
	}
	const maxWordlistBytes = 64 << 20
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maxWordlistBytes+1))
	if err != nil {
		return fmt.Errorf("hash wordlist: %w", err)
	}
	if written > maxWordlistBytes || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != candidate.WordlistSHA256 {
		return errors.New("wordlist does not match approved hash")
	}
	return nil
}
