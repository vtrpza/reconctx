package approval

import (
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/model"
)

func TestPlanDigestIsDeterministicAndIgnoresDisplay(t *testing.T) {
	t.Parallel()

	first := testPlan()
	second := testPlan()
	second.Display = model.PlanDisplay{Title: "colored title", TerminalStyle: "ansi"}
	second.RunID = "run_other"
	second.CreatedAt = "2026-07-13T13:00:00-03:00"

	firstDigest, err := PlanDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := PlanDigest(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("equivalent plans differ: %s != %s", firstDigest, secondDigest)
	}
}

func TestPlanDigestChangesWithBehavior(t *testing.T) {
	t.Parallel()

	base := testPlan()
	baseDigest, err := PlanDigest(base)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*model.Plan){
		"plan version":     func(p *model.Plan) { p.PlanVersion += ".changed" },
		"target":           func(p *model.Plan) { p.Inputs.Target += ".changed" },
		"seeds":            func(p *model.Plan) { p.Inputs.Seeds = append(p.Inputs.Seeds, "https://other.test/") },
		"scope path":       func(p *model.Plan) { p.Inputs.ScopePath = "other-scope.json" },
		"scope digest":     func(p *model.Plan) { p.Inputs.ScopeSHA256 = "sha256:" + strings.Repeat("b", 64) },
		"profile":          func(p *model.Plan) { p.Inputs.Profile += ".changed" },
		"canonical policy": func(p *model.Plan) { p.CanonicalizationPolicy += ".changed" },
		"schema version":   func(p *model.Plan) { p.SchemaVersion += ".changed" },
		"enabled tools":    func(p *model.Plan) { p.Tools = append(p.Tools, p.Tools[0]) },
		"tool name":        func(p *model.Plan) { p.Tools[0].Name += ".changed" },
		"tool path": func(p *model.Plan) {
			p.Tools[0].ResolvedPath = "/other/gau"
			p.Tools[0].Argv[0] = "/other/gau"
		},
		"tool version":          func(p *model.Plan) { p.Tools[0].Version = "2.2.5" },
		"activity class":        func(p *model.Plan) { p.Tools[0].ActivityClass += ".changed" },
		"argv":                  func(p *model.Plan) { p.Tools[0].Argv = append(p.Tools[0].Argv, "--subs") },
		"rate limit":            func(p *model.Plan) { p.Tools[0].Limits.RatePerSecond++ },
		"concurrency":           func(p *model.Plan) { p.Tools[0].Limits.Concurrency++ },
		"parallelism":           func(p *model.Plan) { p.Tools[0].Limits.Parallelism++ },
		"timeout":               func(p *model.Plan) { p.Tools[0].Limits.TimeoutSeconds++ },
		"global limit":          func(p *model.Plan) { p.Limits.ArjunMaxTargets++ },
		"environment allowlist": func(p *model.Plan) { p.EnvironmentAllowlist = append(p.EnvironmentAllowlist, "HTTP_PROXY") },
		"output path":           func(p *model.Plan) { p.Tools[0].OutputPaths[0] = "runs/other/stdout.raw" },
		"workspace":             func(p *model.Plan) { p.WorkspaceRoot = "/other/work" },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			changed := testPlan()
			mutate(&changed)
			got, err := PlanDigest(changed)
			if err != nil {
				if name == "plan version" || name == "canonical policy" || name == "schema version" {
					return
				}
				t.Fatal(err)
			}
			if got == baseDigest {
				t.Fatalf("digest did not change after %s mutation", name)
			}
		})
	}
}

func TestPlanDigestRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()
	first, second := testPlan(), testPlan()
	first.WorkspaceRoot = "/work/" + string([]byte{0xff})
	second.WorkspaceRoot = "/work/" + string([]byte{0xfe})
	if _, err := PlanDigest(first); err == nil {
		t.Fatal("PlanDigest accepted invalid UTF-8")
	}
	if _, err := PlanDigest(second); err == nil {
		t.Fatal("PlanDigest accepted distinct invalid UTF-8")
	}
}

