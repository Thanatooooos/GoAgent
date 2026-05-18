package domain

import (
	"fmt"
	"strings"
)

// NormalizePipelineDefinition fills in defaults (version, edges, entry nodes) for a pipeline definition.
// When nodes are provided alongside an empty definition, they are used as the canonical node list.
func NormalizePipelineDefinition(definition PipelineDefinition, nodes []PipelineNode) PipelineDefinition {
	if len(definition.Nodes) == 0 && len(nodes) > 0 {
		definition.Nodes = ClonePipelineNodes(nodes)
	}
	if len(definition.Nodes) == 0 {
		return PipelineDefinition{}
	}
	if strings.TrimSpace(definition.Version) == "" {
		definition.Version = "v1"
	}
	definition.Nodes = ClonePipelineNodes(definition.Nodes)
	definition.Edges = ClonePipelineEdges(definition.Edges)
	definition.EntryNodeIDs = CloneStringSlice(definition.EntryNodeIDs)
	if len(definition.Edges) == 0 {
		definition = buildDefinitionFromNodes(definition.Nodes)
	}
	if len(definition.EntryNodeIDs) == 0 {
		definition.EntryNodeIDs = InferEntryNodeIDs(definition.Nodes, definition.Edges)
	}
	return definition
}

// ClonePipelineNodes copies node definitions so external slices are not reused.
func ClonePipelineNodes(nodes []PipelineNode) []PipelineNode {
	if len(nodes) == 0 {
		return nil
	}
	result := make([]PipelineNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, PipelineNode{
			NodeID:     strings.TrimSpace(node.NodeID),
			NodeType:   strings.TrimSpace(node.NodeType),
			Settings:   node.Settings,
			Condition:  node.Condition,
			NextNodeID: strings.TrimSpace(node.NextNodeID),
		})
	}
	return result
}

// ClonePipelineEdges copies edge definitions so external slices are not reused.
func ClonePipelineEdges(edges []PipelineEdge) []PipelineEdge {
	if len(edges) == 0 {
		return nil
	}
	result := make([]PipelineEdge, 0, len(edges))
	for _, edge := range edges {
		result = append(result, PipelineEdge{
			EdgeID:      strings.TrimSpace(edge.EdgeID),
			FromNodeID:  strings.TrimSpace(edge.FromNodeID),
			ToNodeID:    strings.TrimSpace(edge.ToNodeID),
			Condition:   edge.Condition,
			Priority:    edge.Priority,
			Description: strings.TrimSpace(edge.Description),
		})
	}
	return result
}

// CloneStringSlice copies a string slice, filtering empty entries.
func CloneStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

// InferEntryNodeIDs returns node IDs with zero in-degree based on the edge set.
func InferEntryNodeIDs(nodes []PipelineNode, edges []PipelineEdge) []string {
	if len(nodes) == 0 {
		return nil
	}
	inDegree := make(map[string]int, len(nodes))
	for _, node := range nodes {
		inDegree[strings.TrimSpace(node.NodeID)] = 0
	}
	for _, edge := range edges {
		toNodeID := strings.TrimSpace(edge.ToNodeID)
		if toNodeID != "" {
			inDegree[toNodeID]++
		}
	}
	result := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		if inDegree[nodeID] == 0 {
			result = append(result, nodeID)
		}
	}
	return result
}

// buildDefinitionFromNodes creates a graph definition from a flat node list,
// inferring edges from NextNodeID or sequential order.
func buildDefinitionFromNodes(nodes []PipelineNode) PipelineDefinition {
	definition := PipelineDefinition{
		Version: "v1",
		Nodes:   ClonePipelineNodes(nodes),
	}
	if len(nodes) == 0 {
		return definition
	}
	edges := make([]PipelineEdge, 0, len(nodes))
	for index, node := range nodes {
		fromNodeID := strings.TrimSpace(node.NodeID)
		toNodeID := strings.TrimSpace(node.NextNodeID)
		if toNodeID == "" && index < len(nodes)-1 {
			toNodeID = strings.TrimSpace(nodes[index+1].NodeID)
		}
		if toNodeID == "" {
			continue
		}
		edges = append(edges, PipelineEdge{
			EdgeID:     fmt.Sprintf("%s__to__%s", fromNodeID, toNodeID),
			FromNodeID: fromNodeID,
			ToNodeID:   toNodeID,
			Condition:  node.Condition,
			Priority:   index,
		})
	}
	definition.Edges = edges
	definition.EntryNodeIDs = InferEntryNodeIDs(definition.Nodes, definition.Edges)
	return definition
}
