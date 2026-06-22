package runner

import (
	"context"

	"local/rag-project/internal/app/ingestion/domain"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
)

// NodeRunner 定义单类 ingestion 节点的统一执行接口。
type NodeRunner interface {
	NodeType() string
	Run(ctx context.Context, state ingestionworkflow.ExecutionState, node domain.PipelineNode) (ingestionworkflow.ExecutionState, map[string]any, error)
}

// NodeRunnerRegistry 负责按节点类型查找运行器。
type NodeRunnerRegistry struct {
	runners map[string]NodeRunner
}

// NewNodeRunnerRegistry 创建节点运行器注册表。
func NewNodeRunnerRegistry(runners ...NodeRunner) *NodeRunnerRegistry {
	registry := &NodeRunnerRegistry{
		runners: make(map[string]NodeRunner, len(runners)),
	}
	for _, runner := range runners {
		registry.Register(runner)
	}
	return registry
}

// Register 注册一个节点运行器。
func (r *NodeRunnerRegistry) Register(runner NodeRunner) {
	if r == nil || runner == nil {
		return
	}
	if r.runners == nil {
		r.runners = make(map[string]NodeRunner)
	}
	nodeType := runner.NodeType()
	if nodeType == "" {
		return
	}
	r.runners[nodeType] = runner
}

// Get 返回指定节点类型的运行器。
func (r *NodeRunnerRegistry) Get(nodeType string) (NodeRunner, bool) {
	if r == nil || r.runners == nil {
		return nil, false
	}
	runner, ok := r.runners[nodeType]
	return runner, ok
}
