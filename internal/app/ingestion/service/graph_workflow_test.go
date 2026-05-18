package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
)

func TestPipelineServiceValidatePipelineDefinition(t *testing.T) {
	t.Parallel()

	registry := NewNodeRunnerRegistry(&graphTestRunner{nodeType: "test"})
	svc := &PipelineService{nodeRunners: registry}

	validDefinition := buildTestPipelineDefinition(
		[]string{"entry", "parse", "index"},
		[][2]string{{"entry", "parse"}, {"parse", "index"}},
		"entry",
	)

	testCases := []struct {
		name       string
		definition domain.PipelineDefinition
		setup      func(*PipelineService)
		wantErr    string
	}{
		{
			name:       "valid dag",
			definition: validDefinition,
		},
		{
			name: "cycle rejected",
			definition: buildTestPipelineDefinition(
				[]string{"entry", "parse"},
				[][2]string{{"entry", "parse"}, {"parse", "entry"}},
				"entry",
			),
			wantErr: "acyclic",
		},
		{
			name: "unreachable node rejected",
			definition: buildTestPipelineDefinition(
				[]string{"entry", "parse", "orphan"},
				[][2]string{{"entry", "parse"}},
				"entry",
			),
			wantErr: "unreachable",
		},
		{
			name: "invalid edge target rejected",
			definition: domain.PipelineDefinition{
				Version:      "v1",
				EntryNodeIDs: []string{"entry"},
				Nodes: []domain.PipelineNode{
					{NodeID: "entry", NodeType: "test"},
				},
				Edges: []domain.PipelineEdge{
					{EdgeID: "entry__to__missing", FromNodeID: "entry", ToNodeID: "missing"},
				},
			},
			wantErr: "existing node",
		},
		{
			name: "invalid entry rejected",
			definition: domain.PipelineDefinition{
				Version:      "v1",
				EntryNodeIDs: []string{"missing"},
				Nodes: []domain.PipelineNode{
					{NodeID: "entry", NodeType: "test"},
				},
			},
			wantErr: "existing node",
		},
		{
			name: "unregistered node type rejected",
			definition: domain.PipelineDefinition{
				Version:      "v1",
				EntryNodeIDs: []string{"entry"},
				Nodes: []domain.PipelineNode{
					{NodeID: "entry", NodeType: "missing"},
				},
			},
			setup: func(s *PipelineService) {
				s.nodeRunners = NewNodeRunnerRegistry()
			},
			wantErr: "not executable",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			localSvc := *svc
			if tc.setup != nil {
				tc.setup(&localSvc)
			}

			err := localSvc.validatePipelineDefinition("demo", tc.definition)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("validatePipelineDefinition() error = %v", err)
			}
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestPipelineServiceValidateNodeContracts(t *testing.T) {
	t.Parallel()

	svc := &PipelineService{}

	t.Run("standard document pipeline is valid", func(t *testing.T) {
		t.Parallel()

		definition := domain.PipelineDefinition{
			Version:      "v1",
			EntryNodeIDs: []string{"fetch"},
			Nodes: []domain.PipelineNode{
				{NodeID: "fetch", NodeType: domain.PipelineNodeTypeFetcher},
				{NodeID: "parse", NodeType: domain.PipelineNodeTypeParser},
				{NodeID: "enhance", NodeType: domain.PipelineNodeTypeEnhancer},
				{NodeID: "chunk", NodeType: domain.PipelineNodeTypeChunker},
				{NodeID: "enrich", NodeType: domain.PipelineNodeTypeEnricher},
				{NodeID: "index", NodeType: domain.PipelineNodeTypeIndexer},
			},
			Edges: []domain.PipelineEdge{
				{EdgeID: "fetch__to__parse", FromNodeID: "fetch", ToNodeID: "parse"},
				{EdgeID: "parse__to__enhance", FromNodeID: "parse", ToNodeID: "enhance"},
				{EdgeID: "enhance__to__chunk", FromNodeID: "enhance", ToNodeID: "chunk"},
				{EdgeID: "chunk__to__enrich", FromNodeID: "chunk", ToNodeID: "enrich"},
				{EdgeID: "enrich__to__index", FromNodeID: "enrich", ToNodeID: "index"},
			},
		}

		if err := svc.validatePipelineDefinition("doc-pipeline", definition); err != nil {
			t.Fatalf("validatePipelineDefinition() error = %v", err)
		}
	})

	t.Run("chunker before parser is rejected", func(t *testing.T) {
		t.Parallel()

		definition := domain.PipelineDefinition{
			Version:      "v1",
			EntryNodeIDs: []string{"fetch"},
			Nodes: []domain.PipelineNode{
				{NodeID: "fetch", NodeType: domain.PipelineNodeTypeFetcher},
				{NodeID: "chunk", NodeType: domain.PipelineNodeTypeChunker},
			},
			Edges: []domain.PipelineEdge{
				{EdgeID: "fetch__to__chunk", FromNodeID: "fetch", ToNodeID: "chunk"},
			},
		}

		err := svc.validatePipelineDefinition("bad-pipeline", definition)
		if err == nil || !strings.Contains(err.Error(), "requires input artifact [parsed]") {
			t.Fatalf("expected parsed artifact validation error, got %v", err)
		}
	})

	t.Run("parser cannot be entry node", func(t *testing.T) {
		t.Parallel()

		definition := domain.PipelineDefinition{
			Version:      "v1",
			EntryNodeIDs: []string{"parse"},
			Nodes: []domain.PipelineNode{
				{NodeID: "parse", NodeType: domain.PipelineNodeTypeParser},
			},
		}

		err := svc.validatePipelineDefinition("bad-entry", definition)
		if err == nil || !strings.Contains(err.Error(), "does not support entry position") {
			t.Fatalf("expected entry contract validation error, got %v", err)
		}
	})

	t.Run("enhancer after chunker is valid", func(t *testing.T) {
		t.Parallel()

		definition := domain.PipelineDefinition{
			Version:      "v1",
			EntryNodeIDs: []string{"fetch"},
			Nodes: []domain.PipelineNode{
				{NodeID: "fetch", NodeType: domain.PipelineNodeTypeFetcher},
				{NodeID: "parse", NodeType: domain.PipelineNodeTypeParser},
				{NodeID: "chunk", NodeType: domain.PipelineNodeTypeChunker},
				{NodeID: "enhance", NodeType: domain.PipelineNodeTypeEnhancer},
				{NodeID: "index", NodeType: domain.PipelineNodeTypeIndexer},
			},
			Edges: []domain.PipelineEdge{
				{EdgeID: "fetch__to__parse", FromNodeID: "fetch", ToNodeID: "parse"},
				{EdgeID: "parse__to__chunk", FromNodeID: "parse", ToNodeID: "chunk"},
				{EdgeID: "chunk__to__enhance", FromNodeID: "chunk", ToNodeID: "enhance"},
				{EdgeID: "enhance__to__index", FromNodeID: "enhance", ToNodeID: "index"},
			},
		}

		if err := svc.validatePipelineDefinition("chunk-enhance", definition); err != nil {
			t.Fatalf("validatePipelineDefinition() error = %v", err)
		}
	})
}

func TestEvaluateWorkflowCondition(t *testing.T) {
	t.Parallel()

	state := ExecutionState{
		Task: domain.Task{
			Metadata: map[string]any{
				"source": "manual",
				"score":  3,
			},
		},
		Artifacts: map[string]any{
			"chunks": map[string]any{
				"count": 3,
			},
		},
		NodeOutputs: map[string]map[string]any{
			"fetch": {
				"success": true,
				"mime":    "text/plain",
			},
		},
	}

	testCases := []struct {
		name      string
		condition map[string]any
		want      bool
	}{
		{
			name: "task metadata eq",
			condition: map[string]any{
				"path":  "task.metadata.source",
				"op":    "eq",
				"value": "manual",
			},
			want: true,
		},
		{
			name: "node output exists",
			condition: map[string]any{
				"path": "nodeOutputs.fetch.mime",
				"op":   "exists",
			},
			want: true,
		},
		{
			name: "artifact numeric comparison",
			condition: map[string]any{
				"path":  "artifacts.chunks.count",
				"op":    "gte",
				"value": 2,
			},
			want: true,
		},
		{
			name: "all condition",
			condition: map[string]any{
				"all": []any{
					map[string]any{"path": "task.metadata.source", "op": "eq", "value": "manual"},
					map[string]any{"path": "nodeOutputs.fetch.success", "op": "eq", "value": true},
				},
			},
			want: true,
		},
		{
			name: "any condition",
			condition: map[string]any{
				"any": []any{
					map[string]any{"path": "task.metadata.source", "op": "eq", "value": "batch"},
					map[string]any{"path": "nodeOutputs.fetch.mime", "op": "in", "value": []any{"application/pdf", "text/plain"}},
				},
			},
			want: true,
		},
		{
			name: "false condition",
			condition: map[string]any{
				"path":  "artifacts.chunks.count",
				"op":    "lt",
				"value": 1,
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := evaluateWorkflowCondition(tc.condition, state)
			if err != nil {
				t.Fatalf("evaluateWorkflowCondition() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("evaluateWorkflowCondition() = %v, want %v", got, tc.want)
			}
		})
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
	runner.outputChecks["join"] = func(state ExecutionState) error {
		if _, ok := state.NodeOutputs["branch_a"]; !ok {
			return errors.New("branch_a output missing before join")
		}
		if _, ok := state.NodeOutputs["branch_b"]; !ok {
			return errors.New("branch_b output missing before join")
		}
		return nil
	}

	svc := NewExecutorService(ExecutorServiceOptions{
		WorkflowBuilder: NewEinoGraphWorkflowBuilder(),
		NodeRunners:     NewNodeRunnerRegistry(runner),
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
		WorkflowBuilder: NewEinoGraphWorkflowBuilder(),
		NodeRunners:     NewNodeRunnerRegistry(runner),
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
	outputChecks  map[string]func(ExecutionState) error
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
		outputChecks: map[string]func(ExecutionState) error{},
		branchReady:  make(chan struct{}),
	}
}

func (r *graphTestRunner) NodeType() string {
	return r.nodeType
}

func (r *graphTestRunner) Run(ctx context.Context, state ExecutionState, node domain.PipelineNode) (ExecutionState, map[string]any, error) {
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
