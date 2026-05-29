package webfetch

import (
	"context"
	"fmt"
	"strings"

	agentfetch "local/rag-project/internal/app/agent/fetch"

	einotool "github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

type Tool struct {
	invokable einotool.InvokableTool
}

type Input struct {
	URLs []string `json:"urls,omitempty" jsonschema_description:"One or more URLs to fetch. Keep it to the most promising results, up to 3 URLs."`
	URL  string   `json:"url,omitempty" jsonschema_description:"Optional single URL fallback when only one page should be fetched."`
}

func New(service *agentfetch.Service) (*Tool, error) {
	if service == nil {
		return nil, fmt.Errorf("fetch service is required")
	}
	invokable, err := toolutils.InferTool[Input, agentfetch.Output](
		"web_fetch",
		"Fetch one or more web pages and extract readable text content. Use this after web_search when snippets are not enough and the agent needs page-level evidence.",
		func(ctx context.Context, input Input) (agentfetch.Output, error) {
			output, err := service.Fetch(ctx, normalizeInputURLs(input))
			if err != nil {
				output.Degraded = true
				if output.DegradeReason == "" {
					output.DegradeReason = err.Error()
				}
				if output.ErrorMessage == "" {
					output.ErrorMessage = err.Error()
				}
				if output.Summary == "" {
					output.Summary = fmt.Sprintf("web fetch failed: %v", err)
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

func normalizeInputURLs(input Input) []string {
	if len(input.URLs) > 0 {
		return input.URLs
	}
	if trimmed := strings.TrimSpace(input.URL); trimmed != "" {
		return []string{trimmed}
	}
	return nil
}
