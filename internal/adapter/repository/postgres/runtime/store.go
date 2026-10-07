package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/persistence"
)

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

var _ persistence.Store = (*Store)(nil)

func (s *Store) CreateOrLoadSession(ctx context.Context, session persistence.Session) (persistence.Session, error) {
	if err := session.Validate(); err != nil {
		return persistence.Session{}, err
	}
	model := toSessionModel(session)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if session.ExecutionManaged {
			if err := ReserveChatExecution(ctx, tx, session); err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "conversation_id"}, {Name: "user_message_id"}},
			DoNothing: true,
		}).Create(&model).Error
	}); err != nil {
		return persistence.Session{}, fmt.Errorf("create runtime session: %w", err)
	}
	var persisted SessionModel
	err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND user_message_id = ?", session.ConversationID, session.UserMessageID).
		First(&persisted).Error
	if err != nil {
		return persistence.Session{}, fmt.Errorf("load runtime session: %w", err)
	}
	return toSession(persisted), nil
}

func (s *Store) FindSessionByTraceID(ctx context.Context, userID, traceID string) (persistence.Session, error) {
	var persisted SessionModel
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND trace_id = ?", strings.TrimSpace(userID), strings.TrimSpace(traceID)).
		Order("create_time DESC").
		First(&persisted).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return persistence.Session{}, nil
	}
	if err != nil {
		return persistence.Session{}, fmt.Errorf("find runtime session by trace id: %w", err)
	}
	return toSession(persisted), nil
}

