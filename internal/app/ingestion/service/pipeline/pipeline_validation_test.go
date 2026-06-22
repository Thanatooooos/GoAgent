package pipeline

import (
	"context"
	"strings"
	"testing"

	ingestionrunner "local/rag-project/internal/app/ingestion/service/runner"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"local/rag-project/internal/app/ingestion/domain"
)

type pipelineGraphTestRunner struct {
	nodeType string
}

func (r *pipelineGraphTestRunner) NodeType() string {
	return r.nodeType
}

func (r *pipelineGraphTestRunner) Run(
	ctx context.Context,
	state ingestionworkflow.ExecutionState,
	node domain.PipelineNode,
) (ingestionworkflow.ExecutionState, map[string]any, error) {
	return state, nil, nil
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

func TestPipelineServiceValidatePipelineDefinition(t *testing.T) {
	t.Parallel()

	registry := ingestionrunner.NewNodeRunnerRegistry(&pipelineGraphTestRunner{nodeType: "test"})
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
				s.nodeRunners = ingestionrunner.NewNodeRunnerRegistry()
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
