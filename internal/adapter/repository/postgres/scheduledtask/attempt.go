package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/scheduledtask/domain"
)

var ErrClaimLost = fmt.Errorf("scheduled task claim is no longer held")

func (s *Store) RenewLease(ctx context.Context, claim Claim, now time.Time, lease time.Duration) error {
	if s == nil || s.db == nil || lease <= 0 {
		return fmt.Errorf("invalid lease renewal")
	}
	result := s.db.WithContext(ctx).Exec(`UPDATE t_scheduled_task_occurrence SET lease_until = ?, update_time = CURRENT_TIMESTAMP
		WHERE id = ? AND lease_owner = ? AND status = 'running' AND deadline_at > ?`,
		now.Add(lease), claim.OccurrenceID, claim.LeaseOwner, now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrClaimLost
	}
	return nil
}

// StartAttempt gives each execution its own runtime session identity.
func (s *Store) StartAttempt(ctx context.Context, claim Claim, now time.Time) (string, error) {
	if s == nil || s.db == nil || claim.OccurrenceID == "" || claim.LeaseOwner == "" || now.IsZero() {
		return "", fmt.Errorf("invalid scheduled task attempt")
	}
	id, err := nextID()
	if err != nil {
		return "", err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var held int
		if err := tx.Raw(`SELECT 1 FROM t_scheduled_task_occurrence o JOIN t_scheduled_task t ON t.id = o.task_id
			WHERE o.id = ? AND o.lease_owner = ? AND o.lease_until > ? AND o.deadline_at >= ?
			AND o.status = 'running' AND t.status = 'active' AND t.deleted_at IS NULL
			AND t.current_version = o.version FOR UPDATE OF o`, claim.OccurrenceID, claim.LeaseOwner, now, now).Scan(&held).Error; err != nil {
			return err
		}
		if held != 1 {
			return ErrClaimLost
		}
		if err := tx.Exec(`UPDATE t_scheduled_task_attempt SET status = 'interrupted', finished_at = ?
			WHERE occurrence_id = ? AND status = 'running'`, now, claim.OccurrenceID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_scheduled_task_attempt (id, occurrence_id, status, started_at)
			VALUES (?, ?, 'running', ?)`, id, claim.OccurrenceID, now).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE t_scheduled_task SET last_attempted_at = ?, update_time = CURRENT_TIMESTAMP
			WHERE id = ?`, now, claim.Task.ID).Error
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

type FinishInput struct {
	Claim            Claim
	AttemptID        string
	RuntimeSessionID string
	Outcome          *domain.Outcome
	TechnicalError   string
	Now              time.Time
	MaxAttempts      int
	RetryDelay       time.Duration
}

// FinishAttempt saves the model conclusion before any user-visible publication.
func (s *Store) FinishAttempt(ctx context.Context, input FinishInput) error {
	if s == nil || s.db == nil || input.AttemptID == "" || input.Claim.OccurrenceID == "" ||
		input.Claim.LeaseOwner == "" || input.Now.IsZero() || input.MaxAttempts < 1 || input.RetryDelay <= 0 ||
		(input.Outcome == nil) == (input.TechnicalError == "") {
		return fmt.Errorf("invalid scheduled task completion")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			Status     string
			LeaseOwner string
			DeadlineAt time.Time
		}
		loaded := tx.Raw(`SELECT status, lease_owner, deadline_at FROM t_scheduled_task_occurrence
			WHERE id = ? FOR UPDATE`, input.Claim.OccurrenceID).Scan(&row)
		if loaded.Error != nil {
			return loaded.Error
		}
		if loaded.RowsAffected != 1 || row.Status != "running" || row.LeaseOwner != input.Claim.LeaseOwner {
			return ErrClaimLost
		}
		var attemptStatus string
		if err := tx.Raw(`SELECT status FROM t_scheduled_task_attempt WHERE id = ? AND occurrence_id = ?`,
			input.AttemptID, input.Claim.OccurrenceID).Scan(&attemptStatus).Error; err != nil {
			return err
		}
		if attemptStatus != "running" {
			return ErrClaimLost
		}
		status := "completed"
		occurrenceStatus := "ready"
		var resultJSON any
		var retryAt any
		if input.TechnicalError != "" {
			status = "failed"
			var count int64
			if err := tx.Raw(`SELECT count(*) FROM t_scheduled_task_attempt WHERE occurrence_id = ?`,
				input.Claim.OccurrenceID).Scan(&count).Error; err != nil {
				return err
			}
			occurrenceStatus = "failed"
			if count < int64(input.MaxAttempts) && input.Now.Add(input.RetryDelay).Before(row.DeadlineAt) {
				occurrenceStatus = "retry"
				retryAt = input.Now.Add(input.RetryDelay)
			}
		} else {
			raw, err := json.Marshal(input.Outcome)
			if err != nil {
				return err
			}
			resultJSON = string(raw)
			if input.Now.After(row.DeadlineAt) {
				occurrenceStatus = "missed"
			}
			if occurrenceStatus != "missed" && input.Outcome.Signal != domain.SignalReport {
				occurrenceStatus = string(input.Outcome.Signal)
			}
			if occurrenceStatus == "ready" {
				retryAt = input.Now
			}
		}
		var task struct {
			CurrentVersion int
			Status         string
			DeletedAt      *time.Time
		}
		if err := tx.Raw(`SELECT current_version, status, deleted_at FROM t_scheduled_task WHERE id = ?`, input.Claim.Task.ID).Scan(&task).Error; err != nil {
			return err
		}
		if task.CurrentVersion != input.Claim.Version.Number {
			occurrenceStatus, retryAt = "superseded", nil
		} else if task.Status != "active" || task.DeletedAt != nil {
			occurrenceStatus, retryAt = "cancelled", nil
		}
		if err := tx.Exec(`UPDATE t_scheduled_task_attempt SET status = ?, runtime_session_id = ?,
			error_message = ?, finished_at = ? WHERE id = ?`, status, input.RuntimeSessionID,
			input.TechnicalError, input.Now, input.AttemptID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE t_scheduled_task_occurrence SET status = ?, result_signal = ?, result_json = ?,
			lease_until = ?, update_time = CURRENT_TIMESTAMP WHERE id = ?`, occurrenceStatus,
			func() any {
				if input.Outcome != nil {
					return input.Outcome.Signal
				}
				return nil
			}(),
			resultJSON, retryAt, input.Claim.OccurrenceID).Error
	})
}
