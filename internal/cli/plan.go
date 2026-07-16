package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vtrpza/reconctx/internal/app"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/scope"
	"github.com/vtrpza/reconctx/internal/workspace"
	"github.com/vtrpza/reconctx/profiles"
)

const maxInputDocument = 1 << 20
const maxWordlistDocument = workspace.MaxFileBytes
const gauConfigIsolationPath = ".reconctx-gau-config-absent"

func runPlan(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var seeds stringList
	target := flags.String("target", "", "root target domain")
	flags.Var(&seeds, "seed", "in-scope seed URL (repeatable)")
	scopePath := flags.String("scope", "", "scope YAML path")
	profileName := flags.String("profile", "web-blackbox", "built-in profile")
	workspacePath := flags.String("workspace", "", "private workspace")
	outputPath := flags.String("out", "", "plan artifact path inside workspace")
	wordlistPath := flags.String("wordlist", "", "Arjun wordlist path")
	gauPath := flags.String("gau-path", "gau", "GAU executable")
	katanaPath := flags.String("katana-path", "katana", "Katana executable")
	arjunPath := flags.String("arjun-path", "arjun", "Arjun executable")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*target) == "" || len(seeds) == 0 || *scopePath == "" || *wordlistPath == "" || *workspacePath == "" || strings.ContainsAny(*target, "\x00\r\n") {
		fmt.Fprintln(stderr, "reconctx plan: --target, --seed, --scope, --wordlist, and --workspace are required")
		return 2
	}
	if !filepath.IsAbs(*workspacePath) || filepath.Clean(*workspacePath) != *workspacePath || strings.ContainsRune(*workspacePath, '\x00') {
		fmt.Fprintln(stderr, "reconctx plan: --workspace must be an absolute clean path")
		return 2
	}
	profile, err := profiles.Load(*profileName)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 2
	}
	scopeDocument, err := readRegularFile(*scopePath, maxInputDocument)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: read scope: %v\n", err)
		return 1
	}
	scopeConfig, err := scope.LoadYAML(bytes.NewReader(scopeDocument))
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 2
	}
	evaluator, err := scope.NewEvaluator(scopeConfig)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 2
	}
	targetName := strings.TrimSpace(*target)
	if strings.ContainsAny(targetName, "/\\?#@:") {
		fmt.Fprintln(stderr, "reconctx plan: target must be a single host name")
		return 2
	}
	targetURL, err := canonical.CanonicalizeURL("https://" + targetName + "/")
	if err != nil {
		fmt.Fprintln(stderr, "reconctx plan: target must be a canonical host name")
		return 2
	}
	canonicalSeeds := make([]string, len(seeds))
	katanaScopePatterns := make([]string, len(seeds))
	targetSeedFound := false
	for index, seed := range seeds {
		value, err := canonical.CanonicalizeURL(seed)
		decision := evaluator.EvaluateURL(seed)
		if err != nil || !decision.AllowedForActive() {
			fmt.Fprintf(stderr, "reconctx plan: seed %d is invalid or out of scope\n", index+1)
			return 2
		}
		katanaScopePatterns[index], err = katanaScopePattern(value, scopeConfig, decision)
		if err != nil {
			fmt.Fprintf(stderr, "reconctx plan: seed %d scope: %v\n", index+1, err)
			return 2
		}
		canonicalSeeds[index] = value.CanonicalObservationURL
		targetSeedFound = targetSeedFound || value.Host == targetURL.Host
	}
	if !targetSeedFound {
		fmt.Fprintln(stderr, "reconctx plan: target host must match at least one approved seed host")
		return 2
	}
	wordlistAbsolute, err := filepath.Abs(*wordlistPath)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: wordlist path: %v\n", err)
		return 2
	}
	wordlistAbsolute, err = filepath.EvalSymlinks(wordlistAbsolute)
	if err != nil || strings.ContainsAny(wordlistAbsolute, "\x00\r\n") {
		fmt.Fprintln(stderr, "reconctx plan: wordlist path is invalid")
		return 2
	}
	wordlistDocument, err := readRegularFile(wordlistAbsolute, maxWordlistDocument)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: read wordlist: %v\n", err)
		return 1
	}
	requestBudget := 0
	for _, line := range bytes.Split(wordlistDocument, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) != 0 {
			requestBudget++
		}
	}
	if requestBudget == 0 {
		fmt.Fprintln(stderr, "reconctx plan: wordlist has no parameter names")
		return 2
	}
	wordlistDigest := sha256.Sum256(wordlistDocument)

	workspaceName, created, err := openWorkspace(*workspacePath)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: create workspace: %v\n", err)
		return 1
	}
	root, err := workspace.Open(workspaceName)
	if err != nil {
		if created {
			_ = os.Remove(workspaceName)
		}
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 1
	}
	defer root.Close()
	runID, err := randomID("run_")
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 1
	}
	wordlistRelative := path.Join("runs", runID, "inputs", "wordlist.txt")
	wordlistPrivate := filepath.Join(workspaceName, filepath.FromSlash(wordlistRelative))
	toolHomeRelative := path.Join("runs", runID, "home")
	toolHome := filepath.Join(workspaceName, filepath.FromSlash(toolHomeRelative))
	if *outputPath == "" {
		*outputPath = filepath.Join(workspaceName, "runs", runID, "plan.json")
	} else if !filepath.IsAbs(*outputPath) {
		*outputPath = filepath.Join(workspaceName, *outputPath)
	}
	outputRelative, err := workspaceRelative(workspaceName, *outputPath)
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 2
	}
	scopeRelative := path.Join("runs", runID, "scope.yaml")
	planRelative := path.Join("runs", runID, "plan.json")
	scopeDigest := sha256.Sum256(scopeDocument)
	plan := model.Plan{
		PlanVersion: "reconctx-plan/v0", RunID: runID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Inputs: model.PlanInputs{
			Target: targetURL.Host, Seeds: canonicalSeeds, ScopePath: scopeRelative,
			ScopeSHA256: "sha256:" + hex.EncodeToString(scopeDigest[:]), Profile: profile.Name,
			WordlistPath: wordlistPrivate, WordlistSHA256: "sha256:" + hex.EncodeToString(wordlistDigest[:]),
		},
		CanonicalizationPolicy: canonical.URLPolicyVersion, SchemaVersion: model.SchemaVersion,
		Limits: model.PlanLimits{ArjunMaxTargets: profile.Limits.ArjunMaxTargets, ArjunRequestBudget: requestBudget}, EnvironmentAllowlist: append([]string(nil), profile.EnvironmentAllowlist...), WorkspaceRoot: workspaceName,
	}
	toolCandidates := map[string]string{"gau": *gauPath, "katana": *katanaPath, "arjun": *arjunPath}
	for _, configured := range profile.Tools {
		switch configured.Name {
		case "gau":
			directory := path.Join("runs", runID, "executions", "tx_gau")
			plan.Tools = append(plan.Tools, toolPlan(configured, []string{"gau", "--config", gauConfigIsolationPath, plan.Inputs.Target, "--subs", "--verbose", "--providers", "otx,urlscan", "--threads", "1", "--timeout", fmt.Sprint(configured.RequestTimeoutSeconds), "--o", "native-output.txt"}, directory, "native-output.txt"))
		case "katana":
			for index, seed := range canonicalSeeds {
				directory := path.Join("runs", runID, "executions", fmt.Sprintf("tx_katana_%02d", index+1))
				argv := []string{"katana", "-u", seed, "-cs", katanaScopePatterns[index], "-d", "2", "-j", "-nc", "-silent", "-rl", fmt.Sprint(configured.RatePerSecond), "-c", fmt.Sprint(configured.Concurrency), "-p", fmt.Sprint(configured.Parallelism), "-timeout", fmt.Sprint(configured.RequestTimeoutSeconds), "-or", "-ob", "-o", "native-output.jsonl"}
				plan.Tools = append(plan.Tools, toolPlan(configured, argv, directory, "native-output.jsonl"))
			}
		case "arjun":
			directory := path.Join("runs", runID, "executions", "tx_arjun_pending")
			plan.Tools = append(plan.Tools, toolPlan(configured, []string{"arjun"}, directory, "native-output.json"))
		}
	}
	rendered, err := app.BuildPlan(context.Background(), plan, toolCandidates, append(os.Environ(), "HOME="+toolHome))
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 1
	}
	if err := root.CreateRunDir(runID); err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 1
	}
	if err := root.MkdirAll(toolHomeRelative); err != nil {
		fmt.Fprintf(stderr, "reconctx plan: create private tool home: %v\n", err)
		return 1
	}
	if err := root.WriteFileExclusive(wordlistRelative, wordlistDocument); err != nil {
		fmt.Fprintf(stderr, "reconctx plan: persist wordlist: %v\n", err)
		return 1
	}
	if err := root.WriteFileExclusive(scopeRelative, scopeDocument); err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 1
	}
	artifact := append([]byte(rendered.ArtifactJSON), '\n')
	if err := root.WriteFileExclusive(planRelative, artifact); err != nil {
		fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
		return 1
	}
	if outputRelative != planRelative {
		if err := root.WriteFileExclusive(outputRelative, artifact); err != nil {
			fmt.Fprintf(stderr, "reconctx plan: %v\n", err)
			return 1
		}
	}
	runState, err := canonical.Marshal(model.Run{ID: runID, State: model.RunPlanned})
	if err != nil {
		fmt.Fprintf(stderr, "reconctx plan: encode run state: %v\n", err)
		return 1
	}
	if err := root.ReplaceFile(path.Join("state", runID+".json"), append(runState, '\n')); err != nil {
		fmt.Fprintf(stderr, "reconctx plan: persist run state: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, rendered.Display)
	fmt.Fprintf(stdout, "plan_file: %s\n", filepath.Join(workspaceName, filepath.FromSlash(outputRelative)))
	return 0
}

func katanaScopePattern(seed canonical.URL, config scope.Config, decision scope.Decision) (string, error) {
	if decision.RuleID == nil {
		return "", errors.New("in-scope decision has no rule ID")
	}
	for index, root := range config.Roots {
		rootID := root.ID
		if rootID == "" {
			rootID = fmt.Sprintf("scope_root_%d", index+1)
		}
		if rootID != *decision.RuleID {
			continue
		}
		if root.Kind != "url_prefix" {
			return "^" + regexp.QuoteMeta(seed.Origin) + "(?:/|$)", nil
		}
		prefix, err := canonical.CanonicalizeURL(root.Value)
		if err != nil || prefix.Origin != seed.Origin {
			return "", errors.New("URL-prefix rule differs from the seed origin")
		}
		base := regexp.QuoteMeta(prefix.Origin + prefix.Path)
		if strings.HasSuffix(prefix.Path, "/") {
			return "^" + base + "[^%]*$", nil
		}
		return "^" + base + `(?:$|[?#][^%]*$|/[^%]*$)`, nil
	}
	return "", fmt.Errorf("scope rule %q is unavailable", *decision.RuleID)
}

func toolPlan(configured profiles.Tool, argv []string, directory, native string) model.ToolPlan {
	return model.ToolPlan{
		Name: configured.Name, ActivityClass: configured.ActivityClass, Argv: argv,
		Limits: model.ToolLimits{
			RatePerSecond: configured.RatePerSecond, Concurrency: configured.Concurrency, Parallelism: configured.Parallelism,
			RequestTimeoutSeconds: configured.RequestTimeoutSeconds, ExecutionTimeoutSeconds: configured.ExecutionTimeoutSeconds,
		},
		OutputPaths: []string{path.Join(directory, "stdout.raw"), path.Join(directory, "stderr.raw"), path.Join(directory, native)},
	}
}
