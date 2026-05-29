package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fetchAgent struct {
	tool einotool.InvokableTool
}

func newFetchAgent(tool einotool.InvokableTool) adk.Agent {
	return &fetchAgent{tool: tool}
}

func (a *fetchAgent) Name(context.Context) string {
	return "fetch"
}

func (a *fetchAgent) Description(context.Context) string {
	return "fetch readable content from the most promising search result pages"
}

func (a *fetchAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()

		if a.tool == nil {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("web_fetch tool is required")})
			return
		}
		outputValue, ok := adk.GetSessionValue(ctx, searchOutputSessionKey)
		if !ok {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("search output is missing")})
			return
		}
		searchOutput, ok := outputValue.(agentsearch.SearchOutput)
		if !ok {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("search output has unexpected type %T", outputValue)})
			return
		}

		urls := searchOutput.URLs
		if len(urls) == 0 {
			generator.Send(&adk.AgentEvent{
				AgentName: a.Name(ctx),
				Output: &adk.AgentOutput{
					MessageOutput: &adk.MessageVariant{
						Message:  schema.ToolMessage("web_fetch skipped: no fetchable urls", "web_fetch"),
						Role:     schema.Tool,
						ToolName: "web_fetch",
					},
				},
			})
			return
		}

		payload, err := json.Marshal(map[string][]string{"urls": urls})
		if err != nil {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("marshal tool arguments: %w", err)})
			return
		}
		rawOutput, err := a.tool.InvokableRun(ctx, string(payload))
		if err != nil {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("invoke web_fetch tool: %w", err)})
			return
		}

		var output agentfetch.Output
		if err := json.Unmarshal([]byte(rawOutput), &output); err != nil {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("decode web_fetch output: %w", err)})
			return
		}

		adk.AddSessionValue(ctx, fetchOutputSessionKey, output)
		generator.Send(&adk.AgentEvent{
			AgentName: a.Name(ctx),
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message:  schema.ToolMessage(output.Summary, "web_fetch"),
					Role:     schema.Tool,
					ToolName: "web_fetch",
				},
			},
		})
	}()
	return iterator
}
