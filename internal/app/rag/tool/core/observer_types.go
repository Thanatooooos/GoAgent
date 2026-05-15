package core

import (
	"context"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
)

// Observer decides whether the agent loop has enough evidence to stop.
type Observer interface {
	Observe(ctx context.Context, input ObserveInput) (ObserveResult, error)
}

// ObserveInput describes the context available to the observe phase.
type ObserveInput struct {
	Question         string
	Round            int
	Results          []Result
	RoundResults     []Result
	PreviousState    AgentState
	MaxIterations    int
	ReachedMaxLoop   bool
	ToolDefinitions  []Definition
	ToolRegistry     *Registry
	KnowledgeBaseIDs []string
	RewriteResult    ragrewrite.Result
	RetrieveResult   ragretrieve.Result
}

// ObserveResult describes the decision produced by the observe phase.
type ObserveResult struct {
	Done          bool
	Reasoning     string
	NextHintCalls []HintCall
	NextHint      string
	Confidence    float64
	State         AgentState
}
