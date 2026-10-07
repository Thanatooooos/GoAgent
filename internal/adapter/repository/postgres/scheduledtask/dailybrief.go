package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"gorm.io/gorm"
	briefrepo "local/rag-project/internal/adapter/repository/postgres/dailybrief"
	briefdomain "local/rag-project/internal/app/dailybrief/domain"
	briefport "local/rag-project/internal/app/dailybrief/port"
	briefservice "local/rag-project/internal/app/dailybrief/service"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/scheduledtask/domain"
	"local/rag-project/internal/app/scheduledtask/service"
	"local/rag-project/internal/framework/config"
)

// CutoverDailyBrief is an explicit, idempotent per-user handoff. It never runs
// automatically during server startup. No conversation is created here.
func (s *Store) CutoverDailyBrief(ctx context.Context, userID string, now time.Time, cfg config.DailyBriefGenerationConfig, resultWindow time.Duration) (string, error) {
	return s.cutoverDailyBrief(ctx, userID, now, cfg, resultWindow, "")
}

// CutoverDailyBriefWithMissedDate explicitly acknowledges one expired local
// date. It retains the legacy issue and records a missed occurrence atomically
// with handoff; ordinary deployment/cutover never silently skips that date.
func (s *Store) CutoverDailyBriefWithMissedDate(ctx context.Context, userID string, now time.Time, cfg config.DailyBriefGenerationConfig, resultWindow time.Duration, missedDate string) (string, error) {
	if _, err := time.Parse(time.DateOnly, missedDate); err != nil {
		return "", fmt.Errorf("acknowledged missed date must use YYYY-MM-DD")
	}
	return s.cutoverDailyBrief(ctx, userID, now, cfg, resultWindow, missedDate)
}

func (s *Store) cutoverDailyBrief(ctx context.Context, userID string, now time.Time, cfg config.DailyBriefGenerationConfig, resultWindow time.Duration, missedDate string) (string, error) {
	if s == nil || s.db == nil || userID == "" || now.IsZero() || resultWindow <= 0 {
		return "", fmt.Errorf("invalid DailyBrief cutover")
	}
	var taskID string
	if err := s.db.WithContext(ctx).Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id=?`, userID).Scan(&taskID).Error; err != nil {
		return "", err
	}
	if taskID != "" {
		return taskID, nil
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var exists int
		if err := tx.Raw(`SELECT 1 FROM t_daily_brief_subscription WHERE user_id = ? FOR UPDATE`, userID).Scan(&exists).Error; err != nil {
			return err
		}
		if exists != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id = ?`, userID).Scan(&taskID).Error; err != nil {
			return err
		}
		if taskID != "" {
			return nil
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, "daily-brief:"+userID).Error; err != nil {
			return err
		}
		sub, err := briefrepo.NewSubscriptionRepository(tx).GetByUserID(ctx, userID)
		if err != nil {
			return err
		}
		if sub.LockUntil != nil && sub.LockUntil.After(now) {
			return fmt.Errorf("DailyBrief legacy lease is active; drain before cutover")
		}
		taskID, err = nextID()
		if err != nil {
			return err
		}
		version := briefVersion(taskID, 1, now, sub, cfg)
		if err := version.Validate(); err != nil {
			return err
		}
		next, err := firstBriefDue(tx, userID, version.Schedule, now)
		if err != nil {
			return err
		}
		// A subscription first created after today's delivery time has no prior
		// delivery obligation. It starts at the next tick rather than importing
		// a missed day that predates its creation.
		if sub.CreatedAt.After(next) {
			next, err = version.Schedule.Next(now)
			if err != nil {
				return err
			}
		}
		var missedDue time.Time
		if sub.Enabled && now.After(next.Add(resultWindow)) {
			loc, err := time.LoadLocation(sub.Timezone)
			if err != nil {
				return err
			}
			if missedDate == "" {
				return fmt.Errorf("today's undelivered DailyBrief is outside the result window; finish legacy delivery, cut over before the next due time, or explicitly acknowledge its missed local date")
			}
			if missedDate != next.In(loc).Format(time.DateOnly) {
				return fmt.Errorf("acknowledged missed date does not match the expired delivery date")
			}
			missedDue = next
			next, err = version.Schedule.Next(now)
			if err != nil {
				return err
			}
		} else if missedDate != "" {
			return fmt.Errorf("no expired undelivered date requires acknowledgement")
		}
		status := "active"
		var due any = next
		if !sub.Enabled {
			status, due = "paused", nil
		}
		if err := tx.Exec(`INSERT INTO t_scheduled_task(id,user_id,status,current_version,confirmed_at,next_due_at)
			VALUES (?, ?, ?, 1, ?, ?)`, taskID, userID, status, now, due).Error; err != nil {
			return err
		}
		if err := insertVersion(tx, version); err != nil {
			return err
		}
		if !missedDue.IsZero() {
			occurrenceID, err := nextID()
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO t_scheduled_task_occurrence(id,task_id,version,scheduled_at,deadline_at,status)
				VALUES (?, ?, 1, ?, ?, 'missed')`, occurrenceID, taskID, missedDue, missedDue.Add(resultWindow)).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`INSERT INTO t_daily_brief_task_binding(user_id,task_id,cutover_at) VALUES (?, ?, ?)`, userID, taskID, now).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE t_daily_brief_subscription SET lock_owner=NULL,lock_until=NULL WHERE user_id=?`, userID).Error
	})
	return taskID, err
}

