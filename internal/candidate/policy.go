package candidate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/vtrpza/reconctx/internal/approval"
	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
)

const PolicyVersion = "arjun-candidate-policy/v0"

type Config struct {
	PlanDigest           string
	ArjunPath            string
	WordlistPath         string
	WordlistSHA256       string
	NativeOutputRoot     string
	Limits               model.ToolLimits
	MaxTargets           int
	RequestBudget        int
	IncludeHistorical    bool
	ExcludedPathPrefixes []string
}

type Result struct {
	Queue       model.CandidateQueue
	QueueDigest string
	Decisions   []Decision
}

// Decision is the redacted public projection written to arjun-candidates.jsonl.
type Decision struct {
	SchemaVersion      string              `json:"schema_version"`
	RecordType         string              `json:"record_type"`
	PolicyVersion      string              `json:"candidate_policy_version"`
	QueueDigest        string              `json:"queue_digest"`
	CandidateID        string              `json:"candidate_id"`
	EndpointID         string              `json:"endpoint_id"`
	SelectedURL        string              `json:"selected_url"`
	CanonicalRouteURL  string              `json:"canonical_route_url"`
	Method             *string             `json:"method"`
	SourceMode         *string             `json:"source_mode"`
	Location           string              `json:"location"`
	Eligible           bool                `json:"eligible"`
	Included           bool                `json:"included"`
	ReasonCodes        []string            `json:"reason_codes"`
	Rank               model.CandidateRank `json:"rank_inputs"`
	RankPosition       *int                `json:"rank_position"`
	ObservationIDs     []string            `json:"observation_ids"`
	EvidenceIDs        []string            `json:"evidence_ids"`
	SourceExecutionIDs []string            `json:"source_execution_ids"`
	Scope              model.ScopeDecision `json:"scope_decision"`
	ArgvRedacted       []string            `json:"argv_redacted"`
	WordlistSHA256     string              `json:"wordlist_sha256"`
	RequestBudget      int                 `json:"request_budget"`
	MaxTargets         int                 `json:"max_targets"`
}

type projection struct {
	decision  Decision
	argv      []string
	nativeOut string
	dedupeKey string
}

type mode struct {
	method, source, location string
	supported                bool
}

func Build(records model.RecordSet, config Config) (Result, error) {
	if err := validateConfig(config); err != nil {
		return Result{}, err
	}
	projections, err := project(records, config)
	if err != nil {
		return Result{}, err
	}
	slices.SortFunc(projections, compareRank)

	seen := make(map[string]bool, len(projections))
	for index := range projections {
		item := &projections[index]
		if len(item.decision.ReasonCodes) != 0 {
			continue
		}
		if seen[item.dedupeKey] {
			item.decision.ReasonCodes = []string{"canonical_duplicate"}
			continue
		}
		seen[item.dedupeKey] = true
		item.decision.Eligible = true
	}

	queue := model.CandidateQueue{
		QueueVersion: "reconctx-candidate-queue/v0", PolicyVersion: PolicyVersion,
		PlanDigest: config.PlanDigest, Candidates: make([]model.Candidate, 0, min(len(projections), config.MaxTargets)),
		Limits: config.Limits, MaxTargets: config.MaxTargets,
	}
	rank := 0
	for index := range projections {
		item := &projections[index]
		if !item.decision.Eligible {
			continue
		}
		rank++
		item.decision.RankPosition = intPointer(rank)
		if rank > config.MaxTargets {
			item.decision.ReasonCodes = []string{"max_targets_overflow"}
			continue
		}
		item.decision.Included = true
		item.decision.ReasonCodes = []string{"selected"}
		ruleID := ""
		if item.decision.Scope.RuleID != nil {
			ruleID = *item.decision.Scope.RuleID
		}
		queue.Candidates = append(queue.Candidates, model.Candidate{
			ID: item.decision.CandidateID, EndpointID: item.decision.EndpointID,
			URL: item.decision.CanonicalRouteURL, Method: *item.decision.Method,
			SourceMode: *item.decision.SourceMode, Location: item.decision.Location,
			ObservationIDs: slices.Clone(item.decision.ObservationIDs), EvidenceIDs: slices.Clone(item.decision.EvidenceIDs),
			SourceExecutionIDs: slices.Clone(item.decision.SourceExecutionIDs), ReasonCodes: slices.Clone(item.decision.ReasonCodes),
			Rank: item.decision.Rank, RankPosition: rank,
			WordlistPath: config.WordlistPath, WordlistSHA256: config.WordlistSHA256,
			NativeOutputPath: item.nativeOut, Argv: slices.Clone(item.argv), RequestBudget: config.RequestBudget,
			Scope: model.CandidateScope{Classification: item.decision.Scope.Classification, RuleID: ruleID, Reason: item.decision.Scope.Reason},
		})
	}

	digest, err := approval.QueueDigest(queue)
	if err != nil {
		return Result{}, fmt.Errorf("digest candidate queue: %w", err)
	}
	decisions := make([]Decision, len(projections))
	for index := range projections {
		projections[index].decision.QueueDigest = digest
		decisions[index] = cloneDecision(projections[index].decision)
	}
	slices.SortFunc(decisions, compareDecision)
	return Result{Queue: queue, QueueDigest: digest, Decisions: decisions}, nil
}