func TestPlanDigestValidatesBehaviorSemantics(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*model.Plan){
		"empty plan version":       func(p *model.Plan) { p.PlanVersion = "" },
		"unknown plan version":     func(p *model.Plan) { p.PlanVersion = "other/v0" },
		"empty canonical policy":   func(p *model.Plan) { p.CanonicalizationPolicy = "" },
		"unknown canonical policy": func(p *model.Plan) { p.CanonicalizationPolicy = "other/v0" },
		"empty schema version":     func(p *model.Plan) { p.SchemaVersion = "" },
		"unknown schema version":   func(p *model.Plan) { p.SchemaVersion = "other/v0" },
		"empty tool version":       func(p *model.Plan) { p.Tools[0].Version = "" },
		"negative global limit":    func(p *model.Plan) { p.Limits.ArjunMaxTargets = -1 },
		"negative rate":            func(p *model.Plan) { p.Tools[0].Limits.RatePerSecond = -1 },
		"negative concurrency":     func(p *model.Plan) { p.Tools[0].Limits.Concurrency = -1 },
		"negative parallelism":     func(p *model.Plan) { p.Tools[0].Limits.Parallelism = -1 },
		"negative timeout":         func(p *model.Plan) { p.Tools[0].Limits.TimeoutSeconds = -1 },
		"relative tool path":       func(p *model.Plan) { p.Tools[0].ResolvedPath = "tools/gau" },
		"relative workspace":       func(p *model.Plan) { p.WorkspaceRoot = "work" },
		"absolute output":          func(p *model.Plan) { p.Tools[0].OutputPaths[0] = "/tmp/output" },
		"traversing output":        func(p *model.Plan) { p.Tools[0].OutputPaths[0] = "runs/../outside" },
		"leading traversal output": func(p *model.Plan) { p.Tools[0].OutputPaths[0] = "../outside" },
		"dot output":               func(p *model.Plan) { p.Tools[0].OutputPaths[0] = "." },
		"backslash output":         func(p *model.Plan) { p.Tools[0].OutputPaths[0] = `runs\\outside` },
		"empty target":             func(p *model.Plan) { p.Inputs.Target = "" },
		"empty seeds":              func(p *model.Plan) { p.Inputs.Seeds = nil },
		"invalid seed":             func(p *model.Plan) { p.Inputs.Seeds[0] = "not-a-url" },
		"empty scope path":         func(p *model.Plan) { p.Inputs.ScopePath = "" },
		"invalid scope digest":     func(p *model.Plan) { p.Inputs.ScopeSHA256 = "sha256:bad" },
		"empty profile":            func(p *model.Plan) { p.Inputs.Profile = "" },
		"no tools":                 func(p *model.Plan) { p.Tools = nil },
		"empty tool name":          func(p *model.Plan) { p.Tools[0].Name = "" },
		"empty activity class":     func(p *model.Plan) { p.Tools[0].ActivityClass = "" },
		"empty argv":               func(p *model.Plan) { p.Tools[0].Argv = nil },
		"argv path mismatch":       func(p *model.Plan) { p.Tools[0].Argv[0] = "/other/gau" },
		"zero timeout":             func(p *model.Plan) { p.Tools[0].Limits.TimeoutSeconds = 0 },
		"empty outputs":            func(p *model.Plan) { p.Tools[0].OutputPaths = nil },
		"empty environment key":    func(p *model.Plan) { p.EnvironmentAllowlist = append(p.EnvironmentAllowlist, "") },
		"zero rate":                func(p *model.Plan) { p.Tools[0].Limits.RatePerSecond = 0 },
		"zero concurrency":         func(p *model.Plan) { p.Tools[0].Limits.Concurrency = 0 },
		"zero parallelism":         func(p *model.Plan) { p.Tools[0].Limits.Parallelism = 0 },
		"NUL tool path": func(p *model.Plan) {
			p.Tools[0].ResolvedPath = "/tools/\x00gau"
			p.Tools[0].Argv[0] = p.Tools[0].ResolvedPath
		},
		"NUL argv": func(p *model.Plan) { p.Tools[0].Argv = append(p.Tools[0].Argv, "bad\x00arg") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			plan := testPlan()
			mutate(&plan)
			if _, err := PlanDigest(plan); err == nil {
				t.Fatal("PlanDigest accepted invalid plan")
			}
		})
	}

	plan := testPlan()
	plan.Tools[0].Version = strings.Repeat("v", 1)
	if _, err := PlanDigest(plan); err != nil {
		t.Fatalf("valid non-empty tool version rejected: %v", err)
	}
}

func TestVerifyRejectsApprovalDigestMismatch(t *testing.T) {
	t.Parallel()

	plan := testPlan()
	record := model.ApprovalRecord{
		Phase:          "initial_recon",
		ApprovedDigest: "sha256:stale",
		OperatorLabel:  "operator",
		Decision:       "approve",
		CreatedAt:      "2026-07-13T12:50:05-03:00",
	}
	if err := Verify(plan, record); err == nil {
		t.Fatal("Verify accepted a stale approval digest")
	}
}

func testPlan() model.Plan {
	return model.Plan{
		PlanVersion:            "reconctx-plan/v0",
		RunID:                  "run_test",
		CreatedAt:              "2026-07-13T12:50:05-03:00",
		CanonicalizationPolicy: "url-canonicalization/v0",
		SchemaVersion:          "reconctx/v0",
		Inputs: model.PlanInputs{
			Target:      "fixture.test",
			Seeds:       []string{"https://fixture.test/"},
			ScopePath:   "scope.json",
			ScopeSHA256: "sha256:" + strings.Repeat("a", 64),
			Profile:     "web-blackbox",
		},
		Tools: []model.ToolPlan{{
			Name:          "gau",
			ResolvedPath:  "/tools/gau",
			Version:       "2.2.4",
			ActivityClass: "passive_external",
			Argv:          []string{"/tools/gau", "fixture.test"},
			Limits:        model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, TimeoutSeconds: 45},
			OutputPaths:   []string{"runs/run_test/stdout.raw"},
		}},
		Limits:               model.PlanLimits{ArjunMaxTargets: 25},
		EnvironmentAllowlist: []string{"LANG", "TZ"},
		WorkspaceRoot:        "/work",
	}
}
