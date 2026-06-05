package planexecute

const (
	defaultMaxReplans = 1

	branchExecute  = "execute_step"
	branchFinalize = "finalize"
	branchContinue = "continue"
	branchReplan   = "replan"
	branchDegrade  = "degrade"

	reasonPlanBuilt            = "plan_built"
	reasonNoActiveStep         = "no_active_step"
	reasonSearchResultsReady   = "search_results_ready"
	reasonSearchResultsMissing = "search_results_missing"
	reasonFetchEvidenceReady   = "fetched_readable_evidence"
	reasonFetchEvidenceMissing = "no_readable_evidence"
	reasonPlanCompleted        = "plan_completed"
	reasonPlanFailed           = "plan_failed"
	progressPlanBuilt          = "plan_built"
	progressStepCompleted      = "step_completed"
	progressPlanReplanned      = "plan_replanned"
	progressPlanDegraded       = "plan_degraded"
	progressPlanFinalized      = "plan_finalized"
)
