package domain

import "time"

const (
	KnowledgeDocumentSourceFile = "file"
	KnowledgeDocumentSourceURL  = "url"
)

const (
	KnowledgeDocumentProcessModeChunk    = "chunk"
	KnowledgeDocumentProcessModePipeline = "pipeline"
)

const (
	KnowledgeDocumentStatusPending = "pending"
	KnowledgeDocumentStatusRunning = "running"
	KnowledgeDocumentStatusSuccess = "success"
	KnowledgeDocumentStatusFailed  = "failed"
)

type KnowledgeDocument struct {
	ID              string
	KnowledgeBaseID string
	Name            string
	Enabled         bool
	ChunkCount      int
	FileURL         string
	FileType        string
	FileSize        int64
	ProcessMode     string
	Status          string
	SourceType      string
	SourceLocation  string
	ScheduleEnabled bool
	ScheduleCron    string
	ChunkStrategy   string
	ChunkConfig     []byte
	PipelineID      string
	CreatedBy       string
	UpdatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewUploadedKnowledgeDocument(id, knowledgeBaseID, name, fileURL, fileType, createdBy string, fileSize int64) KnowledgeDocument {
	now := time.Now()
	return KnowledgeDocument{
		ID:              id,
		KnowledgeBaseID: knowledgeBaseID,
		Name:            name,
		Enabled:         true,
		ChunkCount:      0,
		FileURL:         fileURL,
		FileType:        fileType,
		FileSize:        fileSize,
		ProcessMode:     KnowledgeDocumentProcessModeChunk,
		Status:          KnowledgeDocumentStatusPending,
		SourceType:      KnowledgeDocumentSourceFile,
		SourceLocation:  fileURL,
		CreatedBy:       createdBy,
		UpdatedBy:       createdBy,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (d KnowledgeDocument) IsEnabled() bool {
	return d.Enabled
}

func (d KnowledgeDocument) CanStartProcessing() bool {
	if !d.IsEnabled() {
		return false
	}
	return d.Status == KnowledgeDocumentStatusPending ||
		d.Status == KnowledgeDocumentStatusFailed ||
		d.Status == KnowledgeDocumentStatusSuccess
}

func (d KnowledgeDocument) IsRemote() bool {
	return d.SourceType == KnowledgeDocumentSourceURL
}
