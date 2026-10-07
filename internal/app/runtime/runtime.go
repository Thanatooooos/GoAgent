package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/core/citation"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/persistence"
	fwlog "local/rag-project/internal/framework/log"
)

// Runtime owns one durable conversation execution. Its loop is deliberately
// shaped after Miso: sources, projected history, streamed turn, tool settlement.
type Runtime struct {
	ExecutionLeaseDuration time.Duration
	Model                  StreamModel
	Sources                *ContextSources
	History                ConversationHistory
	Tools                  *capability.Registry
	Lifecycle              *Lifecycle
	Episodes               persistence.EpisodeStore
	TaskJournal            TaskJournal
	// CoreMemoryContextLoader is invoked once per session and again after a
	// successful memory mutation. Sources only render the resulting snapshot.
	CoreMemoryContextLoader     func(context.Context, string) (string, error)
	DerivedProfileContextLoader func(context.Context, string) (string, error)
	// ContextTokenBudget is the configured prompt budget. A per-request policy
	// may narrow it; zero leaves history untouched.
	ContextTokenBudget int
}

func (r *Runtime) Admit(ctx context.Context, request RunRequest) error {
	if r == nil || r.Lifecycle == nil {
		return fmt.Errorf("runtime is not configured")
	}
	_, err := r.Lifecycle.StartSession(ctx, request)
	return err
}

