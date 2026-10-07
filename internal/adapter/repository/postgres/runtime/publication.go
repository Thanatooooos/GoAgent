package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	ragdomain "local/rag-project/internal/app/rag/domain"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/persistence"
	"local/rag-project/internal/framework/distributedid"
)

type PublicationModel struct {
	RuntimeSessionID   string  `gorm:"column:runtime_session_id;primaryKey"`
	State              string  `gorm:"column:state"`
	AssistantMessageID *string `gorm:"column:assistant_message_id"`
	FinishEventID      *string `gorm:"column:finish_event_id"`
}

func (PublicationModel) TableName() string { return "t_runtime_chat_publication" }

type PreparedPublication struct {
	Message ragdomain.ConversationMessage
	Chunks  postgresrag.PreparedSessionChunks
}
type PublicationPreparer interface {
	PreparePublication(context.Context, convruntime.ConversationMessage) (PreparedPublication, error)
}
type ChatPublisher struct {
	db       *gorm.DB
	preparer PublicationPreparer
}

func NewChatPublisher(db *gorm.DB, preparer PublicationPreparer) *ChatPublisher {
	return &ChatPublisher{db: db, preparer: preparer}
}

// ReadyAnswer closes the answer/status crash window. Pending publication becomes
// visible only when its complete answer and successful execution commit together.
func (s *Store) ReadyAnswer(ctx context.Context, answer convruntime.JournalEntry) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChatResources(tx, answer.TraceID); err != nil {
			return err
		}
		var live bool
		if err := tx.Raw(`SELECT (`+liveChatExecutionSQL+`) FROM t_runtime_chat_execution e WHERE task_id=?`, answer.TraceID).Scan(&live).Error; err != nil {
			return err
		}
		if _, owned := persistence.CurrentExecution(ctx); owned && !live {
			return persistence.ErrExecutionLeaseLost
		}
		if err := fenceChatSession(ctx, tx, answer.RuntimeSessionID); err != nil {
			return err
		}
		changed := tx.Exec(`UPDATE t_runtime_session SET status='completed',update_time=CURRENT_TIMESTAMP WHERE id=? AND status='running'`, answer.RuntimeSessionID)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return fmt.Errorf("runtime session is already terminal")
		}
		if err := appendJournalTx(tx, &answer); err != nil {
			return err
		}
		if err := tx.Create(&PublicationModel{RuntimeSessionID: answer.RuntimeSessionID, State: "pending"}).Error; err != nil {
			return err
		}
		return closeChatExecution(ctx, tx, answer.TraceID, convruntime.StatusCompleted)
	})
}

func publicationMessage(m ragdomain.ConversationMessage) convruntime.ConversationMessage {
	return convruntime.ConversationMessage{ID: m.ID, ConversationID: m.ConversationID, UserID: m.UserID,
		Role: convruntime.ModelRoleAssistant, Content: m.DisplayContent(), Sources: m.Sources}
}

