package model

type CandidateQueue struct {
	QueueVersion  string      `json:"queue_version"`
	PolicyVersion string      `json:"policy_version"`
	PlanDigest    string      `json:"plan_digest"`
	Candidates    []Candidate `json:"candidates"`
	Limits        ToolLimits  `json:"limits"`
	MaxTargets    int         `json:"max_targets"`
}

type Candidate struct {
	ID                 string         `json:"id"`
	EndpointID         string         `json:"endpoint_id"`
	URL                string         `json:"url"`
	Method             string         `json:"method"`
	SourceMode         string         `json:"source_mode"`
	Location           string         `json:"location"`
	ObservationIDs     []string       `json:"observation_ids"`
	EvidenceIDs        []string       `json:"evidence_ids"`
	SourceExecutionIDs []string       `json:"source_execution_ids"`
	ReasonCodes        []string       `json:"reason_codes"`
	Rank               CandidateRank  `json:"rank"`
	RankPosition       int            `json:"rank_position"`
	WordlistPath       string         `json:"wordlist_path"`
	WordlistSHA256     string         `json:"wordlist_sha256"`
	NativeOutputPath   string         `json:"native_output_path"`
	Argv               []string       `json:"argv"`
	RequestBudget      int            `json:"request_budget"`
	Scope              CandidateScope `json:"scope"`
}

type CandidateRank struct {
	CurrentlyObservedByKatana bool `json:"currently_observed_by_katana"`
	ExistingQueryNameEvidence bool `json:"existing_query_name_evidence"`
	APILikePath               bool `json:"api_like_path"`
	IndependentExecutions     int  `json:"independent_source_executions"`
	NoStaticExtension         bool `json:"no_static_extension"`
	SupportedMethodLocation   bool `json:"supported_method_location"`
}

type CandidateScope struct {
	Classification string `json:"classification"`
	RuleID         string `json:"rule_id"`
	Reason         string `json:"reason"`
}
