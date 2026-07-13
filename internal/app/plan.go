package app

import (
	"context"
	"fmt"
	"strings"

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
	for index := range plan.Tools {
		tool := &plan.Tools[index]
		candidate, ok := candidates[tool.Name]
		if !ok {
			return RenderedPlan{}, fmt.Errorf("tool %q is missing", tool.Name)
		}
		result, err := preflight.InspectTool(ctx, tool.Name, candidate, environment, plan.EnvironmentAllowlist)
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

func clonePlan(plan model.Plan) model.Plan {
	plan.Inputs.Seeds = append([]string(nil), plan.Inputs.Seeds...)
	plan.EnvironmentAllowlist = append([]string(nil), plan.EnvironmentAllowlist...)
	plan.Tools = append([]model.ToolPlan(nil), plan.Tools...)
	for index := range plan.Tools {
		plan.Tools[index].Argv = append([]string(nil), plan.Tools[index].Argv...)
		plan.Tools[index].OutputPaths = append([]string(nil), plan.Tools[index].OutputPaths...)
	}
	return plan
}

func renderPlan(plan model.Plan, digest string) string {
	var output strings.Builder
	fmt.Fprintf(&output, "plan: %s\ntarget: %s\n", plan.PlanVersion, plan.Inputs.Target)
	for _, seed := range plan.Inputs.Seeds {
		fmt.Fprintf(&output, "seed: %s\n", seed)
	}
	for _, tool := range plan.Tools {
		fmt.Fprintf(&output, "tool: %s %s %s\npath: %s\nargv: %s\nlimits: rate=%d concurrency=%d parallelism=%d timeout=%ds\noutputs: %s\n",
			tool.Name, tool.Version, tool.ActivityClass, tool.ResolvedPath, displayArgv(tool.Argv),
			tool.Limits.RatePerSecond, tool.Limits.Concurrency, tool.Limits.Parallelism, tool.Limits.TimeoutSeconds,
			strings.Join(tool.OutputPaths, ","))
	}
	fmt.Fprintf(&output, "workspace: %s\nenvironment: %s\nplan_digest: %s\n", plan.WorkspaceRoot, strings.Join(plan.EnvironmentAllowlist, ","), digest)
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
		if strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", character) {
			continue
		}
		return "'" + strings.ReplaceAll(argument, "'", "'\\''") + "'"
	}
	return argument
}
