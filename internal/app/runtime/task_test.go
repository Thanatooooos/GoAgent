package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"local/rag-project/internal/app/runtime/capability"
)

type taskJournalStub struct {
	events    []TaskEvent
	appendErr error
}

func (s *taskJournalStub) Start(context.Context, TaskRequest) (TaskSession, error) {
	return TaskSession{ID: "task-session"}, nil
}
func (s *taskJournalStub) Append(_ context.Context, _ TaskSession, event TaskEvent) error {
	s.events = append(s.events, event)
	return s.appendErr
}
func (s *taskJournalStub) Finish(context.Context, TaskSession, string) error { return nil }

func TestRunTaskExecutesOnlyNonConversationTools(t *testing.T) {
	tools := capability.NewRegistry()
	called := false
	if err := tools.Register(capability.Def{
		ID: "lookup", Description: "lookup", JSONSchema: json.RawMessage(`{"type":"object"}`),
		Validate: func(capability.Value) error { return nil },
		Describe: func(v capability.Value, _ capability.Context) (capability.Operation, error) {
			return capability.Operation{ID: "lookup", Input: v}, nil
		},
		Execute: func(_ capability.Value, _ capability.Context) (capability.Result, error) {
			called = true
			return capability.Result{Content: "evidence"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	model := &streamSequence{turns: []Turn{
		{ToolCalls: []ToolCall{{ID: "call-1", CapabilityID: "lookup", Arguments: capability.Value(`{}`)}}},
		{Content: "final"},
	}}
	journal := &taskJournalStub{}
	runtime := &Runtime{Model: model, Tools: tools, TaskJournal: journal}
	result, err := runtime.RunTask(context.Background(), TaskRequest{UserID: "u1", TaskType: "test", TaskID: "brief-1", Question: "write a brief"})
	if err != nil {
		t.Fatal(err)
	}
	if !called || result.Status != StatusCompleted || result.AssistantContent != "final" {
		t.Fatalf("result=%+v called=%v", result, called)
	}
	if len(model.requests) != 2 || len(model.requests[1].Messages) != 3 || model.requests[1].Messages[2].Content != "evidence" {
		t.Fatalf("second request = %+v", model.requests[1])
	}
	if len(journal.events) != 9 || journal.events[3].ToolState != ToolStatePending || journal.events[4].ToolState != ToolStateExecuting || journal.events[5].ToolState != ToolStateCompleted || journal.events[5].Detail != "evidence" {
		t.Fatalf("journal = %+v", journal.events)
	}
}

func TestRunTaskCanDisableTools(t *testing.T) {
	model := &streamSequence{turns: []Turn{{Content: "final"}}}
	runtime := &Runtime{Model: model, Tools: capability.NewRegistry()}
	_, err := runtime.RunTask(context.Background(), TaskRequest{UserID: "u1", TaskType: "test", TaskID: "brief-2", Question: "write a brief", Policy: Policy{DisableTools: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || len(model.requests[0].Tools) != 0 {
		t.Fatalf("model request = %+v", model.requests)
	}
}

func TestRunTaskFormatsJSONAfterToolLoop(t *testing.T) {
	tools := capability.NewRegistry()
	if err := tools.Register(capability.Def{ID: "lookup", Description: "lookup", JSONSchema: json.RawMessage(`{"type":"object"}`),
		Validate: func(capability.Value) error { return nil },
		Describe: func(v capability.Value, _ capability.Context) (capability.Operation, error) {
			return capability.Operation{Input: v}, nil
		},
		Execute: func(capability.Value, capability.Context) (capability.Result, error) {
			return capability.Result{Content: "official evidence"}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	model := &streamSequence{turns: []Turn{
		{ToolCalls: []ToolCall{{ID: "call-1", CapabilityID: "lookup", Arguments: capability.Value(`{}`)}}},
		{Content: "The official evidence supports a report."},
		{Content: `{"signal":"report","body":"Official evidence"}`},
	}}
	r := &Runtime{Model: model, Tools: tools, TaskJournal: &taskJournalStub{}}
	result, err := r.RunTask(context.Background(), TaskRequest{UserID: "u1", TaskType: "test", TaskID: "attempt-1", Question: "run task", JSONMode: true, PreserveRawAnswer: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 3 || model.requests[0].JSONMode || model.requests[1].JSONMode || !model.requests[2].JSONMode || len(model.requests[2].Tools) != 0 {
		t.Fatalf("JSON mode blocked tool execution: %+v", model.requests)
	}
	if model.requests[2].Messages[2].Content != "official evidence" || result.AssistantContent != `{"signal":"report","body":"Official evidence"}` || result.RuntimeSessionID != "task-session" {
		t.Fatalf("result lost evidence or final JSON: %+v", result)
	}
}

func TestRunTaskRejectsUnexpectedToolCallWhenToolsDisabled(t *testing.T) {
	model := &streamSequence{turns: []Turn{{ToolCalls: []ToolCall{{ID: "call-1", CapabilityID: "lookup", Arguments: capability.Value(`{}`)}}}}}
	runtime := &Runtime{Model: model, Tools: capability.NewRegistry()}
	_, err := runtime.RunTask(context.Background(), TaskRequest{UserID: "u1", TaskType: "test", TaskID: "brief-3", Question: "write a brief", Policy: Policy{DisableTools: true}})
	if err == nil || err.Error() != "tools are disabled for this task" {
		t.Fatalf("err = %v", err)
	}
}

func TestTaskToolsHideWriteCapabilitiesAndUnscopedKnowledge(t *testing.T) {
	tools := capability.NewRegistry()
	for _, id := range []string{capability.MemoryAddID, capability.RetrieveKnowledgeID, capability.WebSearchID,
		capability.CreateScheduledTaskID, capability.ListScheduledTasksID, capability.PauseScheduledTaskID} {
		if err := tools.Register(capability.Def{
			ID: id, Description: id, JSONSchema: json.RawMessage(`{"type":"object"}`),
			Validate: func(capability.Value) error { return nil },
			Describe: func(v capability.Value, _ capability.Context) (capability.Operation, error) {
				return capability.Operation{Input: v}, nil
			},
			Execute: func(capability.Value, capability.Context) (capability.Result, error) { return capability.Result{}, nil },
		}); err != nil {
			t.Fatal(err)
		}
	}
	runtime := &Runtime{Tools: tools}
	visible := runtime.taskTools(Policy{AllowWebSearch: true, AllowKnowledgeRetrieval: true, AllowScheduledTasks: true}, false, nil)
	if len(visible) != 1 || visible[0].ID != capability.WebSearchID {
		t.Fatalf("unscoped tools = %+v", visible)
	}
	visible = runtime.taskTools(Policy{AllowWebSearch: true, AllowKnowledgeRetrieval: true}, true, nil)
	if len(visible) != 2 {
		t.Fatalf("scoped tools = %+v", visible)
	}
	visible = runtime.taskTools(Policy{AllowWebSearch: true, AllowKnowledgeRetrieval: true}, true, []string{capability.WebSearchID})
	if len(visible) != 1 || visible[0].ID != capability.WebSearchID {
		t.Fatalf("allowlisted tools = %+v", visible)
	}
}

func TestRunTaskRejectsToolOutsideExplicitScope(t *testing.T) {
	model := &streamSequence{turns: []Turn{{ToolCalls: []ToolCall{{ID: "call-1", CapabilityID: "memory_add", Arguments: capability.Value(`{}`)}}}}}
	runtime := &Runtime{Model: model, Tools: capability.NewRegistry()}
	_, err := runtime.RunTask(context.Background(), TaskRequest{
		UserID: "u1", TaskType: "scheduled", TaskID: "attempt-1", ScheduledTaskID: "task-1", ConfigVersion: 1, OccurrenceID: "occurrence-1", AttemptID: "attempt-1", Question: "run task",
		AllowedToolIDs: []string{capability.WebSearchID}, Policy: Policy{AllowWebSearch: true},
	})
	if err == nil || err.Error() != `tool "memory_add" is outside task scope` {
		t.Fatalf("unexpected tool error = %v", err)
	}
}

func TestRunTaskCannotCompleteWhenJournalAppendFails(t *testing.T) {
	model := &streamSequence{turns: []Turn{{Content: "final"}}}
	runtime := &Runtime{Model: model, Tools: capability.NewRegistry(), TaskJournal: &taskJournalStub{appendErr: errors.New("storage unavailable")}}
	result, err := runtime.RunTask(context.Background(), TaskRequest{UserID: "u1", TaskType: "test", TaskID: "attempt-1", Question: "run task"})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}
