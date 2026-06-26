package runtime

// Decision is the canonical runtime-owned decision vocabulary emitted by the
// runtime engine and shared by replay, projection, and outward service mapping.
type Decision string

const (
	DecisionContinue     Decision = "continue"
	DecisionWaitApproval Decision = "wait_approval"
	DecisionResume       Decision = "resume"
	DecisionReject       Decision = "reject"
	DecisionRetry        Decision = "retry"
	DecisionReplan       Decision = "replan"
	DecisionDegrade      Decision = "degrade"
	DecisionComplete     Decision = "complete"
	DecisionFail         Decision = "fail"
)
