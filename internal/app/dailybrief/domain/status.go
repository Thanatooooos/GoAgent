package domain

const (
	IssueStatusGenerating = "generating"
	IssueStatusReady      = "ready"
	IssueStatusFailed     = "failed"
)

const (
	GenerationRunStatusRunning   = "running"
	GenerationRunStatusSucceeded = "succeeded"
	GenerationRunStatusDegraded  = "degraded"
	GenerationRunStatusFailed    = "failed"
)

const (
	GenerationRunTriggerTypeScheduled = "scheduled"
	GenerationRunTriggerTypeRetry     = "retry"
)

var issueStatusSet = newStringSet([]string{
	IssueStatusGenerating,
	IssueStatusReady,
	IssueStatusFailed,
})

var generationRunStatusSet = newStringSet([]string{
	GenerationRunStatusRunning,
	GenerationRunStatusSucceeded,
	GenerationRunStatusDegraded,
	GenerationRunStatusFailed,
})

var generationRunTriggerTypeSet = newStringSet([]string{
	GenerationRunTriggerTypeScheduled,
	GenerationRunTriggerTypeRetry,
})

func IsValidIssueStatus(status string) bool {
	_, ok := issueStatusSet[status]
	return ok
}

func IsValidGenerationRunStatus(status string) bool {
	_, ok := generationRunStatusSet[status]
	return ok
}

func IsValidGenerationRunTriggerType(triggerType string) bool {
	_, ok := generationRunTriggerTypeSet[triggerType]
	return ok
}

func newStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