// Replay re-emits durable execution facts for one user-owned task. It never
// resumes or re-runs execution; callers may use it only to refill a lost SSE
// cache before subscribing to future events.
func (r *Runtime) Replay(ctx context.Context, userID, taskID string, sink EventSink) (bool, error) {
	if r == nil || r.Lifecycle == nil || r.Lifecycle.store == nil {
		return false, fmt.Errorf("runtime is not configured")
	}
	session, err := r.Lifecycle.store.FindSessionByTraceID(ctx, strings.TrimSpace(userID), strings.TrimSpace(taskID))
	if err != nil {
		return false, err
	}
	if session.ID == "" {
		return false, nil
	}
	entries, err := r.Lifecycle.store.ListJournal(ctx, session.ID)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if sink != nil {
			if err := sink.Append(context.WithoutCancel(ctx), entry); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

func (r *Runtime) Run(ctx context.Context, request RunRequest, sink EventSink) (result RunResult, runErr error) {
	if r == nil || r.Model == nil || r.Tools == nil || r.Lifecycle == nil {
		return RunResult{}, fmt.Errorf("runtime is not configured")
	}
	if err := request.Validate(); err != nil {
		return RunResult{}, err
	}
	if request.publishAnswer {
		if _, ok := r.Lifecycle.store.(interface {
			ReadyAnswer(context.Context, JournalEntry) error
		}); !ok {
			return RunResult{}, fmt.Errorf("runtime store does not support answer publication")
		}
	}
	session, err := r.Lifecycle.StartSession(ctx, request)
	if err != nil {
		return RunResult{}, err
	}
	if session.Status != persistence.StatusRunning {
		return RunResult{}, fmt.Errorf("runtime session is already terminal; replay its result")
	}
	if request.publishAnswer {
		var stop func()
		ctx, stop, err = r.claimChatExecution(ctx, session)
		if err != nil {
			return RunResult{Status: StatusInterrupted, RuntimeSessionID: session.ID}, err
		}
		defer stop()
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr = fmt.Errorf("chat execution panic: %v", recovered)
			}
			if runErr != nil {
				if _, owned := persistence.CurrentExecution(ctx); owned {
					if result.RuntimeSessionID == session.ID && (result.Status == StatusFailed || result.Status == StatusCancelled) {
						return
					}
					if errors.Is(context.Cause(ctx), persistence.ErrExecutionLeaseLost) {
						return
					}
					status := StatusFailed
					if ctx.Err() != nil {
						status = StatusCancelled
					}
					closed, closeErr := r.finishOwnedFailure(ctx, sink, session, status, runErr)
					if closed.Status != StatusInterrupted {
						result = closed
						if closeErr != nil {
							runErr = closeErr
						}
					}
				}
			}
		}()
	}
	ctx = fwlog.NewContext(ctx, "runtimeSessionId", session.ID, "conversationId", session.ConversationID, "userMessageId", session.UserMessageID, "traceId", session.TraceID)
	fwlog.FromContext(ctx).Infow("runtime session started", "knowledgeBaseCount", len(request.KnowledgeBaseIDs))
	if err := r.recordAndEmit(ctx, sink, session, EventSessionStarted, ""); err != nil {
		return RunResult{}, err
	}
	if err := r.Lifecycle.RecoverUnsettledTools(ctx, session); err != nil {
		return r.finish(ctx, sink, session, StatusFailed, "", err)
	}
	runner := NewToolRunner(r.Tools, r.Lifecycle)
	citations := citation.NewRegistry()
	policy := request.EffectivePolicy()
	coreMemoryContext, err := r.loadCoreMemoryContext(ctx, request.UserID)
	if err != nil {
		return r.finish(ctx, sink, session, StatusFailed, "", err)
	}
	derivedProfileContext, err := r.loadDerivedProfileContext(ctx, request.UserID)
	if err != nil {
		return r.finish(ctx, sink, session, StatusFailed, "", err)
	}
	maxTurns, maxCalls := limits(policy)
	calls := 0
	archivedEpisode := false
	lastToolErrorKey := ""
	lastToolErrorCount := 0
	for step := 0; step < maxTurns; step++ {
		modelTools := r.modelTools(policy, request.Timezone, archivedEpisode)
		entries, err := r.Lifecycle.store.ListJournal(ctx, session.ID)
		if err != nil {
			return r.finish(ctx, sink, session, StatusFailed, "", err)
		}
		history, err := r.history(ctx, request)
		if err != nil {
			return r.finish(ctx, sink, session, StatusFailed, "", err)
		}
		history = append(history, ModelMessage{Role: ModelRoleUser, Content: request.Question})
		history, err = ProjectJournalHistory(history, entries)
		if err != nil {
			return r.finish(ctx, sink, session, StatusFailed, "", err)
		}
		system := append(r.Sources.Blocks(SourceContext{Request: request, Session: session, CoreMemoryContext: coreMemoryContext, DerivedProfileContext: derivedProfileContext}), citation.ProtocolPrompt(true))
		history, err = r.compactHistory(ctx, session, request, policy, system, modelTools, entries, history)
		if err != nil {
			return r.finish(ctx, sink, session, StatusFailed, "", err)
		}
		expander := citation.NewStreamExpander(citations, true)
		if err := r.recordModelTurnStarted(ctx, session, step+1, len(history), len(modelTools)); err != nil {
			return r.finish(ctx, sink, session, StatusFailed, "", err)
		}
		fwlog.FromContext(ctx).Infow("runtime model turn started", "turn", step+1, "historyMessages", len(history), "tools", len(modelTools))
		turn, err := r.Model.Stream(ctx, ModelRequest{Thinking: &request.DeepThinking, System: system, Messages: history, Tools: modelTools}, func(event ModelEvent) error {
			switch event.Kind {
			case ModelEventContent:
				if text := expander.Feed(event.Text); text != "" {
					return r.recordAndEmit(ctx, sink, session, EventAnswerDelta, text)
				}
				return nil
			case ModelEventThinking:
				return r.recordAndEmit(ctx, sink, session, EventThinkingDelta, event.Text)
			}
			return nil
		})
		if err != nil {
			fwlog.FromContext(ctx).Warnw("runtime model turn failed", "turn", step+1, "error", err)
			if traceErr := r.recordModelTurnFailed(context.WithoutCancel(ctx), session, step+1, err); traceErr != nil {
				return r.finish(ctx, sink, session, StatusFailed, "", traceErr)
			}
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return r.finish(ctx, sink, session, StatusCancelled, "", context.Canceled)
			}
			return r.finish(ctx, sink, session, StatusFailed, "", fmt.Errorf("stream model turn: %w", err))
		}
		if err := r.recordModelTurnFinished(ctx, session, step+1, turn); err != nil {
			return r.finish(ctx, sink, session, StatusFailed, "", err)
		}
		if text := expander.Flush(); text != "" {
			if err := r.recordAndEmit(ctx, sink, session, EventAnswerDelta, text); err != nil {
				return r.finish(ctx, sink, session, StatusFailed, "", err)
			}
		}
		fwlog.FromContext(ctx).Infow("runtime model turn completed", "turn", step+1, "contentBytes", len(turn.Content), "thinkingBytes", len(turn.Thinking), "toolCalls", len(turn.ToolCalls), "finishReason", turn.FinishReason)
		if _, err := r.Lifecycle.RecordAssistantTurn(ctx, session, turn); err != nil {
			return r.finish(ctx, sink, session, StatusFailed, "", err)
		}
		if len(turn.ToolCalls) == 0 {
			if strings.TrimSpace(turn.Content) == "" {
				return r.finish(ctx, sink, session, StatusFailed, "", fmt.Errorf("model returned neither content nor tool calls"))
			}
			// Publish exactly the body streamed across all model turns. These
			// durable deltas already contain expanded citations, not thinking
			// or tool results.
			entries, err := r.Lifecycle.store.ListJournal(ctx, session.ID)
			if err != nil {
				return r.finish(ctx, sink, session, StatusFailed, "", err)
			}
			var body strings.Builder
			for _, entry := range entries {
				if entry.EventType == EventAnswerDelta {
					body.WriteString(entry.Detail)
				}
			}
			answer := strings.TrimSpace(body.String())
			if answer == "" {
				return r.finish(ctx, sink, session, StatusFailed, "", fmt.Errorf("model returned no visible answer"))
			}
			if request.publishAnswer {
				if ctx.Err() != nil {
					return r.finish(ctx, sink, session, StatusCancelled, "", ctx.Err())
				}
				if err := r.Lifecycle.ReadyAnswer(context.WithoutCancel(ctx), session, answer); err != nil {
					return RunResult{}, err
				}
				return RunResult{Status: StatusCompleted, AssistantContent: answer, RuntimeSessionID: session.ID}, nil
			}
			if err := r.recordAndEmit(ctx, sink, session, EventAnswerFinal, answer); err != nil {
				return r.finish(ctx, sink, session, StatusFailed, "", err)
			}
			return r.finish(ctx, sink, session, StatusCompleted, answer, nil)
		}
		for _, call := range turn.ToolCalls {
			if calls >= maxCalls {
				return r.finish(ctx, sink, session, StatusFailed, "", fmt.Errorf("runtime tool-call budget exhausted"))
			}
			calls++
			fwlog.FromContext(ctx).Infow("runtime tool started", "turn", step+1, "toolCallId", call.ID, "tool", call.CapabilityID)
			before, err := r.Lifecycle.store.ListJournal(ctx, session.ID)
			if err != nil {
				return r.finish(ctx, sink, session, StatusFailed, "", err)
			}
			operation, _, toolErr := runner.Run(ctx, session, call, capability.Context{Work: request.Work, Question: request.Question, KnowledgeBaseIDs: append([]string(nil), request.KnowledgeBaseIDs...), RetrieveSearchMode: policy.RetrieveSearchMode, Timezone: request.Timezone, AllowKnowledgeRetrieval: policy.AllowKnowledgeRetrieval, AllowMemoryRecall: policy.AllowMemoryRecall, AllowMemoryMutation: policy.AllowMemoryMutation, AllowWebSearch: policy.AllowWebSearch, AllowEpisodeArchive: policy.AllowEpisodeArchive && !archivedEpisode, AllowScheduledTasks: policy.AllowScheduledTasks, Citations: citations})
			if operation.ID == capability.ArchiveConversationEpisodeID && toolErr == nil {
				archivedEpisode = true
			}
			if toolErr != nil {
				fwlog.FromContext(ctx).Warnw("runtime tool settled with error", "toolCallId", call.ID, "tool", call.CapabilityID, "error", toolErr)
				errorKey := call.CapabilityID + "\x00" + toolErr.Error()
				if errorKey == lastToolErrorKey {
					lastToolErrorCount++
				} else {
					lastToolErrorKey, lastToolErrorCount = errorKey, 1
				}
			} else {
				lastToolErrorKey, lastToolErrorCount = "", 0
				fwlog.FromContext(ctx).Infow("runtime tool completed", "toolCallId", call.ID, "tool", call.CapabilityID)
				if isCoreMemoryMutation(operation.ID) {
					coreMemoryContext, err = r.loadCoreMemoryContext(ctx, request.UserID)
					if err != nil {
						return r.finish(ctx, sink, session, StatusFailed, "", err)
					}
				}
			}
			if err := r.emitNewJournal(ctx, sink, session.ID, len(before)); err != nil {
				return r.finish(ctx, sink, session, StatusFailed, "", err)
			}
			if toolErr != nil && lastToolErrorCount >= 2 {
				return r.finish(ctx, sink, session, StatusFailed, "", fmt.Errorf("repeated tool error from %s: %w", call.CapabilityID, toolErr))
			}
		}
	}
	return r.finish(ctx, sink, session, StatusFailed, "", fmt.Errorf("runtime turn budget exhausted"))
}

