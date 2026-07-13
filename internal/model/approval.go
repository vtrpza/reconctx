package model

type ApprovalRecord struct {
	Phase          string `json:"phase"`
	ApprovedDigest string `json:"approved_digest"`
	OperatorLabel  string `json:"operator_label"`
	Decision       string `json:"decision"`
	Supersedes     string `json:"supersedes,omitempty"`
	CreatedAt      string `json:"created_at"`
}
