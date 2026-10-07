// Package runtime defines the conversation execution kernel contract.
//
// It intentionally does not depend on the legacy agent package. Chat owns
// transport and durable user/final-assistant messages; runtime owns decisions,
// tool lifecycle, journal facts, and model-visible projections.
package runtime

import (
	"context"
	"fmt"
	"strings"

	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/persistence"
)

type RunRequest struct {
	publishAnswer    bool // only ChatService selects the durable publication protocol
	Work             *capability.WorkScope
	ConversationID   string
	UserID           string
	UserMessageID    string
	Question         string
	DeepThinking     bool
	KnowledgeBaseIDs []string
	// Timezone is the browser-reported IANA zone. Like KB scope it is
	// server-owned input; model arguments can never set it.
	Timezone string
	Policy   Policy
	TraceID  string
}

type Policy struct {
	AllowKnowledgeRetrieval  bool
	AllowMemoryRecall        bool
	AllowMemoryMutation      bool
	AllowWebSearch           bool
	AllowConversationHistory bool
	AllowEpisodeArchive      bool
	AllowScheduledTasks      bool
	RetrieveSearchMode       string
	MaxTurns                 int
	MaxToolCalls             int
	ContextTokenBudget       int
	DisableTools             bool
}

// EffectivePolicy applies the product rule that an empty knowledge-base scope
// is not an unrestricted search scope: retrieval is prohibited.
func (r RunRequest) EffectivePolicy() Policy {
	policy := r.Policy
	if len(uniqueTrimmed(r.KnowledgeBaseIDs)) == 0 {
		policy.AllowKnowledgeRetrieval = false
	}
	return policy
}

func (r RunRequest) Validate() error {
	switch {
	case strings.TrimSpace(r.ConversationID) == "":
		return fmt.Errorf("conversation id is required")
	case strings.TrimSpace(r.UserID) == "":
		return fmt.Errorf("user id is required")
	case strings.TrimSpace(r.UserMessageID) == "":
		return fmt.Errorf("user message id is required")
	case strings.TrimSpace(r.Question) == "":
		return fmt.Errorf("question is required")
	case strings.TrimSpace(r.TraceID) == "":
		return fmt.Errorf("trace id is required")
	default:
		return nil
	}
}

const (
	StatusCompleted   = "completed"
	StatusDegraded    = "degraded"
	StatusFailed      = "failed"
	StatusCancelled   = "cancelled"
	StatusInterrupted = "interrupted"
)

type RunResult struct {
	Status           string
	AssistantContent string
	Evidence         []EvidenceRef
	RuntimeSessionID string
	DegradeReason    string
}

// EvidenceRef is a durable reference to a fact accepted by runtime. Concrete
// capability adapters may add metadata to the backing journal payload, but the
// final answer is limited to these accepted references.
type EvidenceRef = persistence.EvidenceRef

const (
	ToolStatePending   = "pending"
	ToolStateExecuting = "executing"
	ToolStateCompleted = "completed"
	ToolStateDenied    = "denied"
	ToolStateFailed    = "failed"
)

func CanTransitionToolState(from, to string) bool {
	switch from {
	case ToolStatePending:
		return to == ToolStateExecuting || to == ToolStateDenied || to == ToolStateFailed
	case ToolStateExecuting:
		return to == ToolStateCompleted || to == ToolStateFailed
	default:
		return false
	}
}

const (
	EventSessionStarted             = "session_started"
	EventModelTurnStarted           = "model_turn_started"
	EventModelTurnFinished          = "model_turn_finished"
	EventHistoryCompressionStarted  = "history_compression_started"
	EventHistoryCompressionFinished = "history_compression_finished"
	EventAssistantTurn              = "assistant_turn"
	EventToolPending                = "tool_pending"
	EventToolExecuting              = "tool_executing"
	EventToolSettled                = "tool_settled"
	EventThinkingDelta              = "thinking_delta"
	EventAnswerDelta                = "answer_delta"
	EventAnswerFinal                = "answer_final"
	EventCompleted                  = "completed"
	EventDegraded                   = "degraded"
	EventFailed                     = "failed"
	EventCancelled                  = "cancelled"
	EventInterrupted                = "interrupted"
)

// JournalEntry is append-only execution fact. Its durable definition lives in
// persistence so implementations do not depend on the runtime orchestration package.
type JournalEntry = persistence.JournalEntry

type EventSink interface {
	Append(context.Context, JournalEntry) error
}

// ConversationHistory supplies model-visible messages from earlier accepted
// conversation turns. It must not include the current user message or runtime
// journal entries; Runtime composes those in their defined order.
type ConversationHistory interface {
	Messages(context.Context, RunRequest) ([]ModelMessage, error)
}

// CompactionHistory is the optional durable-history port used by Runtime when
// a request exceeds its context budget. It deliberately exposes raw messages
// and the summary coverage boundary: adapters persist them, runtime decides
// when and what to compact.
type CompactionHistory interface {
	ConversationHistory
	Snapshot(context.Context, RunRequest) (HistorySnapshot, error)
	StoreSummary(context.Context, RunRequest, HistorySummary) (bool, error)
}

type HistorySnapshot struct {
	Summary  HistorySummary
	Messages []HistoryMessage
}

type HistoryMessage struct {
	ID      string
	Role    ModelRole
	Content string
}

type HistorySummary struct {
	Content              string
	StructuredJSON       string
	CoveredFromMessageID string
	CoveredToMessageID   string
	SourceMessageCount   int
}

// ConversationRuntime is the future chat-facing execution boundary. Its
// implementation is intentionally not introduced in Slice 0.
type ConversationRuntime interface {
	Run(context.Context, RunRequest, EventSink) (RunResult, error)
}

type ConversationEpisodeFinalizer interface {
	CompleteEpisodes(context.Context, string, string) error
	DiscardEpisodes(context.Context, string) error
}

// ConversationReplay is the read-only reconnect boundary exposed by runtime.
type ConversationReplay interface {
	Replay(context.Context, string, string, EventSink) (bool, error)
}

func uniqueTrimmed(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
