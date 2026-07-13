package model

type RunState string

const (
	RunPlanned                    RunState = "planned"
	RunAwaitingCollectionApproval RunState = "awaiting_collection_approval"
	RunCollecting                 RunState = "collecting"
	RunAwaitingArjunApproval      RunState = "awaiting_arjun_approval"
	RunCompiling                  RunState = "compiling"
	RunPartial                    RunState = "partial"
	RunSuccess                    RunState = "success"
	RunFailed                     RunState = "failed"
	RunInterrupted                RunState = "interrupted"
)

type Run struct {
	ID         string           `json:"id"`
	State      RunState         `json:"state"`
	PlanDigest string           `json:"plan_digest"`
	Approvals  []ApprovalRecord `json:"approvals"`
}
