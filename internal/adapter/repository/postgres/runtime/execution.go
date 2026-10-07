package runtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/persistence"
	"local/rag-project/internal/framework/distributedid"
)

const chatAdmissionLease = 3 * time.Minute
const interruptedMessage = "执行已中断，请查看已保存内容后重新发起请求。"

type executionRow struct {
	TaskID, ConversationID, UserMessageID, UserID, State string
	Owner                                                *string
	Epoch                                                int64
	LeaseUntil                                           *time.Time
}

func ReserveChatExecution(ctx context.Context, tx *gorm.DB, session persistence.Session) error {
	if err := tx.WithContext(ctx).Exec(`INSERT INTO t_runtime_chat_execution(task_id,conversation_id,user_message_id,user_id,lease_until) VALUES(?,?,?,?,clock_timestamp()+?*INTERVAL '1 second') ON CONFLICT(task_id) DO NOTHING`, session.TraceID, session.ConversationID, session.UserMessageID, session.UserID, chatAdmissionLease.Seconds()).Error; err != nil {
		return err
	}
	var row executionRow
	if err := tx.WithContext(ctx).Raw(`SELECT * FROM t_runtime_chat_execution WHERE task_id=?`, session.TraceID).Scan(&row).Error; err != nil {
		return err
	}
	if row.ConversationID != session.ConversationID || row.UserMessageID != session.UserMessageID || row.UserID != session.UserID {
		return fmt.Errorf("chat execution identity cannot change")
	}
	return nil
}

// Work writes and recovery share topic -> turn -> execution -> session lock order.
func lockChatResources(tx *gorm.DB, task string) error {
	var id string
	if err := tx.Raw(`SELECT t.id FROM t_work_topic t JOIN t_work_conversation w ON w.topic_id=t.id JOIN t_runtime_chat_execution e ON e.conversation_id=w.conversation_id WHERE e.task_id=? FOR UPDATE OF t`, task).Scan(&id).Error; err != nil {
		return err
	}
	if id != "" {
		if err := tx.Raw(`SELECT id FROM t_work_turn WHERE id=? FOR UPDATE`, task).Scan(&id).Error; err != nil {
			return err
		}
	}
	return nil
}

const chatResourcesLiveSQL = `EXISTS(SELECT 1 FROM t_conversation c JOIN t_message m ON m.conversation_id=c.conversation_id AND m.user_id=c.user_id
 WHERE c.conversation_id=e.conversation_id AND c.user_id=e.user_id AND c.deleted=0 AND m.id=e.user_message_id AND m.deleted=0 AND m.role='user')
 AND NOT EXISTS(SELECT 1 FROM t_work_conversation w JOIN t_work_topic t ON t.id=w.topic_id LEFT JOIN t_work_turn v ON v.id=e.task_id
 WHERE w.conversation_id=e.conversation_id AND (t.status<>'active' OR v.id IS NULL))`

const chatDeadlineExpiredSQL = `EXISTS(SELECT 1 FROM t_work_turn v WHERE v.id=e.task_id AND v.deadline_at<=clock_timestamp())`
const chatTurnActiveSQL = `NOT EXISTS(SELECT 1 FROM t_work_turn v WHERE v.id=e.task_id AND v.status NOT IN ('accepted','running'))`
const liveChatExecutionSQL = chatResourcesLiveSQL + ` AND (` + chatTurnActiveSQL + `) AND NOT (` + chatDeadlineExpiredSQL + `)`

func (s *Store) ClaimChatExecution(ctx context.Context, session persistence.Session, owner string, ttl time.Duration) (out persistence.Execution, err error) {
	if owner == "" || ttl <= 0 {
		return out, fmt.Errorf("chat execution owner and positive lease required")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChatResources(tx, session.TraceID); err != nil {
			return err
		}
		var row executionRow
		result := tx.Raw(`UPDATE t_runtime_chat_execution e SET state='running',owner=?,epoch=epoch+1,lease_until=clock_timestamp()+?*INTERVAL '1 second',heartbeat_at=clock_timestamp(),updated_at=clock_timestamp()
 WHERE task_id=? AND conversation_id=? AND user_message_id=? AND user_id=? AND state='pending' AND lease_until>clock_timestamp() AND `+liveChatExecutionSQL+` RETURNING e.*`, owner, ttl.Seconds(), session.TraceID, session.ConversationID, session.UserMessageID, session.UserID).Scan(&row)
		if result.Error != nil {
			return result.Error
		}
		if row.TaskID == "" {
			return persistence.ErrExecutionLeaseLost
		}
		out = persistence.Execution{TaskID: row.TaskID, ConversationID: row.ConversationID, UserMessageID: row.UserMessageID, UserID: row.UserID, Owner: owner, Epoch: row.Epoch}
		return nil
	})
	return
}

