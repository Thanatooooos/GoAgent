package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/compose"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// WorkflowBuilder 定义 pipeline 到编排工作流的转换边界。
type WorkflowBuilder interface {
	Build(ctx context.Context, pipeline domain.Pipeline, task domain.Task) (WorkflowSpec, error)
}

// EinoGraphWorkflowBuilder normalizes a pipeline DAG into an executable workflow spec.
type EinoGraphWorkflowBuilder struct{}

// NewEinoGraphWorkflowBuilder creates a workflow builder for DAG-style pipelines.
func NewEinoGraphWorkflowBuilder() *EinoGraphWorkflowBuilder {
	return &EinoGraphWorkflowBuilder{}
}

// Build validates and orders the pipeline graph for the EINO executor.
func (b *EinoGraphWorkflowBuilder) Build(ctx context.Context, pipeline domain.Pipeline, task domain.Task) (WorkflowSpec, error) {
	_ = ctx

	if strings.TrimSpace(task.ID) == "" {
		return WorkflowSpec{}, exception.NewClientException("task id is required", nil)
	}
	if strings.TrimSpace(pipeline.ID) == "" {
		return WorkflowSpec{}, exception.NewClientException("pipeline id is required", nil)
	}

	definition := domain.NormalizePipelineDefinition(pipeline.Definition, pipeline.Nodes)
	if len(definition.Nodes) == 0 {
		return WorkflowSpec{}, exception.NewClientException("pipeline definition nodes are required", nil)
	}
	if len(definition.EntryNodeIDs) == 0 {
		return WorkflowSpec{}, exception.NewClientException("pipeline entry node ids are required", nil)
	}

	nodesByID := make(map[string]domain.PipelineNode, len(definition.Nodes))
	nodePos := make(map[string]int, len(definition.Nodes))
	inDegree := make(map[string]int, len(definition.Nodes))
	adjacency := make(map[string][]WorkflowEdgeSpec, len(definition.Nodes))
	for index, node := range definition.Nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID == compose.START || nodeID == compose.END {
			return WorkflowSpec{}, exception.NewClientException("pipeline node id is reserved: "+nodeID, nil)
		}
		nodesByID[nodeID] = node
		nodePos[nodeID] = index
		inDegree[nodeID] = 0
	}

	for _, edge := range definition.Edges {
		fromNodeID := strings.TrimSpace(edge.FromNodeID)
		toNodeID := strings.TrimSpace(edge.ToNodeID)
		if fromNodeID == "" || toNodeID == "" {
			return WorkflowSpec{}, exception.NewClientException("pipeline edges must reference existing nodes", nil)
		}
		spec := WorkflowEdgeSpec{
			EdgeID:     strings.TrimSpace(edge.EdgeID),
			FromNodeID: fromNodeID,
			ToNodeID:   toNodeID,
			Condition:  edge.Condition,
			Priority:   edge.Priority,
		}
		adjacency[fromNodeID] = append(adjacency[fromNodeID], spec)
		inDegree[toNodeID]++
	}

	for nodeID := range adjacency {
		sort.SliceStable(adjacency[nodeID], func(i, j int) bool {
			if adjacency[nodeID][i].Priority == adjacency[nodeID][j].Priority {
				return nodePos[adjacency[nodeID][i].ToNodeID] < nodePos[adjacency[nodeID][j].ToNodeID]
			}
			return adjacency[nodeID][i].Priority < adjacency[nodeID][j].Priority
		})
	}

	queue := make([]string, 0, len(definition.Nodes))
	for _, entryNodeID := range definition.EntryNodeIDs {
		entryNodeID = strings.TrimSpace(entryNodeID)
		if entryNodeID != "" {
			queue = append(queue, entryNodeID)
		}
	}
	if len(queue) == 0 {
		for nodeID, degree := range inDegree {
			if degree == 0 {
				queue = append(queue, nodeID)
			}
		}
		sort.SliceStable(queue, func(i, j int) bool { return nodePos[queue[i]] < nodePos[queue[j]] })
	}

	seen := make(map[string]bool, len(definition.Nodes))
	nodeOrder := make([]WorkflowNodeSpec, 0, len(definition.Nodes))
	nextQueue := append([]string(nil), queue...)
	for len(nextQueue) > 0 {
		nodeID := nextQueue[0]
		nextQueue = nextQueue[1:]
		if seen[nodeID] {
			continue
		}
		seen[nodeID] = true
		nodeOrder = append(nodeOrder, WorkflowNodeSpec{
			Order: len(nodeOrder) + 1,
			Node:  nodesByID[nodeID],
		})
		for _, edge := range adjacency[nodeID] {
			inDegree[edge.ToNodeID]--
			if inDegree[edge.ToNodeID] == 0 {
				nextQueue = append(nextQueue, edge.ToNodeID)
			}
		}
		sort.SliceStable(nextQueue, func(i, j int) bool { return nodePos[nextQueue[i]] < nodePos[nextQueue[j]] })
	}
	if len(nodeOrder) != len(definition.Nodes) {
		return WorkflowSpec{}, exception.NewClientException("pipeline graph must be acyclic", nil)
	}

	pipeline.Definition = definition
	pipeline.Nodes = domain.ClonePipelineNodes(definition.Nodes)
	return WorkflowSpec{
		TaskID:        task.ID,
		Pipeline:      pipeline,
		Definition:    definition,
		CacheKey:      fmt.Sprintf("%s:%s:%d", pipeline.ID, definition.Version, pipeline.UpdatedAt.UnixNano()),
		EntryNodeIDs:  domain.CloneStringSlice(definition.EntryNodeIDs),
		NodeOrder:     nodeOrder,
		EdgesBySource: adjacency,
	}, nil
}
