package reactive

import (
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	progressEvidenceGained       = "progress_evidence_gained"
	progressNewSourcesFound      = "progress_new_sources_found"
	progressRetryableFetchFailed = "progress_retryable_fetch_failure"
	progressNone                 = "progress_none"
)

type observePolicyResult struct {
	Branch                string
	Reason                string
	ApprovalCapability    string
	ApprovalRerunNode     string
	ProgressKind          string
	Confidence            float64
	NextQuery             string
	PreferredURLs         []string
	AvoidURLs             []string
	NewURLCount           int
	NewEvidenceCount      int
	NoProgressRounds      int
	RetryableFetchFailure bool
	Answerable            bool
}

func evaluateObservePolicy(session *agentruntime.RuntimeSession, outputMode string, capabilityPolicy capabilityRuntimePolicy) observePolicyResult {
	newEvidenceItems := buildNewEvidence(session)
	newURLCount := countNewURLs(session)
	newEvidenceCount := len(newEvidenceItems)
	retryableFetchFailure := hasRetryableFetchFailure(session)
	retryableSearchFailure := hasRetryableSearchFailure(session)
	progressKind := assessProgressKind(newURLCount, newEvidenceCount, retryableFetchFailure)
	if retryableSearchFailure && progressKind == progressNone {
		progressKind = progressRetryableSearchFailed
	}
	noProgressRounds := nextNoProgressRounds(session, progressKind)
	answerable := len(session.Snapshot.Evidence.Items)+newEvidenceCount > 0

	result := observePolicyResult{
		ProgressKind:          progressKind,
		NewURLCount:           newURLCount,
		NewEvidenceCount:      newEvidenceCount,
		NoProgressRounds:      noProgressRounds,
		RetryableFetchFailure: retryableFetchFailure,
		Answerable:            answerable,
	}

	switch {
	case answerable:
		result.Branch = terminalBranchForMode(session, outputMode)
		result.Reason = "fetched_readable_evidence"
		result.Confidence = 0.90
	case retryDirectiveApplies(capabilityPolicy, session, &result):
	case !withinIterationBudget(session):
		result.Branch = branchDegrade
		result.Reason = "iteration_budget_exhausted"
		result.Confidence = 0.45
	case !hasFetchableURLs(session):
		result.Branch = branchDegrade
		result.Reason = "no_new_fetchable_urls"
		result.Confidence = 0.45
	case progressKind == progressNewSourcesFound:
		result.Branch = branchContinue
		result.Reason = "need_more_sources"
		result.Confidence = 0.65
	case progressKind == progressRetryableFetchFailed && noProgressRounds < 2:
		result.Branch = branchContinue
		result.Reason = "fetch_failed_retryable"
		result.Confidence = 0.55
	case noProgressRounds >= 2:
		result.Branch = branchDegrade
		result.Reason = "no_progress_across_rounds"
		result.Confidence = 0.50
	default:
		result.Branch = branchDegrade
		result.Reason = "no_new_fetchable_urls"
		result.Confidence = 0.45
	}

	return result
}

func terminalBranchForMode(session *agentruntime.RuntimeSession, outputMode string) string {
	if effectiveOutputMode(session, outputMode) == agentstate.OutputModeHandoff {
		return branchHandoff
	}
	return branchAnswer
}

func assessProgressKind(newURLCount int, newEvidenceCount int, retryableFetchFailure bool) string {
	switch {
	case newEvidenceCount > 0:
		return progressEvidenceGained
	case newURLCount > 0:
		return progressNewSourcesFound
	case retryableFetchFailure:
		return progressRetryableFetchFailed
	default:
		return progressNone
	}
}

func retryDirectiveApplies(capabilityPolicy capabilityRuntimePolicy, session *agentruntime.RuntimeSession, result *observePolicyResult) bool {
	if result == nil {
		return false
	}
	branch, reason, confidence, progressKind, capabilityName, rerunNode, applied := capabilityPolicy.retryDirective(session)
	if !applied {
		return false
	}
	result.Branch = branch
	result.Reason = reason
	result.Confidence = confidence
	result.ApprovalCapability = capabilityName
	result.ApprovalRerunNode = rerunNode
	if progressKind != "" {
		result.ProgressKind = progressKind
	}
	if progressKind == progressRetryableFetchFailed || progressKind == progressRetryableSearchFailed {
		result.RetryableFetchFailure = true
	}
	return true
}
