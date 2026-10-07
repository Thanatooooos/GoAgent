package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	ragdomain "local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/scheduledtask/domain"
	"local/rag-project/internal/framework/distributedid"
)

var ErrTaskNotPublishable = fmt.Errorf("scheduled task is not publishable")

type PublishReportInput struct {
	TaskID       string
	UserID       string
	Version      int
	OccurrenceID string
	Body         string
	Sources      []string
	Now          time.Time
}

type PublishReportResult struct {
	ConversationID string
	MessageID      string
	AlreadySent    bool
}

// PublishReport commits the first conversation, one message, unread count,
// notification key, and occurrence state together. Retrying this call cannot
// create a second user-visible message for the same occurrence.
func (s *Store) PublishReport(ctx context.Context, input PublishReportInput) (PublishReportResult, error) {
	if s == nil || s.db == nil {
		return PublishReportResult{}, fmt.Errorf("scheduled task database is required")
	}
	if input.TaskID == "" || input.UserID == "" || input.OccurrenceID == "" || input.Version < 1 || strings.TrimSpace(input.Body) == "" || input.Now.IsZero() {
		return PublishReportResult{}, fmt.Errorf("invalid scheduled report")
	}
	var published PublishReportResult
	sources, err := json.Marshal(reportSources(input.Sources))
	if err != nil {
		return PublishReportResult{}, err
	}
	pausedForDeletedConversation := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task taskRow
		loaded := tx.Raw(`SELECT id, user_id, status, current_version, conversation_id, confirmed_at FROM t_scheduled_task
			WHERE id = ? AND user_id = ? AND deleted_at IS NULL FOR UPDATE`, input.TaskID, input.UserID).Scan(&task)
		if loaded.Error != nil {
			return loaded.Error
		}
		if loaded.RowsAffected != 1 {
			return ErrTaskNotPublishable
		}
		var occurrence struct {
			ScheduledAt        time.Time
			ResultJSON         []byte
			Status             string
			PublishedMessageID *string
			DeadlineAt         time.Time
		}
		loaded = tx.Raw(`SELECT scheduled_at, result_json, status, published_message_id, deadline_at FROM t_scheduled_task_occurrence
			WHERE id = ? AND task_id = ? AND version = ? FOR UPDATE`, input.OccurrenceID, input.TaskID, input.Version).Scan(&occurrence)
		if loaded.Error != nil {
			return loaded.Error
		}
		if loaded.RowsAffected != 1 {
			return ErrTaskNotPublishable
		}
		if occurrence.PublishedMessageID != nil {
			published.MessageID = *occurrence.PublishedMessageID
			published.AlreadySent = true
			if task.ConversationID != nil {
				published.ConversationID = *task.ConversationID
			}
			return nil
		}
		if task.Status != "active" || task.CurrentVersion != input.Version || occurrence.Status != "ready" || input.Now.After(occurrence.DeadlineAt) {
			return ErrTaskNotPublishable
		}
		version, err := loadVersion(tx, input.TaskID, input.Version)
		if err != nil {
			return err
		}
		// Validate the conversation before projecting an issue: a deleted report
		// conversation must suppress both views of the in-flight publication.
		conversationID := ""
		if task.ConversationID != nil {
			conversationID = *task.ConversationID
			var exists int
			if err := tx.Raw(`SELECT 1 FROM t_conversation WHERE conversation_id = ? AND user_id = ? AND deleted = 0`, conversationID, input.UserID).Scan(&exists).Error; err != nil {
				return err
			}
			if exists != 1 {
				if err := tx.Exec(`UPDATE t_scheduled_task SET status = 'paused', conversation_id = NULL,
					next_due_at = NULL, update_time = CURRENT_TIMESTAMP WHERE id = ?`, input.TaskID).Error; err != nil {
					return err
				}
				pausedForDeletedConversation = true
				return nil
			}
		}
		if version.DailyBrief != nil {
			var outcome domain.Outcome
			if err := json.Unmarshal(occurrence.ResultJSON, &outcome); err != nil {
				return err
			}
			if outcome.Signal != domain.SignalReport {
				return ErrTaskNotPublishable
			}
			outcome, skipped, err := projectDailyBrief(tx, ctx, input, version, occurrence.ScheduledAt, outcome)
			if err != nil {
				return err
			}
			if skipped {
				return nil
			}
			input.Body = outcome.Body
			sources, err = json.Marshal(reportSources(outcome.Sources))
			if err != nil {
				return err
			}
			if err := saveBriefOutcome(tx, input.OccurrenceID, outcome); err != nil {
				return err
			}
		}
		if conversationID == "" {
			id, err := nextID()
			if err != nil {
				return err
			}
			conversationID = id
			version, err := loadVersion(tx, input.TaskID, input.Version)
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO t_conversation (id, conversation_id, user_id, title, last_time,
				create_time, update_time, deleted) VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
				id, id, input.UserID, conversationTitle(version.DisplayName()), input.Now, input.Now, input.Now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE t_scheduled_task SET conversation_id = ?, update_time = CURRENT_TIMESTAMP WHERE id = ?`, conversationID, input.TaskID).Error; err != nil {
				return err
			}
		}
		messageID, err := nextID()
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_message (id, conversation_id, user_id, role, content, sources,
			create_time, update_time, deleted) VALUES (?, ?, ?, 'assistant', ?, ?::jsonb, ?, ?, 0)`,
			messageID, conversationID, input.UserID, strings.TrimSpace(input.Body), string(sources), input.Now, input.Now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE t_conversation SET last_time = ?, update_time = ?
			WHERE conversation_id = ? AND user_id = ?`, input.Now, input.Now, conversationID, input.UserID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_conversation_unread (user_id, conversation_id, unread_count, last_message_at)
			VALUES (?, ?, 1, ?) ON CONFLICT (user_id, conversation_id)
			DO UPDATE SET unread_count = t_conversation_unread.unread_count + 1, last_message_at = EXCLUDED.last_message_at`,
			input.UserID, conversationID, input.Now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE t_scheduled_task_occurrence SET status = 'reported', published_message_id = ?,
			update_time = CURRENT_TIMESTAMP WHERE id = ?`, messageID, input.OccurrenceID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE t_scheduled_task SET last_reported_at = ?,
			status = CASE WHEN (SELECT report_mode = 'on_condition' OR schedule_json->>'kind' = 'once' FROM t_scheduled_task_version WHERE task_id = ? AND version = ?)
				THEN 'completed' ELSE status END,
			next_due_at = CASE WHEN (SELECT report_mode = 'on_condition' OR schedule_json->>'kind' = 'once' FROM t_scheduled_task_version WHERE task_id = ? AND version = ?)
				THEN NULL ELSE next_due_at END,
			update_time = CURRENT_TIMESTAMP WHERE id = ?`, input.Now, input.TaskID, input.Version, input.TaskID, input.Version, input.TaskID).Error; err != nil {
			return err
		}
		notificationID, err := nextID()
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_scheduled_task_notification
			(id, task_id, occurrence_id, event_key, conversation_id, message_id)
			VALUES (?, ?, ?, ?, ?, ?)`, notificationID, input.TaskID, input.OccurrenceID,
			"occ:"+input.OccurrenceID, conversationID, messageID).Error; err != nil {
			return err
		}
		published.ConversationID, published.MessageID = conversationID, messageID
		return nil
	})
	if err != nil {
		return PublishReportResult{}, err
	}
	if pausedForDeletedConversation {
		return PublishReportResult{}, ErrTaskNotPublishable
	}
	return published, nil
}

func conversationTitle(prompt string) string {
	title := []rune(strings.Join(strings.Fields(prompt), " "))
	if len(title) > 40 {
		title = append(title[:40], '…')
	}
	return "定时任务 · " + string(title)
}

func reportSources(sources []string) []ragdomain.MessageSource {
	result := make([]ragdomain.MessageSource, 0, len(sources))
	seen := map[string]bool{}
	for _, raw := range sources {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		u, err := url.Parse(raw)
		if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			result = append(result, ragdomain.MessageSource{Type: "web", URL: raw, Title: raw})
		} else {
			result = append(result, ragdomain.MessageSource{Type: "reference", Title: raw})
		}
	}
	return result
}

func nextID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", err
	}
	return fmt.Sprint(id), nil
}
