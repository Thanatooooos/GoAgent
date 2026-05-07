package tool

import (
	"context"
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

// PlanInput 描述 planner 的输入。
type PlanInput struct {
	Question        string
	ToolDefinitions []Definition
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
}

// CallSummary 是一次工具调用的简要结果，可用于 trace / SSE / 前端展示。
type CallSummary struct {
	Name       string
	Status     string
	Summary    string
	DurationMs int64
}

// WorkflowResult 表示 tool workflow 的执行结果。
type WorkflowResult struct {
	Used           bool
	Context        string
	AnswerGuidance string
	Calls          []CallSummary
	Degraded       bool
	DegradeReason  string
}

// HasContext 表示 workflow 是否生成了可注入 prompt 的上下文。
func (r WorkflowResult) HasContext() bool {
	return strings.TrimSpace(r.Context) != ""
}
