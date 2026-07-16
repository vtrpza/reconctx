package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/model"
)

func TestPlanPreflightsAndRendersImmutableArtifact(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	source, err := filepath.Abs(filepath.Join("..", "..", "integration", "faketools", "gau"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "gau")
	if err := os.WriteFile(binary, content, 0o700); err != nil {
		t.Fatal(err)
	}

	plan := model.Plan{
		PlanVersion:            "reconctx-plan/v0",
		RunID:                  "run_test",
		CreatedAt:              "2026-07-13T12:50:05-03:00",
		CanonicalizationPolicy: "url-canonicalization/v0",
		SchemaVersion:          "reconctx/v0",
		Inputs: model.PlanInputs{
			Target:         "fixture.test",
			Seeds:          []string{"https://fixture.test/"},
			ScopePath:      "scope.yaml",
			ScopeSHA256:    "sha256:" + strings.Repeat("a", 64),
			Profile:        "web-blackbox",
			WordlistPath:   "/wordlists/params.txt",
			WordlistSHA256: "sha256:" + strings.Repeat("b", 64),
		},
		Tools: []model.ToolPlan{{
			Name:          "gau",
			ActivityClass: "passive_external",
			Argv:          []string{"gau", "seed with space"},
			Limits:        model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, TimeoutSeconds: 45},
			OutputPaths:   []string{"runs/run_test/executions/gau/stdout.raw"},
		}},
		Limits:               model.PlanLimits{ArjunMaxTargets: 25, ArjunRequestBudget: 100},
		EnvironmentAllowlist: []string{"LANG", "PATH"},
		WorkspaceRoot:        "/private/work",
	}

	rendered, err := BuildPlan(context.Background(), plan, map[string]string{"gau": binary}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	tool := rendered.Plan.Tools[0]
	if tool.ResolvedPath != binary || tool.Version != "2.2.4" || tool.Argv[0] != binary {
		t.Fatalf("preflighted tool = %#v", tool)
	}
	if tool.Binary.SHA256 == "" || tool.Binary.Inode == 0 {
		t.Fatalf("binary identity missing: %#v", tool.Binary)
	}
	if !strings.HasPrefix(rendered.PlanDigest, "sha256:") || !strings.Contains(rendered.ArtifactJSON, rendered.PlanDigest) {
		t.Fatalf("rendered digest/artifact = %q / %q", rendered.PlanDigest, rendered.ArtifactJSON)
	}
	if !strings.Contains(rendered.Display, "'seed with space'") {
		t.Fatalf("argv display was not shell-escaped: %q", rendered.Display)
	}
	for _, field := range []string{
		"scope: path=scope.yaml sha256=", "profile: web-blackbox", "wordlist: path=/wordlists/params.txt sha256=",
		"policies: canonicalization=url-canonicalization/v0 schema=reconctx/v0", "global_limits: arjun_max_targets=25 arjun_request_budget=100",
		"binary: sha256=sha256:", " mode=0700 uid=", " gid=", " device=", " inode=", "environment_allowlist: LANG,PATH", "environment: PATH=",
	} {
		if !strings.Contains(rendered.Display, field) {
			t.Errorf("approval display missing %q:\n%s", field, rendered.Display)
		}
	}
	if len(rendered.Plan.Environment) == 0 {
		t.Fatal("effective environment was not captured in the plan")
	}

	plan.Tools[0].Argv[1] = "mutated"
	if rendered.Plan.Tools[0].Argv[1] != "seed with space" {
		t.Fatal("rendered plan aliases mutable input")
	}
	second, err := BuildPlan(context.Background(), rendered.Plan, map[string]string{"gau": binary}, os.Environ())
	if err != nil || second.ArtifactJSON != rendered.ArtifactJSON || second.Display != rendered.Display {
		t.Fatalf("plan render is not deterministic: %v", err)
	}
	if _, err := BuildPlan(context.Background(), plan, nil, os.Environ()); err == nil {
		t.Fatal("BuildPlan accepted a missing tool")
	}
}

func TestPlanDisplayEscapesControlCharacters(t *testing.T) {
	if got := quoteDisplayArgument("safe\nforged: value"); strings.ContainsRune(got, '\n') {
		t.Fatalf("argument display contains a raw line break: %q", got)
	}
	if got := safeDisplay("target\x1b[2J"); strings.ContainsRune(got, '\x1b') {
		t.Fatalf("value display contains a raw escape: %q", got)
	}
	if got := safeDisplay("safe\u202eforged"); strings.ContainsRune(got, '\u202e') {
		t.Fatalf("value display contains a bidi format control: %q", got)
	}
	if got := quoteDisplayArgument("safe\u2066forged"); strings.ContainsRune(got, '\u2066') {
		t.Fatalf("argument display contains a bidi format control: %q", got)
	}
}
