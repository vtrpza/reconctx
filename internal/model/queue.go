package model

type CandidateQueue struct {
	QueueVersion string      `json:"queue_version"`
	PlanDigest   string      `json:"plan_digest"`
	Candidates   []Candidate `json:"candidates"`
	Limits       ToolLimits  `json:"limits"`
	MaxTargets   int         `json:"max_targets"`
}

type Candidate struct {
	URL            string         `json:"url"`
	Method         string         `json:"method"`
	Location       string         `json:"location"`
	WordlistPath   string         `json:"wordlist_path"`
	WordlistSHA256 string         `json:"wordlist_sha256"`
	Argv           []string       `json:"argv"`
	RequestBudget  int            `json:"request_budget"`
	Scope          CandidateScope `json:"scope"`
}

type CandidateScope struct {
	Classification string `json:"classification"`
	RuleID         string `json:"rule_id"`
	Reason         string `json:"reason"`
}
