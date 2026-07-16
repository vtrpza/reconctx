package approval

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/preflight"
)

func PlanDigest(plan model.Plan) (string, error) {
	if err := validatePlan(plan); err != nil {
		return "", err
	}
	behavior := struct {
		PlanVersion            string           `json:"plan_version"`
		Inputs                 model.PlanInputs `json:"inputs"`
		CanonicalizationPolicy string           `json:"canonicalization_policy"`
		SchemaVersion          string           `json:"schema_version"`
		Tools                  []model.ToolPlan `json:"tools"`
		Limits                 model.PlanLimits `json:"limits"`
		EnvironmentAllowlist   []string         `json:"environment_allowlist"`
		Environment            []string         `json:"environment"`
		WorkspaceRoot          string           `json:"workspace_root"`
	}{
		PlanVersion:            plan.PlanVersion,
		Inputs:                 plan.Inputs,
		CanonicalizationPolicy: plan.CanonicalizationPolicy,
		SchemaVersion:          plan.SchemaVersion,
		Tools:                  plan.Tools,
		Limits:                 plan.Limits,
		EnvironmentAllowlist:   plan.EnvironmentAllowlist,
		Environment:            plan.Environment,
		WorkspaceRoot:          plan.WorkspaceRoot,
	}
	encoded, err := canonical.Marshal(behavior)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func validatePlan(plan model.Plan) error {
	if plan.PlanVersion != "reconctx-plan/v0" {
		return fmt.Errorf("unsupported plan version %q", plan.PlanVersion)
	}
	if plan.CanonicalizationPolicy != canonical.URLPolicyVersion {
		return fmt.Errorf("unsupported canonicalization policy %q", plan.CanonicalizationPolicy)
	}
	if plan.SchemaVersion != "reconctx/v0" {
		return fmt.Errorf("unsupported schema version %q", plan.SchemaVersion)
	}
	if !filepath.IsAbs(plan.WorkspaceRoot) || filepath.Clean(plan.WorkspaceRoot) != plan.WorkspaceRoot || strings.ContainsRune(plan.WorkspaceRoot, '\x00') {
		return errors.New("workspace root must be an absolute clean path")
	}
	if strings.TrimSpace(plan.Inputs.Target) == "" || strings.ContainsAny(plan.Inputs.Target, "/\\?#@:") || len(plan.Inputs.Seeds) == 0 || plan.Inputs.ScopePath == "" || plan.Inputs.Profile == "" {
		return errors.New("plan inputs are incomplete")
	}
	if !safeOutputPath(plan.Inputs.ScopePath) || !validDigest(plan.Inputs.ScopeSHA256) || !filepath.IsAbs(plan.Inputs.WordlistPath) || filepath.Clean(plan.Inputs.WordlistPath) != plan.Inputs.WordlistPath || strings.ContainsRune(plan.Inputs.WordlistPath, '\x00') || !validDigest(plan.Inputs.WordlistSHA256) {
		return errors.New("scope or wordlist path/digest is invalid")
	}
	target, err := canonical.CanonicalizeURL("https://" + plan.Inputs.Target + "/")
	if err != nil || target.Host != plan.Inputs.Target {
		return errors.New("target is not a canonical host name")
	}
	targetSeedFound := false
	for index, seed := range plan.Inputs.Seeds {
		value, err := canonical.CanonicalizeURL(seed)
		if err != nil {
			return fmt.Errorf("seed %d: %w", index+1, err)
		}
		targetSeedFound = targetSeedFound || value.Host == target.Host
	}
	if !targetSeedFound {
		return errors.New("target host does not match an approved seed host")
	}
	if plan.Limits.ArjunMaxTargets < 0 || plan.Limits.ArjunRequestBudget <= 0 {
		return errors.New("global limits are invalid")
	}
	if len(plan.Tools) == 0 {
		return errors.New("plan requires at least one tool")
	}
	for index, tool := range plan.Tools {
		if tool.Name == "" || tool.Version == "" || tool.ActivityClass == "" {
			return fmt.Errorf("tool %d identity is incomplete", index+1)
		}
		if !validDigest(tool.Binary.SHA256) || tool.Binary.Mode == 0 || tool.Binary.Mode&^0o777 != 0 || tool.Binary.Mode&0o111 == 0 || tool.Binary.Mode&0o022 != 0 || tool.Binary.Device == 0 || tool.Binary.Inode == 0 {
			return fmt.Errorf("tool %d binary identity is invalid", index+1)
		}
		if !filepath.IsAbs(tool.ResolvedPath) || filepath.Clean(tool.ResolvedPath) != tool.ResolvedPath || strings.ContainsRune(tool.ResolvedPath, '\x00') {
			return fmt.Errorf("tool %d path must be absolute and clean", index+1)
		}
		if tool.Limits.RatePerSecond <= 0 || tool.Limits.Concurrency <= 0 || tool.Limits.Parallelism <= 0 || tool.Limits.TimeoutSeconds <= 0 {
			return fmt.Errorf("tool %d limits are invalid", index+1)
		}
		if len(tool.Argv) == 0 || tool.Argv[0] != tool.ResolvedPath {
			return fmt.Errorf("tool %d argv does not start with resolved path", index+1)
		}
		for _, argument := range tool.Argv {
			if strings.ContainsRune(argument, '\x00') {
				return fmt.Errorf("tool %d argv contains NUL", index+1)
			}
		}
		if len(tool.OutputPaths) == 0 {
			return fmt.Errorf("tool %d requires output paths", index+1)
		}
		for _, output := range tool.OutputPaths {
			if !safeOutputPath(output) {
				return fmt.Errorf("tool %d has unsafe output path %q", index+1, output)
			}
		}
	}
	effectiveEnvironment, err := preflight.CaptureEnvironment(plan.Environment, plan.EnvironmentAllowlist)
	if err != nil || !slices.Equal(effectiveEnvironment, plan.Environment) {
		return errors.New("environment is not the canonical allowlisted snapshot")
	}
	return nil
}

func safeOutputPath(output string) bool {
	if output == "" || path.IsAbs(output) || path.Clean(output) != output || strings.ContainsAny(output, "\\\x00") {
		return false
	}
	for _, component := range strings.Split(output, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

// Verify fails before any later execution layer can consume an approval that
// is not an explicit approval for the current behavior digest.
func Verify(plan model.Plan, record model.ApprovalRecord) error {
	if record.Decision != "approve" {
		return fmt.Errorf("approval decision is %q, not approve", record.Decision)
	}
	if record.OperatorLabel == "" || record.Phase == "" || record.CreatedAt == "" {
		return errors.New("approval record is incomplete")
	}
	digest, err := PlanDigest(plan)
	if err != nil {
		return err
	}
	if !validDigest(record.ApprovedDigest) || subtle.ConstantTimeCompare([]byte(digest), []byte(record.ApprovedDigest)) != 1 {
		return errors.New("approval digest does not match current plan")
	}
	return nil
}

func validDigest(value string) bool {
	hexDigest, ok := strings.CutPrefix(value, "sha256:")
	if !ok || len(hexDigest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(hexDigest)
	return err == nil && hexDigest == strings.ToLower(hexDigest)
}
