package work

import (
	"context"
	"encoding/json"
	"fmt"

	"local/rag-project/internal/app/work/domain"
)

type DocumentDraft struct {
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	BaseRevision int             `json:"baseRevision"`
	Body         domain.Document `json:"body"`
}

// Failed tool inputs are already journaled before execution. Expose a scoped
// preview for manual comparison without replaying them or changing the document.
func (s *Store) ReadDocumentDraft(ctx context.Context, user, topic, turnID string) (DocumentDraft, error) {
	turn, err := s.GetTurn(ctx, user, topic, turnID)
	if err != nil {
		return DocumentDraft{}, err
	}
	for _, output := range turn.Outputs {
		if output.Kind == "document" {
			return DocumentDraft{}, domain.ErrNotFound
		}
	}
	if turn.Action != "edit_document" && turn.Action != "rewrite_document" && turn.Action != "create_document" {
		return DocumentDraft{}, domain.ErrNotFound
	}
	var row struct{ Detail string }
	query := s.db.WithContext(ctx).Raw(`SELECT pending.detail FROM t_runtime_journal pending JOIN t_runtime_session session ON session.id=pending.runtime_session_id WHERE session.trace_id=? AND session.user_id=? AND pending.tool_name='work_write_document' AND pending.tool_state='pending' AND EXISTS(SELECT 1 FROM t_runtime_journal failed WHERE failed.runtime_session_id=pending.runtime_session_id AND failed.tool_call_id=pending.tool_call_id AND failed.tool_state='failed') ORDER BY pending.sequence DESC LIMIT 1`, turnID, user).Scan(&row)
	if query.Error != nil {
		return DocumentDraft{}, query.Error
	}
	if query.RowsAffected != 1 {
		return DocumentDraft{}, domain.ErrNotFound
	}
	var input domain.AIDocumentChange
	if err = json.Unmarshal([]byte(row.Detail), &input); err != nil {
		return DocumentDraft{}, domain.ErrNotFound
	}
	draft := DocumentDraft{Title: input.Title, Summary: input.Summary, BaseRevision: turn.ArtifactRevision}
	if input.Body != nil {
		draft.Body = domain.PrepareNewDocument(*input.Body)
	} else {
		version, err := s.GetVersion(ctx, user, topic, turn.ArtifactID, turn.ArtifactRevision)
		if err != nil {
			return DocumentDraft{}, err
		}
		if draft.Title == "" {
			draft.Title = version.Title
		}
		draft.Body, err = version.Body.ApplyBlocks(input.Changes)
		if err != nil {
			return DocumentDraft{}, domain.ErrNotFound
		}
	}
	if err = draft.Body.Validate(); err != nil {
		return DocumentDraft{}, fmt.Errorf("本轮未保存内容无法生成文档预览")
	}
	return draft, nil
}
