package work

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/work/domain"
)

func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func runningTurn(tx *gorm.DB, user string, scope *capability.WorkScope) (domain.Turn, error) {
	if scope == nil {
		return domain.Turn{}, fmt.Errorf("Work scope required")
	}
	if _, err := activeTopic(tx, user, scope.TopicID); err != nil {
		return domain.Turn{}, err
	}
	var row turnRow
	r := tx.Raw(`SELECT * FROM t_work_turn WHERE id=? AND topic_id=? AND user_id=? AND status='running' AND deadline_at>CURRENT_TIMESTAMP FOR UPDATE`, scope.TurnID, scope.TopicID, user).Scan(&row)
	if r.Error != nil {
		return domain.Turn{}, r.Error
	}
	if r.RowsAffected != 1 {
		return domain.Turn{}, fmt.Errorf("this Work turn is no longer active")
	}
	return row.value()
}
func aiMutation[T any](s *Store, ctx capability.Context, kind string, input any, fn func(*gorm.DB, domain.Turn) (T, error)) (T, error) {
	var out T
	if ctx.Work == nil || ctx.ToolCallID == "" {
		return out, fmt.Errorf("accepted turn and tool identity required")
	}
	hash := fingerprint(input)
	err := s.db.WithContext(ctx.Context).Transaction(func(tx *gorm.DB) error {
		// The topic lock serializes writes with archive, user saves and cancellation.
		if _, err := getTopic(tx, ctx.UserID, ctx.Work.TopicID, true); err != nil {
			return err
		}
		var prior struct {
			InputHash  string
			ResultJSON []byte
		}
		r := tx.Raw(`SELECT o.input_hash,o.result_json FROM t_work_turn_operation o JOIN t_work_turn t ON t.id=o.turn_id WHERE o.turn_id=? AND o.kind=? AND t.topic_id=? AND t.user_id=?`, ctx.Work.TurnID, kind, ctx.Work.TopicID, ctx.UserID).Scan(&prior)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			if prior.InputHash != hash {
				return domain.ErrRequestReused
			}
			return json.Unmarshal(prior.ResultJSON, &out)
		}
		// Replaying a saved result is read-only; only new writes need live ownership.
		if err := postgresruntime.FenceChatTask(ctx.Context, tx, ctx.Work.TurnID); err != nil {
			return err
		}
		turn, err := runningTurn(tx, ctx.UserID, ctx.Work)
		if err != nil {
			return err
		}
		out, err = fn(tx, turn)
		if err != nil {
			return err
		}
		body, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_work_turn_operation(turn_id,kind,tool_call_id,input_hash,result_json) VALUES(?,?,?,?,CAST(? AS jsonb))`, turn.ID, kind, ctx.ToolCallID, hash, string(body)).Error; err != nil {
			return err
		}
		return postgresruntime.FenceChatTask(ctx.Context, tx, ctx.Work.TurnID)
	})
	return out, err
}
func (s *Store) WriteDocument(ctx capability.Context, in domain.AIDocumentChange) (domain.ArtifactDetail, error) {
	return aiMutation(s, ctx, "document", in, func(tx *gorm.DB, t domain.Turn) (domain.ArtifactDetail, error) {
		if len(in.Summary) > 8000 {
			return domain.ArtifactDetail{}, fmt.Errorf("summary is too long")
		}
		now := time.Now().UTC()
		if t.Action == "create_document" {
			if in.Body == nil || len(in.Changes) > 0 {
				return domain.ArtifactDetail{}, fmt.Errorf("creation requires a complete structured body")
			}
			body := domain.PrepareNewDocument(*in.Body)
			if err := body.Validate(); err != nil {
				return domain.ArtifactDetail{}, err
			}
			if err := domain.ValidateName(in.Title); err != nil {
				return domain.ArtifactDetail{}, err
			}
			id, err := nextID()
			if err != nil {
				return domain.ArtifactDetail{}, err
			}
			a := domain.Artifact{ID: id, TopicID: t.TopicID, ItemID: t.ItemID, Title: in.Title, Revision: 1, CreatedAt: now, UpdatedAt: now}
			if err := tx.Exec(`INSERT INTO t_work_artifact(id,topic_id,item_id,title,revision,created_at,updated_at) VALUES(?,?,?,?,1,?,?)`, id, t.TopicID, nullable(t.ItemID), in.Title, now, now).Error; err != nil {
				return domain.ArtifactDetail{}, err
			}
			v := domain.ArtifactVersion{ArtifactID: id, Revision: 1, Title: in.Title, Body: body, Author: "ai", Summary: in.Summary, ConversationID: t.ConversationID, CreatedAt: now}
			if err := insertVersion(tx, v); err != nil {
				return domain.ArtifactDetail{}, err
			}
			return domain.ArtifactDetail{Artifact: a, Version: v}, touch(tx, t.TopicID)
		}
		if t.Action != "edit_document" && t.Action != "rewrite_document" {
			return domain.ArtifactDetail{}, fmt.Errorf("the user authorized discussion only; suggest the edit in chat and ask them to hand over the document")
		}
		a, err := loadArtifact(tx, t.TopicID, t.ArtifactID)
		if err != nil {
			return domain.ArtifactDetail{}, err
		}
		if a.Revision != t.ArtifactRevision {
			return domain.ArtifactDetail{}, &domain.Conflict{CurrentRevision: a.Revision}
		}
		v, err := loadVersion(tx, a.ID, a.Revision)
		if err != nil {
			return domain.ArtifactDetail{}, err
		}
		if t.Action == "edit_document" {
			if in.Body != nil {
				return domain.ArtifactDetail{}, fmt.Errorf("targeted editing permits named block changes only")
			}
			v.Body, err = v.Body.ApplyBlocks(in.Changes)
		} else {
			if in.Body == nil || len(in.Changes) > 0 {
				return domain.ArtifactDetail{}, fmt.Errorf("rewrite requires a complete body")
			}
			v.Body = domain.PrepareNewDocument(*in.Body)
			err = v.Body.Validate()
		}
		if err != nil {
			return domain.ArtifactDetail{}, err
		}
		if in.Title != "" {
			if err := domain.ValidateName(in.Title); err != nil {
				return domain.ArtifactDetail{}, err
			}
			v.Title = in.Title
		}
		v.Revision++
		v.Author = "ai"
		v.Summary = in.Summary
		v.ConversationID = t.ConversationID
		v.CreatedAt = now
		v.RestoredFrom = 0
		return saveVersion(tx, a, v, a.ItemID)
	})
}

type proposalRow struct {
	ID, TopicID, TurnID, Status   string
	BaseRevision, AppliedRevision int
	ChangesJSON                   []byte
	CreatedAt                     time.Time
}

func (r proposalRow) value() (domain.Proposal, error) {
	v := domain.Proposal{ID: r.ID, TopicID: r.TopicID, TurnID: r.TurnID, Status: r.Status, BaseRevision: r.BaseRevision, AppliedRevision: r.AppliedRevision, CreatedAt: r.CreatedAt}
	if len(r.ChangesJSON) > 0 && r.ChangesJSON[0] == '{' {
		v.Document = &domain.DocumentProposal{}
		return v, json.Unmarshal(r.ChangesJSON, v.Document)
	}
	err := json.Unmarshal(r.ChangesJSON, &v.Changes)
	return v, err
}
func (s *Store) SuggestProgress(ctx capability.Context, changes []domain.StateChange) (domain.Proposal, error) {
	changes = append([]domain.StateChange(nil), changes...)
	return aiMutation(s, ctx, "progress", changes, func(tx *gorm.DB, t domain.Turn) (domain.Proposal, error) {
		state, err := loadState(tx, t.TopicID)
		if err != nil {
			return domain.Proposal{}, err
		}
		if state.Revision != t.StateRevision {
			return domain.Proposal{}, &domain.Conflict{CurrentRevision: state.Revision}
		}
		// The user message is the basis; the model cannot fabricate message anchors.
		for i := range changes {
			if changes[i].Kind != "remove" {
				changes[i].Entry.References = []domain.Reference{{Kind: "message", ID: t.UserMessageID}}
			}
			if changes[i].Entry.ItemID != t.ItemID {
				return domain.Proposal{}, fmt.Errorf("progress suggestion must use the current item")
			}
		}
		filtered := make([]domain.StateChange, 0, len(changes))
		for _, change := range changes {
			unchanged := false
			for _, entry := range state.Entries {
				if change.Kind == "add" || (change.Kind == "update" && entry.ID == change.Entry.ID) {
					unchanged = unchanged || (entry.Kind == change.Entry.Kind && entry.ItemID == change.Entry.ItemID && entry.Text == change.Entry.Text)
				}
			}
			if !unchanged {
				filtered = append(filtered, change)
			}
		}
		changes = filtered
		if len(changes) == 0 {
			return domain.Proposal{}, fmt.Errorf("these changes are already reflected in confirmed progress; no new suggestion is needed")
		}
		entries, err := domain.ApplyStateChanges(state.Entries, changes)
		if err != nil {
			return domain.Proposal{}, err
		}
		if err := checkEntries(tx, ctx.UserID, t.TopicID, entries, state.Entries); err != nil {
			return domain.Proposal{}, err
		}
		semantic := append([]domain.StateChange(nil), changes...)
		for i := range semantic {
			semantic[i].Entry.References = nil
			if semantic[i].Kind == "add" {
				semantic[i].Entry.ID = ""
			}
		}
		hash := fingerprint(struct {
			Changes  []domain.StateChange
			Revision int
		}{semantic, state.Revision})
		var prior proposalRow
		r := tx.Raw(`SELECT * FROM t_work_proposal WHERE topic_id=? AND fingerprint=?`, t.TopicID, hash).Scan(&prior)
		if r.Error != nil {
			return domain.Proposal{}, r.Error
		}
		if r.RowsAffected > 0 {
			return prior.value()
		}
		id, err := nextID()
		if err != nil {
			return domain.Proposal{}, err
		}
		body, _ := json.Marshal(changes)
		p := domain.Proposal{ID: id, TopicID: t.TopicID, TurnID: t.ID, BaseRevision: state.Revision, Changes: changes, Status: "pending", CreatedAt: time.Now().UTC()}
		return p, tx.Exec(`INSERT INTO t_work_proposal(id,topic_id,turn_id,base_revision,changes_json,fingerprint,created_at) VALUES(?,?,?,?,CAST(? AS jsonb),?,?)`, id, t.TopicID, t.ID, state.Revision, string(body), hash, p.CreatedAt).Error
	})
}
func (s *Store) ListProposals(ctx context.Context, user, topic string, page domain.Page) ([]domain.Proposal, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	rows := []proposalRow{}
	if err := s.db.WithContext(ctx).Raw(`SELECT * FROM t_work_proposal WHERE topic_id=? ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, topic, page.Limit, page.Offset).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := []domain.Proposal{}
	for _, r := range rows {
		v, err := r.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) ResolveProposal(ctx context.Context, user, topic, id string, in domain.ResolveProposal) (domain.Proposal, error) {
	return mutate(s, ctx, user, topic, "proposal.resolve/"+id, in.Mutation, in, func(tx *gorm.DB) (domain.Proposal, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.Proposal{}, err
		}
		var row proposalRow
		r := tx.Raw(`SELECT * FROM t_work_proposal WHERE id=? AND topic_id=? FOR UPDATE`, id, topic).Scan(&row)
		if r.Error != nil {
			return domain.Proposal{}, r.Error
		}
		if r.RowsAffected != 1 {
			return domain.Proposal{}, domain.ErrNotFound
		}
		p, err := row.value()
		if err != nil {
			return p, err
		}
		if p.Status != "pending" {
			return p, nil
		}
		if p.Document != nil {
			return resolveDocumentProposal(tx, p, in)
		}
		if in.Ignore {
			p.Status = "ignored"
		} else {
			state, err := loadState(tx, topic)
			if err != nil {
				return p, err
			}
			if state.Revision != in.ExpectedRevision {
				return p, &domain.Conflict{CurrentRevision: state.Revision}
			}
			// A stale base needs an explicitly edited/reviewed change set.
			if p.BaseRevision != state.Revision && in.Changes == nil {
				return p, &domain.Conflict{CurrentRevision: state.Revision}
			}
			if in.Changes != nil {
				p.Changes = in.Changes
			}
			entries, err := domain.ApplyStateChanges(state.Entries, p.Changes)
			if err != nil {
				return p, err
			}
			if err := checkEntries(tx, user, topic, entries, state.Entries); err != nil {
				return p, err
			}
			saved, err := insertState(tx, topic, state.Revision+1, entries, "user")
			if err != nil {
				return p, err
			}
			if err := tx.Exec(`UPDATE t_work_state SET revision=? WHERE topic_id=?`, saved.Revision, topic).Error; err != nil {
				return p, err
			}
			p.Status = "applied"
			p.AppliedRevision = saved.Revision
		}
		body, _ := json.Marshal(p.Changes)
		return p, tx.Exec(`UPDATE t_work_proposal SET status=?,applied_revision=?,changes_json=CAST(? AS jsonb) WHERE id=?`, p.Status, p.AppliedRevision, string(body), id).Error
	})
}
