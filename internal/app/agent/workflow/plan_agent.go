package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type planAgent struct{}

func newPlanAgent() adk.Agent {
	return &planAgent{}
}

func (a *planAgent) Name(context.Context) string {
	return "plan"
}

func (a *planAgent) Description(context.Context) string {
	return "derive a search query from the user question"
}

func (a *planAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()

		request, ok := adk.GetSessionValue(ctx, requestSessionKey)
		if !ok {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("workflow request is missing")})
			return
		}
		req, ok := request.(Request)
		if !ok {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("workflow request has unexpected type %T", request)})
			return
		}

		query := normalizeQuery(req.Question)
		if query == "" {
			generator.Send(&adk.AgentEvent{Err: fmt.Errorf("question is required")})
			return
		}
		adk.AddSessionValue(ctx, searchQuerySessionKey, query)
		generator.Send(&adk.AgentEvent{
			AgentName: a.Name(ctx),
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message: schema.AssistantMessage("Planned web search query: "+query, nil),
					Role:    schema.Assistant,
				},
			},
		})
	}()
	return iterator
}

func normalizeQuery(question string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(question)), " ")
}
