package domain

import "time"

const (
	KnowledgeDocumentSourceFile   = "file"
	KnowledgeDocumentSourceURL    = "url"
	KnowledgeDocumentSourceFeishu = "feishu"
)

const (
	KnowledgeDocumentProcessModeChunk = "chunk"
)

const (
	KnowledgeDocumentStatusPending  = "pending"
	KnowledgeDocumentStatusRunning  = "running"
	KnowledgeDocumentStatusSuccess  = "success"
	KnowledgeDocumentStatusPartial  = "partial"
	KnowledgeDocumentStatusFailed   = "failed"
	KnowledgeDocumentStatusDeleting = "deleting"
)

type KnowledgeDocument struct {
	ID                  string
	KnowledgeBaseID     string
	Name                string
	Enabled             bool
	ChunkCount          int
	ImageCount          int
	ImageCompletedCount int
	ImageFailedCount    int
	FileURL             string
	FileType            string
	FileSize            int64
	ProcessMode         string
	Status              string
	SourceType          string
	SourceLocation      string
	ScheduleEnabled     bool
	ScheduleCron        string
	ChunkStrategy       string
	ChunkConfig         []byte
	Summary             string
	SummaryStatus       string
	SummaryErrorMessage string
	CreatedBy           string
	UpdatedBy           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
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
	return d.IsEnabled() && CanKnowledgeDocumentTransition(d.Status, KnowledgeDocumentStatusRunning)
}

func (d KnowledgeDocument) IsRemote() bool {
	return d.SourceType == KnowledgeDocumentSourceURL
}

func (d KnowledgeDocument) CanDelete() bool {
	return CanKnowledgeDocumentTransition(d.Status, KnowledgeDocumentStatusDeleting)
}

func CanKnowledgeDocumentTransition(fromStatus string, toStatus string) bool {
	switch toStatus {
	case KnowledgeDocumentStatusRunning:
		return fromStatus == KnowledgeDocumentStatusPending ||
			fromStatus == KnowledgeDocumentStatusFailed ||
			fromStatus == KnowledgeDocumentStatusSuccess ||
			fromStatus == KnowledgeDocumentStatusPartial
	case KnowledgeDocumentStatusSuccess, KnowledgeDocumentStatusPartial, KnowledgeDocumentStatusFailed:
		return fromStatus == KnowledgeDocumentStatusRunning
	case KnowledgeDocumentStatusDeleting:
		return fromStatus == KnowledgeDocumentStatusPending ||
			fromStatus == KnowledgeDocumentStatusFailed ||
			fromStatus == KnowledgeDocumentStatusSuccess ||
			fromStatus == KnowledgeDocumentStatusPartial
	default:
		return false
	}
}
