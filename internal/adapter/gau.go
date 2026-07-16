package adapter

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/vtrpza/reconctx/internal/model"
)

const GAUAdapterVersion = "gau-adapter/v0"

var gauProvider = regexp.MustCompile(`(?:^|\s)provider=([^\s]+)`)

type GAUOptions struct {
	Context
	Format     string
	Providers  []string
	ExitCode   *int
	Stderr     *Source
	Incomplete bool
}

func ParseGAU(native Source, options GAUOptions) (Result, error) {
	if err := validateContext(options.Context); err != nil {
		return Result{}, err
	}
	providers := slices.Clone(options.Providers)
	slices.Sort(providers)
	providers = slices.Compact(providers)
	if len(providers) == 0 || providers[0] == "" {
		return Result{}, fmt.Errorf("GAU provider set cannot be empty")
	}
	raw, err := readSource(native)
	if err != nil {
		return Result{}, err
	}
	var stderrRaw []byte
	if options.Stderr != nil {
		stderrRaw, err = readSource(*options.Stderr)
		if err != nil {
			return Result{}, err
		}
	}
	builder := newRecordBuilder(options.Context)
	result := Result{Status: "success", Coverage: "complete", ProviderStatus: []model.ProviderStatus{}, Warnings: []model.Diagnostic{}, Gaps: []model.Diagnostic{}}
	result.Warnings = append(result.Warnings, diagnostic(
		"gau.provider_attribution_run_level",
		"Provider set is known, but individual URL lines have no provider field.",
		"warning",
	))
	result.ProviderStatus = make([]model.ProviderStatus, len(providers))
	providerIndexes := make(map[string]int, len(providers))
	providerStarted := make([]bool, len(providers))
	for index, provider := range providers {
		providerIndexes[provider] = index
		message := "no provider lifecycle evidence has been observed"
		result.ProviderStatus[index] = model.ProviderStatus{Provider: provider, Status: "unknown", Message: &message, EvidenceIDs: []string{}}
	}

	format := options.Format
	if format == "" {
		format = "text"
	}
	if format != "text" {
		result.Status, result.Coverage = "unsupported_format", "unknown"
		result.Gaps = append(result.Gaps, diagnostic(
			"gau.json_unsupported",
			"GAU 2.2.4 JSON output is unsupported because it can silently drop extensionless URLs.",
			"error",
		))
		result.Records = builder.finish()
		return result, nil
	}

	nativeLines, err := lines(raw)
	if err != nil {
		return Result{}, err
	}
	valid, invalid := 0, 0
	for index, rawLine := range nativeLines {
		value := strings.TrimSpace(string(rawLine))
		if value == "" {
			continue
		}
		decision := builder.decision(value)
		evidence := builder.addEvidence(native.Artifact, model.Locator{Kind: "line_range", LineStart: index + 1, LineEnd: index + 1}, decision)
		endpoint, canonicalURL, parseErr := builder.addEndpoint(nil, value)
		if parseErr != nil {
			invalid++
			result.Gaps = append(result.Gaps, diagnostic("gau.invalid_url", fmt.Sprintf("line %d is not an absolute canonical HTTP(S) URL", index+1), "warning", evidence.ID))
			continue
		}
		valid++
		builder.addObservation(
			"historical_url", "historical", endpointEntity{endpoint}, nil, []string{evidence.ID},
			model.HistoricalDetails{
				URLRaw: value, CanonicalObservationURL: canonicalURL.CanonicalObservationURL, CanonicalRouteURL: canonicalURL.CanonicalRouteURL,
				QueryPairs: queryPairs(canonicalURL.QueryPairs), ProviderSet: slices.Clone(providers), CurrentReachability: "unknown",
			},
			endpoint.Scope,
		)
	}

	if len(stderrRaw) > 0 {
		stderrLines, lineErr := lines(stderrRaw)
		if lineErr != nil {
			return Result{}, lineErr
		}
		unknown := model.ScopeDecision{Classification: "unknown", Reason: "log line is not URL-scoped"}
		for index, rawLine := range stderrLines {
			line := string(rawLine)
			if line == "" {
				continue
			}
			if strings.Contains(line, "error reading config:") {
				evidence := builder.addEvidence(options.Stderr.Artifact, model.Locator{Kind: "line_range", LineStart: index + 1, LineEnd: index + 1}, unknown)
				if strings.Contains(line, "not found, using default config") {
					result.Warnings = append(result.Warnings, diagnostic("gau.config_defaulted", "The approved isolated GAU config path was absent; built-in defaults were used.", "warning", evidence.ID))
				} else {
					result.Gaps = append(result.Gaps, diagnostic("gau.config_error", "GAU reported an unexpected config-read error and fell back to built-in defaults.", "error", evidence.ID))
				}
				continue
			}
			match := gauProvider.FindStringSubmatch(line)
			if len(match) != 2 {
				continue
			}
			provider := strings.Trim(match[1], `"`)
			providerIndex, selected := providerIndexes[provider]
			switch {
			case strings.Contains(line, "level=warning") || strings.Contains(line, "level=error"):
				evidence := builder.addEvidence(options.Stderr.Artifact, model.Locator{Kind: "line_range", LineStart: index + 1, LineEnd: index + 1}, unknown)
				gap := diagnostic("gau.provider_error", "GAU reported a warning or error for provider "+provider+".", "error", evidence.ID)
				gap.Provider = &provider
				result.Gaps = append(result.Gaps, gap)
				if selected {
					code, message := "provider_error", "GAU verbose stderr reported a provider warning or error"
					status := &result.ProviderStatus[providerIndex]
					status.Status, status.ErrorCode, status.Message = "error", &code, &message
					status.EvidenceIDs = append(status.EvidenceIDs, evidence.ID)
				}
			case strings.Contains(line, "level=info") && strings.Contains(line, `msg="fetching `):
				evidence := builder.addEvidence(options.Stderr.Artifact, model.Locator{Kind: "line_range", LineStart: index + 1, LineEnd: index + 1}, unknown)
				if !selected {
					gap := diagnostic("gau.unexpected_provider", "GAU started an unapproved provider "+provider+".", "error", evidence.ID)
					gap.Provider = &provider
					result.Gaps = append(result.Gaps, gap)
					continue
				}
				providerStarted[providerIndex] = true
				status := &result.ProviderStatus[providerIndex]
				status.EvidenceIDs = append(status.EvidenceIDs, evidence.ID)
				if status.Status != "error" {
					message := "provider start observed; awaiting clean process completion"
					status.Message = &message
				}
			}
		}
	}

	processComplete := options.ExitCode != nil && *options.ExitCode == 0 && !options.Incomplete
	if options.ExitCode == nil {
		result.Status, result.Coverage = "failed", "unknown"
		result.Gaps = append(result.Gaps, diagnostic("gau.exit_unknown", "GAU exit status is unavailable.", "error"))
	} else if *options.ExitCode != 0 {
		if valid > 0 {
			result.Status, result.Coverage = "partial", "partial"
		} else {
			result.Status, result.Coverage = "failed", "unknown"
		}
	}
	for index := range result.ProviderStatus {
		status := &result.ProviderStatus[index]
		if status.Status == "error" {
			continue
		}
		if processComplete && providerStarted[index] {
			message := "provider start observed and GAU completed without a provider warning or error"
			status.Status, status.ErrorCode, status.Message = "success", nil, &message
			continue
		}
		code := "provider_state_unknown"
		message := "GAU provider start and clean process completion were not both observed"
		status.Status, status.ErrorCode, status.Message = "unknown", &code, &message
		provider := status.Provider
		gap := diagnostic("gau.provider_state_unknown", "GAU could not establish a terminal state for provider "+provider+".", "error", status.EvidenceIDs...)
		gap.Provider = &provider
		result.Gaps = append(result.Gaps, gap)
	}
	if len(result.Gaps) > 0 && result.Status == "success" {
		result.Status, result.Coverage = "partial", "partial"
	}
	if valid == 0 && invalid == 0 && result.Status == "success" {
		result.Status, result.Coverage = "success_zero", "zero"
	}
	result.Records = builder.finish()
	return result, nil
}
