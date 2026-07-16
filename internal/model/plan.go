package model

type Plan struct {
	PlanVersion            string      `json:"plan_version"`
	RunID                  string      `json:"run_id"`
	CreatedAt              string      `json:"created_at"`
	Inputs                 PlanInputs  `json:"inputs"`
	CanonicalizationPolicy string      `json:"canonicalization_policy"`
	SchemaVersion          string      `json:"schema_version"`
	Tools                  []ToolPlan  `json:"tools"`
	Limits                 PlanLimits  `json:"limits"`
	EnvironmentAllowlist   []string    `json:"environment_allowlist"`
	Environment            []string    `json:"environment"`
	WorkspaceRoot          string      `json:"workspace_root"`
	Display                PlanDisplay `json:"-"`
}

type PlanInputs struct {
	Target         string   `json:"target"`
	Seeds          []string `json:"seeds"`
	ScopePath      string   `json:"scope_path"`
	ScopeSHA256    string   `json:"scope_sha256"`
	Profile        string   `json:"profile"`
	WordlistPath   string   `json:"wordlist_path,omitempty"`
	WordlistSHA256 string   `json:"wordlist_sha256,omitempty"`
}

type ToolPlan struct {
	Name          string     `json:"name"`
	ResolvedPath  string     `json:"resolved_path"`
	Version       string     `json:"version"`
	Binary        ToolBinary `json:"binary"`
	ActivityClass string     `json:"activity_class"`
	Argv          []string   `json:"argv"`
	Limits        ToolLimits `json:"limits"`
	OutputPaths   []string   `json:"output_paths"`
}

type ToolBinary struct {
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type ToolLimits struct {
	RatePerSecond  int `json:"rate_limit_per_second"`
	Concurrency    int `json:"concurrency"`
	Parallelism    int `json:"parallelism"`
	TimeoutSeconds int `json:"timeout_seconds"`
}

type PlanLimits struct {
	ArjunMaxTargets    int `json:"arjun_max_targets"`
	ArjunRequestBudget int `json:"arjun_request_budget,omitempty"`
}

type PlanDisplay struct {
	Title         string
	TerminalStyle string
}
