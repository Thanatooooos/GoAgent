package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/scheduledtask/domain"
)

type runtimeStub struct {
	request runtime.TaskRequest
	answer  string
}

type accessStub []string

func (s accessStub) AccessibleIDs(context.Context, string) ([]string, error) { return s, nil }

func (s *runtimeStub) RunTask(_ context.Context, request runtime.TaskRequest) (runtime.RunResult, error) {
	s.request = request
	return runtime.RunResult{Status: runtime.StatusCompleted, AssistantContent: s.answer, RuntimeSessionID: "session-1"}, nil
}

func TestExecutorUsesConfirmedTaskScopeAndIsolatedHistory(t *testing.T) {
	stub := &runtimeStub{answer: `{"signal":"no_report"}`}
	input := executionInput()
	input.Version.KnowledgeBaseIDs = []string{"kb-1"}
	input.Version.AllowedWebDomains = []string{"sports.example"}
	input.Version.AllowedToolIDs = []string{capability.WebSearchID, capability.RetrieveKnowledgeID}
	input.History = "Only this task's prior reports"
	result, err := (Executor{Runtime: stub, Access: accessStub{"kb-1"}}).Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Signal != domain.SignalNoReport || stub.request.TaskID != "attempt-1" || stub.request.TaskType != "scheduled" || stub.request.ScheduledTaskID != input.Task.ID || stub.request.ConfigVersion != 1 || stub.request.OccurrenceID != "occurrence-1" || stub.request.AttemptID != input.AttemptID {
		t.Fatalf("result = %+v, request = %+v", result, stub.request)
	}
	if !stub.request.JSONMode || !stub.request.PreserveRawAnswer || len(stub.request.System) == 0 || stub.request.System[0] != input.Version.Prompt ||
		len(stub.request.KnowledgeBaseIDs) != 1 || stub.request.KnowledgeBaseIDs[0] != "kb-1" ||
		len(stub.request.AllowedWebDomains) != 1 || !strings.Contains(stub.request.Sources[0].Render(stub.request), input.History) {
		t.Fatalf("task request lost confirmed scope: %+v", stub.request)
	}
}

func TestExecutorRejectsNoReportForReminder(t *testing.T) {
	stub := &runtimeStub{answer: `{"signal":"no_report"}`}
	input := executionInput()
	input.Version.ReportMode = domain.ReportAlways
	input.Version.ConditionKind = domain.ConditionNone
	if _, err := (Executor{Runtime: stub}).Run(context.Background(), input); err == nil {
		t.Fatal("expected no_report to fail for reminder")
	}
}

func TestExecutorRejectsUnapprovedTool(t *testing.T) {
	stub := &runtimeStub{answer: `{"signal":"report","body":"done"}`}
	input := executionInput()
	input.Version.AllowedToolIDs = []string{capability.MemoryAddID}
	if _, err := (Executor{Runtime: stub}).Run(context.Background(), input); err == nil {
		t.Fatal("expected a read-only tool policy error")
	}
}

func executionInput() ExecutionInput {
	at := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	return ExecutionInput{
		Task: domain.Task{ID: "task-1", UserID: "user-1", CurrentVersion: 1},
		Version: domain.Version{
			TaskID: "task-1", Number: 1, Prompt: "Check official schedule", ReportMode: domain.ReportOnCondition, ConfirmedAt: at,
			ConditionKind: domain.ConditionEvent,
			Schedule:      domain.Schedule{Kind: domain.ScheduleDaily, Timezone: "UTC", LocalTime: "08:00"},
		},
		AttemptID: "attempt-1", OccurrenceID: "occurrence-1", ScheduledAt: at,
	}
}
