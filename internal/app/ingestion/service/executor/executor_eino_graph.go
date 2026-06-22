package executor

import (
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/compose"

	"local/rag-project/internal/framework/exception"
)

// EinoGraphExecutor compiles normalized workflow specs into reusable EINO runners.
type EinoGraphExecutor struct {
	owner *ExecutorService

	mu    sync.RWMutex
	cache map[string]compose.Runnable[*einoGraphContext, *einoGraphContext]
}

var registerEinoGraphContextMergeOnce sync.Once

func NewEinoGraphExecutor(owner *ExecutorService) *EinoGraphExecutor {
	registerEinoGraphContextMergeOnce.Do(func() {
		compose.RegisterValuesMergeFunc(func(values []*einoGraphContext) (*einoGraphContext, error) {
			if len(values) == 0 {
				return nil, nil
			}
			merged := values[0]
			for _, item := range values[1:] {
				if item == nil {
					continue
				}
				if merged == nil {
					merged = item
					continue
				}
				if merged.runtime != nil && item.runtime != nil && merged.runtime != item.runtime {
					return nil, fmt.Errorf("multiple runtimes reached the same fan-in node")
				}
			}
			return merged, nil
		})
	})
	return &EinoGraphExecutor{
		owner: owner,
		cache: make(map[string]compose.Runnable[*einoGraphContext, *einoGraphContext]),
	}
}

func (e *EinoGraphExecutor) Execute(ctx context.Context, workflow ingestionworkflow.WorkflowSpec, state ingestionworkflow.ExecutionState) (ingestionworkflow.ExecutionState, error) {
	if e == nil || e.owner == nil {
		return state, exception.NewServiceException("eino graph executor is required", nil)
	}
	runner, err := e.compiledRunner(workflow)
	if err != nil {
		return state, err
	}

	runtime := newEinoTaskRuntime(state)
	graphCtx := &einoGraphContext{runtime: runtime}
	if _, err := runner.Invoke(ctx, graphCtx); err != nil {
		return runtime.State(), err
	}
	return runtime.State(), nil
}

func (e *EinoGraphExecutor) compiledRunner(workflow ingestionworkflow.WorkflowSpec) (compose.Runnable[*einoGraphContext, *einoGraphContext], error) {
	cacheKey := workflow.CacheKey
	e.mu.RLock()
	runner, ok := e.cache[cacheKey]
	e.mu.RUnlock()
	if ok {
		return runner, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if runner, ok := e.cache[cacheKey]; ok {
		return runner, nil
	}

	graph := compose.NewGraph[*einoGraphContext, *einoGraphContext]()
	for _, item := range workflow.NodeOrder {
		node := item
		graph.AddLambdaNode(node.Node.NodeID, compose.InvokableLambda(
			func(ctx context.Context, in *einoGraphContext) (*einoGraphContext, error) {
				if in == nil || in.runtime == nil {
					return in, exception.NewServiceException("graph execution context is required", nil)
				}
				if err := e.owner.executeWorkflowNode(ctx, in.runtime, node); err != nil {
					return in, err
				}
				return in, nil
			},
		))
	}

	for _, entryNodeID := range workflow.EntryNodeIDs {
		if err := graph.AddEdge(compose.START, entryNodeID); err != nil {
			return nil, fmt.Errorf("add graph start edge for %s: %w", entryNodeID, err)
		}
	}
	for _, item := range workflow.NodeOrder {
		nodeID := item.Node.NodeID
		edges := workflow.EdgesBySource[nodeID]
		if len(edges) == 0 {
			if err := graph.AddEdge(nodeID, compose.END); err != nil {
				return nil, fmt.Errorf("add graph end edge for %s: %w", nodeID, err)
			}
			continue
		}
		endNodes := map[string]bool{compose.END: true}
		for _, edge := range edges {
			endNodes[edge.ToNodeID] = true
		}
		branch := compose.NewGraphMultiBranch(func(ctx context.Context, in *einoGraphContext) (map[string]bool, error) {
			if in == nil || in.runtime == nil {
				return map[string]bool{compose.END: true}, nil
			}
			selected, err := in.runtime.SelectSuccessors(nodeID, edges)
			if err != nil {
				return nil, err
			}
			if len(selected) == 0 {
				return map[string]bool{compose.END: true}, nil
			}
			return selected, nil
		}, endNodes)
		if err := graph.AddBranch(nodeID, branch); err != nil {
			return nil, fmt.Errorf("add graph branch for %s: %w", nodeID, err)
		}
	}

	compiled, err := graph.Compile(context.Background(),
		compose.WithGraphName("ingestion_pipeline_"+workflow.Pipeline.ID),
		compose.WithNodeTriggerMode(compose.AllPredecessor),
	)
	if err != nil {
		return nil, fmt.Errorf("compile ingestion graph: %w", err)
	}
	e.cache[cacheKey] = compiled
	return compiled, nil
}

type einoGraphContext struct {
	runtime *einoTaskRuntime
}

type einoTaskRuntime struct {
	mu    sync.RWMutex
	state ingestionworkflow.ExecutionState
}

func newEinoTaskRuntime(state ingestionworkflow.ExecutionState) *einoTaskRuntime {
	if state.Artifacts == nil {
		state.Artifacts = map[string]any{}
	}
	if state.NodeOutputs == nil {
		state.NodeOutputs = map[string]map[string]any{}
	}
	return &einoTaskRuntime{state: state}
}

func (r *einoTaskRuntime) Snapshot() ingestionworkflow.ExecutionState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state.Clone()
}