func (s *Store) RenewChatExecution(ctx context.Context, execution persistence.Execution, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("positive execution lease required")
	}
	r := s.db.WithContext(ctx).Exec(`UPDATE t_runtime_chat_execution e SET lease_until=clock_timestamp()+?*INTERVAL '1 second',heartbeat_at=clock_timestamp(),updated_at=clock_timestamp()
 WHERE task_id=? AND conversation_id=? AND user_message_id=? AND user_id=? AND owner=? AND epoch=? AND state='running' AND lease_until>clock_timestamp() AND `+liveChatExecutionSQL, ttl.Seconds(), execution.TaskID, execution.ConversationID, execution.UserMessageID, execution.UserID, execution.Owner, execution.Epoch)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return persistence.ErrExecutionLeaseLost
	}
	return nil
}

// FenceChatTask also protects Work's database mutations. Unclaimed reservations
// permit existing internal command callers; claimed executions require their token.
func FenceChatTask(ctx context.Context, tx *gorm.DB, task string) error {
	var row executionRow
	if err := tx.WithContext(ctx).Raw(`SELECT * FROM t_runtime_chat_execution WHERE task_id=? FOR UPDATE`, task).Scan(&row).Error; err != nil {
		return err
	}
	execution, owned := persistence.CurrentExecution(ctx)
	if row.TaskID == "" && !owned {
		return nil
	} // Historical/standalone kernel calls.
	if !owned {
		if row.State != "pending" || row.Owner != nil {
			return persistence.ErrExecutionLeaseLost
		}
	} else if row.TaskID != execution.TaskID || row.UserID != execution.UserID || row.ConversationID != execution.ConversationID || row.UserMessageID != execution.UserMessageID || row.Owner == nil || *row.Owner != execution.Owner || row.Epoch != execution.Epoch || row.State != "running" {
		return persistence.ErrExecutionLeaseLost
	}
	var live bool
	if err := tx.Raw(`SELECT lease_until>clock_timestamp() AND (`+liveChatExecutionSQL+`) FROM t_runtime_chat_execution e WHERE task_id=?`, task).Scan(&live).Error; err != nil {
		return err
	}
	if !live {
		return persistence.ErrExecutionLeaseLost
	}
	return nil
}

func fenceChatSession(ctx context.Context, tx *gorm.DB, id string) error {
	var task string
	if err := tx.Raw(`SELECT trace_id FROM t_runtime_session WHERE id=?`, id).Scan(&task).Error; err != nil {
		return err
	}
	return FenceChatTask(ctx, tx, task)
}

func closeChatExecution(ctx context.Context, tx *gorm.DB, task, state string) error {
	if execution, owned := persistence.CurrentExecution(ctx); owned {
		r := tx.Exec(`UPDATE t_runtime_chat_execution SET state=?,lease_until=NULL,updated_at=clock_timestamp() WHERE task_id=? AND owner=? AND epoch=? AND state='running' AND lease_until>clock_timestamp()`, state, task, execution.Owner, execution.Epoch)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return persistence.ErrExecutionLeaseLost
		}
		return nil
	}
	return tx.Exec(`UPDATE t_runtime_chat_execution SET state=?,lease_until=NULL,updated_at=clock_timestamp() WHERE task_id=? AND state='pending' AND owner IS NULL`, state, task).Error
}

