package service

import (
	"time"

	"local/rag-project/internal/app/ingestion/domain"
)

// SourcePayload 描述 fetcher 阶段准备或拉取到的源数据。
type SourcePayload struct {
	Type        string
	Location    string
	FileName    string
	ContentType string
	Bytes       []byte
	Metadata    map[string]any
}

// ParsedDocument 描述 parser 阶段产出的标准文本载荷。
type ParsedDocument struct {
	Content  string
	Title    string
	Metadata map[string]any
}

// ChunkPayload 描述 chunker 阶段产出的结构化分块结果。
type ChunkPayload struct {
	Index    int
	Content  string
	Metadata map[string]any
}

// IndexResult 描述 indexer 阶段产出的写入摘要。
type IndexResult struct {
	Target     string
	ChunkCount int
	Metadata   map[string]any
}

// ExecutionState 描述一条 ingestion task 在编排过程中的共享上下文。
type ExecutionState struct {
	Task        domain.Task
	Pipeline    domain.Pipeline
	Source      SourcePayload
	Parsed      ParsedDocument
	Chunks      []ChunkPayload
	IndexResult IndexResult
	NodeOutputs map[string]map[string]any
	Error       error
	StartedAt   time.Time
	CompletedAt *time.Time
}

// Clone 为后续编排层保留一个轻量复制入口。
func (s ExecutionState) Clone() ExecutionState {
	cloned := s
	if len(s.Chunks) > 0 {
		cloned.Chunks = append([]ChunkPayload(nil), s.Chunks...)
	}
	if len(s.NodeOutputs) > 0 {
		cloned.NodeOutputs = make(map[string]map[string]any, len(s.NodeOutputs))
		for key, value := range s.NodeOutputs {
			cloned.NodeOutputs[key] = value
		}
	}
	return cloned
}

// WorkflowSpec 描述由 pipeline 转换得到的最小可执行工作流定义。
type WorkflowSpec struct {
	TaskID    string
	Pipeline  domain.Pipeline
	NodeOrder []WorkflowNodeSpec
}

// WorkflowNodeSpec 描述工作流中的单个节点。
type WorkflowNodeSpec struct {
	Order int
	Node  domain.PipelineNode
}