func (r *einoTaskRuntime) State() ingestionworkflow.ExecutionState {
	return r.Snapshot()
}

func (r *einoTaskRuntime) Commit(node ingestionworkflow.WorkflowNodeSpec, nextState ingestionworkflow.ExecutionState, output map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if nextState.Source.Type != "" || len(nextState.Source.Bytes) > 0 || nextState.Source.Location != "" {
		r.state.Source = nextState.Source
	}
	if nextState.Parsed.Content != "" || nextState.Parsed.Title != "" || len(nextState.Parsed.Metadata) > 0 {
		r.state.Parsed = nextState.Parsed
	}
	if nextState.Chunks != nil {
		r.state.Chunks = nextState.Chunks
	}
	if nextState.IndexResult.Target != "" || nextState.IndexResult.ChunkCount > 0 || len(nextState.IndexResult.Metadata) > 0 {
		r.state.IndexResult = nextState.IndexResult
	}
	if r.state.Artifacts == nil {
		r.state.Artifacts = map[string]any{}
	}
	if r.state.NodeOutputs == nil {
		r.state.NodeOutputs = map[string]map[string]any{}
	}
	if output != nil {
		r.state.NodeOutputs[node.Node.NodeID] = output
	}
	for key, value := range nextState.Artifacts {
		r.state.Artifacts[key] = value
	}
	syncBuiltInArtifacts(&r.state, node)
}

func (r *einoTaskRuntime) SelectSuccessors(nodeID string, edges []ingestionworkflow.WorkflowEdgeSpec) (map[string]bool, error) {
	r.mu.RLock()
	state := r.state.Clone()
	r.mu.RUnlock()

	selected := make(map[string]bool)
	for _, edge := range edges {
		matched, err := ingestionworkflow.EvaluateWorkflowCondition(edge.Condition, state)
		if err != nil {
			return nil, fmt.Errorf("evaluate edge %s condition: %w", edge.EdgeID, err)
		}
		if matched {
			selected[edge.ToNodeID] = true
		}
	}
	return selected, nil
}

func syncBuiltInArtifacts(state *ingestionworkflow.ExecutionState, node ingestionworkflow.WorkflowNodeSpec) {
	if state == nil {
		return
	}
	if state.Artifacts == nil {
		state.Artifacts = map[string]any{}
	}
	switch node.Node.NodeType {
	case "fetcher":
		state.Artifacts["source"] = buildSourceArtifact(state)
	case "parser":
		state.Artifacts["parsed"] = buildParsedArtifact(state)
	case "chunker":
		state.Artifacts["chunks"] = buildChunksArtifact(state)
	case "enhancer":
		if state.Parsed.Content != "" || len(state.Parsed.Metadata) > 0 {
			state.Artifacts["parsed"] = buildParsedArtifact(state)
		}
		if len(state.Chunks) > 0 {
			state.Artifacts["chunks"] = buildChunksArtifact(state)
		}
	case "enricher":
		state.Artifacts["chunks"] = buildChunksArtifact(state)
	case "indexer":
		state.Artifacts["index"] = buildIndexArtifact(state)
	}
}

func buildSourceArtifact(state *ingestionworkflow.ExecutionState) map[string]any {
	if state == nil {
		return nil
	}
	return map[string]any{
		"type":        state.Source.Type,
		"location":    state.Source.Location,
		"fileName":    state.Source.FileName,
		"contentType": state.Source.ContentType,
		"bytes":       state.Source.Bytes,
		"metadata":    state.Source.Metadata,
	}
}

func buildParsedArtifact(state *ingestionworkflow.ExecutionState) map[string]any {
	if state == nil {
		return nil
	}
	return map[string]any{
		"content":  state.Parsed.Content,
		"title":    state.Parsed.Title,
		"metadata": state.Parsed.Metadata,
	}
}

func buildChunksArtifact(state *ingestionworkflow.ExecutionState) []map[string]any {
	if state == nil || len(state.Chunks) == 0 {
		return nil
	}
	chunks := make([]map[string]any, 0, len(state.Chunks))
	for _, item := range state.Chunks {
		chunks = append(chunks, map[string]any{
			"index":    item.Index,
			"content":  item.Content,
			"metadata": item.Metadata,
		})
	}
	return chunks
}

func buildIndexArtifact(state *ingestionworkflow.ExecutionState) map[string]any {
	if state == nil {
		return nil
	}
	return map[string]any{
		"target":     state.IndexResult.Target,
		"chunkCount": state.IndexResult.ChunkCount,
		"metadata":   state.IndexResult.Metadata,
	}
}