func (s *Store) FinishChatExecution(ctx context.Context, session persistence.Session, status, detail string) (event convruntime.JournalEntry, err error) {
	if status != convruntime.StatusFailed && status != convruntime.StatusCancelled {
		return event, fmt.Errorf("invalid chat failure status")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChatResources(tx, session.TraceID); err != nil {
			return err
		}
		if err := fenceChatSession(ctx, tx, session.ID); err != nil {
			return err
		}
		changed := tx.Exec(`UPDATE t_runtime_session SET status=?,update_time=CURRENT_TIMESTAMP WHERE id=? AND status='running'`, status, session.ID)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return persistence.ErrExecutionLeaseLost
		}
		if err := settleInterruptedTools(tx, session, detail); err != nil {
			return err
		}
		if err := NewStore(tx).DiscardEpisodes(ctx, session.ID); err != nil {
			return err
		}
		typeName := convruntime.EventFailed
		if status == convruntime.StatusCancelled {
			typeName = convruntime.EventCancelled
		}
		var err error
		event, err = appendExecutionTerminal(tx, session, typeName, detail)
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE t_work_turn SET status=?,error=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=? AND status IN ('accepted','running')`, status, detail, session.TraceID, session.UserID).Error; err != nil {
			return err
		}
		return closeChatExecution(ctx, tx, session.TraceID, status)
	})
	return
}

func appendExecutionTerminal(tx *gorm.DB, session persistence.Session, kind, detail string) (convruntime.JournalEntry, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return convruntime.JournalEntry{}, err
	}
	event := convruntime.JournalEntry{ID: strconv.FormatInt(id, 10), RuntimeSessionID: session.ID, ConversationID: session.ConversationID, UserMessageID: session.UserMessageID, TraceID: session.TraceID, EventType: kind, Detail: detail, CreatedAt: time.Now().UTC()}
	err = appendJournalTx(tx, &event)
	return event, err
}

func settleInterruptedTools(tx *gorm.DB, session persistence.Session, detail string) error {
	calls, err := NewStore(tx).ListUnsettledToolCalls(tx.Statement.Context, session.ID)
	if err != nil {
		return err
	}
	for _, call := range calls {
		id, err := distributedid.NextID()
		if err != nil {
			return err
		}
		entry := convruntime.JournalEntry{ID: strconv.FormatInt(id, 10), RuntimeSessionID: session.ID, ConversationID: session.ConversationID, UserMessageID: session.UserMessageID, TraceID: session.TraceID, EventType: convruntime.EventToolSettled, ToolCallID: call.ToolCallID, ToolName: call.ToolName, ToolState: convruntime.ToolStateFailed, Detail: detail, CreatedAt: time.Now().UTC()}
		if err := appendJournalTx(tx, &entry); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) recoverChatTask(ctx context.Context, task string, cancel bool) (event convruntime.JournalEntry, err error) {
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChatResources(tx, task); err != nil {
			return err
		}
		var row executionRow
		if err := tx.Raw(`SELECT * FROM t_runtime_chat_execution WHERE task_id=? FOR UPDATE`, task).Scan(&row).Error; err != nil {
			return err
		}
		if row.TaskID == "" || (row.State != "pending" && row.State != "running") {
			return nil
		}
		var readiness int64
		if err := tx.Raw(`SELECT count(*) FROM t_runtime_chat_publication p JOIN t_runtime_session r ON r.id=p.runtime_session_id WHERE r.trace_id=? AND r.user_id=?`, task, row.UserID).Scan(&readiness).Error; err != nil {
			return err
		}
		if readiness > 0 {
			return tx.Exec(`UPDATE t_runtime_chat_execution SET state='completed',lease_until=NULL WHERE task_id=?`, task).Error
		}
		var condition struct{ Expired, Live, Active bool }
		if err := tx.Raw(`SELECT (lease_until<=clock_timestamp() OR `+chatDeadlineExpiredSQL+`) AS expired,(`+chatResourcesLiveSQL+`) AS live,(`+chatTurnActiveSQL+`) AS active FROM t_runtime_chat_execution e WHERE task_id=?`, task).Scan(&condition).Error; err != nil {
			return err
		}
		if !cancel && !condition.Expired && condition.Live && condition.Active {
			return nil
		}
		status, kind, detail := convruntime.StatusInterrupted, convruntime.EventInterrupted, interruptedMessage
		if cancel || !condition.Live {
			status, kind, detail = convruntime.StatusCancelled, convruntime.EventCancelled, "执行已取消，已保存内容仍可查看。"
		} else if !condition.Active {
			var terminal string
			if err := tx.Raw(`SELECT status FROM t_work_turn WHERE id=?`, task).Scan(&terminal).Error; err != nil {
				return err
			}
			switch terminal {
			case convruntime.StatusFailed:
				status, kind, detail = terminal, convruntime.EventFailed, "执行失败，已保存内容仍可查看。"
			case convruntime.StatusCancelled:
				status, kind, detail = terminal, convruntime.EventCancelled, "执行已取消，已保存内容仍可查看。"
			case convruntime.StatusCompleted:
				// Trusted standalone Work commands can finish without the chat kernel.
				status, kind, detail = terminal, convruntime.EventCompleted, ""
			}
		}
		session, err := NewStore(tx).CreateOrLoadSession(ctx, persistence.Session{ID: task, TraceID: task, ConversationID: row.ConversationID, UserMessageID: row.UserMessageID, UserID: row.UserID, Status: persistence.StatusRunning, NextSequence: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
		if err != nil {
			return err
		}
		if session.Status != persistence.StatusRunning {
			return tx.Exec(`UPDATE t_runtime_chat_execution SET state=?,lease_until=NULL WHERE task_id=?`, session.Status, task).Error
		}
		if err := lockSession(tx, session.ID); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE t_runtime_session SET status=?,update_time=CURRENT_TIMESTAMP WHERE id=?`, status, session.ID).Error; err != nil {
			return err
		}
		if err := settleInterruptedTools(tx, session, detail); err != nil {
			return err
		}
		if err := NewStore(tx).DiscardEpisodes(ctx, session.ID); err != nil {
			return err
		}
		event, err = appendExecutionTerminal(tx, session, kind, detail)
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE t_work_turn SET status=?,error=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=? AND status IN ('accepted','running')`, status, detail, task, row.UserID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE t_runtime_chat_execution SET state=?,lease_until=NULL,updated_at=clock_timestamp() WHERE task_id=?`, status, task).Error
	})
	return
}

