package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/preflight"
)

type RenderedPlan struct {
	Plan         model.Plan
	PlanDigest   string
	ArtifactJSON string
	Display      string
}

func BuildPlan(ctx context.Context, input model.Plan, candidates map[string]string, environment []string) (RenderedPlan, error) {
	plan := clonePlan(input)
	effectiveEnvironment, err := preflight.CaptureEnvironment(environment, plan.EnvironmentAllowlist)
	if err != nil {
		return RenderedPlan{}, fmt.Errorf("environment: %w", err)
	}
	plan.Environment = effectiveEnvironment
	for index := range plan.Tools {
		tool := &plan.Tools[index]
		candidate, ok := candidates[tool.Name]
		if !ok {
			return RenderedPlan{}, fmt.Errorf("tool %q is missing", tool.Name)
		}
		result, err := preflight.InspectTool(ctx, tool.Name, candidate, plan.Environment, plan.EnvironmentAllowlist)
		if err != nil {
			return RenderedPlan{}, fmt.Errorf("preflight %s: %w", tool.Name, err)
		}
		if len(tool.Argv) == 0 {
			return RenderedPlan{}, fmt.Errorf("tool %q argv is empty", tool.Name)
		}
		tool.ResolvedPath = result.Identity.ResolvedPath
		tool.Version = result.Version
		tool.Argv[0] = result.Identity.ResolvedPath
		tool.Binary = model.ToolBinary{
			SHA256: result.Identity.SHA256,
			Mode:   uint32(result.Identity.Mode.Perm()),
			UID:    result.Identity.UID,
			GID:    result.Identity.GID,
			Device: result.Identity.Device,
			Inode:  result.Identity.Inode,
		}
	}
	digest, err := approval.PlanDigest(plan)
	if err != nil {
		return RenderedPlan{}, err
	}
	artifact, err := canonical.Marshal(struct {
		Plan       model.Plan `json:"plan"`
		PlanDigest string     `json:"plan_digest"`
	}{Plan: plan, PlanDigest: digest})
	if err != nil {
		return RenderedPlan{}, err
	}
	return RenderedPlan{
		Plan:         plan,
		PlanDigest:   digest,
		ArtifactJSON: string(artifact),
		Display:      renderPlan(plan, digest),
	}, nil
}

// DisplayPlan renders an already validated immutable plan for an interactive
// approval prompt without probing tools or changing behavior-bearing data.
func DisplayPlan(plan model.Plan, digest string) string {
	return renderPlan(plan, digest)
}

func clonePlan(plan model.Plan) model.Plan {
	plan.Inputs.Seeds = append([]string(nil), plan.Inputs.Seeds...)
	plan.EnvironmentAllowlist = append([]string(nil), plan.EnvironmentAllowlist...)
	plan.Environment = append([]string(nil), plan.Environment...)
	plan.Tools = append([]model.ToolPlan(nil), plan.Tools...)
	for index := range plan.Tools {
		plan.Tools[index].Argv = append([]string(nil), plan.Tools[index].Argv...)
		plan.Tools[index].OutputPaths = append([]string(nil), plan.Tools[index].OutputPaths...)
	}
	return plan
}

func renderPlan(plan model.Plan, digest string) string {
	var output strings.Builder
	fmt.Fprintf(&output, "plan: %s\ntarget: %s\n", safeDisplay(plan.PlanVersion), safeDisplay(plan.Inputs.Target))
	for _, seed := range plan.Inputs.Seeds {
		fmt.Fprintf(&output, "seed: %s\n", safeDisplay(seed))
	}
	fmt.Fprintf(&output, "scope: path=%s sha256=%s\nprofile: %s\nwordlist: path=%s sha256=%s\npolicies: canonicalization=%s schema=%s\nglobal_limits: arjun_max_targets=%d arjun_request_budget=%d\n",
		safeDisplay(plan.Inputs.ScopePath), safeDisplay(plan.Inputs.ScopeSHA256), safeDisplay(plan.Inputs.Profile),
		safeDisplay(plan.Inputs.WordlistPath), safeDisplay(plan.Inputs.WordlistSHA256), safeDisplay(plan.CanonicalizationPolicy), safeDisplay(plan.SchemaVersion),
		plan.Limits.ArjunMaxTargets, plan.Limits.ArjunRequestBudget)
	for _, tool := range plan.Tools {
		fmt.Fprintf(&output, "tool: %s %s %s\npath: %s\nbinary: sha256=%s mode=%#o uid=%d gid=%d device=%d inode=%d\nargv: %s\nlimits: rate=%d concurrency=%d parallelism=%d request_timeout=%ds execution_timeout=%ds\noutputs: %s\n",
			safeDisplay(tool.Name), safeDisplay(tool.Version), safeDisplay(tool.ActivityClass), safeDisplay(tool.ResolvedPath),
			safeDisplay(tool.Binary.SHA256), tool.Binary.Mode, tool.Binary.UID, tool.Binary.GID, tool.Binary.Device, tool.Binary.Inode, displayArgv(tool.Argv),
			tool.Limits.RatePerSecond, tool.Limits.Concurrency, tool.Limits.Parallelism, tool.Limits.RequestTimeoutSeconds, tool.Limits.ExecutionTimeoutSeconds,
			safeDisplay(strings.Join(tool.OutputPaths, ",")))
	}
	fmt.Fprintf(&output, "workspace: %s\nenvironment_allowlist: %s\n", safeDisplay(plan.WorkspaceRoot), safeDisplay(strings.Join(plan.EnvironmentAllowlist, ",")))
	for _, value := range plan.Environment {
		fmt.Fprintf(&output, "environment: %s\n", safeDisplay(value))
	}
	fmt.Fprintf(&output, "plan_digest: %s\n", digest)
	return output.String()
}

func displayArgv(arguments []string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = quoteDisplayArgument(argument)
	}
	return strings.Join(quoted, " ")
}

func quoteDisplayArgument(argument string) string {
	if argument == "" {
		return "''"
	}
	for _, character := range argument {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return strconv.QuoteToASCII(argument)
		}
		if strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", character) {
			continue
		}
		return "'" + strings.ReplaceAll(argument, "'", "'\\''") + "'"
	}
	return argument
}

func safeDisplay(value string) string {
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return strconv.QuoteToASCII(value)
		}
	}
	return value
}
