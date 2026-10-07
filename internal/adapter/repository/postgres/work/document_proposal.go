package work

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/work/domain"
	"time"
)

func (s *Store) SuggestDocumentText(ctx capability.Context, artifact, target, text, summary string) (domain.Proposal, error) {
	if ctx.Work == nil {
		return domain.Proposal{}, fmt.Errorf("Work scope required")
	}
	turn, err := s.GetTurn(ctx.Context, ctx.UserID, ctx.Work.TopicID, ctx.Work.TurnID)
	if err != nil {
		return domain.Proposal{}, err
	}
	if turn.ArtifactID != artifact {
		return domain.Proposal{}, fmt.Errorf("select the document before proposing its edit")
	}
	version, err := s.GetVersion(ctx.Context, ctx.UserID, ctx.Work.TopicID, artifact, turn.ArtifactRevision)
	if err != nil {
		return domain.Proposal{}, err
	}
	var node *domain.Node
	var find func(domain.Node)
	find = func(current domain.Node) {
		if current.ID == target {
			copied := current
			node = &copied
			return
		}
		for _, child := range current.Content {
			find(child)
		}
	}
	find(version.Body.Root)
	if node == nil || (node.Type != "paragraph" && node.Type != "heading") {
		return domain.Proposal{}, fmt.Errorf("targetId must name an existing paragraph or heading; use the ID returned by work_read_document")
	}
	node.Content = nil
	if text != "" {
		node.Content = []domain.Node{{Type: "text", Text: text}}
	}
	return s.SuggestDocument(ctx, domain.SuggestDocument{ArtifactID: artifact, Change: domain.AIDocumentChange{Summary: summary, Changes: []domain.BlockChange{{Kind: "replace", TargetID: target, Node: node}}}})
}

func (s *Store) SuggestDocument(ctx capability.Context, in domain.SuggestDocument) (domain.Proposal, error) {
	return aiMutation(s, ctx, "document_suggestion", in, func(tx *gorm.DB, t domain.Turn) (domain.Proposal, error) {
		d := domain.DocumentProposal{ArtifactID: in.ArtifactID, ItemID: t.ItemID, Title: in.Change.Title, Summary: in.Change.Summary}
		base := 0
		if len(d.Summary) > 8000 {
			return domain.Proposal{}, fmt.Errorf("summary is too long")
		}
		if in.ArtifactID == "" {
			if in.Change.Body == nil || len(in.Change.Changes) > 0 {
				return domain.Proposal{}, fmt.Errorf("new document proposal requires a complete body")
			}
			d.Kind = "artifact_create"
			d.Body = domain.PrepareNewDocument(*in.Change.Body)
		} else {
			if in.ArtifactID != t.ArtifactID {
				return domain.Proposal{}, fmt.Errorf("select the document before proposing its edit")
			}
			a, err := loadArtifact(tx, t.TopicID, in.ArtifactID)
			if err != nil {
				return domain.Proposal{}, err
			}
			if a.ItemID != "" && a.ItemID != t.ItemID {
				return domain.Proposal{}, fmt.Errorf("document suggestion must use the current item")
			}
			base = a.Revision
			if a.ID == t.ArtifactID && base != t.ArtifactRevision {
				return domain.Proposal{}, &domain.Conflict{CurrentRevision: base}
			}
			v, err := loadVersion(tx, a.ID, base)
			if err != nil {
				return domain.Proposal{}, err
			}
			if in.Change.Body != nil {
				return domain.Proposal{}, fmt.Errorf("existing document suggestions require named block changes")
			}
			d.Kind = "artifact_update"
			d.ItemID = a.ItemID
			if d.Title == "" {
				d.Title = v.Title
			}
			d.Body, err = v.Body.ApplyBlocks(in.Change.Changes)
			if err != nil {
				return domain.Proposal{}, err
			}
			if d.Title == v.Title && fingerprint(d.Body) == fingerprint(v.Body) {
				return domain.Proposal{}, fmt.Errorf("the proposed document is unchanged; no new suggestion is needed")
			}
		}
		if err := domain.ValidateName(d.Title); err != nil {
			return domain.Proposal{}, err
		}
		if err := d.Body.Validate(); err != nil {
			return domain.Proposal{}, err
		}
		semantic := d
		semantic.Summary = ""
		hash := fingerprint(struct {
			Document domain.DocumentProposal
			Revision int
		}{semantic, base})
		var prior proposalRow
		lookup := tx.Raw(`SELECT * FROM t_work_proposal WHERE topic_id=? AND fingerprint=?`, t.TopicID, hash).Scan(&prior)
		if lookup.Error != nil {
			return domain.Proposal{}, lookup.Error
		}
		if lookup.RowsAffected > 0 {
			return prior.value()
		}
		id, err := nextID()
		if err != nil {
			return domain.Proposal{}, err
		}
		p := domain.Proposal{ID: id, TopicID: t.TopicID, TurnID: t.ID, BaseRevision: base, Document: &d, Status: "pending", CreatedAt: time.Now().UTC()}
		body, _ := json.Marshal(d)
		return p, tx.Exec(`INSERT INTO t_work_proposal(id,topic_id,turn_id,base_revision,changes_json,fingerprint,created_at) VALUES(?,?,?,?,CAST(? AS jsonb),?,?)`, id, t.TopicID, t.ID, base, string(body), hash, p.CreatedAt).Error
	})
}

