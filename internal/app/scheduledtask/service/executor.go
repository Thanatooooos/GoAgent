package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/scheduledtask/domain"
)

type Executor struct {
	Runtime runtime.TaskRuntime
	Access  runtime.KnowledgeBaseAccessResolver
}

type ExecutionInput struct {
	Task         domain.Task
	Version      domain.Version
	AttemptID    string
	OccurrenceID string
	ScheduledAt  time.Time
	History      string // Only this task's accepted run state.
}

type ExecutionResult struct {
	Outcome          domain.Outcome
	RuntimeSessionID string
}

func (e Executor) Run(ctx context.Context, input ExecutionInput) (ExecutionResult, error) {
	if e.Runtime == nil {
		return ExecutionResult{}, fmt.Errorf("task runtime is required")
	}
	if strings.TrimSpace(input.Task.ID) == "" || strings.TrimSpace(input.Task.UserID) == "" ||
		strings.TrimSpace(input.AttemptID) == "" || strings.TrimSpace(input.OccurrenceID) == "" || input.ScheduledAt.IsZero() {
		return ExecutionResult{}, fmt.Errorf("task, user, attempt, and scheduled time are required")
	}
	if err := input.Version.Validate(); err != nil {
		return ExecutionResult{}, err
	}
	if input.Version.TaskID != input.Task.ID || input.Version.Number != input.Task.CurrentVersion {
		return ExecutionResult{}, fmt.Errorf("task version does not match current task")
	}
	if err := e.CheckAccess(ctx, input.Task.UserID, input.Version); err != nil {
		return ExecutionResult{}, err
	}
	tools := input.Version.AllowedToolIDs
	policy := runtime.Policy{
		DisableTools:            len(tools) == 0,
		AllowWebSearch:          containsTool(tools, capability.WebSearchID) || containsTool(tools, capability.WebFetchID),
		AllowKnowledgeRetrieval: containsTool(tools, capability.RetrieveKnowledgeID),
	}
	system := []string{input.Version.Prompt, scheduledTaskExecutionInstruction}
	gapStart := input.Version.ConfirmedAt
	if input.Task.LastAttemptedAt.After(gapStart) {
		gapStart = input.Task.LastAttemptedAt
	}
	system = append(system, scheduledTaskGapInstructionPrefix+gapStart.UTC().Format(time.RFC3339)+scheduledTaskGapInstructionSuffix)
	switch input.Version.ConditionKind {
	case domain.ConditionEvent:
		system = append(system, scheduledTaskEventInstructionPrefix+input.Version.ConfirmedAt.UTC().Format(time.RFC3339)+scheduledTaskEventInstructionSuffix)
	case domain.ConditionState:
		system = append(system, scheduledTaskStateInstruction)
	}
	question := scheduledTaskExecutionRequestPrefix + input.ScheduledAt.UTC().Format(time.RFC3339) + scheduledTaskResultInstruction
	if input.Version.DailyBrief != nil {
		system = append(system, dailyBriefInstructions(*input.Version.DailyBrief))
		loc, _ := time.LoadLocation(input.Version.Schedule.Timezone)
		question = dailyBriefExecutionRequestPrefix + input.ScheduledAt.In(loc).Format("2006-01-02") + dailyBriefResultInstruction
	}
	result, err := e.Runtime.RunTask(ctx, runtime.TaskRequest{
		UserID:            input.Task.UserID,
		TaskType:          "scheduled",
		TaskID:            input.AttemptID,
		ScheduledTaskID:   input.Task.ID,
		ConfigVersion:     input.Version.Number,
		OccurrenceID:      input.OccurrenceID,
		AttemptID:         input.AttemptID,
		Question:          question,
		System:            system,
		KnowledgeBaseIDs:  input.Version.KnowledgeBaseIDs,
		AllowedWebDomains: input.Version.AllowedWebDomains,
		AllowedToolIDs:    tools,
		PreserveRawAnswer: true,
		JSONMode:          true,
		Policy:            policy,
		Sources: []runtime.TaskSource{{Key: "scheduled-task/history", Render: func(runtime.TaskRequest) string {
			return strings.TrimSpace(input.History)
		}}},
	})
	if err != nil {
		return ExecutionResult{RuntimeSessionID: result.RuntimeSessionID}, err
	}
	outcome, err := domain.ParseOutcome(result.AssistantContent)
	if err != nil {
		return ExecutionResult{RuntimeSessionID: result.RuntimeSessionID}, err
	}
	if input.Version.ReportMode == domain.ReportAlways && outcome.Signal == domain.SignalNoReport {
		return ExecutionResult{RuntimeSessionID: result.RuntimeSessionID}, fmt.Errorf("always-report task returned no_report")
	}
	if input.Version.DailyBrief != nil {
		outcome, _, err = NormalizeDailyBrief(outcome, *input.Version.DailyBrief)
		if err != nil {
			return ExecutionResult{RuntimeSessionID: result.RuntimeSessionID}, err
		}
	}
	return ExecutionResult{Outcome: outcome, RuntimeSessionID: result.RuntimeSessionID}, nil
}

// CheckAccess is used both before execution and immediately before publication.
// A report obtained before knowledge access was revoked must not be delivered.
func (e Executor) CheckAccess(ctx context.Context, userID string, version domain.Version) error {
	if len(version.KnowledgeBaseIDs) == 0 {
		return nil
	}
	if e.Access == nil {
		return fmt.Errorf("knowledge base access resolver is required")
	}
	allowed, err := e.Access.AccessibleIDs(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range version.KnowledgeBaseIDs {
		if !containsTool(allowed, id) {
			return fmt.Errorf("knowledge base %q is no longer accessible", id)
		}
	}
	return nil
}

func containsTool(ids []string, wanted string) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}