// CanAccessChatTask checks durable ownership before any stream/cache access.
// Work executions are deliberately accessible only through their topic routes.
func (s *Store) CanAccessChatTask(ctx context.Context, userID, taskID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM t_runtime_session r
		JOIN t_conversation c ON c.conversation_id=r.conversation_id AND c.user_id=r.user_id
		JOIN t_message m ON m.id=r.user_message_id AND m.conversation_id=r.conversation_id AND m.user_id=r.user_id
		WHERE r.user_id=? AND r.trace_id=? AND c.deleted=0 AND m.deleted=0
		AND NOT EXISTS (SELECT 1 FROM t_work_conversation w WHERE w.conversation_id=r.conversation_id)`, strings.TrimSpace(userID), strings.TrimSpace(taskID)).Scan(&count).Error
	return count > 0, err
}

// CountTraceSessions and ListTraceSessions are the read model consumed by the
// runtime trace UI. The journal remains the source of execution facts.
func (s *Store) CountTraceSessions(ctx context.Context, traceID, conversationID, status string) (int, error) {
	query := applyTraceSessionFilter(s.db.WithContext(ctx).Model(&SessionModel{}), traceID, conversationID, status)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count runtime trace sessions: %w", err)
	}
	return int(count), nil
}

func (s *Store) ListTraceSessions(ctx context.Context, traceID, conversationID, status string, offset, limit int) ([]persistence.Session, error) {
	query := applyTraceSessionFilter(s.db.WithContext(ctx), traceID, conversationID, status).Order("create_time DESC")
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	var models []SessionModel
	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list runtime trace sessions: %w", err)
	}
	result := make([]persistence.Session, 0, len(models))
	for _, model := range models {
		result = append(result, toSession(model))
	}
	return result, nil
}

func (s *Store) FindTraceSession(ctx context.Context, traceID string) (persistence.Session, error) {
	var model SessionModel
	err := s.db.WithContext(ctx).Where("trace_id = ?", strings.TrimSpace(traceID)).Order("create_time DESC").First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return persistence.Session{}, nil
	}
	if err != nil {
		return persistence.Session{}, fmt.Errorf("find runtime trace session: %w", err)
	}
	return toSession(model), nil
}

func applyTraceSessionFilter(query *gorm.DB, traceID, conversationID, status string) *gorm.DB {
	if value := strings.TrimSpace(traceID); value != "" {
		query = query.Where("trace_id = ?", value)
	}
	if value := strings.TrimSpace(conversationID); value != "" {
		query = query.Where("conversation_id = ?", value)
	}
	if value := strings.TrimSpace(status); value != "" {
		query = query.Where("status = ?", value)
	}
	return query
}

func (s *Store) SetStatus(ctx context.Context, runtimeSessionID, status string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := fenceChatSession(ctx, tx, runtimeSessionID); err != nil {
			return err
		}
		result := tx.Model(&SessionModel{}).Where("id = ? AND status = ?", runtimeSessionID, persistence.StatusRunning).Updates(map[string]any{"status": status, "update_time": gorm.Expr("CURRENT_TIMESTAMP")})
		if result.Error != nil {
			return fmt.Errorf("set runtime session status: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("running runtime session %q not found", runtimeSessionID)
		}
		var task string
		if err := tx.Raw(`SELECT trace_id FROM t_runtime_session WHERE id=?`, runtimeSessionID).Scan(&task).Error; err != nil {
			return err
		}
		return closeChatExecution(ctx, tx, task, status)
	})
}

func (s *Store) Append(ctx context.Context, entry convruntime.JournalEntry) (convruntime.JournalEntry, error) {
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.RuntimeSessionID) == "" || strings.TrimSpace(entry.EventType) == "" {
		return convruntime.JournalEntry{}, fmt.Errorf("journal id, runtime session id, and event type are required")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := fenceChatSession(ctx, tx, entry.RuntimeSessionID); err != nil {
			return err
		}
		if err := appendJournalTx(tx, &entry); err != nil {
			return err
		}
		return fenceChatSession(ctx, tx, entry.RuntimeSessionID)
	})
	if err != nil {
		return convruntime.JournalEntry{}, err
	}
	return entry, nil
}

func (s *Store) TransitionTool(ctx context.Context, entry convruntime.JournalEntry, allowedFrom []string) (convruntime.JournalEntry, error) {
	if strings.TrimSpace(entry.ToolCallID) == "" || strings.TrimSpace(entry.ToolState) == "" {
		return convruntime.JournalEntry{}, fmt.Errorf("tool call id and tool state are required")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := fenceChatSession(ctx, tx, entry.RuntimeSessionID); err != nil {
			return err
		}
		if err := lockSession(tx, entry.RuntimeSessionID); err != nil {
			return err
		}
		var latest JournalModel
		err := tx.Where("runtime_session_id = ? AND tool_call_id = ? AND tool_state <> ''", entry.RuntimeSessionID, entry.ToolCallID).Order("sequence DESC").First(&latest).Error
		from := ""
		if err == nil {
			from = latest.ToolState
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load latest runtime tool state: %w", err)
		}
		if !contains(allowedFrom, from) {
			return fmt.Errorf("invalid tool state transition %q -> %q", from, entry.ToolState)
		}
		if err := appendJournalTx(tx, &entry); err != nil {
			return err
		}
		return fenceChatSession(ctx, tx, entry.RuntimeSessionID)
	})
	if err != nil {
		return convruntime.JournalEntry{}, err
	}
	return entry, nil
}

func (s *Store) ListJournal(ctx context.Context, runtimeSessionID string) ([]persistence.JournalEntry, error) {
	var models []JournalModel
	if err := s.db.WithContext(ctx).Where("runtime_session_id = ?", runtimeSessionID).Order("sequence ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list runtime journal: %w", err)
	}
	entries := make([]persistence.JournalEntry, 0, len(models))
	for _, model := range models {
		entry, err := toJournal(model)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Store) ListUnsettledToolCalls(ctx context.Context, runtimeSessionID string) ([]persistence.UnsettledToolCall, error) {
	var entries []JournalModel
	if err := s.db.WithContext(ctx).
		Where("runtime_session_id = ? AND tool_call_id <> '' AND tool_state <> ''", runtimeSessionID).
		Order("sequence ASC").
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("list runtime tool journal: %w", err)
	}
	latest := make(map[string]JournalModel, len(entries))
	for _, entry := range entries {
		latest[entry.ToolCallID] = entry
	}
	result := make([]persistence.UnsettledToolCall, 0, len(latest))
	for _, entry := range latest {
		if entry.ToolState == convruntime.ToolStatePending || entry.ToolState == convruntime.ToolStateExecuting {
			result = append(result, persistence.UnsettledToolCall{ToolCallID: entry.ToolCallID, ToolName: entry.ToolName, State: entry.ToolState})
		}
	}
	return result, nil
}

func toSessionModel(session persistence.Session) SessionModel {
	return SessionModel{ID: session.ID, ConversationID: session.ConversationID, UserMessageID: session.UserMessageID, UserID: session.UserID, TraceID: session.TraceID, Status: session.Status, NextSequence: session.NextSequence, CreateTime: session.CreatedAt, UpdateTime: session.UpdatedAt}
}

func toSession(model SessionModel) persistence.Session {
	return persistence.Session{ID: model.ID, ConversationID: model.ConversationID, UserMessageID: model.UserMessageID, UserID: model.UserID, TraceID: model.TraceID, Status: model.Status, NextSequence: model.NextSequence, CreatedAt: model.CreateTime, UpdatedAt: model.UpdateTime}
}

func toJournalModel(entry convruntime.JournalEntry) JournalModel {
	evidence, err := json.Marshal(entry.Evidence)
	if err != nil {
		evidence = []byte("[]")
	}
	return JournalModel{ID: entry.ID, RuntimeSessionID: entry.RuntimeSessionID, Sequence: entry.Sequence, ConversationID: entry.ConversationID, UserMessageID: entry.UserMessageID, TraceID: entry.TraceID, EventType: entry.EventType, ToolCallID: entry.ToolCallID, ToolName: entry.ToolName, ToolState: entry.ToolState, EvidenceJSON: string(evidence), Detail: entry.Detail, CreateTime: entry.CreatedAt}
}

func toJournal(model JournalModel) (persistence.JournalEntry, error) {
	var evidence []persistence.EvidenceRef
	if err := json.Unmarshal([]byte(model.EvidenceJSON), &evidence); err != nil {
		return persistence.JournalEntry{}, fmt.Errorf("decode runtime journal evidence: %w", err)
	}
	return persistence.JournalEntry{ID: model.ID, RuntimeSessionID: model.RuntimeSessionID, Sequence: model.Sequence, ConversationID: model.ConversationID, UserMessageID: model.UserMessageID, TraceID: model.TraceID, EventType: model.EventType, ToolCallID: model.ToolCallID, ToolName: model.ToolName, ToolState: model.ToolState, Evidence: evidence, Detail: model.Detail, CreatedAt: model.CreateTime}, nil
}

func lockSession(tx *gorm.DB, sessionID string) error {
	var session SessionModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("runtime session %q not found", sessionID)
	}
	if err != nil {
		return fmt.Errorf("lock runtime session: %w", err)
	}
	return nil
}

func appendJournalTx(tx *gorm.DB, entry *convruntime.JournalEntry) error {
	var next struct{ Sequence int64 }
	if err := tx.Raw(`UPDATE t_runtime_session SET next_sequence = next_sequence + 1, update_time = CURRENT_TIMESTAMP WHERE id = ? RETURNING next_sequence - 1 AS sequence`, entry.RuntimeSessionID).Scan(&next).Error; err != nil {
		return fmt.Errorf("allocate runtime journal sequence: %w", err)
	}
	if next.Sequence == 0 {
		return fmt.Errorf("runtime session %q not found", entry.RuntimeSessionID)
	}
	entry.Sequence = next.Sequence
	model := toJournalModel(*entry)
	if err := tx.Create(&model).Error; err != nil {
		return fmt.Errorf("create runtime journal entry: %w", err)
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