func validateConfig(config Config) error {
	if !filepath.IsAbs(config.ArjunPath) || !filepath.IsAbs(config.WordlistPath) || !filepath.IsAbs(config.NativeOutputRoot) {
		return errors.New("Arjun, wordlist, and native output paths must be absolute")
	}
	if config.MaxTargets < 0 || config.RequestBudget <= 0 || config.Limits.RatePerSecond <= 0 || config.Limits.Concurrency <= 0 || config.Limits.Parallelism <= 0 || config.Limits.TimeoutSeconds <= 0 {
		return errors.New("candidate limits and request budget must be positive")
	}
	if !validDigest(config.WordlistSHA256) {
		return errors.New("wordlist digest must be a SHA-256 digest")
	}
	for _, prefix := range config.ExcludedPathPrefixes {
		if prefix == "" || prefix[0] != '/' || strings.ContainsAny(prefix, "?#") {
			return fmt.Errorf("invalid excluded path prefix %q", prefix)
		}
	}
	return nil
}

func project(records model.RecordSet, config Config) ([]projection, error) {
	executions := make(map[string]string, len(records.ToolExecutions))
	for _, execution := range records.ToolExecutions {
		executions[execution.ID] = strings.ToLower(execution.Tool.Name)
	}
	observations := make(map[string]model.Observation, len(records.Observations))
	endpointObservationIDs := make(map[string][]string)
	for _, observation := range records.Observations {
		observations[observation.ID] = observation
		if observation.Subject.RecordType == "endpoint" {
			endpointObservationIDs[observation.Subject.ID] = append(endpointObservationIDs[observation.Subject.ID], observation.ID)
		}
	}
	endpoints := make(map[string]model.Endpoint, len(records.Endpoints))
	for _, endpoint := range records.Endpoints {
		endpoints[endpoint.ID] = endpoint
		endpointObservationIDs[endpoint.ID] = append(endpointObservationIDs[endpoint.ID], endpoint.ObservationIDs...)
	}
	parameterObservations := make(map[string][]string)
	for _, observation := range records.Observations {
		if observation.Subject.RecordType == "parameter" {
			parameterObservations[observation.Subject.ID] = append(parameterObservations[observation.Subject.ID], observation.ID)
		}
	}

	routeObservationIDs := make(map[string][]string)
	routeEvidenceIDs := make(map[string][]string)
	routeParameters := make(map[string][]model.Parameter)
	for _, endpoint := range records.Endpoints {
		routeObservationIDs[endpoint.CanonicalRouteURL] = append(routeObservationIDs[endpoint.CanonicalRouteURL], endpointObservationIDs[endpoint.ID]...)
		routeEvidenceIDs[endpoint.CanonicalRouteURL] = append(routeEvidenceIDs[endpoint.CanonicalRouteURL], endpoint.EvidenceIDs...)
	}
	for _, parameter := range records.Parameters {
		endpoint, ok := endpoints[parameter.EndpointID]
		if !ok {
			continue
		}
		route := endpoint.CanonicalRouteURL
		routeParameters[route] = append(routeParameters[route], parameter)
		routeObservationIDs[route] = append(routeObservationIDs[route], parameter.ObservationIDs...)
		routeObservationIDs[route] = append(routeObservationIDs[route], parameterObservations[parameter.ID]...)
		routeEvidenceIDs[route] = append(routeEvidenceIDs[route], parameter.EvidenceIDs...)
	}
	for route, ids := range routeObservationIDs {
		ids = unique(ids)
		routeObservationIDs[route] = ids
		for _, id := range ids {
			if observation, ok := observations[id]; ok {
				routeEvidenceIDs[route] = append(routeEvidenceIDs[route], observation.EvidenceIDs...)
			}
		}
		routeEvidenceIDs[route] = unique(routeEvidenceIDs[route])
	}

	result := make([]projection, 0, len(records.Endpoints))
	for _, endpoint := range records.Endpoints {
		if endpoint.MethodKnown != (endpoint.Method != nil) {
			return nil, fmt.Errorf("endpoint %s has inconsistent method state", endpoint.ID)
		}
		canonicalURL, err := canonical.CanonicalizeURL(endpoint.CanonicalRouteURL)
		if err != nil || canonicalURL.CanonicalRouteURL != endpoint.CanonicalRouteURL {
			return nil, fmt.Errorf("endpoint %s has non-canonical route URL", endpoint.ID)
		}
		expectedID, err := canonical.EndpointID(endpoint.Method, endpoint.CanonicalRouteURL)
		if err != nil || expectedID != endpoint.ID {
			return nil, fmt.Errorf("endpoint %s identity does not match its route and method", endpoint.ID)
		}
		ownIDs := unique(endpointObservationIDs[endpoint.ID])
		routeIDs := unique(routeObservationIDs[endpoint.CanonicalRouteURL])
		evidenceIDs := unique(routeEvidenceIDs[endpoint.CanonicalRouteURL])
		sourceIDs := sourceExecutions(routeIDs, observations)
		currentKatana := observedByKatana(ownIDs, observations, executions)
		historicalOnly, hasSource := sourceState(ownIDs, observations)
		staticMIME := hasStaticMIME(ownIDs, observations)
		fragmentOnly := isFragmentOnly(endpoint.CanonicalRouteURL, ownIDs, observations)
		queryEvidence := hasQueryEvidence(routeParameters[endpoint.CanonicalRouteURL])
		for _, candidateMode := range modes(endpoint, routeParameters[endpoint.CanonicalRouteURL]) {
			methodPointer, sourcePointer := stringPointerOrNil(candidateMode.method), stringPointerOrNil(candidateMode.source)
			rank := model.CandidateRank{
				CurrentlyObservedByKatana: currentKatana, ExistingQueryNameEvidence: queryEvidence,
				APILikePath: apiLike(endpoint.Path), IndependentExecutions: len(sourceIDs),
				NoStaticExtension: !hasStaticExtension(endpoint.Path), SupportedMethodLocation: candidateMode.supported,
			}
			decision := Decision{
				SchemaVersion: model.SchemaVersion, RecordType: "arjun_candidate", PolicyVersion: PolicyVersion,
				CandidateID: candidateID(endpoint.ID, candidateMode.method, candidateMode.location), EndpointID: endpoint.ID,
				SelectedURL: endpoint.CanonicalRouteURL, CanonicalRouteURL: endpoint.CanonicalRouteURL,
				Method: methodPointer, SourceMode: sourcePointer, Location: candidateMode.location,
				ReasonCodes: exclusionReasons(endpoint, candidateMode, config, historicalOnly, hasSource, staticMIME, fragmentOnly, len(routeIDs) > 0 && len(evidenceIDs) > 0),
				Rank:        rank, ObservationIDs: routeIDs, EvidenceIDs: evidenceIDs, SourceExecutionIDs: sourceIDs,
				Scope: endpoint.Scope, WordlistSHA256: config.WordlistSHA256, RequestBudget: config.RequestBudget, MaxTargets: config.MaxTargets,
			}
			nativeOut := filepath.Join(config.NativeOutputRoot, decision.CandidateID, "native-output.json")
			argv := renderArgv(config, endpoint.CanonicalRouteURL, candidateMode, nativeOut)
			decision.ArgvRedacted = redactArgv(argv)
			result = append(result, projection{decision: decision, argv: argv, nativeOut: nativeOut, dedupeKey: endpoint.CanonicalRouteURL + "\x00" + candidateMode.method + "\x00" + candidateMode.location})
		}
	}
	return result, nil
}

