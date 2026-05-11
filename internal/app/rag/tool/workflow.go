package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

const (
	CallStatusSuccess = "success"
	CallStatusFailed  = "failed"
	CallStatusSkipped = "skipped"
)

// Workflow 定义 tool workflow 的最小抽象，后续可由 EINO 实现。
type Workflow interface {
	Run(ctx context.Context, input WorkflowInput) (WorkflowResult, error)
}

// Planner 负责根据用户问题规划需要调用的 tool 列表。
type Planner interface {
	Plan(ctx context.Context, input PlanInput) (PlanResult, error)
}

type HintCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type AgentState struct {
	Phase         string
	Hypothesis    string
	Confidence    float64
	OpenQuestions []string
	CheckedTools  []string
	NextHintCalls []HintCall
	NextHint      string
}

func (s AgentState) Normalize() AgentState {
	s.Phase = strings.TrimSpace(s.Phase)
	s.Hypothesis = strings.TrimSpace(s.Hypothesis)
	s.Confidence = clampConfidence(s.Confidence)
	s.OpenQuestions = uniqueTrimmedStrings(s.OpenQuestions)
	s.CheckedTools = uniqueTrimmedStrings(s.CheckedTools)
	s.NextHint = strings.TrimSpace(s.NextHint)
	s.NextHintCalls = normalizeHintCalls(s.NextHintCalls)
	if len(s.NextHintCalls) == 0 && s.NextHint != "" {
		s.NextHintCalls = parseHintCallsFromLegacyString(s.NextHint)
	}
	if len(s.NextHintCalls) > 0 {
		s.NextHint = serializeHintCalls(s.NextHintCalls)
	}
	return s
}

func (s AgentState) Empty() bool {
	normalized := s.Normalize()
	return normalized.Phase == "" &&
		normalized.Hypothesis == "" &&
		normalized.Confidence == 0 &&
		len(normalized.OpenQuestions) == 0 &&
		len(normalized.CheckedTools) == 0 &&
		len(normalized.NextHintCalls) == 0 &&
		normalized.NextHint == ""
}

func (s AgentState) PromptString() string {
	normalized := s.Normalize()
	if normalized.Empty() {
		return ""
	}
	payload := map[string]any{}
	if normalized.Phase != "" {
		payload["phase"] = normalized.Phase
	}
	if normalized.Hypothesis != "" {
		payload["hypothesis"] = normalized.Hypothesis
	}
	if normalized.Confidence > 0 {
		payload["confidence"] = normalized.Confidence
	}
	if len(normalized.OpenQuestions) > 0 {
		payload["openQuestions"] = normalized.OpenQuestions
	}
	if len(normalized.CheckedTools) > 0 {
		payload["checkedTools"] = normalized.CheckedTools
	}
	if len(normalized.NextHintCalls) > 0 {
		payload["nextHintCalls"] = normalized.NextHintCalls
	}
	if normalized.NextHint != "" {
		payload["nextHint"] = normalized.NextHint
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("phase=%s hint=%s", normalized.Phase, normalized.NextHint)
	}
	return string(encoded)
}

// PlanInput 描述 planner 的输入。
type PlanInput struct {
	Question         string
	ToolDefinitions  []Definition
	AgentState       AgentState
	PreviousResults  []Result
	KnowledgeBaseIDs []string
	RewriteResult    ragrewrite.Result
	RetrieveResult   ragretrieve.Result
}

// PlanResult 描述 planner 的输出。
type PlanResult struct {
	Calls        []Call
	DirectAnswer string
}

// HasTools 表示 planner 是否规划了 tool 调用。
func (r PlanResult) HasTools() bool {
	return len(r.Calls) > 0
}

// WorkflowInput 描述 tool workflow 执行时可用的 RAG 上下文。
type WorkflowInput struct {
	Question         string
	UserID           string
	ConversationID   string
	TraceID          string
	KnowledgeBaseIDs []string
	SearchMode       string
	History          []convention.ChatMessage
	RewriteResult    ragrewrite.Result
	RetrieveResult   ragretrieve.Result
	EventSink        WorkflowEventSink
}

// CallSummary 是一次工具调用的简要结果，可用于 trace / SSE / 前端展示。
type CallSummary struct {
	CallID     string
	Round      int
	Sequence   int
	Name       string
	Status     string
	Summary    string
	DurationMs int64
	Arguments  map[string]any
	Data       map[string]any
}

// RoundSummary 描述 agent 每一轮执行和观察的结果。
type RoundSummary struct {
	Round               int
	Calls               []CallSummary
	Done                bool
	Reasoning           string
	NextHintCalls       []HintCall
	NextHint            string
	Confidence          float64
	State               AgentState
	ExecutionMode       string
	WallClockDurationMs int64
	ToolCallCount       int
	TotalToolDurationMs int64
}

// WorkflowResult 表示 tool workflow 的执行结果。
type WorkflowResult struct {
	Used           bool
	Context        string
	AnswerGuidance string
	Calls          []CallSummary
	Rounds         []RoundSummary
	Degraded       bool
	DegradeReason  string
}

// ToolCallEvent 表示一次工具调用事件。
type ToolCallEvent struct {
	CallID     string
	Round      int
	Sequence   int
	Name       string
	Status     string
	Summary    string
	DurationMs int64
	Arguments  map[string]any
	Data       map[string]any
}

// WorkflowEventSink 用于把 workflow 内部的 agent/tool 事件推送给上层。
type WorkflowEventSink interface {
	OnAgentThink(message string) error
	OnToolStart(event ToolCallEvent) error
	OnToolResult(event ToolCallEvent) error
}

// HasContext 表示 workflow 是否生成了可注入 prompt 的上下文。
func (r WorkflowResult) HasContext() bool {
	return strings.TrimSpace(r.Context) != ""
}

func uniqueTrimmedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
