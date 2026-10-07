package knowledge

import "time"

type knowledgeDocumentVO struct {
	ID                  string     `json:"id"`
	KnowledgeBaseID     string     `json:"kbId"`
	Name                string     `json:"docName"`
	SourceType          string     `json:"sourceType,omitempty"`
	SourceLocation      string     `json:"sourceLocation,omitempty"`
	ScheduleEnabled     int        `json:"scheduleEnabled"`
	ScheduleCron        string     `json:"scheduleCron,omitempty"`
	Enabled             bool       `json:"enabled"`
	ChunkCount          int        `json:"chunkCount"`
	ImageCount          int        `json:"imageCount"`
	ImageCompletedCount int        `json:"imageCompletedCount"`
	ImageFailedCount    int        `json:"imageFailedCount"`
	FileURL             string     `json:"fileUrl,omitempty"`
	FileType            string     `json:"fileType,omitempty"`
	FileSize            int64      `json:"fileSize"`
	ProcessMode         string     `json:"processMode,omitempty"`
	ChunkStrategy       string     `json:"chunkStrategy,omitempty"`
	ChunkConfig         string     `json:"chunkConfig,omitempty"`
	Status              string     `json:"status,omitempty"`
	CreatedBy           string     `json:"createdBy,omitempty"`
	UpdatedBy           string     `json:"updatedBy,omitempty"`
	CreateTime          *time.Time `json:"createTime,omitempty"`
	UpdateTime          *time.Time `json:"updateTime,omitempty"`
}

type knowledgeDocumentSearchVO struct {
	ID              string `json:"id"`
	KnowledgeBaseID string `json:"kbId"`
	Name            string `json:"docName"`
}

type knowledgeDocumentChunkLogVO struct {
	ID              string     `json:"id"`
	DocumentID      string     `json:"docId"`
	Status          string     `json:"status"`
	ProcessMode     string     `json:"processMode,omitempty"`
	ChunkStrategy   string     `json:"chunkStrategy,omitempty"`
	ExtractDuration int64      `json:"extractDuration"`
	ChunkDuration   int64      `json:"chunkDuration"`
	EmbedDuration   int64      `json:"embedDuration"`
	PersistDuration int64      `json:"persistDuration"`
	TotalDuration   int64      `json:"totalDuration"`
	ChunkCount      int        `json:"chunkCount"`
	ErrorMessage    string     `json:"errorMessage,omitempty"`
	StartTime       *time.Time `json:"startTime,omitempty"`
	EndTime         *time.Time `json:"endTime,omitempty"`
	CreateTime      *time.Time `json:"createTime,omitempty"`
}

type knowledgeDocumentScheduleExecVO struct {
	ID              string     `json:"id"`
	ScheduleID      string     `json:"scheduleId"`
	DocumentID      string     `json:"docId"`
	KnowledgeBaseID string     `json:"kbId"`
	Status          string     `json:"status"`
	Message         string     `json:"message,omitempty"`
	FileName        string     `json:"fileName,omitempty"`
	FileSize        *int64     `json:"fileSize,omitempty"`
	ContentHash     string     `json:"contentHash,omitempty"`
	ETag            string     `json:"etag,omitempty"`
	LastModified    string     `json:"lastModified,omitempty"`
	StartTime       *time.Time `json:"startTime,omitempty"`
	EndTime         *time.Time `json:"endTime,omitempty"`
	CreateTime      *time.Time `json:"createTime,omitempty"`
	UpdateTime      *time.Time `json:"updateTime,omitempty"`
}