func modes(endpoint model.Endpoint, parameters []model.Parameter) []mode {
	if !endpoint.MethodKnown || endpoint.Method == nil {
		return []mode{{location: "unknown"}}
	}
	method := strings.ToUpper(*endpoint.Method)
	if method == "GET" {
		return []mode{{method: method, source: "GET", location: "query", supported: true}}
	}
	if method != "POST" {
		return []mode{{method: method, location: "unknown"}}
	}
	locations := make(map[string]bool, 2)
	for _, parameter := range parameters {
		if parameter.EndpointID == endpoint.ID && (parameter.Location == "form" || parameter.Location == "json") {
			locations[parameter.Location] = true
		}
	}
	result := make([]mode, 0, len(locations))
	if locations["form"] {
		result = append(result, mode{method: "POST", source: "POST", location: "form", supported: true})
	}
	if locations["json"] {
		result = append(result, mode{method: "POST", source: "JSON", location: "json", supported: true})
	}
	if len(result) == 0 {
		return []mode{{method: "POST", location: "unknown"}}
	}
	return result
}

func exclusionReasons(endpoint model.Endpoint, candidateMode mode, config Config, historicalOnly, hasSource, staticMIME, fragmentOnly, hasProvenance bool) []string {
	reasons := make([]string, 0, 5)
	switch endpoint.Scope.Classification {
	case "out_of_scope":
		reasons = append(reasons, "out_of_scope")
	case "in_scope":
		if endpoint.Scope.RuleID == nil || endpoint.Scope.Reason == "" {
			reasons = append(reasons, "scope_unknown")
		}
	default:
		reasons = append(reasons, "scope_unknown")
	}
	if hasStaticExtension(endpoint.Path) {
		reasons = append(reasons, "static_extension")
	}
	if staticMIME {
		reasons = append(reasons, "static_mime")
	}
	if fragmentOnly {
		reasons = append(reasons, "fragment_only")
	}
	if excludedPath(endpoint.Path, config.ExcludedPathPrefixes) {
		reasons = append(reasons, "excluded_path")
	}
	if historicalOnly && !config.IncludeHistorical {
		reasons = append(reasons, "historical_only")
	}
	if !candidateMode.supported {
		reasons = append(reasons, "unsupported_method_location")
	}
	if !hasSource || !hasProvenance {
		reasons = append(reasons, "missing_provenance")
	}
	return reasons
}

