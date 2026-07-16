package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/scope"
)

func TestKatanaScopePatternDoesNotWidenURLPrefix(t *testing.T) {
	config := scope.Config{Mode: "allowlist", ExternalPolicy: "reject", Roots: []scope.Root{{ID: "allowed", Kind: "url_prefix", Value: "https://fixture.test/allowed"}}}
	evaluator, err := scope.NewEvaluator(config)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := canonical.CanonicalizeURL("https://fixture.test/allowed/start")
	if err != nil {
		t.Fatal(err)
	}
	pattern, err := katanaScopePattern(seed, config, evaluator.EvaluateURL(seed.CanonicalObservationURL))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatal(err)
	}
	for _, allowed := range []string{"https://fixture.test/allowed", "https://fixture.test/allowed/child", "https://fixture.test/allowed?q=1"} {
		if !compiled.MatchString(allowed) {
			t.Errorf("scope pattern %q rejected %q", pattern, allowed)
		}
	}
	for _, rejected := range []string{"https://fixture.test/admin", "https://fixture.test/allowedness", "https://fixture.test/allowed/%2Fadmin"} {
		if compiled.MatchString(rejected) {
			t.Errorf("scope pattern %q widened to %q", pattern, rejected)
		}
	}
}

func TestPlanWritesPreflightedArtifactWithoutActiveExecution(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	if err := os.Chmod(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	scopePath := filepath.Join(t.TempDir(), "scope.yaml")
	scopeDocument := []byte("mode: allowlist\nroots:\n  - id: loopback_prefix\n    kind: url_prefix\n    value: http://127.0.0.1:18080/allowed\nexternal_policy: reject\n")
	if err := os.WriteFile(scopePath, scopeDocument, 0o600); err != nil {
		t.Fatal(err)
	}
	wordlistPath := filepath.Join(t.TempDir(), "params.txt")
	wordlistDocument := []byte("id\nquery\n")
	if err := os.WriteFile(wordlistPath, wordlistDocument, 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	toolParent := t.TempDir()
	if err := os.Chmod(toolParent, 0o700); err != nil {
		t.Fatal(err)
	}
	toolDirectory := filepath.Join(toolParent, "tools")
	if err := os.Mkdir(toolDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := func(name string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(repository, "integration", "faketools", name))
		if err != nil {
			t.Fatal(err)
		}
		name = filepath.Join(toolDirectory, name)
		if err := os.WriteFile(name, content, 0o700); err != nil {
			t.Fatal(err)
		}
		return name
	}
	gauPath, katanaPath, arjunPath := tool("gau"), tool("katana"), tool("arjun")
	args := []string{
		"plan", "--target", "127.0.0.1", "--seed", "http://127.0.0.1:18080/allowed/start",
		"--scope", scopePath, "--wordlist", wordlistPath, "--workspace", workspace, "--out", "approved-plan.json",
		"--gau-path", gauPath, "--katana-path", katanaPath, "--arjun-path", arjunPath,
	}
	var stdout, stderr bytes.Buffer
	if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "plan_digest: sha256:") || !strings.Contains(stdout.String(), "tool: gau 2.2.4 passive_external") {
		t.Fatalf("plan display missing preflight evidence:\n%s", stdout.String())
	}
	encoded, err := os.ReadFile(filepath.Join(workspace, "approved-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		Plan       model.Plan `json:"plan"`
		PlanDigest string     `json:"plan_digest"`
	}
	if err := json.Unmarshal(encoded, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Plan.RunID == "" || artifact.PlanDigest == "" || len(artifact.Plan.Tools) != 3 || artifact.Plan.Limits.ArjunRequestBudget != 2 {
		t.Fatalf("incomplete plan artifact: %+v", artifact)
	}
	toolHome := filepath.Join(workspace, "runs", artifact.Plan.RunID, "home")
	if !slices.Contains(artifact.Plan.Environment, "HOME="+toolHome) {
		t.Fatalf("plan environment does not bind private HOME %q: %#v", toolHome, artifact.Plan.Environment)
	}
	if info, err := os.Stat(toolHome); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("private tool HOME: info=%v err=%v", info, err)
	}
	if entries, err := os.ReadDir(toolHome); err != nil || len(entries) != 0 {
		t.Fatalf("private tool HOME is not empty: entries=%v err=%v", entries, err)
	}
	wordlistCopy := filepath.Join(workspace, "runs", artifact.Plan.RunID, "inputs", "wordlist.txt")
	if artifact.Plan.Inputs.WordlistPath != wordlistCopy || artifact.Plan.Inputs.WordlistPath == wordlistPath {
		t.Fatalf("plan wordlist path = %q, want private copy %q", artifact.Plan.Inputs.WordlistPath, wordlistCopy)
	}
	assertFrozenWordlist := func(stage string) {
		t.Helper()
		content, err := readRegularFile(artifact.Plan.Inputs.WordlistPath, maxWordlistDocument)
		if err != nil || !bytes.Equal(content, wordlistDocument) {
			t.Fatalf("%s: private wordlist = %q, err=%v", stage, content, err)
		}
	}
	assertFrozenWordlist("after planning")
	if err := os.WriteFile(wordlistPath, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertFrozenWordlist("after source mutation")
	if err := os.Remove(wordlistPath); err != nil {
		t.Fatal(err)
	}
	assertFrozenWordlist("after source deletion")
	gau := findTool(artifact.Plan, "gau")
	configIndex := slices.Index(gau.Argv, "--config")
	if configIndex < 0 || configIndex+1 >= len(gau.Argv) || gau.Argv[configIndex+1] != gauConfigIsolationPath || filepath.IsAbs(gau.Argv[configIndex+1]) {
		t.Fatalf("GAU config is not isolated from ambient HOME: %#v", gau.Argv)
	}
	katana := findTool(artifact.Plan, "katana")
	scopeIndex := slices.Index(katana.Argv, "-cs")
	if scopeIndex < 0 || scopeIndex+1 >= len(katana.Argv) {
		t.Fatalf("Katana argv has no scope regex: %#v", katana.Argv)
	}
	compiledScope, err := regexp.Compile(katana.Argv[scopeIndex+1])
	if err != nil || !compiledScope.MatchString("http://127.0.0.1:18080/allowed/child") || compiledScope.MatchString("http://127.0.0.1:18080/admin") {
		t.Fatalf("Katana URL-prefix scope = %q, err=%v", katana.Argv[scopeIndex+1], err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "runs", artifact.Plan.RunID, "executions")); !os.IsNotExist(err) {
		t.Fatalf("plan created an execution directory: %v", err)
	}
	state, err := os.ReadFile(filepath.Join(workspace, "state", artifact.Plan.RunID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(state, []byte(`"state":"planned"`)) {
		t.Fatalf("unexpected run state: %s", state)
	}
}

func TestPlanRejectsTargetOutsideApprovedSeedHostBeforePreflight(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	scopePath := filepath.Join(directory, "scope.yaml")
	wordlistPath := filepath.Join(directory, "params.txt")
	if err := os.WriteFile(scopePath, []byte("mode: allowlist\nroots:\n  - id: loopback\n    kind: origin\n    value: http://127.0.0.1:18080\nexternal_policy: reject\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wordlistPath, []byte("id\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(directory, "workspace")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"plan", "--target", "outside.test", "--seed", "http://127.0.0.1:18080/", "--scope", scopePath, "--wordlist", wordlistPath, "--workspace", workspace}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "target host must match") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("out-of-scope target created workspace state: %v", err)
	}
}

func TestPlanRejectsRelativeWorkspaceBeforeReadingInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"plan", "--target", "fixture.test", "--seed", "https://fixture.test/",
		"--scope", "missing-scope.yaml", "--wordlist", "missing-wordlist.txt", "--workspace", "relative-workspace",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "absolute clean path") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
}