func (r *Runtime) loadDerivedProfileContext(ctx context.Context, userID string) (string, error) {
	if r == nil || r.DerivedProfileContextLoader == nil {
		return "", nil
	}
	value, err := r.DerivedProfileContextLoader(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("load derived profile context: %w", err)
	}
	return value, nil
}

func (r *Runtime) recordModelTurnStarted(ctx context.Context, session persistence.Session, turn, historyMessages, toolCount int) error {
	detail, err := json.Marshal(struct {
		Turn            int `json:"turn"`
		HistoryMessages int `json:"historyMessages"`
		ToolCount       int `json:"toolCount"`
	}{turn, historyMessages, toolCount})
	if err != nil {
		return fmt.Errorf("encode model turn start: %w", err)
	}
	_, err = r.Lifecycle.RecordEvent(ctx, session, EventModelTurnStarted, string(detail))
	return err
}

func (r *Runtime) recordModelTurnFinished(ctx context.Context, session persistence.Session, turnNumber int, turn Turn) error {
	detail, err := json.Marshal(struct {
		Turn          int    `json:"turn"`
		Status        string `json:"status"`
		FinishReason  string `json:"finishReason,omitempty"`
		ContentBytes  int    `json:"contentBytes"`
		ThinkingBytes int    `json:"thinkingBytes"`
		ToolCalls     int    `json:"toolCalls"`
	}{turnNumber, "completed", turn.FinishReason, len(turn.Content), len(turn.Thinking), len(turn.ToolCalls)})
	if err != nil {
		return fmt.Errorf("encode model turn finish: %w", err)
	}
	_, err = r.Lifecycle.RecordEvent(ctx, session, EventModelTurnFinished, string(detail))
	return err
}

