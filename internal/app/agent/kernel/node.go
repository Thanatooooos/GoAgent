package kernel

import (
	"context"
	"fmt"
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
)

// Node is the runtime-native node contract used by the M1 kernel.
// Nodes should treat the incoming session as read-only and express changes
// through NodeResult.
type Node interface {
	Name() string
	Run(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error)
}

// BranchFunc selects the next node after a branch point.
type BranchFunc func(ctx context.Context, session *agentruntime.RuntimeSession) (string, error)

type nodeFunc struct {
	name string
	run  func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error)
}

// NewNodeFunc creates a Node from a function.
func NewNodeFunc(name string, run func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error)) (Node, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if run == nil {
		return nil, fmt.Errorf("node run func is required")
	}
	return nodeFunc{name: trimmed, run: run}, nil
}

func (n nodeFunc) Name() string {
	return n.name
}

func (n nodeFunc) Run(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
	return n.run(ctx, session)
}
