package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/scheduledtask/domain"
)

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

type taskRow struct {
	ID              string
	UserID          string
	Status          string
	CurrentVersion  int
	ConversationID  *string
	ConfirmedAt     time.Time
	NextDueAt       *time.Time
	LastReportedAt  *time.Time
	LastAttemptedAt *time.Time
}

type versionRow struct {
	DailyBriefJSON        []byte
	TaskID                string
	Version               int
	Prompt                string
	Name                  string
	ScheduleJSON          []byte
	ReportMode            string
	ConditionKind         string
	KnowledgeBaseIDsJSON  []byte
	AllowedWebDomainsJSON []byte
	AllowedToolIDsJSON    []byte
	ConfirmedAt           time.Time
}

func (s *Store) Create(ctx context.Context, task domain.Task, version domain.Version) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("scheduled task database is required")
	}
	if task.ID == "" || task.UserID == "" || task.ConfirmedAt.IsZero() || task.CurrentVersion != version.Number || version.TaskID != task.ID || task.Status != domain.TaskActive {
		return fmt.Errorf("invalid new task identity or status")
	}
	if err := version.Validate(); err != nil {
		return err
	}
	if task.NextDueAt.IsZero() {
		return fmt.Errorf("new task requires next due time")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO t_scheduled_task (id, user_id, status, current_version, confirmed_at, next_due_at)
			VALUES (?, ?, ?, ?, ?, ?)`, task.ID, task.UserID, task.Status, task.CurrentVersion, task.ConfirmedAt, task.NextDueAt).Error; err != nil {
			return err
		}
		return insertVersion(tx, version)
	})
}

func (s *Store) Get(ctx context.Context, userID, taskID string) (domain.Task, domain.Version, error) {
	if s == nil || s.db == nil {
		return domain.Task{}, domain.Version{}, fmt.Errorf("scheduled task database is required")
	}
	var row taskRow
	result := s.db.WithContext(ctx).Raw(`SELECT id, user_id, status, current_version, conversation_id, confirmed_at,
		next_due_at, last_reported_at, last_attempted_at FROM t_scheduled_task
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, taskID, userID).Scan(&row)
	if result.Error != nil {
		return domain.Task{}, domain.Version{}, result.Error
	}
	if result.RowsAffected == 0 {
		return domain.Task{}, domain.Version{}, gorm.ErrRecordNotFound
	}
	version, err := loadVersion(s.db.WithContext(ctx), taskID, row.CurrentVersion)
	if err != nil {
		return domain.Task{}, domain.Version{}, err
	}
	return row.toDomain(), version, nil
}

func (s *Store) List(ctx context.Context, userID string) ([]domain.Task, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("scheduled task database is required")
	}
	var rows []taskRow
	if err := s.db.WithContext(ctx).Raw(`SELECT id, user_id, status, current_version, conversation_id, confirmed_at,
		next_due_at, last_reported_at, last_attempted_at FROM t_scheduled_task
		WHERE user_id = ? AND deleted_at IS NULL ORDER BY create_time DESC`, userID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	tasks := make([]domain.Task, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, row.toDomain())
	}
	return tasks, nil
}

// ReplaceVersion uses compare-and-swap, so a stale preview cannot change a task.
func (s *Store) ReplaceVersion(ctx context.Context, userID, taskID string, expected int, version domain.Version, nextDue time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("scheduled task database is required")
	}
	if version.TaskID != taskID || version.Number != expected+1 || nextDue.IsZero() {
		return fmt.Errorf("invalid replacement version")
	}
	if err := version.Validate(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Exec(`UPDATE t_scheduled_task SET current_version = ?, confirmed_at = ?, next_due_at = ?, update_time = CURRENT_TIMESTAMP
			WHERE id = ? AND user_id = ? AND current_version = ? AND deleted_at IS NULL`, version.Number, version.ConfirmedAt, nextDue, taskID, userID, expected)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("task version conflict or task not found")
		}
		return insertVersion(tx, version)
	})
}