func (r *Runtime) recordModelTurnFailed(ctx context.Context, session persistence.Session, turnNumber int, cause error) error {
	status, errorClass := "failed", "model"
	if errors.Is(cause, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		status, errorClass = "cancelled", "cancelled"
	}
	detail, err := json.Marshal(struct {
		Turn       int    `json:"turn"`
		Status     string `json:"status"`
		ErrorClass string `json:"errorClass"`
	}{turnNumber, status, errorClass})
	if err != nil {
		return fmt.Errorf("encode failed model turn: %w", err)
	}
	_, err = r.Lifecycle.RecordEvent(ctx, session, EventModelTurnFinished, string(detail))
	return err
}

// modelTools projects only the capabilities executable in this run.  A model
// must never be invited to call a tool that the runtime will subsequently deny.
func (r *Runtime) modelTools(policy Policy, timezone string, archivedEpisode bool) []capability.ModelDefinition {
	all := r.Tools.List()
	// Scheduled tasks need the user's zone to turn "tomorrow at nine" into an
	// instant. Without one the capability denies every call, so it stays hidden.
	scheduledTasks := policy.AllowScheduledTasks && usableTimezone(timezone)
	available := make([]capability.ModelDefinition, 0, len(all))
	for _, tool := range all {
		switch tool.ID {
		case capability.CreateScheduledTaskID, capability.ListScheduledTasksID, capability.PauseScheduledTaskID:
			if !scheduledTasks {
				continue
			}
		case capability.ArchiveConversationEpisodeID:
			if !policy.AllowEpisodeArchive || archivedEpisode {
				continue
			}
		case capability.SearchConversationHistoryID:
			if !policy.AllowConversationHistory {
				continue
			}
		case capability.RetrieveKnowledgeID:
			if !policy.AllowKnowledgeRetrieval {
				continue
			}
		case capability.WebSearchID, capability.WebFetchID:
			if !policy.AllowWebSearch {
				continue
			}
		case capability.MemoryAddID, capability.MemoryUpdateID, capability.MemoryDeleteID:
			if !policy.AllowMemoryMutation {
				continue
			}
		}
		available = append(available, tool)
	}
	return available
}

