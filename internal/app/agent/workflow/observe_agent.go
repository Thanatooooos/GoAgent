package workflow

import (
	"context"
	"fmt"
	"strings"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type observeAgent struct{}

func newObserveAgent() adk.Agent {
	return &observeAgent{}
}

func (a *observeAgent) Name(context.Context) string {
	return "observe"
}

func (a *observeAgent) Description(context.Context) string {
	return "evaluate the search and fetch results and finish the current loop"
}

func (a *observeAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()

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

		var fetchOutput *agentfetch.Output
		if fetchValue, ok := adk.GetSessionValue(ctx, fetchOutputSessionKey); ok {
			typed, ok := fetchValue.(agentfetch.Output)
			if !ok {
				generator.Send(&adk.AgentEvent{Err: fmt.Errorf("fetch output has unexpected type %T", fetchValue)})
				return
			}
			copyValue := typed
			fetchOutput = &copyValue
		}

		degraded, degradeReason := determineFinalStatus(searchOutput, fetchOutput)
		summary := searchOutput.Summary
		if fetchOutput != nil && strings.TrimSpace(fetchOutput.Summary) != "" {
			summary = fetchOutput.Summary
		}

		state := FinalState{
			Query:         query,
			SearchOutput:  searchOutput,
			FetchOutput:   fetchOutput,
			Degraded:      degraded,
			DegradeReason: degradeReason,
		}
		adk.AddSessionValue(ctx, finalStateSessionKey, state)

		generator.Send(&adk.AgentEvent{
			AgentName: a.Name(ctx),
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message: schema.AssistantMessage(summary, nil),
					Role:    schema.Assistant,
				},
				CustomizedOutput: state,
			},
			Action: adk.NewBreakLoopAction(a.Name(ctx)),
		})
	}()
	return iterator
}

func determineFinalStatus(searchOutput agentsearch.SearchOutput, fetchOutput *agentfetch.Output) (bool, string) {
	if fetchOutput != nil {
		if fetchOutput.Degraded {
			return true, firstStatusReason(fetchOutput.DegradeReason, fetchOutput.ErrorMessage)
		}
		if fetchOutput.SuccessCount > 0 {
			return false, ""
		}
	}
	if searchOutput.Degraded {
		return true, firstStatusReason(searchOutput.DegradeReason, searchOutput.ErrorMessage)
	}
	return false, ""
}

func firstStatusReason(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
