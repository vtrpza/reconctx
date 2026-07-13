package model

type RunState string

const (
	RunPlanned                    RunState = "planned"
	RunPreflightFailed            RunState = "preflight_failed"
	RunAwaitingCollectionApproval RunState = "awaiting_collection_approval"
	RunCollecting                 RunState = "collecting"
	RunNormalizingInitial         RunState = "normalizing_initial"
	RunAwaitingArjunApproval      RunState = "awaiting_arjun_approval"
	RunArjunSkipped               RunState = "arjun_skipped"
	RunDiscoveringParameters      RunState = "discovering_parameters"
	RunNormalizingFinal           RunState = "normalizing_final"
	RunCompiling                  RunState = "compiling"
	RunPartial                    RunState = "partial"
	RunSuccess                    RunState = "success"
	RunFailed                     RunState = "failed"
	RunInterrupted                RunState = "interrupted"
	RunCancelled                  RunState = "cancelled"
)

type Run struct {
	ID           string           `json:"id"`
	State        RunState         `json:"state"`
	PlanDigest   string           `json:"plan_digest"`
	QueueDigest  string           `json:"queue_digest,omitempty"`
	Approvals    []ApprovalRecord `json:"approvals"`
	CoverageGaps []string         `json:"coverage_gaps,omitempty"`
}
