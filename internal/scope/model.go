package scope

type Classification string

const (
	InScope    Classification = "in_scope"
	OutOfScope Classification = "out_of_scope"
	Unknown    Classification = "unknown"
)

type Config struct {
	Mode           string `json:"mode" yaml:"mode"`
	Roots          []Root `json:"roots" yaml:"roots"`
	ExternalPolicy string `json:"external_policy" yaml:"external_policy"`
}

type Root struct {
	ID    string `json:"id,omitempty" yaml:"id,omitempty"`
	Kind  string `json:"kind" yaml:"kind"`
	Value string `json:"value" yaml:"value"`
}

type Decision struct {
	Classification Classification `json:"classification"`
	RuleID         *string        `json:"rule_id"`
	Reason         string         `json:"reason"`
}

func (decision Decision) AllowedForActive() bool {
	return decision.Classification == InScope
}
