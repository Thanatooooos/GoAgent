package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	agentsearch "local/rag-project/internal/app/agent/search"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type searchAgent struct {
	tool einotool.InvokableTool
}

func newSearchAgent(tool einotool.InvokableTool) adk.Agent {
	return &searchAgent{tool: tool}
}

func (a *searchAgent) Name(context.Context) string {
	return "search"
}

func (a *searchAgent) Description(context.Context) string {
	return "execute the web_search tool"
}

func (a *searchAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()

		if a.tool == nil {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("web_search tool is required")})
			return
		}
		queryValue, ok := adk.GetSessionValue(ctx, searchQuerySessionKey)
		if !ok {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("search query is missing")})
			return
		}
		query, ok := queryValue.(string)
		if !ok {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("search query has unexpected type %T", queryValue)})
			return
		}

		payload, err := json.Marshal(map[string]string{"query": query})
		if err != nil {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("marshal tool arguments: %w", err)})
			return
		}
		rawOutput, err := a.tool.InvokableRun(ctx, string(payload))
		if err != nil {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("invoke web_search tool: %w", err)})
			return
		}

		var output agentsearch.SearchOutput
		if err := json.Unmarshal([]byte(rawOutput), &output); err != nil {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("decode web_search output: %w", err)})
			return
		}

		adk.AddSessionValue(ctx, searchOutputSessionKey, output)
		generator.Send(&adk.AgentEvent{
			AgentName: a.Name(ctx),
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message:  schema.ToolMessage(output.Summary, "web_search"),
					Role:     schema.Tool,
					ToolName: "web_search",
				},
			},
		})
	}()
	return iterator
}
