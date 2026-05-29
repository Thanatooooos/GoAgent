package websearch

import (
	"context"
	"fmt"
	"strings"

	agentsearch "local/rag-project/internal/app/agent/search"

	einotool "github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

type Tool struct {
	invokable einotool.InvokableTool
}

type Input struct {
	Query string `json:"query" jsonschema_description:"The search query. Keep it concise and specific, for example 'Go generics tutorial'."`
}

func New(service *agentsearch.Service) (*Tool, error) {
	if service == nil {
		return nil, fmt.Errorf("search service is required")
	}
	invokable, err := toolutils.InferTool[Input, agentsearch.SearchOutput](
		"web_search",
		"Search the web for information beyond the local knowledge base. Use this when the user asks about general concepts, troubleshooting steps, or anything not covered by local documents.",
		func(ctx context.Context, input Input) (agentsearch.SearchOutput, error) {
			query := strings.TrimSpace(input.Query)
			output, err := service.Search(ctx, query)
			if err != nil {
				output.Degraded = true
				if output.DegradeReason == "" {
					output.DegradeReason = err.Error()
				}
				if output.ErrorMessage == "" {
					output.ErrorMessage = err.Error()
				}
				if output.Summary == "" {
					output.Summary = fmt.Sprintf("web search failed: %v", err)
				}
				return output, nil
			}
			return output, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &Tool{invokable: invokable}, nil
}

func (t *Tool) Invokable() einotool.InvokableTool {
	if t == nil {
		return nil
	}
	return t.invokable
}
