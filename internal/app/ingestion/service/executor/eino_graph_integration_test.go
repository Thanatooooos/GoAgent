package executor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	ingestionrunner "local/rag-project/internal/app/ingestion/service/runner"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"local/rag-project/internal/app/ingestion/domain"
)

func TestSyncBuiltInArtifactsRefreshesChunksAfterEnricher(t *testing.T) {
	state := &ingestionworkflow.ExecutionState{
		Chunks: []ingestionworkflow.ChunkPayload{
			{
				Index:   0,
				Content: "chunk content",
				Metadata: map[string]any{
					"summary": "chunk summary",
				},
			},
		},
		Artifacts: map[string]any{},
	}
	syncBuiltInArtifacts(state, ingestionworkflow.WorkflowNodeSpec{
		Node: domain.PipelineNode{
			NodeID:   "enrich-1",
			NodeType: domain.PipelineNodeTypeEnricher,
		},
	})

	rawChunks, ok := state.Artifacts["chunks"].([]map[string]any)
	if !ok || len(rawChunks) != 1 {
		t.Fatalf("expected chunk artifacts after enricher, got %#v", state.Artifacts["chunks"])
	}
	metadata, ok := rawChunks[0]["metadata"].(map[string]any)
	if !ok || metadata["summary"] != "chunk summary" {
		t.Fatalf("expected refreshed chunk metadata in artifacts, got %#v", rawChunks[0]["metadata"])
	}
}

func TestEinoGraphExecutorRunsFanOutAndJoin(t *testing.T) {
	runner := newGraphTestRunner("test")
	runner.branchNodes = map[string]struct{}{
		"branch_a": {},
		"branch_b": {},
	}
	runner.outputs["entry"] = map[string]any{"route": "both"}
	runner.outputs["branch_a"] = map[string]any{"done": true}
	runner.outputs["branch_b"] = map[string]any{"done": true}
	runner.outputChecks["join"] = func(state ingestionworkflow.ExecutionState) error {
		if _, ok := state.NodeOutputs["branch_a"]; !ok {
			return errors.New("branch_a output missing before join")
		}
		if _, ok := state.NodeOutputs["branch_b"]; !ok {
			return errors.New("branch_b output missing before join")
		}
		return nil
	}

	svc := NewExecutorService(ExecutorServiceOptions{
		WorkflowBuilder: ingestionworkflow.NewEinoGraphWorkflowBuilder(),
		NodeRunners:     ingestionrunner.NewNodeRunnerRegistry(runner),
		MaxConcurrent:   4,
	})

	pipeline := buildTestPipeline(
		"pipeline-fanout",
		buildTestPipelineDefinition(
			[]string{"entry", "branch_a", "branch_b", "join"},
			[][2]string{{"entry", "branch_a"}, {"entry", "branch_b"}, {"branch_a", "join"}, {"branch_b", "join"}},
			"entry",
		),
	)
	task := domain.Task{ID: "task-fanout", PipelineID: pipeline.ID}

	workflow, err := svc.workflowBuilder.Build(context.Background(), pipeline, task)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	state, err := svc.graphExecutor.Execute(ctx, workflow, svc.newExecutionState(task, pipeline))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.callCount("entry") != 1 || runner.callCount("branch_a") != 1 || runner.callCount("branch_b") != 1 || runner.callCount("join") != 1 {
		t.Fatalf("unexpected node call counts: entry=%d branch_a=%d branch_b=%d join=%d",
			runner.callCount("entry"), runner.callCount("branch_a"), runner.callCount("branch_b"), runner.callCount("join"))
	}
	if _, ok := state.NodeOutputs["join"]; !ok {
		t.Fatalf("expected join output to be committed")
	}
}

