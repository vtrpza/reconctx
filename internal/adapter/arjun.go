package adapter

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
)

const (
	ArjunAdapterVersion = "arjun-adapter/v0"
	maxArjunTargets     = 25
)

type ArjunOptions struct {
	Context
	Native       *Source
	Stdout       *Source
	Stderr       *Source
	TargetURL    string
	SourceMethod string
	ObservedAt   *string
	ExitCode     *int
	Interrupted  bool
	TimedOut     bool
}

type arjunFinding struct {
	Headers map[string]string `json:"headers"`
	Method  string            `json:"method"`
	Params  []string          `json:"params"`
}

func ParseArjun(options ArjunOptions) (Result, error) {
	if err := validateContext(options.Context); err != nil {
		return Result{}, err
	}
	var stdoutRaw, stderrRaw []byte
	var err error
	if options.Stdout != nil {
		stdoutRaw, err = readSource(*options.Stdout)
		if err != nil {
			return Result{}, err
		}
		if _, err = lines(stdoutRaw); err != nil {
			return Result{}, err
		}
	}
	if options.Stderr != nil {
		stderrRaw, err = readSource(*options.Stderr)
		if err != nil {
			return Result{}, err
		}
		if _, err = lines(stderrRaw); err != nil {
			return Result{}, err
		}
	}
	builder := newRecordBuilder(options.Context)
	result := Result{Status: "success", Coverage: "complete", ProviderStatus: []model.ProviderStatus{}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{}}
	valid, malformed := 0, 0
	if options.Native != nil {
		nativeRaw, readErr := readSource(*options.Native)
		if readErr != nil {
			return Result{}, readErr
		}
		canonicalJSON, parseErr := canonical.Canonicalize(nativeRaw)
		findings := map[string]json.RawMessage{}
		if parseErr == nil {
			parseErr = json.Unmarshal(canonicalJSON, &findings)
		}
		if parseErr != nil || len(findings) == 0 {
			malformed++
			evidence := builder.addEvidence(options.Native.Artifact, model.Locator{Kind: "whole_artifact"}, model.ScopeDecision{Classification: "unknown", Reason: "native JSON could not be attributed to a target"})
			if parseErr == nil {
				parseErr = fmt.Errorf("top-level result object is empty")
			}
			result.Gaps = append(result.Gaps, diagnostic("arjun.malformed_native", parseErr.Error(), "error", evidence.ID))
		} else {
			targets := make([]string, 0, len(findings))
			for target := range findings {
				targets = append(targets, target)
			}
			slices.Sort(targets)
			if len(targets) > maxArjunTargets {
				return Result{}, fmt.Errorf("Arjun native output exceeds %d targets", maxArjunTargets)
			}
			for _, target := range targets {
				validCount, malformedCount := parseArjunTarget(builder, &result, *options.Native, target, findings[target], options)
				valid += validCount
				malformed += malformedCount
			}
		}
	}

	zeroLine := findLine(stdoutRaw, "No parameters were discovered.", false)
	if options.Native == nil && zeroLine > 0 && options.ExitCode != nil && *options.ExitCode == 0 && !options.Interrupted && !options.TimedOut {
		if options.Stdout == nil {
			return Result{}, fmt.Errorf("Arjun zero marker has no stdout artifact")
		}
		method, methodErr := canonical.NormalizeSourceMethod(&options.SourceMethod, "arjun")
		if methodErr != nil {
			return Result{}, methodErr
		}
		endpoint, _, endpointErr := builder.addEndpoint(method.HTTPMethod, options.TargetURL)
		if endpointErr != nil {
			return Result{}, fmt.Errorf("Arjun zero target: %w", endpointErr)
		}
		evidence := builder.addEvidence(options.Stdout.Artifact, model.Locator{Kind: "line_range", LineStart: zeroLine, LineEnd: zeroLine}, endpoint.Scope)
		message, target := "No parameters were discovered.", options.TargetURL
		builder.addObservation(
			"zero_result", "observed", endpointEntity{endpoint}, options.ObservedAt, []string{evidence.ID},
			model.ZeroDetails{ResultKind: "parameter_discovery", TargetURL: &target, Message: message}, endpoint.Scope,
		)
		result.Status, result.Coverage = "success_zero", "zero"
	}

	switch {
	case options.Interrupted:
		result.Status, result.Coverage = "interrupted", "partial"
		evidenceIDs := arjunDiagnosticEvidence(builder, options.Stdout, stdoutRaw, "Processing chunks:", options.TargetURL)
		result.Gaps = append(result.Gaps, diagnostic("arjun.interrupted", "Arjun was interrupted; parameter presence and absence remain unknown.", "error", evidenceIDs...))
	case options.TimedOut:
		result.Status = "timed_out"
		if valid > 0 {
			result.Coverage = "partial"
		} else {
			result.Coverage = "unknown"
		}
		evidenceIDs := arjunDiagnosticEvidence(builder, options.Stdout, stdoutRaw, "Processing chunks:", options.TargetURL)
		result.Gaps = append(result.Gaps, diagnostic("arjun.timed_out", "Arjun exceeded the outer execution timeout; parameter absence is unknown.", "error", evidenceIDs...))
	case options.ExitCode == nil:
		result.Status, result.Coverage = "failed", "unknown"
		result.Gaps = append(result.Gaps, diagnostic("arjun.exit_unknown", "Arjun exit status is unavailable.", "error"))
	case *options.ExitCode != 0:
		if valid > 0 {
			result.Status, result.Coverage = "partial", "partial"
		} else {
			result.Status, result.Coverage = "failed", "unknown"
		}
		evidenceIDs := arjunDiagnosticEvidence(builder, options.Stderr, stderrRaw, "AttributeError:", options.TargetURL)
		result.Gaps = append(result.Gaps, diagnostic("arjun.tool_error", "Arjun exited with a tool error; parameter absence is unknown.", "error", evidenceIDs...))
	case result.Status == "success_zero":
	case malformed > 0 && valid > 0:
		result.Status, result.Coverage = "partial", "partial"
	case malformed > 0:
		result.Status, result.Coverage = "unsupported_format", "unknown"
	case valid == 0:
		result.Status, result.Coverage = "unsupported_format", "unknown"
		result.Gaps = append(result.Gaps, diagnostic("arjun.result_unknown", "Arjun produced neither valid native results nor an explicit zero-result marker.", "error"))
	}
	result.Records = builder.finish()
	return result, nil
}