func renderArgv(config Config, target string, candidateMode mode, nativeOut string) []string {
	if !candidateMode.supported {
		return []string{}
	}
	argv := []string{
		config.ArjunPath, "-u", target, "-m", candidateMode.source, "-w", config.WordlistPath,
		"--rate-limit", strconv.Itoa(config.Limits.RatePerSecond), "-t", strconv.Itoa(config.Limits.Concurrency), "-T", strconv.Itoa(config.Limits.TimeoutSeconds),
	}
	if candidateMode.location == "form" {
		argv = append(argv, "--headers", "Content-Type: application/x-www-form-urlencoded")
	} else if candidateMode.location == "json" {
		argv = append(argv, "--headers", "Content-Type: application/json")
	}
	return append(argv, "-oJ", nativeOut)
}

func redactArgv(argv []string) []string {
	if len(argv) == 0 {
		return []string{}
	}
	result := append([]string(nil), argv...)
	result[0] = "<ARJUN>"
	for index := 1; index+1 < len(result); index++ {
		switch result[index] {
		case "-w":
			result[index+1] = "<WORDLIST>"
		case "-oJ":
			result[index+1] = "<NATIVE_OUTPUT>"
		}
	}
	return result
}

func compareRank(a, b projection) int {
	if value := compareBool(a.decision.Rank.CurrentlyObservedByKatana, b.decision.Rank.CurrentlyObservedByKatana); value != 0 {
		return value
	}
	if value := compareBool(a.decision.Rank.ExistingQueryNameEvidence, b.decision.Rank.ExistingQueryNameEvidence); value != 0 {
		return value
	}
	if value := compareBool(a.decision.Rank.APILikePath, b.decision.Rank.APILikePath); value != 0 {
		return value
	}
	if a.decision.Rank.IndependentExecutions != b.decision.Rank.IndependentExecutions {
		return b.decision.Rank.IndependentExecutions - a.decision.Rank.IndependentExecutions
	}
	if value := compareBool(a.decision.Rank.NoStaticExtension, b.decision.Rank.NoStaticExtension); value != 0 {
		return value
	}
	if value := compareBool(a.decision.Rank.SupportedMethodLocation, b.decision.Rank.SupportedMethodLocation); value != 0 {
		return value
	}
	if a.decision.EndpointID != b.decision.EndpointID {
		return strings.Compare(a.decision.EndpointID, b.decision.EndpointID)
	}
	if a.decision.CandidateID != b.decision.CandidateID {
		return strings.Compare(a.decision.CandidateID, b.decision.CandidateID)
	}
	return strings.Compare(strings.Join(a.decision.EvidenceIDs, "\x00"), strings.Join(b.decision.EvidenceIDs, "\x00"))
}

func compareDecision(a, b Decision) int {
	category := func(value Decision) int {
		if value.Included {
			return 0
		}
		if value.Eligible {
			return 1
		}
		return 2
	}
	if category(a) != category(b) {
		return category(a) - category(b)
	}
	return compareRank(projection{decision: a}, projection{decision: b})
}

