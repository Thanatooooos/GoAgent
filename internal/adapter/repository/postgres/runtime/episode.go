package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/runtime/persistence"
)

type EpisodeModel struct {
	ID                       string     `gorm:"column:id;primaryKey"`
	RuntimeSessionID         string     `gorm:"column:runtime_session_id"`
	ConversationID           string     `gorm:"column:conversation_id"`
	UserID                   string     `gorm:"column:user_id"`
	SourceUserMessageID      string     `gorm:"column:source_user_message_id"`
	SourceAssistantMessageID string     `gorm:"column:source_assistant_message_id"`
	Summary                  string     `gorm:"column:summary"`
	TopicsJSON               string     `gorm:"column:topics_json"`
	Importance               string     `gorm:"column:importance"`
	MentionedStart           *time.Time `gorm:"column:mentioned_start"`
	MentionedEnd             *time.Time `gorm:"column:mentioned_end"`
	Status                   string     `gorm:"column:status"`
	CreateTime               time.Time  `gorm:"column:create_time"`
	UpdateTime               time.Time  `gorm:"column:update_time"`
}

func (EpisodeModel) TableName() string { return "t_runtime_episode" }

var _ persistence.EpisodeStore = (*Store)(nil)

func (s *Store) CreateEpisode(ctx context.Context, episode persistence.Episode, vector []float32) error {
	if s == nil || s.db == nil || len(vector) == 0 {
		return fmt.Errorf("runtime episode store and embedding are required")
	}
	topics, err := json.Marshal(episode.Topics)
	if err != nil {
		return fmt.Errorf("encode episode topics: %w", err)
	}
	model := EpisodeModel{ID: episode.ID, RuntimeSessionID: episode.RuntimeSessionID, ConversationID: episode.ConversationID, UserID: episode.UserID, SourceUserMessageID: episode.SourceUserMessageID, Summary: episode.Summary, TopicsJSON: string(topics), Importance: episode.Importance, MentionedStart: episode.MentionedStart, MentionedEnd: episode.MentionedEnd, Status: episode.Status, CreateTime: episode.CreatedAt, UpdateTime: episode.UpdatedAt}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := fenceChatSession(ctx, tx, episode.RuntimeSessionID); err != nil {
			return err
		}
		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("create runtime episode: %w", err)
		}
		if err := tx.Exec(`INSERT INTO t_runtime_episode_embedding (episode_id, embedding, create_time, update_time) VALUES (?, CAST(? AS vector), ?, ?)`, model.ID, formatEpisodeVector(vector), model.CreateTime, model.UpdateTime).Error; err != nil {
			return err
		}
		return fenceChatSession(ctx, tx, episode.RuntimeSessionID)
	})
}

func (s *Store) CompleteEpisodes(ctx context.Context, sessionID, assistantMessageID string) error {
	return s.db.WithContext(ctx).Model(&EpisodeModel{}).Where("runtime_session_id = ? AND status = ?", strings.TrimSpace(sessionID), persistence.EpisodePending).Updates(map[string]any{"status": persistence.EpisodeReady, "source_assistant_message_id": strings.TrimSpace(assistantMessageID), "update_time": gorm.Expr("CURRENT_TIMESTAMP")}).Error
}

func (s *Store) DiscardEpisodes(ctx context.Context, sessionID string) error {
	return s.db.WithContext(ctx).Model(&EpisodeModel{}).Where("runtime_session_id = ? AND status = ?", strings.TrimSpace(sessionID), persistence.EpisodePending).Updates(map[string]any{"status": persistence.EpisodeDiscarded, "update_time": gorm.Expr("CURRENT_TIMESTAMP")}).Error
}

func (s *Store) SearchEpisodes(ctx context.Context, search persistence.EpisodeSearch) ([]persistence.EpisodeHit, error) {
	if s == nil || s.db == nil || len(search.Vector) == 0 {
		return nil, fmt.Errorf("runtime episode store and query embedding are required")
	}
	if search.Limit <= 0 {
		search.Limit = 8
	}
	query := `SELECT e.id, e.runtime_session_id, e.conversation_id, e.user_id, e.source_user_message_id, e.source_assistant_message_id, e.summary, e.topics_json, e.importance, e.mentioned_start, e.mentioned_end, e.status, e.create_time, e.update_time, 1 - (ee.embedding <=> CAST(? AS vector)) AS score FROM t_runtime_episode e JOIN t_runtime_episode_embedding ee ON ee.episode_id = e.id WHERE e.user_id = ? AND e.status = ?`
	args := []any{formatEpisodeVector(search.Vector), strings.TrimSpace(search.UserID), persistence.EpisodeReady}
	if id := strings.TrimSpace(search.ConversationID); id != "" {
		query += " AND e.conversation_id = ?"
		args = append(args, id)
	}
	if search.Start != nil && search.End != nil {
		query += " AND COALESCE(e.mentioned_start, e.create_time) < ? AND COALESCE(e.mentioned_end, e.create_time) >= ?"
		args = append(args, *search.End, *search.Start)
	}
	query += " ORDER BY ee.embedding <=> CAST(? AS vector) LIMIT ?"
	args = append(args, formatEpisodeVector(search.Vector), search.Limit)
	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("search runtime episodes: %w", err)
	}
	defer rows.Close()
	result := make([]persistence.EpisodeHit, 0, search.Limit)
	for rows.Next() {
		var model EpisodeModel
		var score float32
		if err := rows.Scan(&model.ID, &model.RuntimeSessionID, &model.ConversationID, &model.UserID, &model.SourceUserMessageID, &model.SourceAssistantMessageID, &model.Summary, &model.TopicsJSON, &model.Importance, &model.MentionedStart, &model.MentionedEnd, &model.Status, &model.CreateTime, &model.UpdateTime, &score); err != nil {
			return nil, fmt.Errorf("scan runtime episode: %w", err)
		}
		var topics []string
		_ = json.Unmarshal([]byte(model.TopicsJSON), &topics)
		result = append(result, persistence.EpisodeHit{Episode: persistence.Episode{ID: model.ID, RuntimeSessionID: model.RuntimeSessionID, ConversationID: model.ConversationID, UserID: model.UserID, SourceUserMessageID: model.SourceUserMessageID, SourceAssistantMessageID: model.SourceAssistantMessageID, Summary: model.Summary, Topics: topics, Importance: model.Importance, MentionedStart: model.MentionedStart, MentionedEnd: model.MentionedEnd, Status: model.Status, CreatedAt: model.CreateTime, UpdatedAt: model.UpdateTime}, Score: score})
	}
	return result, rows.Err()
}

func formatEpisodeVector(values []float32) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