func (p *ChatPublisher) Publish(ctx context.Context, id string) (message convruntime.ConversationMessage, finish convruntime.JournalEntry, err error) {
	defer func() {
		if err != nil {
			// Retry bookkeeping cannot undo a committed publication.
			_ = p.db.WithContext(ctx).Exec(`UPDATE t_runtime_chat_publication SET attempts=attempts+1,last_error=?,
				next_attempt_at=CURRENT_TIMESTAMP+INTERVAL '10 seconds',updated_at=CURRENT_TIMESTAMP
				WHERE runtime_session_id=? AND state='pending'`, err.Error(), id).Error
		}
	}()
	var session SessionModel
	if err = p.db.WithContext(ctx).Where("id=?", id).First(&session).Error; err != nil {
		return
	}
	var initial PublicationModel
	if err = p.db.WithContext(ctx).Where("runtime_session_id=?", id).First(&initial).Error; err != nil {
		return
	}
	if initial.State == "abandoned" {
		err = fmt.Errorf("answer publication is abandoned")
		return
	}
	var prepared PreparedPublication
	if initial.State == "pending" {
		var answer JournalModel
		if err = p.db.WithContext(ctx).Where("runtime_session_id=? AND event_type=?", id, convruntime.EventAnswerFinal).Order("sequence DESC").First(&answer).Error; err != nil {
			return
		}
		prepared, err = p.preparer.PreparePublication(ctx, convruntime.ConversationMessage{ConversationID: session.ConversationID,
			UserID: session.UserID, Role: convruntime.ModelRoleAssistant, Content: answer.Detail})
		if err != nil {
			return
		}
		if prepared.Message.ID == "" || prepared.Message.UserID != session.UserID || prepared.Message.ConversationID != session.ConversationID || prepared.Message.Role != "assistant" {
			err = fmt.Errorf("prepared publication identity is invalid")
			return
		}
	}
	var abandoned bool
	err = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Match Work deletion/cancellation's topic-first lock order. Ordinary chat
		// has no topic. Hold the conversation and input against concurrent deletion.
		var topic struct{ ID, Status string }
		if err := tx.Raw(`SELECT t.id,t.status FROM t_work_topic t JOIN t_work_conversation w ON w.topic_id=t.id
			WHERE w.conversation_id=? AND w.user_id=? FOR UPDATE OF t`, session.ConversationID, session.UserID).Scan(&topic).Error; err != nil {
			return err
		}
		var conversation struct {
			ID      string
			Deleted int
		}
		if err := tx.Raw(`SELECT id,deleted FROM t_conversation WHERE conversation_id=? AND user_id=? FOR UPDATE`, session.ConversationID, session.UserID).Scan(&conversation).Error; err != nil {
			return err
		}
		var input struct {
			ID      string
			Deleted int
		}
		if err := tx.Raw(`SELECT id,deleted FROM t_message WHERE id=? AND conversation_id=? AND user_id=? AND role='user' FOR SHARE`, session.UserMessageID, session.ConversationID, session.UserID).Scan(&input).Error; err != nil {
			return err
		}
		var turn struct {
			ID, Status         string
			AssistantMessageID *string
		}
		if topic.ID != "" {
			if err := tx.Raw(`SELECT id,status,assistant_message_id FROM t_work_turn WHERE conversation_id=? AND user_message_id=? AND user_id=? FOR UPDATE`, session.ConversationID, session.UserMessageID, session.UserID).Scan(&turn).Error; err != nil {
				return err
			}
		}
		var publication PublicationModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("runtime_session_id=?", id).First(&publication).Error; err != nil {
			return err
		}
		live := conversation.ID != "" && conversation.Deleted == 0 && input.ID != "" && input.Deleted == 0 && (topic.ID == "" || (topic.Status == "active" && turn.ID != "" && turn.Status != "cancelled"))
		if !live {
			if publication.State == "pending" {
				if err := abandonPublication(ctx, tx, session); err != nil {
					return err
				}
			}
			abandoned = true
			return nil
		}
		if publication.State == "published" {
			persisted, err := postgresrag.NewConversationMessageRepository(tx).GetByID(ctx, *publication.AssistantMessageID)
			if err != nil {
				return err
			}
			if persisted.ID == "" {
				return fmt.Errorf("published assistant message is no longer available")
			}
			var event JournalModel
			if err := tx.Where("id=?", *publication.FinishEventID).First(&event).Error; err != nil {
				return err
			}
			finish, err = toJournal(event)
			message = publicationMessage(persisted)
			return err
		}
		if publication.State != "pending" {
			return fmt.Errorf("answer publication is abandoned")
		}
		// No model/embedding calls occur in this transaction. One execution owns
		// exactly one assistant ID and one replayable finish event.
		created, err := postgresrag.NewConversationMessageRepository(tx).Create(ctx, prepared.Message)
		if err != nil {
			return err
		}
		if err := postgresrag.PersistPreparedChunks(ctx, tx, prepared.Chunks); err != nil {
			return err
		}
		if err := NewStore(tx).CompleteEpisodes(ctx, id, created.ID); err != nil {
			return err
		}
		if turn.ID != "" {
			if turn.AssistantMessageID != nil && *turn.AssistantMessageID != "" && *turn.AssistantMessageID != created.ID {
				return fmt.Errorf("Work turn already has another assistant message")
			}
			if err := tx.Exec(`UPDATE t_work_turn SET status='completed',assistant_message_id=?,error='',updated_at=CURRENT_TIMESTAMP WHERE id=?`, created.ID, turn.ID).Error; err != nil {
				return err
			}
		}
		finishID, err := distributedid.NextID()
		if err != nil {
			return err
		}
		detail, err := json.Marshal(struct {
			MessageID string                    `json:"messageId"`
			Content   string                    `json:"content"`
			Sources   []ragdomain.MessageSource `json:"sources"`
		}{created.ID, created.DisplayContent(), created.Sources})
		if err != nil {
			return err
		}
		finish = convruntime.JournalEntry{ID: strconv.FormatInt(finishID, 10), RuntimeSessionID: id, ConversationID: session.ConversationID,
			UserMessageID: session.UserMessageID, TraceID: session.TraceID, EventType: convruntime.EventCompleted, Detail: string(detail), CreatedAt: time.Now().UTC()}
		if err := appendJournalTx(tx, &finish); err != nil {
			return err
		}
		if err := tx.Model(&PublicationModel{}).Where("runtime_session_id=?", id).Updates(map[string]any{
			"state": "published", "assistant_message_id": created.ID, "finish_event_id": finish.ID, "last_error": "", "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
			return err
		}
		message = publicationMessage(created)
		return nil
	})
	if err != nil {
		message = convruntime.ConversationMessage{}
		finish = convruntime.JournalEntry{}
	}
	if abandoned {
		err = fmt.Errorf("conversation, input or Work turn is no longer publishable")
	}
	return
}

