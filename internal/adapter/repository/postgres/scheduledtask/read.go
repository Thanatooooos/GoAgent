package scheduledtask

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"local/rag-project/internal/app/scheduledtask/domain"
)

type RunView struct {
	ID                 string    `json:"id"`
	Version            int       `json:"version"`
	ScheduledAt        time.Time `json:"scheduledAt"`
	Status             string    `json:"status"`
	ResultSignal       string    `json:"resultSignal,omitempty"`
	PublishedMessageID *string   `json:"publishedMessageId,omitempty"`
}

type ToolEventView struct {
	Sequence  int64           `json:"sequence"`
	EventType string          `json:"eventType"`
	ToolName  string          `json:"toolName"`
	ToolState string          `json:"toolState"`
	Detail    string          `json:"detail"`
	Evidence  json.RawMessage `json:"evidence" gorm:"column:evidence_json"`
}

type AttemptView struct {
	ID               string          `json:"id"`
	RuntimeSessionID string          `json:"runtimeSessionId,omitempty"`
	Status           string          `json:"status"`
	ErrorMessage     string          `json:"errorMessage,omitempty"`
	StartedAt        time.Time       `json:"startedAt"`
	FinishedAt       *time.Time      `json:"finishedAt,omitempty"`
	Tools            []ToolEventView `json:"tools" gorm:"-"`
	ModelOutput      string          `json:"modelOutput,omitempty" gorm:"-"`
}

type RunDetail struct {
	Run      RunView         `json:"run"`
	Version  domain.Version  `json:"version"`
	Outcome  json.RawMessage `json:"outcome,omitempty"`
	Attempts []AttemptView   `json:"attempts"`
}

func (s *Store) GetRun(ctx context.Context, userID, taskID, occurrenceID string) (RunDetail, error) {
	var row struct {
		RunView
		ResultJSON json.RawMessage
	}
	loaded := s.db.WithContext(ctx).Raw(`SELECT o.id, o.version, o.scheduled_at, o.status,
		o.result_signal, o.published_message_id, o.result_json FROM t_scheduled_task_occurrence o
		JOIN t_scheduled_task t ON t.id = o.task_id
		WHERE t.user_id = ? AND t.id = ? AND t.deleted_at IS NULL AND o.id = ?`, userID, taskID, occurrenceID).Scan(&row)
	if loaded.Error != nil {
		return RunDetail{}, loaded.Error
	}
	if loaded.RowsAffected != 1 {
		return RunDetail{}, gorm.ErrRecordNotFound
	}
	version, err := loadVersion(s.db.WithContext(ctx), taskID, row.Version)
	if err != nil {
		return RunDetail{}, err
	}
	result := RunDetail{Run: row.RunView, Version: version, Outcome: row.ResultJSON, Attempts: make([]AttemptView, 0)}
	if err := s.db.WithContext(ctx).Raw(`SELECT id, runtime_session_id, status, error_message, started_at, finished_at
		FROM t_scheduled_task_attempt WHERE occurrence_id = ? ORDER BY started_at, id`, occurrenceID).Scan(&result.Attempts).Error; err != nil {
		return RunDetail{}, err
	}
	for i := range result.Attempts {
		attempt := &result.Attempts[i]
		attempt.Tools = make([]ToolEventView, 0)
		if attempt.RuntimeSessionID == "" {
			continue
		}
		if err := s.db.WithContext(ctx).Raw(`SELECT j.detail FROM t_runtime_task_journal j
			JOIN t_runtime_task_session r ON r.id = j.runtime_task_session_id
			WHERE r.id = ? AND r.user_id = ? AND r.task_type = 'scheduled' AND r.task_id = ?
			AND j.event_type = 'answer_final' ORDER BY j.sequence DESC LIMIT 1`, attempt.RuntimeSessionID, userID, attempt.ID).Scan(&attempt.ModelOutput).Error; err != nil {
			return RunDetail{}, err
		}
		if err := s.db.WithContext(ctx).Raw(`SELECT j.sequence, j.event_type, j.tool_name, j.tool_state, j.detail, j.evidence_json::jsonb AS evidence_json
			FROM t_runtime_task_journal j JOIN t_runtime_task_session r ON r.id = j.runtime_task_session_id
			WHERE r.id = ? AND r.user_id = ? AND r.task_type = 'scheduled' AND r.task_id = ? AND j.tool_name <> ''
			ORDER BY j.sequence LIMIT 200`, attempt.RuntimeSessionID, userID, attempt.ID).Scan(&attempt.Tools).Error; err != nil {
			return RunDetail{}, err
		}
	}
	return result, nil
}

func (s *Store) ListRuns(ctx context.Context, userID, taskID string) ([]RunView, error) {
	var rows []RunView
	err := s.db.WithContext(ctx).Raw(`SELECT o.id, o.version, o.scheduled_at, o.status,
		o.result_signal, o.published_message_id FROM t_scheduled_task_occurrence o
		JOIN t_scheduled_task t ON t.id = o.task_id
		WHERE t.user_id = ? AND t.id = ? AND t.deleted_at IS NULL
		ORDER BY o.scheduled_at DESC LIMIT 100`, userID, taskID).Scan(&rows).Error
	return rows, err
}

// LatestReport is independent of the recent-run limit: silent checks must not
// hide the last report, and unpublished or superseded results are not reports.
func (s *Store) LatestReport(ctx context.Context, userID, taskID string) (*RunDetail, error) {
	if _, _, err := s.Get(ctx, userID, taskID); err != nil {
		return nil, err
	}
	var id string
	err := s.db.WithContext(ctx).Raw(`SELECT o.id FROM t_scheduled_task_occurrence o
		JOIN t_scheduled_task t ON t.id = o.task_id
		WHERE t.user_id = ? AND t.id = ? AND t.deleted_at IS NULL
		AND o.result_signal = 'report' AND o.published_message_id IS NOT NULL
		ORDER BY o.scheduled_at DESC, o.id DESC LIMIT 1`, userID, taskID).Scan(&id).Error
	if err != nil || id == "" {
		return nil, err
	}
	detail, err := s.GetRun(ctx, userID, taskID, id)
	return &detail, err
}

type UnreadView struct {
	ConversationID string `json:"conversationId"`
	UnreadCount    int    `json:"unreadCount"`
}

func (s *Store) ListUnread(ctx context.Context, userID string) ([]UnreadView, error) {
	var rows []UnreadView
	err := s.db.WithContext(ctx).Raw(`SELECT u.conversation_id, u.unread_count
		FROM t_conversation_unread u JOIN t_conversation c ON c.conversation_id = u.conversation_id AND c.user_id = u.user_id
		WHERE u.user_id = ? AND c.deleted = 0 AND u.unread_count > 0`, userID).Scan(&rows).Error
	return rows, err
}

func (s *Store) MarkRead(ctx context.Context, userID, conversationID string) error {
	result := s.db.WithContext(ctx).Exec(`UPDATE t_conversation_unread SET unread_count = 0
		WHERE user_id = ? AND conversation_id = ? AND EXISTS
		(SELECT 1 FROM t_conversation WHERE user_id = ? AND conversation_id = ? AND deleted = 0)`,
		userID, conversationID, userID, conversationID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var exists int
		if err := s.db.WithContext(ctx).Raw(`SELECT 1 FROM t_conversation WHERE user_id = ? AND conversation_id = ? AND deleted = 0`,
			userID, conversationID).Scan(&exists).Error; err != nil {
			return err
		}
		if exists != 1 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}