func briefVersion(id string, number int, now time.Time, sub briefdomain.Subscription, cfg config.DailyBriefGenerationConfig) domain.Version {
	max := cfg.MaxItems
	if max < 1 {
		max = 5
	}
	return domain.Version{TaskID: id, Number: number, Name: "每日简报", ConfirmedAt: now,
		Prompt:     dailyBriefDefaultTaskPrompt,
		Schedule:   domain.Schedule{Kind: domain.ScheduleDaily, Timezone: sub.Timezone, LocalTime: sub.DeliveryTimeLocal},
		ReportMode: domain.ReportAlways, ConditionKind: domain.ConditionNone,
		AllowedToolIDs: []string{capability.WebSearchID, capability.WebFetchID},
		DailyBrief:     &briefdomain.TaskContract{Topics: sub.Topics, Sources: sub.Sources, MaxItems: max, MaxItemsPerTopic: cfg.MaxItemsPerTopic}}
}

// Preserve today's undelivered tick within the normal result window. A ready
// legacy issue is never republished into a newly created task conversation.
func firstBriefDue(tx *gorm.DB, userID string, schedule domain.Schedule, now time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	due, err := schedule.Next(start.Add(-time.Nanosecond))
	if err != nil {
		return time.Time{}, err
	}
	var ready int
	if err := tx.Raw(`SELECT 1 FROM t_daily_brief_issue WHERE user_id=? AND brief_date=? AND status='ready'`, userID, local.Format("2006-01-02")).Scan(&ready).Error; err != nil {
		return time.Time{}, err
	}
	if ready != 1 {
		return due, nil
	}
	return schedule.Next(start.AddDate(0, 0, 1).Add(-time.Nanosecond))
}

// SyncDailyBriefSubscription runs inside the subscription write transaction.
// An unmigrated subscription keeps the legacy path; no implicit handoff occurs.
func SyncDailyBriefSubscription(tx *gorm.DB, sub briefdomain.Subscription) error {
	var id string
	if err := tx.Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id=?`, sub.UserID).Scan(&id).Error; err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	var task taskRow
	if err := tx.Raw(`SELECT id,user_id,status,current_version,deleted_at FROM t_scheduled_task WHERE id=? FOR UPDATE`, id).Scan(&task).Error; err != nil {
		return err
	}
	var deleted struct{ DeletedAt *time.Time }
	if err := tx.Raw(`SELECT deleted_at FROM t_scheduled_task WHERE id=?`, id).Scan(&deleted).Error; err != nil {
		return err
	}
	current, err := loadVersion(tx, id, task.CurrentVersion)
	if err != nil {
		return err
	}
	if current.DailyBrief == nil {
		return fmt.Errorf("bound DailyBrief task lacks its artifact contract")
	}
	now := time.Now()
	if deleted.DeletedAt != nil {
		if !sub.Enabled {
			return nil
		}
		// Only a new explicit subscription enable can create a replacement;
		// scans and repeated cutover calls never resurrect a deleted task.
		newID, err := nextID()
		if err != nil {
			return err
		}
		version := briefVersion(newID, 1, now, sub, config.DailyBriefGenerationConfig{MaxItems: current.DailyBrief.MaxItems, MaxItemsPerTopic: current.DailyBrief.MaxItemsPerTopic})
		version.Prompt = current.Prompt
		if err := version.Validate(); err != nil {
			return err
		}
		next, err := version.Schedule.Next(now)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_scheduled_task(id,user_id,status,current_version,confirmed_at,next_due_at) VALUES (?,?,'active',1,?,?)`, newID, sub.UserID, now, next).Error; err != nil {
			return err
		}
		if err := insertVersion(tx, version); err != nil {
			return err
		}
		return tx.Exec(`UPDATE t_daily_brief_task_binding SET task_id=?,cutover_at=? WHERE user_id=?`, newID, now, sub.UserID).Error
	}
	version := current
	contract := *current.DailyBrief
	contract.Topics, contract.Sources = sub.Topics, sub.Sources
	version.DailyBrief = &contract
	version.Schedule.Timezone, version.Schedule.LocalTime = sub.Timezone, sub.DeliveryTimeLocal
	changed := !reflect.DeepEqual(current.DailyBrief, &contract) || current.Schedule != version.Schedule
	if changed {
		version.Number++
		version.ConfirmedAt = now
		if err := version.Validate(); err != nil {
			return err
		}
		if err := insertVersion(tx, version); err != nil {
			return err
		}
	}
	status := "paused"
	var due any
	if sub.Enabled {
		status = "active"
		if changed || task.Status != "active" {
			next, err := version.Schedule.Next(now)
			if err != nil {
				return err
			}
			due = next
		} else {
			return nil // Preserve the claimed cursor on an unchanged save.
		}
	}
	return tx.Exec(`UPDATE t_scheduled_task SET status=?,current_version=?,confirmed_at=?,next_due_at=?,update_time=CURRENT_TIMESTAMP WHERE id=?`, status, version.Number, version.ConfirmedAt, due, id).Error
}