func (s *Store) RecoverChatExecutions(ctx context.Context, limit int, notify func(context.Context, convruntime.JournalEntry) error) error {
	var tasks []string
	if err := s.db.WithContext(ctx).Raw(`SELECT task_id FROM t_runtime_chat_execution e WHERE state IN ('pending','running') AND (lease_until<=clock_timestamp() OR NOT (`+liveChatExecutionSQL+`)) ORDER BY lease_until,task_id LIMIT ?`, limit).Scan(&tasks).Error; err != nil {
		return err
	}
	var result error
	for _, task := range tasks {
		event, err := s.recoverChatTask(ctx, task, false)
		if err == nil && event.ID != "" && notify != nil {
			err = notify(ctx, event)
		}
		result = errors.Join(result, err)
	}
	return result
}

func (s *Store) RecoverChatExecution(ctx context.Context, user, task string) (convruntime.JournalEntry, error) {
	var count int64
	if err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM t_runtime_chat_execution WHERE task_id=? AND user_id=?`, task, user).Scan(&count).Error; err != nil || count == 0 {
		return convruntime.JournalEntry{}, err
	}
	if _, err := s.recoverChatTask(ctx, task, false); err != nil {
		return convruntime.JournalEntry{}, err
	}
	var entry JournalModel
	if err := s.db.WithContext(ctx).Raw(`SELECT j.* FROM t_runtime_journal j JOIN t_runtime_session r ON r.id=j.runtime_session_id WHERE r.trace_id=? AND r.user_id=? AND j.event_type IN ('interrupted','failed','cancelled') ORDER BY j.sequence DESC LIMIT 1`, task, user).Scan(&entry).Error; err != nil || entry.ID == "" {
		return convruntime.JournalEntry{}, err
	}
	return toJournal(entry)
}

func (s *Store) CancelChatExecution(ctx context.Context, user, task string) error {
	var count int64
	if err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM t_runtime_chat_execution WHERE task_id=? AND user_id=?`, task, user).Scan(&count).Error; err != nil || count == 0 {
		return err
	}
	_, err := s.recoverChatTask(ctx, task, true)
	return err
}
