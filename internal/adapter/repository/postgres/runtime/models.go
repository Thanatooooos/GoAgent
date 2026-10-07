package runtime

import "time"

type SessionModel struct {
	ID             string    `gorm:"column:id;type:varchar(64);primaryKey"`
	ConversationID string    `gorm:"column:conversation_id;type:varchar(64);not null;uniqueIndex:uk_runtime_session_message"`
	UserMessageID  string    `gorm:"column:user_message_id;type:varchar(64);not null;uniqueIndex:uk_runtime_session_message"`
	UserID         string    `gorm:"column:user_id;type:varchar(64);not null;index:idx_runtime_session_user"`
	TraceID        string    `gorm:"column:trace_id;type:varchar(64);not null;index:idx_runtime_session_trace"`
	Status         string    `gorm:"column:status;type:varchar(32);not null"`
	NextSequence   int64     `gorm:"column:next_sequence;not null"`
	CreateTime     time.Time `gorm:"column:create_time;not null"`
	UpdateTime     time.Time `gorm:"column:update_time;not null"`
}

func (SessionModel) TableName() string { return "t_runtime_session" }

type JournalModel struct {
	ID               string    `gorm:"column:id;type:varchar(64);primaryKey"`
	RuntimeSessionID string    `gorm:"column:runtime_session_id;type:varchar(64);not null;uniqueIndex:uk_runtime_journal_sequence;index:idx_runtime_journal_session"`
	Sequence         int64     `gorm:"column:sequence;not null;uniqueIndex:uk_runtime_journal_sequence"`
	ConversationID   string    `gorm:"column:conversation_id;type:varchar(64);not null;index:idx_runtime_journal_conversation"`
	UserMessageID    string    `gorm:"column:user_message_id;type:varchar(64);not null"`
	TraceID          string    `gorm:"column:trace_id;type:varchar(64);not null;index:idx_runtime_journal_trace"`
	EventType        string    `gorm:"column:event_type;type:varchar(32);not null"`
	ToolCallID       string    `gorm:"column:tool_call_id;type:varchar(64);index:idx_runtime_journal_tool"`
	ToolName         string    `gorm:"column:tool_name;type:varchar(128)"`
	ToolState        string    `gorm:"column:tool_state;type:varchar(16)"`
	EvidenceJSON     string    `gorm:"column:evidence_json;type:text;not null"`
	Detail           string    `gorm:"column:detail;type:text;not null"`
	CreateTime       time.Time `gorm:"column:create_time;not null"`
}

func (JournalModel) TableName() string { return "t_runtime_journal" }
