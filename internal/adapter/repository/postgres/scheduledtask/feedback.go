package scheduledtask

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/scheduledtask/domain"
)

type FeedbackOptions struct {
	Daily            time.Duration
	Weekly           time.Duration
	Monthly          time.Duration
	FailureThreshold int
}

func feedbackInterval(schedule domain.Schedule, opts FeedbackOptions) time.Duration {
	switch schedule.Kind {
	case domain.ScheduleDaily:
		return opts.Daily
	case domain.ScheduleWeekly:
		return opts.Weekly
	case domain.ScheduleMonthly:
		return opts.Monthly
	case domain.ScheduleInterval:
		if schedule.EverySeconds <= 24*3600 {
			return opts.Daily
		}
		return opts.Weekly
	default:
		return 0
	}
}

// PublishDueFeedback sends a low-frequency in-app status message after
// repeated model no_report outcomes. It does not complete the watch task.
func (s *Store) PublishDueFeedback(ctx context.Context, now time.Time, limit int, opts FeedbackOptions) (int, error) {
	if s == nil || s.db == nil || now.IsZero() || limit < 1 || opts.Daily <= 0 || opts.Weekly <= 0 || opts.Monthly <= 0 {
		return 0, fmt.Errorf("invalid feedback sweep")
	}
	published := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			ID             string
			UserID         string
			CurrentVersion int
			ConversationID *string
			ConfirmedAt    time.Time
			LastReportedAt *time.Time
			LastFeedbackAt *time.Time
		}
		if err := tx.Raw(`SELECT t.id, t.user_id, t.current_version, t.conversation_id,
			t.confirmed_at, t.last_reported_at, t.last_feedback_at FROM t_scheduled_task t
			JOIN t_scheduled_task_version v ON v.task_id = t.id AND v.version = t.current_version
			WHERE t.status = 'active' AND t.deleted_at IS NULL
			ORDER BY COALESCE(t.last_status_scan_at, t.create_time), t.id
			LIMIT ? FOR UPDATE OF t SKIP LOCKED`, limit).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Exec(`UPDATE t_scheduled_task SET last_status_scan_at = ? WHERE id = ?`, now, row.ID).Error; err != nil {
				return err
			}
			version, err := loadVersion(tx, row.ID, row.CurrentVersion)
			if err != nil {
				return err
			}
			failureKey, err := dueFailureNotification(tx, row.ID, row.CurrentVersion, opts.FailureThreshold)
			if err != nil {
				return err
			}
			interval := feedbackInterval(version.Schedule, opts)
			base := version.ConfirmedAt
			if row.LastReportedAt != nil && row.LastReportedAt.After(base) {
				base = *row.LastReportedAt
			}
			if row.LastFeedbackAt != nil && row.LastFeedbackAt.After(base) {
				base = *row.LastFeedbackAt
			}
			if failureKey == "" && (version.ReportMode != domain.ReportOnCondition || interval == 0 || now.Before(base.Add(interval))) {
				continue
			}
			var noReportCount int64
			if err := tx.Raw(`SELECT count(*) FROM t_scheduled_task_occurrence
				WHERE task_id = ? AND version = ? AND status = 'no_report' AND scheduled_at > ?`,
				row.ID, row.CurrentVersion, base).Scan(&noReportCount).Error; err != nil {
				return err
			}
			if failureKey == "" && noReportCount == 0 {
				continue
			}
			conversationID := ""
			if row.ConversationID != nil {
				conversationID = *row.ConversationID
				var exists int
				if err := tx.Raw(`SELECT 1 FROM t_conversation WHERE conversation_id = ? AND user_id = ? AND deleted = 0`,
					conversationID, row.UserID).Scan(&exists).Error; err != nil {
					return err
				}
				if exists != 1 {
					if err := tx.Exec(`UPDATE t_scheduled_task SET status = 'paused', next_due_at = NULL,
						conversation_id = NULL, update_time = CURRENT_TIMESTAMP WHERE id = ?`, row.ID).Error; err != nil {
						return err
					}
					continue
				}
			} else {
				conversationID, err = nextID()
				if err != nil {
					return err
				}
				if err := tx.Exec(`INSERT INTO t_conversation (id, conversation_id, user_id, title,
					last_time, create_time, update_time, deleted) VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
					conversationID, conversationID, row.UserID, conversationTitle(version.DisplayName()), now, now, now).Error; err != nil {
					return err
				}
				if err := tx.Exec(`UPDATE t_scheduled_task SET conversation_id = ? WHERE id = ?`, conversationID, row.ID).Error; err != nil {
					return err
				}
			}
			messageID, err := nextID()
			if err != nil {
				return err
			}
			body := "这个定时任务已连续一段时间没有值得汇报的新结果。要继续按当前条件监测，还是调整条件或频率？你可以在定时任务页面修改。"
			if failureKey != "" {
				body = fmt.Sprintf("这个定时任务最近连续 %d 次计划运行出现技术失败或模型无法判断。任务配置未改变；请在定时任务页面查看运行记录、资料范围和频率。", opts.FailureThreshold)
			}
			if err := tx.Exec(`INSERT INTO t_message (id, conversation_id, user_id, role, content, sources,
				create_time, update_time, deleted) VALUES (?, ?, ?, 'assistant', ?, '[]'::jsonb, ?, ?, 0)`,
				messageID, conversationID, row.UserID, body, now, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE t_conversation SET last_time = ?, update_time = ? WHERE conversation_id = ? AND user_id = ?`,
				now, now, conversationID, row.UserID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO t_conversation_unread (user_id, conversation_id, unread_count, last_message_at)
				VALUES (?, ?, 1, ?) ON CONFLICT (user_id, conversation_id) DO UPDATE
				SET unread_count = t_conversation_unread.unread_count + 1, last_message_at = EXCLUDED.last_message_at`,
				row.UserID, conversationID, now).Error; err != nil {
				return err
			}
			notificationID, err := nextID()
			if err != nil {
				return err
			}
			eventKey := "feedback:" + notificationID
			if failureKey != "" {
				eventKey = failureKey
			}
			if err := tx.Exec(`INSERT INTO t_scheduled_task_notification
				(id, task_id, event_key, conversation_id, message_id) VALUES (?, ?, ?, ?, ?)`,
				notificationID, row.ID, eventKey, conversationID, messageID).Error; err != nil {
				return err
			}
			if failureKey == "" {
				if err := tx.Exec(`UPDATE t_scheduled_task SET last_feedback_at = ?, update_time = CURRENT_TIMESTAMP WHERE id = ?`,
					now, row.ID).Error; err != nil {
					return err
				}
			}
			published++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return published, nil
}

// A failure episode is reset by a terminal non-failure result. Retries and
// in-flight runs do not count; the unique event key survives process restarts.
func dueFailureNotification(tx *gorm.DB, taskID string, version, threshold int) (string, error) {
	if threshold < 1 {
		return "", nil
	}
	var recent []struct{ Status string }
	if err := tx.Raw(`SELECT status FROM t_scheduled_task_occurrence WHERE task_id = ? AND version = ?
		AND status IN ('failed', 'uncertain', 'reported', 'no_report', 'missed')
		ORDER BY scheduled_at DESC, id DESC LIMIT ?`, taskID, version, threshold).Scan(&recent).Error; err != nil {
		return "", err
	}
	if len(recent) < threshold {
		return "", nil
	}
	for _, row := range recent {
		if row.Status != "failed" && row.Status != "uncertain" {
			return "", nil
		}
	}
	var anchor string
	if err := tx.Raw(`SELECT id FROM t_scheduled_task_occurrence WHERE task_id = ? AND version = ?
		AND status IN ('reported', 'no_report', 'missed') ORDER BY scheduled_at DESC, id DESC LIMIT 1`, taskID, version).Scan(&anchor).Error; err != nil {
		return "", err
	}
	key := fmt.Sprintf("failure:v%d:after:%s", version, anchor)
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM t_scheduled_task_notification WHERE task_id = ? AND event_key = ?`, taskID, key).Scan(&count).Error; err != nil {
		return "", err
	}
	if count > 0 {
		return "", nil
	}
	return key, nil
}
