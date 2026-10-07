package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/core/citation"
	"local/rag-project/internal/app/runtime/capability"
	fwlog "local/rag-project/internal/framework/log"
)

var ErrTaskToolCallBudgetExhausted = errors.New("runtime tool-call budget exhausted")
var ErrTaskTurnBudgetExhausted = errors.New("runtime turn budget exhausted")

// TaskRequest is a non-conversational runtime invocation.  It deliberately
// has no conversation or message identity: scheduled work must not fabricate
// chat records merely to use the agent loop.
type TaskRequest struct {
	UserID            string
	TaskType          string
	TaskID            string // Unique for one runtime attempt, not the recurring task.
	ScheduledTaskID   string
	ConfigVersion     int
	OccurrenceID      string
	AttemptID         string
	Question          string
	System            []string
	Sources           []TaskSource
	KnowledgeBaseIDs  []string
	AllowedWebDomains []string // Empty means the task's confirmed public-web scope is broad.
	AllowedToolIDs    []string // Non-empty for scheduled tasks; existing callers retain their catalog.
	PreserveRawAnswer bool     // Structured task output is parsed before citation rendering.
	JSONMode          bool     // Request structured JSON content from the model provider.
	Policy            Policy
}

type TaskSession struct{ ID string }
type TaskEvent struct {
	EventType, ToolCallID, ToolName, ToolState, Detail string
	Evidence                                           []EvidenceRef
}
type TaskJournal interface {
	Start(context.Context, TaskRequest) (TaskSession, error)
	Append(context.Context, TaskSession, TaskEvent) error
	Finish(context.Context, TaskSession, string) error
}

// TaskSource is a stable, named context input for a non-conversational task.
// It keeps domain facts separate from the task instruction and tool history.
type TaskSource struct {
	Key    string
	Render func(TaskRequest) string
}

