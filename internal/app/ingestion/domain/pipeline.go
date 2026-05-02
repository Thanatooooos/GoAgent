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
	Nodes       []PipelineNode
	CreatedBy   string
	UpdatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PipelineNode 描述 pipeline 中的单个节点定义。
type PipelineNode struct {
	NodeID     string
	NodeType   string
	Settings   map[string]any
	Condition  map[string]any
	NextNodeID string
}
