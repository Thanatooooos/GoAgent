package agent

import (
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
)

type Request struct {
	Question string
	UserID   string
	TraceID  string
}

type Response struct {
	Query         string
	Results       []agentsearch.SearchResultItem
	Pages         []agentfetch.PageResult
	CombinedText  string
	Summary       string
	Provider      string
	Degraded      bool
	DegradeReason string
}