func (r TaskRequest) Validate() error {
	if strings.TrimSpace(r.UserID) == "" {
		return fmt.Errorf("task user id is required")
	}
	if strings.TrimSpace(r.TaskType) == "" {
		return fmt.Errorf("task type is required")
	}
	if strings.TrimSpace(r.TaskID) == "" {
		return fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(r.Question) == "" {
		return fmt.Errorf("task question is required")
	}
	if r.TaskType == "scheduled" && !r.Policy.DisableTools && len(r.AllowedToolIDs) == 0 {
		return fmt.Errorf("scheduled task requires an explicit tool scope")
	}
	if r.TaskType == "scheduled" && (strings.TrimSpace(r.ScheduledTaskID) == "" || r.ConfigVersion < 1 || strings.TrimSpace(r.OccurrenceID) == "" || r.AttemptID != r.TaskID) {
		return fmt.Errorf("scheduled task requires task, version, occurrence, and matching attempt identities")
	}
	return nil
}

// TaskRuntime is the reusable, non-chat runtime boundary used by scheduled
// product workflows such as Daily Brief.
type TaskRuntime interface {
	RunTask(context.Context, TaskRequest) (RunResult, error)
}

// RunTask runs the same streamed model/tool loop as chat, but retains its
// transient turns in memory. Its owning domain persists the task result.
func (r *Runtime) RunTask(ctx context.Context, request TaskRequest) (runResult RunResult, runErr error) {
	if r == nil || r.Model == nil || r.Tools == nil {
		return RunResult{}, fmt.Errorf("runtime is not configured")
	}
	if err := request.Validate(); err != nil {
		return RunResult{}, err
	}
	ctx = fwlog.NewContext(ctx, "runtimeTaskId", request.TaskID, "runtimeTaskUserId", request.UserID)
	var journalSession TaskSession
	var journalErr error
	defer func() {
		runResult.RuntimeSessionID = journalSession.ID
		if journalErr != nil {
			runResult.Status = StatusFailed
			runErr = errors.Join(runErr, fmt.Errorf("runtime task journal: %w", journalErr))
		}
	}()
	record := func(eventCtx context.Context, event TaskEvent) {
		if err := r.taskEvent(eventCtx, journalSession, event); err != nil {
			journalErr = errors.Join(journalErr, err)
		}
	}
	if r.TaskJournal != nil {
		var startErr error
		journalSession, startErr = r.TaskJournal.Start(ctx, request)
		if startErr != nil {
			return RunResult{}, fmt.Errorf("start runtime task journal: %w", startErr)
		}
		record(ctx, TaskEvent{EventType: "task_started"})
	}
	finish := func(status string) {
		if journalSession.ID != "" {
			if journalErr != nil {
				status = StatusFailed
			}
			if err := r.TaskJournal.Finish(context.WithoutCancel(ctx), journalSession, status); err != nil {
				journalErr = errors.Join(journalErr, err)
			}
		}
	}
	fwlog.FromContext(ctx).Infow("runtime task started")
	policy := request.Policy
	turns, _ := limits(policy)
	calls := 0
	messages := []ModelMessage{{Role: ModelRoleUser, Content: request.Question}}
	citations := citation.NewRegistry()
	var evidence []EvidenceRef
	system := append([]string(nil), request.System...)
	for _, source := range request.Sources {
		if source.Render == nil || strings.TrimSpace(source.Key) == "" {
			continue
		}
		if block := strings.TrimSpace(source.Render(request)); block != "" {
			system = append(system, block)
		}
	}
	modelTools := r.taskTools(policy, len(request.KnowledgeBaseIDs) > 0, request.AllowedToolIDs)
	// Some providers suppress function calls under JSON response mode. Keep the
	// agent's tool loop unrestricted by that transport format, then format its
	// final conclusion using the same instructions and accumulated evidence.
	formatAfterTools := request.JSONMode && len(modelTools) > 0
	if formatAfterTools {
		system = append(system, taskJSONToolLoopInstruction)
	}
	for turnNumber := 0; turnNumber < turns; turnNumber++ {
		fwlog.FromContext(ctx).Infow("runtime task model turn started", "turn", turnNumber+1)
		record(ctx, TaskEvent{EventType: EventModelTurnStarted, Detail: fmt.Sprintf("turn=%d", turnNumber+1)})
		turn, err := r.Model.Stream(ctx, ModelRequest{System: system, Messages: messages, Tools: modelTools, JSONMode: request.JSONMode && !formatAfterTools}, func(ModelEvent) error { return nil })
		if err != nil {
			record(context.WithoutCancel(ctx), TaskEvent{EventType: EventModelTurnFinished, Detail: "failed"})
			finish(StatusFailed)
			fwlog.FromContext(ctx).Warnw("runtime task model turn failed", "turn", turnNumber+1, "error", err)
			if ctx.Err() != nil {
				return RunResult{Status: StatusCancelled}, ctx.Err()
			}
			return RunResult{Status: StatusFailed}, fmt.Errorf("stream model turn: %w", err)
		}
		messages = append(messages, ModelMessage{Role: ModelRoleAssistant, Content: turn.Content, ToolCalls: turn.ToolCalls})
		record(ctx, TaskEvent{EventType: EventModelTurnFinished, Detail: fmt.Sprintf("tool_calls=%d", len(turn.ToolCalls))})
		if len(turn.ToolCalls) == 0 {
			if formatAfterTools {
				record(ctx, TaskEvent{EventType: "answer_draft", Detail: turn.Content})
				messages = append(messages, ModelMessage{Role: ModelRoleUser, Content: taskJSONFinalInstruction})
				record(ctx, TaskEvent{EventType: EventModelTurnStarted, Detail: "final_json"})
				turn, err = r.Model.Stream(ctx, ModelRequest{System: system, Messages: messages, JSONMode: true}, func(ModelEvent) error { return nil })
				if err != nil || len(turn.ToolCalls) > 0 {
					record(context.WithoutCancel(ctx), TaskEvent{EventType: EventModelTurnFinished, Detail: "final_json failed"})
					finish(StatusFailed)
					if err == nil {
						err = fmt.Errorf("final JSON formatting returned a tool call")
					}
					return RunResult{Status: StatusFailed}, fmt.Errorf("format task result: %w", err)
				}
				record(ctx, TaskEvent{EventType: EventModelTurnFinished, Detail: "final_json completed"})
			}
			answer := strings.TrimSpace(turn.Content)
			if !request.PreserveRawAnswer {
				answer = strings.TrimSpace(citations.ExpandText(answer, true))
			}
			if answer == "" {
				finish(StatusFailed)
				return RunResult{Status: StatusFailed}, fmt.Errorf("model returned neither content nor tool calls")
			}
			fwlog.FromContext(ctx).Infow("runtime task completed", "turns", turnNumber+1, "toolCalls", calls, "answerBytes", len(answer))
			record(ctx, TaskEvent{EventType: EventAnswerFinal, Detail: answer, Evidence: evidence})
			finish(StatusCompleted)
			return RunResult{Status: StatusCompleted, AssistantContent: answer, Evidence: evidence, RuntimeSessionID: journalSession.ID}, nil
		}
		for _, call := range turn.ToolCalls {
			if policy.DisableTools {
				finish(StatusFailed)
				return RunResult{Status: StatusFailed}, fmt.Errorf("tools are disabled for this task")
			}
			if !taskToolAllowed(call.CapabilityID, request.AllowedToolIDs) {
				finish(StatusFailed)
				return RunResult{Status: StatusFailed}, fmt.Errorf("tool %q is outside task scope", call.CapabilityID)
			}
			if calls >= policyMaxToolCalls(policy) {
				finish(StatusFailed)
				return RunResult{Status: StatusFailed}, ErrTaskToolCallBudgetExhausted
			}
			calls++
			fwlog.FromContext(ctx).Infow("runtime task tool started", "turn", turnNumber+1, "toolCallId", call.ID, "tool", call.CapabilityID)
			record(ctx, TaskEvent{EventType: EventToolPending, ToolCallID: call.ID, ToolName: call.CapabilityID, ToolState: ToolStatePending, Detail: string(call.Arguments)})
			toolContext := capability.Context{Context: ctx, UserID: request.UserID, Question: request.Question, KnowledgeBaseIDs: request.KnowledgeBaseIDs, AllowedWebDomains: request.AllowedWebDomains, AllowKnowledgeRetrieval: policy.AllowKnowledgeRetrieval && len(request.KnowledgeBaseIDs) > 0, AllowWebSearch: policy.AllowWebSearch, Citations: citations}
			def, _, prepareErr := r.Tools.Prepare(call.CapabilityID, call.Arguments, toolContext)
			if prepareErr != nil {
				state := ToolStateFailed
				if capability.IsDenied(prepareErr) {
					state = ToolStateDenied
				}
				record(ctx, TaskEvent{EventType: EventToolSettled, ToolCallID: call.ID, ToolName: call.CapabilityID, ToolState: state, Detail: prepareErr.Error()})
				fwlog.FromContext(ctx).Warnw("runtime task tool unavailable", "toolCallId", call.ID, "tool", call.CapabilityID, "error", prepareErr)
				messages = append(messages, ModelMessage{Role: ModelRoleTool, ToolCallID: call.ID, Content: taskToolUnavailablePrefix + prepareErr.Error()})
				continue
			}
			record(ctx, TaskEvent{EventType: EventToolExecuting, ToolCallID: call.ID, ToolName: call.CapabilityID, ToolState: ToolStateExecuting})
			result, executeErr := def.Execute(call.Arguments, toolContext)
			if executeErr != nil {
				record(ctx, TaskEvent{EventType: EventToolSettled, ToolCallID: call.ID, ToolName: call.CapabilityID, ToolState: ToolStateFailed, Detail: executeErr.Error()})
				fwlog.FromContext(ctx).Warnw("runtime task tool failed", "toolCallId", call.ID, "tool", call.CapabilityID, "error", executeErr)
				messages = append(messages, ModelMessage{Role: ModelRoleTool, ToolCallID: call.ID, Content: taskToolFailedPrefix + executeErr.Error()})
				continue
			}
			evidence = append(evidence, result.Evidence...)
			record(ctx, TaskEvent{EventType: EventToolSettled, ToolCallID: call.ID, ToolName: call.CapabilityID, ToolState: ToolStateCompleted, Detail: result.Content, Evidence: result.Evidence})
			fwlog.FromContext(ctx).Infow("runtime task tool completed", "toolCallId", call.ID, "tool", call.CapabilityID, "evidenceCount", len(result.Evidence))
			messages = append(messages, ModelMessage{Role: ModelRoleTool, ToolCallID: call.ID, Content: result.Content})
		}
	}
	finish(StatusFailed)
	return RunResult{Status: StatusFailed}, ErrTaskTurnBudgetExhausted
}

func (r *Runtime) taskEvent(ctx context.Context, session TaskSession, event TaskEvent) error {
	if r.TaskJournal == nil || session.ID == "" {
		return nil
	}
	return r.TaskJournal.Append(ctx, session, event)
}

func (r *Runtime) taskTools(policy Policy, hasKnowledgeScope bool, allowedToolIDs []string) []capability.ModelDefinition {
	if policy.DisableTools {
		return nil
	}
	available := make([]capability.ModelDefinition, 0)
	for _, tool := range r.Tools.List() {
		if !taskToolAllowed(tool.ID, allowedToolIDs) {
			continue
		}
		switch tool.ID {
		case capability.ArchiveConversationEpisodeID, capability.SearchConversationHistoryID,
			capability.MemoryAddID, capability.MemoryUpdateID, capability.MemoryDeleteID,
			capability.CreateScheduledTaskID, capability.ListScheduledTasksID, capability.PauseScheduledTaskID:
			continue
		case capability.RetrieveKnowledgeID:
			if !policy.AllowKnowledgeRetrieval || !hasKnowledgeScope {
				continue
			}
		case capability.WebSearchID, capability.WebFetchID:
			if !policy.AllowWebSearch {
				continue
			}
		}
		available = append(available, tool)
	}
	return available
}

func taskToolAllowed(id string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if item == id {
			return true
		}
	}
	return false
}

func policyMaxToolCalls(policy Policy) int { _, calls := limits(policy); return calls }

var _ TaskRuntime = (*Runtime)(nil)
