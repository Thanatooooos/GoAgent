package builtin

import (
	"context"
	"strings"

	ragtool "local/rag-project/internal/app/rag/tool"
)

// ThinkTool lets the agent record a reasoning step before committing to other tool calls.
// It performs no side effects — the thought is stored as the call summary for trace and SSE visibility.
type ThinkTool struct{}

func NewThinkTool() *ThinkTool {
	return &ThinkTool{}
}

func (t *ThinkTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "think",
		Description: "Record a reasoning thought before taking action. Use this when the next step is not obvious and you need to weigh multiple options. This tool obtains no new information and has no side effects.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "thought",
				Type:        ragtool.ParamTypeString,
				Description: "Your reasoning or thought process about what to do next.",
				Required:    true,
			},
		},
	}
}

func (t *ThinkTool) Invoke(_ context.Context, call ragtool.Call) (ragtool.Result, error) {
	thought := strings.TrimSpace(readStringArg(call.Arguments, "thought"))
	if thought == "" {
		return ragtool.Result{Name: "think", Status: ragtool.CallStatusFailed, ErrorMessage: "thought is required"}, nil
	}
	return ragtool.Result{
		Name:    "think",
		Status:  ragtool.CallStatusSuccess,
		Summary: thought,
	}, nil
}