func compareBool(a, b bool) int {
	if a == b {
		return 0
	}
	if a {
		return -1
	}
	return 1
}

func observedByKatana(ids []string, observations map[string]model.Observation, executions map[string]string) bool {
	for _, id := range ids {
		observation, ok := observations[id]
		if !ok || observation.ObservationType != "http_response" || observation.SemanticState != "observed" {
			continue
		}
		tool, toolKnown := executions[observation.ToolExecutionID]
		// v0 initial normalization emits http_response only from Katana; orchestration normally supplies the execution record.
		if !toolKnown || tool == "katana" {
			return true
		}
	}
	return false
}

func sourceState(ids []string, observations map[string]model.Observation) (historicalOnly, hasSource bool) {
	historical := false
	for _, id := range ids {
		observation, ok := observations[id]
		if !ok || observation.ObservationType == "tool_warning" {
			continue
		}
		hasSource = true
		if observation.SemanticState == "historical" {
			historical = true
		} else {
			return false, true
		}
	}
	return historical && hasSource, hasSource
}

func sourceExecutions(ids []string, observations map[string]model.Observation) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if observation, ok := observations[id]; ok && observation.ToolExecutionID != "" {
			result = append(result, observation.ToolExecutionID)
		}
	}
	return unique(result)
}

func hasQueryEvidence(parameters []model.Parameter) bool {
	return slices.ContainsFunc(parameters, func(parameter model.Parameter) bool { return parameter.Location == "query" })
}

func hasStaticMIME(ids []string, observations map[string]model.Observation) bool {
	for _, id := range ids {
		observation, ok := observations[id]
		if !ok || observation.ObservationType != "http_response" {
			continue
		}
		details, ok := observation.Details.(model.HTTPDetails)
		if !ok || details.ContentType == nil {
			continue
		}
		mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(*details.ContentType, ";", 2)[0]))
		if strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(mediaType, "font/") || mediaType == "text/css" || mediaType == "application/javascript" || mediaType == "application/pdf" {
			return true
		}
	}
	return false
}

func isFragmentOnly(route string, ids []string, observations map[string]model.Observation) bool {
	sawFragment := false
	for _, id := range ids {
		observation, ok := observations[id]
		if !ok {
			continue
		}
		raw := ""
		switch details := observation.Details.(type) {
		case model.HistoricalDetails:
			raw = details.URLRaw
		case model.HTTPDetails:
			raw = details.RequestURLRaw
		}
		if raw == "" {
			continue
		}
		before, _, found := strings.Cut(raw, "#")
		if !found || strings.Contains(before, "?") {
			return false
		}
		value, err := canonical.CanonicalizeURL(before)
		if err != nil || value.CanonicalRouteURL != route {
			return false
		}
		sawFragment = true
	}
	return sawFragment
}

var staticExtensions = map[string]bool{
	".css": true, ".js": true, ".map": true, ".ico": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".webp": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".mp3": true, ".mp4": true, ".webm": true, ".pdf": true,
}

func hasStaticExtension(value string) bool { return staticExtensions[strings.ToLower(path.Ext(value))] }

func excludedPath(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if value == prefix || (strings.HasSuffix(prefix, "/") && strings.HasPrefix(value, prefix)) || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}

// ponytail: this intentionally-small segment heuristic can be expanded only when fixtures prove a missed API route.
func apiLike(value string) bool {
	for _, segment := range strings.Split(strings.ToLower(value), "/") {
		if segment == "api" || segment == "graphql" || segment == "rest" {
			return true
		}
		if len(segment) > 1 && segment[0] == 'v' && strings.IndexFunc(segment[1:], func(character rune) bool { return character < '0' || character > '9' }) == -1 {
			return true
		}
	}
	return false
}

func candidateID(endpointID, method, location string) string {
	digest := sha256.Sum256([]byte(PolicyVersion + "\x00" + endpointID + "\x00" + method + "\x00" + location))
	return "candidate_sha256_" + hex.EncodeToString(digest[:])
}

func stringPointerOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func intPointer(value int) *int { return &value }

func cloneDecision(value Decision) Decision {
	value.ReasonCodes = slices.Clone(value.ReasonCodes)
	value.ObservationIDs = slices.Clone(value.ObservationIDs)
	value.EvidenceIDs = slices.Clone(value.EvidenceIDs)
	value.SourceExecutionIDs = slices.Clone(value.SourceExecutionIDs)
	value.ArgvRedacted = slices.Clone(value.ArgvRedacted)
	return value
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func unique(values []string) []string {
	result := make([]string, len(values))
	copy(result, values)
	slices.Sort(result)
	return slices.Compact(result)
}