func (s *Store) SetStatus(ctx context.Context, userID, taskID string, status domain.TaskStatus, nextDue time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("scheduled task database is required")
	}
	if status != domain.TaskActive && status != domain.TaskPaused && status != domain.TaskCompleted {
		return fmt.Errorf("invalid task status %q", status)
	}
	var due any
	if status == domain.TaskActive {
		if nextDue.IsZero() {
			return fmt.Errorf("active task requires next due time")
		}
		due = nextDue
	}
	result := s.db.WithContext(ctx).Exec(`UPDATE t_scheduled_task SET status = ?, next_due_at = ?, update_time = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, status, due, taskID, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, userID, taskID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("scheduled task database is required")
	}
	result := s.db.WithContext(ctx).Exec(`UPDATE t_scheduled_task SET deleted_at = CURRENT_TIMESTAMP, next_due_at = NULL,
		conversation_id = NULL, update_time = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, taskID, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Resume starts a new immutable stage for a completed task. Paused tasks keep
// their approved version; both paths retain any surviving report conversation.
func (s *Store) Resume(ctx context.Context, userID, taskID string, now time.Time) error {
	if s == nil || s.db == nil || now.IsZero() {
		return fmt.Errorf("invalid task resume")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row taskRow
		loaded := tx.Raw(`SELECT id, status, current_version FROM t_scheduled_task
			WHERE id = ? AND user_id = ? AND deleted_at IS NULL FOR UPDATE`, taskID, userID).Scan(&row)
		if loaded.Error != nil {
			return loaded.Error
		}
		if loaded.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if row.Status == "active" {
			return nil
		}
		version, err := loadVersion(tx, taskID, row.CurrentVersion)
		if err != nil {
			return err
		}
		next, err := version.Schedule.Next(now)
		if err != nil || next.IsZero() {
			return fmt.Errorf("task has no future occurrence; edit its schedule before resuming")
		}
		if row.Status == "completed" {
			version.Number++
			version.ConfirmedAt = now
			if err := insertVersion(tx, version); err != nil {
				return err
			}
			return tx.Exec(`UPDATE t_scheduled_task SET status = 'active', current_version = ?, confirmed_at = ?,
				next_due_at = ?, last_feedback_at = NULL, update_time = CURRENT_TIMESTAMP WHERE id = ?`, version.Number, now, next, taskID).Error
		}
		return tx.Exec(`UPDATE t_scheduled_task SET status = 'active', next_due_at = ?, update_time = CURRENT_TIMESTAMP WHERE id = ?`, next, taskID).Error
	})
}

func (s *Store) PauseByConversation(ctx context.Context, userID, conversationID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("scheduled task database is required")
	}
	return s.db.WithContext(ctx).Exec(`UPDATE t_scheduled_task SET status = 'paused', next_due_at = NULL,
		conversation_id = NULL, update_time = CURRENT_TIMESTAMP
		WHERE user_id = ? AND conversation_id = ? AND deleted_at IS NULL`, userID, conversationID).Error
}

func insertVersion(tx *gorm.DB, version domain.Version) error {
	var brief any
	if version.DailyBrief != nil {
		raw, err := json.Marshal(version.DailyBrief)
		if err != nil {
			return err
		}
		brief = string(raw)
	}
	schedule, err := json.Marshal(version.Schedule)
	if err != nil {
		return err
	}
	kb, err := json.Marshal(version.KnowledgeBaseIDs)
	if err != nil {
		return err
	}
	web, err := json.Marshal(version.AllowedWebDomains)
	if err != nil {
		return err
	}
	tools, err := json.Marshal(version.AllowedToolIDs)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO t_scheduled_task_version (daily_brief_json, task_id, version, name, prompt, schedule_json, report_mode,
		condition_kind, knowledge_base_ids_json, allowed_web_domains_json, allowed_tool_ids_json, confirmed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, brief, version.TaskID, version.Number, version.Name, version.Prompt, string(schedule), version.ReportMode,
		version.ConditionKind, string(kb), string(web), string(tools), version.ConfirmedAt).Error
}

func loadVersion(db *gorm.DB, taskID string, number int) (domain.Version, error) {
	var row versionRow
	result := db.Raw(`SELECT daily_brief_json, task_id, version, name, prompt, schedule_json, report_mode, condition_kind,
		knowledge_base_ids_json, allowed_web_domains_json, allowed_tool_ids_json, confirmed_at
		FROM t_scheduled_task_version WHERE task_id = ? AND version = ?`, taskID, number).Scan(&row)
	if result.Error != nil {
		return domain.Version{}, result.Error
	}
	if result.RowsAffected == 0 {
		return domain.Version{}, gorm.ErrRecordNotFound
	}
	version := domain.Version{TaskID: row.TaskID, Number: row.Version, Name: row.Name, Prompt: row.Prompt,
		ReportMode: domain.ReportMode(row.ReportMode), ConditionKind: domain.ConditionKind(row.ConditionKind), ConfirmedAt: row.ConfirmedAt}
	if len(row.DailyBriefJSON) > 0 {
		if err := json.Unmarshal(row.DailyBriefJSON, &version.DailyBrief); err != nil {
			return domain.Version{}, err
		}
	}
	if err := json.Unmarshal(row.ScheduleJSON, &version.Schedule); err != nil {
		return domain.Version{}, err
	}
	for _, item := range []struct {
		raw []byte
		to  *[]string
	}{
		{row.KnowledgeBaseIDsJSON, &version.KnowledgeBaseIDs},
		{row.AllowedWebDomainsJSON, &version.AllowedWebDomains},
		{row.AllowedToolIDsJSON, &version.AllowedToolIDs},
	} {
		if err := json.Unmarshal(item.raw, item.to); err != nil {
			return domain.Version{}, err
		}
	}
	return version, nil
}

func (r taskRow) toDomain() domain.Task {
	result := domain.Task{ID: r.ID, UserID: r.UserID, Status: domain.TaskStatus(r.Status),
		CurrentVersion: r.CurrentVersion, ConfirmedAt: r.ConfirmedAt}
	if r.ConversationID != nil {
		result.ConversationID = *r.ConversationID
	}
	if r.NextDueAt != nil {
		result.NextDueAt = *r.NextDueAt
	}
	if r.LastReportedAt != nil {
		result.LastReportedAt = *r.LastReportedAt
	}
	if r.LastAttemptedAt != nil {
		result.LastAttemptedAt = *r.LastAttemptedAt
	}
	return result
}