func resolveDocumentProposal(tx *gorm.DB, p domain.Proposal, in domain.ResolveProposal) (domain.Proposal, error) {
	if in.Ignore {
		p.Status = "ignored"
	} else {
		if in.Changes != nil {
			return p, fmt.Errorf("progress changes cannot edit a document proposal")
		}
		d := p.Document
		if err := d.Body.Validate(); err != nil {
			return p, err
		}
		var conversation string
		if err := tx.Raw(`SELECT conversation_id FROM t_work_turn WHERE id=? AND topic_id=?`, p.TurnID, p.TopicID).Scan(&conversation).Error; err != nil {
			return p, err
		}
		now := time.Now().UTC()
		if d.ArtifactID == "" {
			if in.ExpectedRevision != 0 {
				return p, &domain.Conflict{CurrentRevision: 0}
			}
			id, err := nextID()
			if err != nil {
				return p, err
			}
			a := domain.Artifact{ID: id, TopicID: p.TopicID, ItemID: d.ItemID, Title: d.Title, Revision: 1, CreatedAt: now, UpdatedAt: now}
			if err := tx.Exec(`INSERT INTO t_work_artifact(id,topic_id,item_id,title,revision,created_at,updated_at) VALUES(?,?,?,?,1,?,?)`, id, p.TopicID, nullable(d.ItemID), d.Title, now, now).Error; err != nil {
				return p, err
			}
			if err := insertVersion(tx, domain.ArtifactVersion{ArtifactID: id, Revision: 1, Title: a.Title, Body: d.Body, Author: "ai", Summary: d.Summary, ConversationID: conversation, CreatedAt: now}); err != nil {
				return p, err
			}
			d.ArtifactID = id
			p.AppliedRevision = 1
		} else {
			a, err := loadArtifact(tx, p.TopicID, d.ArtifactID)
			if err != nil {
				return p, err
			}
			if a.Revision != p.BaseRevision || a.Revision != in.ExpectedRevision {
				return p, &domain.Conflict{CurrentRevision: a.Revision}
			}
			v, err := loadVersion(tx, a.ID, a.Revision)
			if err != nil {
				return p, err
			}
			v.Title = d.Title
			v.Body = d.Body
			v.Revision++
			v.Author = "ai"
			v.Summary = d.Summary
			v.ConversationID = conversation
			v.CreatedAt = now
			v.RestoredFrom = 0
			if _, err := saveVersion(tx, a, v, a.ItemID); err != nil {
				return p, err
			}
			p.AppliedRevision = v.Revision
		}
		p.Status = "applied"
		if err := touch(tx, p.TopicID); err != nil {
			return p, err
		}
	}
	body, _ := json.Marshal(p.Document)
	return p, tx.Exec(`UPDATE t_work_proposal SET status=?,applied_revision=?,changes_json=CAST(? AS jsonb) WHERE id=?`, p.Status, p.AppliedRevision, string(body), p.ID).Error
}