func (p *ChatPublisher) Recover(ctx context.Context, user, trace string) (convruntime.JournalEntry, error) {
	terminal, err := NewStore(p.db).RecoverChatExecution(ctx, user, trace)
	if err != nil || terminal.ID != "" {
		return terminal, err
	}
	session, err := NewStore(p.db).FindSessionByTraceID(ctx, user, trace)
	if err != nil || session.ID == "" {
		return convruntime.JournalEntry{}, err
	}
	var row PublicationModel
	err = p.db.WithContext(ctx).Where("runtime_session_id=?", session.ID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return convruntime.JournalEntry{}, nil
	} // historical sessions have no protocol marker
	if err != nil {
		return convruntime.JournalEntry{}, err
	}
	if row.State == "abandoned" {
		return convruntime.JournalEntry{}, nil
	}
	_, finish, err := p.Publish(ctx, session.ID)
	if err != nil {
		// Revocation committed an explicit cancellation event; replay that fate.
		var current PublicationModel
		if readErr := p.db.WithContext(ctx).Where("runtime_session_id=?", session.ID).First(&current).Error; readErr == nil && current.State == "abandoned" {
			return convruntime.JournalEntry{}, nil
		}
	}
	return finish, err
}

func abandonPublication(ctx context.Context, tx *gorm.DB, session SessionModel) error {
	if err := tx.Model(&PublicationModel{}).Where("runtime_session_id=?", session.ID).Updates(map[string]any{"state": "abandoned", "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
		return err
	}
	if err := NewStore(tx).DiscardEpisodes(ctx, session.ID); err != nil {
		return err
	}
	id, err := distributedid.NextID()
	if err != nil {
		return err
	}
	event := convruntime.JournalEntry{ID: strconv.FormatInt(id, 10), RuntimeSessionID: session.ID, ConversationID: session.ConversationID, UserMessageID: session.UserMessageID, TraceID: session.TraceID, EventType: convruntime.EventCancelled, Detail: "pending answer publication revoked", CreatedAt: time.Now().UTC()}
	return appendJournalTx(tx, &event)
}

func (p *ChatPublisher) CancelPending(ctx context.Context, user, trace string) error {
	if err := NewStore(p.db).CancelChatExecution(ctx, user, trace); err != nil {
		return err
	}
	session, err := NewStore(p.db).FindSessionByTraceID(ctx, user, trace)
	if err != nil || session.ID == "" {
		return err
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row PublicationModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("runtime_session_id=?", session.ID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.State != "pending" {
			return nil
		}
		return abandonPublication(ctx, tx, toSessionModel(session))
	})
}

func (p *ChatPublisher) RecoverPending(ctx context.Context, limit int, notify func(context.Context, convruntime.JournalEntry) error) error {
	var rows []PublicationModel
	if err := p.db.WithContext(ctx).Where("state='pending' AND next_attempt_at<=CURRENT_TIMESTAMP").Order("next_attempt_at,runtime_session_id").Limit(limit).Find(&rows).Error; err != nil {
		return err
	}
	var result error
	for _, row := range rows {
		if ctx.Err() != nil {
			return errors.Join(result, ctx.Err())
		}
		_, finish, err := p.Publish(ctx, row.RuntimeSessionID)
		if err == nil && notify != nil {
			err = notify(ctx, finish)
		}
		result = errors.Join(result, err)
	}
	return result
}

var _ convruntime.ChatPublication = (*ChatPublisher)(nil)