func parseArjunTarget(builder *recordBuilder, result *Result, native Source, target string, raw json.RawMessage, options ArjunOptions) (int, int) {
	decision := builder.decision(target)
	wholeEvidence := func() model.Evidence {
		return builder.addEvidence(native.Artifact, model.Locator{Kind: "json_pointer", Pointer: "/" + jsonPointer(target)}, decision)
	}
	var finding arjunFinding
	if err := json.Unmarshal(raw, &finding); err != nil {
		evidence := wholeEvidence()
		result.Gaps = append(result.Gaps, diagnostic("arjun.malformed_target", fmt.Sprintf("target %q: %v", target, err), "warning", evidence.ID))
		return 0, 1
	}
	method, err := canonical.NormalizeSourceMethod(&finding.Method, "arjun")
	if err != nil {
		evidence := wholeEvidence()
		result.Gaps = append(result.Gaps, diagnostic("arjun.invalid_method", fmt.Sprintf("target %q: %v", target, err), "warning", evidence.ID))
		return 0, 1
	}
	if options.SourceMethod != "" && !strings.EqualFold(options.SourceMethod, finding.Method) {
		evidence := wholeEvidence()
		result.Gaps = append(result.Gaps, diagnostic("arjun.method_mismatch", fmt.Sprintf("target %q native method does not match approved execution mode", target), "error", evidence.ID))
		return 0, 1
	}
	actual, err := canonical.CanonicalizeURL(target)
	if err != nil {
		evidence := wholeEvidence()
		result.Gaps = append(result.Gaps, diagnostic("arjun.invalid_target", fmt.Sprintf("target %q: %v", target, err), "warning", evidence.ID))
		return 0, 1
	}
	if options.TargetURL != "" {
		expected, expectedErr := canonical.CanonicalizeURL(options.TargetURL)
		if expectedErr != nil || expected.CanonicalObservationURL != actual.CanonicalObservationURL {
			evidence := wholeEvidence()
			result.Gaps = append(result.Gaps, diagnostic("arjun.unexpected_target", fmt.Sprintf("native target %q does not match approved target", target), "error", evidence.ID))
			return 0, 1
		}
	}
	if len(finding.Params) == 0 {
		evidence := wholeEvidence()
		result.Gaps = append(result.Gaps, diagnostic("arjun.empty_native_result", fmt.Sprintf("target %q contains no parameters", target), "warning", evidence.ID))
		return 0, 1
	}
	if !slices.ContainsFunc(finding.Params, func(name string) bool { return name != "" }) {
		for index := range finding.Params {
			pointer := fmt.Sprintf("/%s/params/%d", jsonPointer(target), index)
			evidence := builder.addEvidence(native.Artifact, model.Locator{Kind: "json_pointer", Pointer: pointer}, decision)
			result.Gaps = append(result.Gaps, diagnostic("arjun.invalid_parameter", fmt.Sprintf("target %q parameter %d is empty", target, index), "warning", evidence.ID))
		}
		return 0, len(finding.Params)
	}
	endpoint, _, err := builder.addEndpoint(method.HTTPMethod, target)
	if err != nil {
		evidence := wholeEvidence()
		result.Gaps = append(result.Gaps, diagnostic("arjun.invalid_target", fmt.Sprintf("target %q: %v", target, err), "warning", evidence.ID))
		return 0, 1
	}
	valid, malformed := 0, 0
	for index, name := range finding.Params {
		pointer := fmt.Sprintf("/%s/params/%d", jsonPointer(target), index)
		evidence := builder.addEvidence(native.Artifact, model.Locator{Kind: "json_pointer", Pointer: pointer}, endpoint.Scope)
		if name == "" {
			malformed++
			result.Gaps = append(result.Gaps, diagnostic("arjun.invalid_parameter", fmt.Sprintf("target %q parameter %d is empty", target, index), "warning", evidence.ID))
			continue
		}
		parameter, err := builder.addParameter(endpoint, name, method.ParameterLocation, "bruteforced")
		if err != nil {
			malformed++
			result.Gaps = append(result.Gaps, diagnostic("arjun.invalid_parameter", fmt.Sprintf("target %q parameter %d: %v", target, index, err), "warning", evidence.ID))
			continue
		}
		sourceMode := *method.SourceLabel
		builder.addObservation(
			"parameter_discovery", "bruteforced", parameterEntity{parameter}, options.ObservedAt, []string{evidence.ID},
			model.ParameterDetails{ParameterName: name, Location: method.ParameterLocation, SourceMode: &sourceMode, AcceptanceState: "unknown"},
			endpoint.Scope,
		)
		valid++
	}
	return valid, malformed
}

func findLine(raw []byte, marker string, last bool) int {
	parsed, err := lines(raw)
	if err != nil {
		return 0
	}
	found := 0
	for index, line := range parsed {
		if strings.Contains(string(line), marker) {
			found = index + 1
			if !last {
				return found
			}
		}
	}
	return found
}

func arjunDiagnosticEvidence(builder *recordBuilder, source *Source, raw []byte, marker, target string) []string {
	if source == nil {
		return []string{}
	}
	decision := model.ScopeDecision{Classification: "unknown", Reason: "diagnostic artifact is not URL-scoped"}
	if target != "" {
		decision = builder.decision(target)
	}
	line := findLine(raw, marker, true)
	locator := model.Locator{Kind: "whole_artifact"}
	if line > 0 {
		locator = model.Locator{Kind: "line_range", LineStart: line, LineEnd: line}
	}
	evidence := builder.addEvidence(source.Artifact, locator, decision)
	return []string{evidence.ID}
}