func projectDailyBrief(tx *gorm.DB, ctx context.Context, input PublishReportInput, version domain.Version, scheduledAt time.Time, outcome domain.Outcome) (domain.Outcome, bool, error) {
	var id string
	if err := tx.Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id=?`, input.UserID).Scan(&id).Error; err != nil {
		return outcome, false, err
	}
	if id != input.TaskID {
		return outcome, false, ErrTaskNotPublishable
	}
	outcome, artifact, err := service.NormalizeDailyBrief(outcome, *version.DailyBrief)
	if err != nil {
		return outcome, false, err
	}
	date, err := briefdomain.ResolveBriefDate(scheduledAt, version.Schedule.Timezone)
	if err != nil {
		return outcome, false, err
	}
	issueID, err := nextID()
	if err != nil {
		return outcome, false, err
	}
	if err := tx.Exec(`INSERT INTO t_daily_brief_issue(id,user_id,brief_date,status,sections_json,item_count,published_run_id,create_time,update_time)
		VALUES (?, ?, ?, 'generating', '[]', 0, ?, ?, ?) ON CONFLICT(user_id,brief_date) DO NOTHING`, issueID, input.UserID, date, input.OccurrenceID, input.Now, input.Now).Error; err != nil {
		return outcome, false, err
	}
	var issue struct{ ID, Status, PublishedRunID string }
	if err := tx.Raw(`SELECT id,status,published_run_id FROM t_daily_brief_issue WHERE user_id=? AND brief_date=? FOR UPDATE`, input.UserID, date).Scan(&issue).Error; err != nil {
		return outcome, false, err
	}
	if issue.Status == "ready" {
		if err := tx.Exec(`UPDATE t_scheduled_task_occurrence SET status='superseded',lease_until=NULL,update_time=CURRENT_TIMESTAMP WHERE id=?`, input.OccurrenceID).Error; err != nil {
			return outcome, false, err
		}
		return outcome, true, nil
	}
	if err := tx.Exec(`UPDATE t_daily_brief_issue SET status='generating',published_run_id=? WHERE id=?`, input.OccurrenceID, issue.ID).Error; err != nil {
		return outcome, false, err
	}
	issueDomain := briefdomain.NewIssue(issue.ID, input.UserID, date)
	// Publisher's transaction is deliberately the enclosing message transaction.
	publisher := briefservice.NewPublisher(func(ctx context.Context, fn func(context.Context, briefport.IssueRepository, briefport.ItemRepository) error) error {
		return fn(ctx, briefrepo.NewIssueRepository(tx), briefrepo.NewItemRepository(tx))
	})
	_, _, err = publisher.Publish(ctx, briefservice.PublishInput{Issue: issueDomain, Artifact: artifact, PublishedRunID: input.OccurrenceID, PublishedAt: input.Now})
	return outcome, false, err
}

// Keep the persisted result and both views identical on crash recovery too.
func saveBriefOutcome(tx *gorm.DB, occurrenceID string, outcome domain.Outcome) error {
	raw, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	return tx.Exec(`UPDATE t_scheduled_task_occurrence SET result_json=?::jsonb WHERE id=?`, string(raw), occurrenceID).Error
}
