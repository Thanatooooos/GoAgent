package planexecute

const (
	defaultMaxReplans = 1

	completionPolicySearchResults       = "expect_search_results"
	completionPolicyFetchResults        = "expect_fetch_results"
	completionPolicyEvidence            = "expect_evidence"
	completionPolicyStructuredOutput    = "expect_structured_output"
	completionPolicyNonEmptyObservation = "expect_non_empty_observation"

	failurePolicyReplan  = "replan"
	failurePolicyDegrade = "degrade"

	branchExecute  = "execute_step"
	branchFinalize = "finalize"
	branchContinue = "continue"
	branchReplan   = "replan"
	branchDegrade  = "degrade"

	reasonPlanBuilt            = "plan_built"
	reasonNoActiveStep         = "no_active_step"
	reasonSearchResultsReady   = "search_results_ready"
	reasonSearchResultsMissing = "search_results_missing"
	reasonFetchResultsReady    = "fetch_results_ready"
	reasonFetchResultsMissing  = "fetch_results_missing"
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
