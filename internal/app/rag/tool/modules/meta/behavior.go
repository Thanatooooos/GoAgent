package meta

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

type ThinkResultView struct {
	Thought string
}

func ThinkBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			if strings.TrimSpace(result.Name) != "think" {
				return nil, fmt.Errorf("think result view unavailable")
			}
			return ThinkResultView{Thought: strings.TrimSpace(result.Summary)}, nil
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "think_terminal", Terminal: true}
		},
	}
}
