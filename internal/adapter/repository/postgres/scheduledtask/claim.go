package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/scheduledtask/domain"
	"local/rag-project/internal/framework/distributedid"
)

type ClaimOptions struct {
	Limit             int
	WorkerID          string
	Lease             time.Duration
	OnceDeadline      time.Duration
	RecurringDeadline time.Duration
	MaxAttempts       int
}

type Claim struct {
	OccurrenceID string
	LeaseOwner   string
	Task         domain.Task
	Version      domain.Version
	ScheduledAt  time.Time
	DeadlineAt   time.Time
	ReadyOutcome *domain.Outcome
}

// ClaimDue advances each task's cursor in the same transaction that creates
// its occurrence. Missed ticks are not replayed individually after an outage.
func (s *Store) ClaimDue(ctx context.Context, now time.Time, opts ClaimOptions) ([]Claim, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("scheduled task database is required")
	}
	if opts.Limit < 1 || opts.WorkerID == "" || opts.Lease <= 0 || opts.OnceDeadline <= 0 || opts.RecurringDeadline <= 0 || opts.MaxAttempts < 1 {
		return nil, fmt.Errorf("invalid scheduled task claim options")
	}
	var claims []Claim
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tasks []taskRow
		if err := tx.Raw(`SELECT id, user_id, status, current_version, conversation_id, confirmed_at,
			next_due_at, last_reported_at, last_attempted_at FROM t_scheduled_task
			WHERE status = 'active' AND deleted_at IS NULL AND next_due_at <= ?
			ORDER BY next_due_at LIMIT ? FOR UPDATE SKIP LOCKED`, now, opts.Limit).Scan(&tasks).Error; err != nil {
			return err
		}
		for _, row := range tasks {
			if row.NextDueAt == nil {
				continue
			}
			version, err := loadVersion(tx, row.ID, row.CurrentVersion)
			if err != nil {
				return err
			}
			next, err := version.Schedule.Next(now)
			if err != nil {
				return fmt.Errorf("next due for task %s: %w", row.ID, err)
			}
			var nextDue any
			if !next.IsZero() {
				nextDue = next
			}
			if err := tx.Exec(`UPDATE t_scheduled_task SET next_due_at = ?, update_time = CURRENT_TIMESTAMP WHERE id = ?`, nextDue, row.ID).Error; err != nil {
				return err
			}
			id, err := distributedid.NextID()
			if err != nil {
				return err
			}
			deadline := opts.RecurringDeadline
			if version.Schedule.Kind == domain.ScheduleOnce {
				deadline = opts.OnceDeadline
			}
			due := *row.NextDueAt
			deadlineAt := due.Add(deadline)
			status := "running"
			if now.After(deadlineAt) {
				status = "missed"
			}
			owner := opts.WorkerID + "/" + fmt.Sprint(id)
			inserted := tx.Exec(`INSERT INTO t_scheduled_task_occurrence
				(id, task_id, version, scheduled_at, deadline_at, status, lease_owner, lease_until)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (task_id, version, scheduled_at) DO NOTHING`,
				fmt.Sprint(id), row.ID, row.CurrentVersion, due, deadlineAt, status, owner, now.Add(opts.Lease))
			if inserted.Error != nil {
				return inserted.Error
			}
			if inserted.RowsAffected == 1 && status == "running" {
				claims = append(claims, Claim{OccurrenceID: fmt.Sprint(id), LeaseOwner: owner, Task: row.toDomain(), Version: version,
					ScheduledAt: due, DeadlineAt: deadlineAt})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// ClaimPending reclaims expired executions and reports saved before a crash.
// A ready report is published without running the model again.
func (s *Store) ClaimPending(ctx context.Context, now time.Time, opts ClaimOptions) ([]Claim, error) {
	if s == nil || s.db == nil || opts.Limit < 1 || opts.WorkerID == "" || opts.Lease <= 0 || opts.MaxAttempts < 1 {
		return nil, fmt.Errorf("invalid scheduled task claim options")
	}
	var claims []Claim
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			ID          string
			TaskID      string
			Version     int
			ScheduledAt time.Time
			DeadlineAt  time.Time
			Status      string
			ResultJSON  []byte
		}
		if err := tx.Raw(`SELECT o.id, o.task_id, o.version, o.scheduled_at, o.deadline_at, o.status, o.result_json
			FROM t_scheduled_task_occurrence o JOIN t_scheduled_task t ON t.id = o.task_id
			WHERE o.status IN ('running', 'retry', 'ready') AND o.lease_until <= ?
			ORDER BY o.scheduled_at LIMIT ? FOR UPDATE OF o SKIP LOCKED`, now, opts.Limit).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			var task taskRow
			loaded := tx.Raw(`SELECT id, user_id, status, current_version, conversation_id, confirmed_at,
				next_due_at, last_reported_at, last_attempted_at FROM t_scheduled_task WHERE id = ?`, row.TaskID).Scan(&task)
			if loaded.Error != nil {
				return loaded.Error
			}
			if loaded.RowsAffected != 1 {
				return fmt.Errorf("missing task %s", row.TaskID)
			}
			if task.Status != string(domain.TaskActive) || task.CurrentVersion != row.Version {
				status := "cancelled"
				if task.CurrentVersion != row.Version {
					status = "superseded"
				}
				if err := tx.Exec(`UPDATE t_scheduled_task_occurrence SET status = ?, update_time = CURRENT_TIMESTAMP WHERE id = ?`, status, row.ID).Error; err != nil {
					return err
				}
				continue
			}
			if now.After(row.DeadlineAt) {
				if err := tx.Exec(`UPDATE t_scheduled_task_occurrence SET status = 'missed', update_time = CURRENT_TIMESTAMP WHERE id = ?`, row.ID).Error; err != nil {
					return err
				}
				continue
			}
			if row.Status != "ready" {
				var attempts int64
				if err := tx.Raw(`SELECT count(*) FROM t_scheduled_task_attempt WHERE occurrence_id = ?`, row.ID).Scan(&attempts).Error; err != nil {
					return err
				}
				if attempts >= int64(opts.MaxAttempts) {
					if err := tx.Exec(`UPDATE t_scheduled_task_occurrence SET status = 'failed', update_time = CURRENT_TIMESTAMP WHERE id = ?`, row.ID).Error; err != nil {
						return err
					}
					continue
				}
			}
			version, err := loadVersion(tx, row.TaskID, row.Version)
			if err != nil {
				return err
			}
			ownerID, err := nextID()
			if err != nil {
				return err
			}
			owner := opts.WorkerID + "/" + ownerID
			claim := Claim{OccurrenceID: row.ID, LeaseOwner: owner, Task: task.toDomain(), Version: version,
				ScheduledAt: row.ScheduledAt, DeadlineAt: row.DeadlineAt}
			if row.Status == "ready" {
				var outcome domain.Outcome
				if err := json.Unmarshal(row.ResultJSON, &outcome); err != nil {
					return err
				}
				claim.ReadyOutcome = &outcome
			} else {
				if err := tx.Exec(`UPDATE t_scheduled_task_occurrence SET status = 'running' WHERE id = ?`, row.ID).Error; err != nil {
					return err
				}
			}
			leaseUntil := now.Add(opts.Lease)
			if row.Status == "ready" {
				leaseUntil = now.Add(time.Minute)
			}
			if err := tx.Exec(`UPDATE t_scheduled_task_occurrence SET lease_owner = ?, lease_until = ?, update_time = CURRENT_TIMESTAMP WHERE id = ?`,
				owner, leaseUntil, row.ID).Error; err != nil {
				return err
			}
			claims = append(claims, claim)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}
