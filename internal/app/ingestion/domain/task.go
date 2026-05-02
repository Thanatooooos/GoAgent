package domain

import "time"

const (
	// TaskSourceTypeFile 表示文件上传输入。
	TaskSourceTypeFile = "file"
	// TaskSourceTypeURL 表示 URL 输入。
	TaskSourceTypeURL = "url"
	// TaskSourceTypeFeishu 表示飞书输入。
	TaskSourceTypeFeishu = "feishu"
	// TaskSourceTypeS3 表示对象存储输入。
	TaskSourceTypeS3 = "s3"
)

const (
	// TaskStatusPending 表示任务已创建，等待执行。
	TaskStatusPending = "pending"
	// TaskStatusRunning 表示任务执行中。
	TaskStatusRunning = "running"
	// TaskStatusSuccess 表示任务执行成功。
	TaskStatusSuccess = "success"
	// TaskStatusFailed 表示任务执行失败。
	TaskStatusFailed = "failed"
)

// Task 描述一次 ingestion 执行任务。
type Task struct {
	ID             string
	PipelineID     string
	SourceType     string
	SourceLocation string
	SourceFileName string
	Status         string
	ChunkCount     int
	ErrorMessage   string
	Metadata       map[string]any
	StartedAt      *time.Time
	CompletedAt    *time.Time
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TaskNode 描述任务执行过程中的单节点记录。
type TaskNode struct {
	ID           string
	TaskID       string
	PipelineID   string
	NodeID       string
	NodeType     string
	NodeOrder    int
	Status       string
	DurationMs   int64
	Message      string
	ErrorMessage string
	Output       map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