func TestEinoGraphExecutorSkipsInactiveConditionalBranch(t *testing.T) {
	runner := newGraphTestRunner("test")
	runner.outputs["entry"] = map[string]any{"route": "left"}
	runner.outputs["left"] = map[string]any{"selected": true}
	runner.outputs["right"] = map[string]any{"selected": true}

	svc := NewExecutorService(ExecutorServiceOptions{
		WorkflowBuilder: ingestionworkflow.NewEinoGraphWorkflowBuilder(),
		NodeRunners:     ingestionrunner.NewNodeRunnerRegistry(runner),
		MaxConcurrent:   4,
	})

	definition := buildTestPipelineDefinition(
		[]string{"entry", "left", "right"},
		nil,
		"entry",
	)
	definition.Edges = []domain.PipelineEdge{
		{
			EdgeID:     "entry__to__left",
			FromNodeID: "entry",
			ToNodeID:   "left",
			Condition:  map[string]any{"path": "nodeOutputs.entry.route", "op": "eq", "value": "left"},
			Priority:   0,
		},
		{
			EdgeID:     "entry__to__right",
			FromNodeID: "entry",
			ToNodeID:   "right",
			Condition:  map[string]any{"path": "nodeOutputs.entry.route", "op": "eq", "value": "right"},
			Priority:   1,
		},
	}

	pipeline := buildTestPipeline("pipeline-conditional", definition)
	task := domain.Task{ID: "task-conditional", PipelineID: pipeline.ID}

	workflow, err := svc.workflowBuilder.Build(context.Background(), pipeline, task)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	state, err := svc.graphExecutor.Execute(context.Background(), workflow, svc.newExecutionState(task, pipeline))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if runner.callCount("left") != 1 {
		t.Fatalf("expected left branch to execute once, got %d", runner.callCount("left"))
	}
	if runner.callCount("right") != 0 {
		t.Fatalf("expected right branch to be skipped, got %d", runner.callCount("right"))
	}
	if _, ok := state.NodeOutputs["right"]; ok {
		t.Fatalf("expected right branch output to be absent")
	}
}

func buildTestPipeline(id string, definition domain.PipelineDefinition) domain.Pipeline {
	return domain.Pipeline{
		ID:         id,
		Name:       id,
		Definition: definition,
		Nodes:      domain.ClonePipelineNodes(definition.Nodes),
		UpdatedAt:  time.Unix(1, 0),
	}
}

func buildTestPipelineDefinition(nodeIDs []string, edges [][2]string, entryNodeID string) domain.PipelineDefinition {
	nodes := make([]domain.PipelineNode, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		nodes = append(nodes, domain.PipelineNode{
			NodeID:   nodeID,
			NodeType: "test",
		})
	}
	definitionEdges := make([]domain.PipelineEdge, 0, len(edges))
	for index, edge := range edges {
		definitionEdges = append(definitionEdges, domain.PipelineEdge{
			EdgeID:     edge[0] + "__to__" + edge[1],
			FromNodeID: edge[0],
			ToNodeID:   edge[1],
			Priority:   index,
		})
	}
	return domain.PipelineDefinition{
		Version:      "v1",
		EntryNodeIDs: []string{entryNodeID},
		Nodes:        nodes,
		Edges:        definitionEdges,
	}
}

type graphTestRunner struct {
	nodeType string

	mu            sync.Mutex
	calls         map[string]int
	outputs       map[string]map[string]any
	outputChecks  map[string]func(ingestionworkflow.ExecutionState) error
	branchNodes   map[string]struct{}
	branchReady   chan struct{}
	branchReadyMu sync.Once
	branchStarts  int
}

func newGraphTestRunner(nodeType string) *graphTestRunner {
	return &graphTestRunner{
		nodeType:     nodeType,
		calls:        map[string]int{},
		outputs:      map[string]map[string]any{},
		outputChecks: map[string]func(ingestionworkflow.ExecutionState) error{},
		branchReady:  make(chan struct{}),
	}
}

func (r *graphTestRunner) NodeType() string {
	return r.nodeType
}

func (r *graphTestRunner) Run(
	ctx context.Context,
	state ingestionworkflow.ExecutionState,
	node domain.PipelineNode,
) (ingestionworkflow.ExecutionState, map[string]any, error) {
	r.mu.Lock()
	r.calls[node.NodeID]++
	_, isBranch := r.branchNodes[node.NodeID]
	if isBranch {
		r.branchStarts++
		if r.branchStarts == len(r.branchNodes) {
			r.branchReadyMu.Do(func() {
				close(r.branchReady)
			})
		}
	}
	check := r.outputChecks[node.NodeID]
	output := cloneStringAnyMap(r.outputs[node.NodeID])
	r.mu.Unlock()

	if isBranch {
		select {
		case <-r.branchReady:
		case <-ctx.Done():
			return state, nil, ctx.Err()
		}
	}
	if check != nil {
		if err := check(state); err != nil {
			return state, nil, err
		}
	}
	if output == nil {
		output = map[string]any{"nodeId": node.NodeID}
	}
	return state, output, nil
}

func (r *graphTestRunner) callCount(nodeID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[nodeID]
}

func cloneStringAnyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