// usableTimezone mirrors the capability's own gate. LoadLocation("") silently
// means UTC, so an absent zone must be rejected before it is called.
func usableTimezone(timezone string) bool {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return false
	}
	_, err := time.LoadLocation(timezone)
	return err == nil
}

func (r *Runtime) loadCoreMemoryContext(ctx context.Context, userID string) (string, error) {
	if r == nil || r.CoreMemoryContextLoader == nil {
		return "", nil
	}
	contextText, err := r.CoreMemoryContextLoader(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("load core memory context: %w", err)
	}
	return contextText, nil
}

func isCoreMemoryMutation(id string) bool {
	return id == capability.MemoryAddID || id == capability.MemoryUpdateID || id == capability.MemoryDeleteID
}

func (r *Runtime) history(ctx context.Context, request RunRequest) ([]ModelMessage, error) {
	if r.History == nil {
		return []ModelMessage{}, nil
	}
	history, err := r.History.Messages(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("load conversation history: %w", err)
	}
	return append([]ModelMessage(nil), history...), nil
}

func (r *Runtime) finish(ctx context.Context, sink EventSink, session persistence.Session, status, answer string, cause error) (RunResult, error) {
	if _, owned := persistence.CurrentExecution(ctx); owned && cause != nil {
		if errors.Is(context.Cause(ctx), persistence.ErrExecutionLeaseLost) {
			return RunResult{Status: StatusInterrupted, RuntimeSessionID: session.ID}, persistence.ErrExecutionLeaseLost
		}
		return r.finishOwnedFailure(ctx, sink, session, status, cause)
	}
	durableCtx := context.WithoutCancel(ctx)
	if status != StatusCompleted && status != StatusDegraded && r.Episodes != nil {
		if err := r.Episodes.DiscardEpisodes(durableCtx, session.ID); err != nil {
			return RunResult{}, fmt.Errorf("discard pending conversation episodes: %w", err)
		}
	}
	if err := r.Lifecycle.FinishSession(durableCtx, session, status); err != nil {
		return RunResult{}, err
	}
	if cause != nil {
		fwlog.FromContext(ctx).Warnw("runtime session finished", "status", status, "error", cause)
		event := EventFailed
		if status == StatusCancelled {
			event = EventCancelled
		}
		if appendErr := r.recordAndEmit(durableCtx, sink, session, event, cause.Error()); appendErr != nil {
			return RunResult{}, appendErr
		}
		return RunResult{Status: status, RuntimeSessionID: session.ID}, cause
	}
	fwlog.FromContext(ctx).Infow("runtime session finished", "status", status, "answerBytes", len(answer))
	if _, err := r.Lifecycle.RecordEvent(durableCtx, session, EventCompleted, ""); err != nil {
		return RunResult{}, err
	}
	return RunResult{Status: status, AssistantContent: answer, RuntimeSessionID: session.ID}, nil
}

func (r *Runtime) CompleteEpisodes(ctx context.Context, runtimeSessionID, assistantMessageID string) error {
	if r == nil || r.Episodes == nil {
		return nil
	}
	return r.Episodes.CompleteEpisodes(ctx, runtimeSessionID, assistantMessageID)
}

func (r *Runtime) DiscardEpisodes(ctx context.Context, runtimeSessionID string) error {
	if r == nil || r.Episodes == nil {
		return nil
	}
	return r.Episodes.DiscardEpisodes(ctx, runtimeSessionID)
}

func (r *Runtime) emitNewJournal(ctx context.Context, sink EventSink, sessionID string, from int) error {
	entries, err := r.Lifecycle.store.ListJournal(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, entry := range entries[from:] {
		if sink != nil {
			if err := sink.Append(context.WithoutCancel(ctx), entry); err != nil {
				return err
			}
		}
	}
	return nil
}
func (r *Runtime) recordAndEmit(ctx context.Context, sink EventSink, session persistence.Session, kind, detail string) error {
	entry, err := r.Lifecycle.RecordEvent(ctx, session, kind, detail)
	if err != nil {
		return err
	}
	if sink != nil {
		return sink.Append(context.WithoutCancel(ctx), entry)
	}
	return nil
}

func limits(policy Policy) (turns, calls int) {
	turns, calls = policy.MaxTurns, policy.MaxToolCalls
	if turns <= 0 {
		turns = 8
	}
	if calls <= 0 {
		calls = 16
	}
	return turns, calls
}

var _ ConversationRuntime = (*Runtime)(nil)
