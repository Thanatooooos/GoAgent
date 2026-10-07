package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/distributedid"
)

type TaskJournal struct{ db *gorm.DB }

func NewTaskJournal(db *gorm.DB) *TaskJournal { return &TaskJournal{db: db} }
func (j *TaskJournal) Start(ctx context.Context, request conversationruntime.TaskRequest) (conversationruntime.TaskSession, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return conversationruntime.TaskSession{}, err
	}
	if err := j.db.WithContext(ctx).Exec(`INSERT INTO t_runtime_task_session (id, task_type, task_id, user_id, status, next_sequence) VALUES (?, ?, ?, ?, 'running', 1) ON CONFLICT (task_type, task_id) DO NOTHING`, fmt.Sprint(id), request.TaskType, request.TaskID, request.UserID).Error; err != nil {
		return conversationruntime.TaskSession{}, err
	}
	var session struct{ ID string }
	if err := j.db.WithContext(ctx).Raw(`SELECT id FROM t_runtime_task_session WHERE task_type = ? AND task_id = ?`, request.TaskType, request.TaskID).Scan(&session).Error; err != nil {
		return conversationruntime.TaskSession{}, err
	}
	return conversationruntime.TaskSession{ID: session.ID}, nil
}
func (j *TaskJournal) Append(ctx context.Context, session conversationruntime.TaskSession, event conversationruntime.TaskEvent) error {
	id, err := distributedid.NextID()
	if err != nil {
		return err
	}
	evidence, _ := json.Marshal(event.Evidence)
	return j.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sequence int64
		if err := tx.Raw(`UPDATE t_runtime_task_session SET next_sequence = next_sequence + 1, update_time = CURRENT_TIMESTAMP WHERE id = ? RETURNING next_sequence - 1`, session.ID).Scan(&sequence).Error; err != nil {
			return err
		}
		if sequence == 0 {
			return fmt.Errorf("runtime task session %q not found", session.ID)
		}
		return tx.Exec(`INSERT INTO t_runtime_task_journal (id, runtime_task_session_id, sequence, event_type, tool_call_id, tool_name, tool_state, evidence_json, detail) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, fmt.Sprint(id), session.ID, sequence, event.EventType, event.ToolCallID, event.ToolName, event.ToolState, string(evidence), event.Detail).Error
	})
}
func (j *TaskJournal) Finish(ctx context.Context, session conversationruntime.TaskSession, status string) error {
	return j.db.WithContext(ctx).Exec(`UPDATE t_runtime_task_session SET status = ?, update_time = CURRENT_TIMESTAMP WHERE id = ?`, strings.TrimSpace(status), session.ID).Error
}

var _ conversationruntime.TaskJournal = (*TaskJournal)(nil)
