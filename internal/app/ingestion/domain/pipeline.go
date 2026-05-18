package domain

import "time"

const (
	// PipelineNodeTypeFetcher 表示拉取输入内容的节点。
	PipelineNodeTypeFetcher = "fetcher"
	// PipelineNodeTypeParser 表示解析输入内容的节点。
	PipelineNodeTypeParser = "parser"
	// PipelineNodeTypeEnhancer 表示增强内容的节点。
	PipelineNodeTypeEnhancer = "enhancer"
	// PipelineNodeTypeChunker 表示分块节点。
	PipelineNodeTypeChunker = "chunker"
	// PipelineNodeTypeEnricher 表示补充元数据的节点。
	PipelineNodeTypeEnricher = "enricher"
	// PipelineNodeTypeIndexer 表示写入下游索引的节点。
	PipelineNodeTypeIndexer = "indexer"
)

// Pipeline 描述一条可配置的数据处理流水线。
type Pipeline struct {
	ID          string
	Name        string
	Description string
	Definition  PipelineDefinition
	Nodes       []PipelineNode
	CreatedBy   string
	UpdatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PipelineDefinition describes a DAG-style ingestion pipeline definition.
type PipelineDefinition struct {
	Version      string
	EntryNodeIDs []string
	Nodes        []PipelineNode
	Edges        []PipelineEdge
}

// PipelineNode 描述 pipeline 中的单个节点定义。
type PipelineNode struct {
	NodeID     string
	NodeType   string
	Settings   map[string]any
	Condition  map[string]any
	NextNodeID string
}

// PipelineEdge describes a directed edge between two pipeline nodes.
type PipelineEdge struct {
	EdgeID      string
	FromNodeID  string
	ToNodeID    string
	Condition   map[string]any
	Priority    int
	Description string
}
