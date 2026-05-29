package workflow

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
)

type Config struct {
	SearchTool    einotool.InvokableTool
	FetchTool     einotool.InvokableTool
	MaxIterations int
}

func New(ctx context.Context, cfg Config) (adk.ResumableAgent, error) {
	if cfg.SearchTool == nil {
		return nil, fmt.Errorf("web_search tool is required")
	}
	if cfg.FetchTool == nil {
		return nil, fmt.Errorf("web_fetch tool is required")
	}
	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 1
	}

	round, err := adk.NewSequentialAgent(ctx, &adk.SequentialAgentConfig{
		Name:        "search_round",
		Description: "plan, execute web_search, fetch relevant pages, and observe the result",
		SubAgents: []adk.Agent{
			newPlanAgent(),
			newSearchAgent(cfg.SearchTool),
			newFetchAgent(cfg.FetchTool),
			newObserveAgent(),
		},
	})
	if err != nil {
		return nil, err
	}
	return adk.NewLoopAgent(ctx, &adk.LoopAgentConfig{
		Name:          "search_loop",
		Description:   "minimal Eino-native web search and fetch loop",
		SubAgents:     []adk.Agent{round},
		MaxIterations: maxIterations,
	})
}
