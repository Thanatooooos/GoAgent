package workflow

import (
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
)

type Request struct {
	Question string
	UserID   string
	TraceID  string
}

type FinalState struct {
	Query         string
	SearchOutput  agentsearch.SearchOutput
	FetchOutput   *agentfetch.Output
	Degraded      bool
	DegradeReason string
}
