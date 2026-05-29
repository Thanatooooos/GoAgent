package agent

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	"local/rag-project/internal/app/agent/webfetch"
	"local/rag-project/internal/app/agent/websearch"
	agentworkflow "local/rag-project/internal/app/agent/workflow"
	"local/rag-project/internal/framework/config"
	inframcp "local/rag-project/internal/infra-mcp"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type ServiceOptions struct {
	Config        *config.Config
	Provider      searchprovider.SearchProvider
	SourcePolicy  *searchprovider.SourcePolicyEngine
	HTTPClient    *http.Client
	FetchService  *agentfetch.Service
	MCPManager    inframcp.ToolClient
	MaxIterations int
}

type Service struct {
	runner *adk.Runner
}

func NewService(opts ServiceOptions) (*Service, error) {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.Get()
	}

	provider := opts.Provider
	if provider == nil {
		provider = searchprovider.BuildProvider(cfg, opts.MCPManager)
	}
	policy := opts.SourcePolicy
	if policy == nil {
		policy = searchprovider.BuildSourcePolicy(cfg)
	}

	searchService := agentsearch.NewService(provider, policy)
	searchTool, err := websearch.New(searchService)
	if err != nil {
		return nil, err
	}
	fetchService := opts.FetchService
	if fetchService == nil {
		fetchService = agentfetch.NewService(opts.HTTPClient)
	}
	fetchTool, err := webfetch.New(fetchService)
	if err != nil {
		return nil, err
	}

	workflowAgent, err := agentworkflow.New(context.Background(), agentworkflow.Config{
		SearchTool:    searchTool.Invokable(),
		FetchTool:     fetchTool.Invokable(),
		MaxIterations: opts.MaxIterations,
	})
	if err != nil {
		return nil, err
	}

	return &Service{
		runner: adk.NewRunner(context.Background(), adk.RunnerConfig{Agent: workflowAgent}),
	}, nil
}

func (s *Service) Run(ctx context.Context, req Request) (Response, error) {
	if s == nil || s.runner == nil {
		return Response{}, fmt.Errorf("agent service is not initialized")
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return Response{}, fmt.Errorf("question is required")
	}

	iterator := s.runner.Run(
		ctx,
		[]adk.Message{schema.UserMessage(question)},
		adk.WithSessionValues(agentworkflow.SessionValues(agentworkflow.Request{
			Question: question,
			UserID:   strings.TrimSpace(req.UserID),
			TraceID:  strings.TrimSpace(req.TraceID),
		})),
	)

	var final *agentworkflow.FinalState
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return Response{}, event.Err
		}
		if event.Output != nil && event.Output.CustomizedOutput != nil {
			switch typed := event.Output.CustomizedOutput.(type) {
			case agentworkflow.FinalState:
				state := typed
				final = &state
			case *agentworkflow.FinalState:
				final = typed
			}
		}
	}
	if final == nil {
		return Response{}, fmt.Errorf("agent finished without a final state")
	}
	return Response{
		Query:         final.Query,
		Results:       append([]agentsearch.SearchResultItem(nil), final.SearchOutput.Results...),
		Pages:         clonePages(final.FetchOutput),
		CombinedText:  fetchCombinedText(final.FetchOutput),
		Summary:       finalSummary(final),
		Provider:      strings.TrimSpace(firstNonEmpty(final.SearchOutput.ProviderActual, final.SearchOutput.Provider)),
		Degraded:      final.Degraded,
		DegradeReason: strings.TrimSpace(firstNonEmpty(final.DegradeReason, final.SearchOutput.ErrorMessage, fetchErrorMessage(final.FetchOutput))),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func clonePages(output *agentfetch.Output) []agentfetch.PageResult {
	if output == nil || len(output.Pages) == 0 {
		return nil
	}
	return append([]agentfetch.PageResult(nil), output.Pages...)
}

func fetchCombinedText(output *agentfetch.Output) string {
	if output == nil {
		return ""
	}
	return output.CombinedText
}

func fetchErrorMessage(output *agentfetch.Output) string {
	if output == nil {
		return ""
	}
	return output.ErrorMessage
}

func finalSummary(state *agentworkflow.FinalState) string {
	if state == nil {
		return ""
	}
	if state.FetchOutput != nil && strings.TrimSpace(state.FetchOutput.Summary) != "" {
		return state.FetchOutput.Summary
	}
	return state.SearchOutput.Summary
}
