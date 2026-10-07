package work

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"local/rag-project/internal/app/work/domain"
)

func loadState(tx *gorm.DB, topic string) (domain.State, error) {
	var row struct {
		TopicID     string
		Revision    int
		EntriesJSON []byte
		Author      string
		CreatedAt   time.Time
	}
	r := tx.Raw(`SELECT r.* FROM t_work_state s JOIN t_work_state_revision r ON r.topic_id=s.topic_id AND r.revision=s.revision WHERE s.topic_id=?`, topic).Scan(&row)
	if r.Error != nil {
		return domain.State{}, r.Error
	}
	if r.RowsAffected == 0 {
		return domain.State{}, domain.ErrNotFound
	}
	out := domain.State{TopicID: topic, Revision: row.Revision, Author: row.Author, CreatedAt: row.CreatedAt}
	err := json.Unmarshal(row.EntriesJSON, &out.Entries)
	return out, err
}
func insertState(tx *gorm.DB, topic string, revision int, entries []domain.StateEntry, author string) (domain.State, error) {
	if entries == nil {
		entries = []domain.StateEntry{}
	}
	body, err := json.Marshal(entries)
	if err != nil {
		return domain.State{}, err
	}
	now := time.Now().UTC()
	err = tx.Exec(`INSERT INTO t_work_state_revision(topic_id,revision,entries_json,author,created_at) VALUES(?,?,CAST(? AS jsonb),?,?)`, topic, revision, string(body), author, now).Error
	return domain.State{TopicID: topic, Revision: revision, Entries: entries, Author: author, CreatedAt: now}, err
}
func (s *Store) GetState(ctx context.Context, user, topic string) (domain.State, error) {
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return domain.State{}, err
	}
	return loadState(s.db.WithContext(ctx), topic)
}
func checkEntries(tx *gorm.DB, user, topic string, entries []domain.StateEntry, previous ...[]domain.StateEntry) error {
	// Deleting a discussion preserves already-confirmed progress and its historical
	// basis. Only newly attached references require a currently readable source.
	known := map[string]bool{}
	for _, list := range previous {
		for _, entry := range list {
			for _, ref := range entry.References {
				known[fmt.Sprintf("%s/%s/%s/%d", entry.ID, ref.Kind, ref.ID, ref.Revision)] = true
			}
		}
	}
	for _, e := range entries {
		if err := checkItem(tx, topic, e.ItemID); err != nil {
			return err
		}
		for _, ref := range e.References {
			if known[fmt.Sprintf("%s/%s/%s/%d", e.ID, ref.Kind, ref.ID, ref.Revision)] {
				continue
			}
			var n int64
			var err error
			switch ref.Kind {
			case "message":
				err = tx.Raw(`SELECT COUNT(*) FROM t_message m JOIN t_work_conversation w ON w.conversation_id=m.conversation_id AND w.user_id=m.user_id JOIN t_conversation c ON c.conversation_id=m.conversation_id AND c.user_id=m.user_id WHERE m.id=? AND w.topic_id=? AND w.user_id=? AND m.deleted=0 AND c.deleted=0`, ref.ID, topic, user).Scan(&n).Error
			case "artifact":
				err = tx.Raw(`SELECT COUNT(*) FROM t_work_artifact a JOIN t_work_artifact_version v ON v.artifact_id=a.id WHERE a.id=? AND a.topic_id=? AND v.revision=?`, ref.ID, topic, ref.Revision).Scan(&n).Error
			default:
				return fmt.Errorf("unsupported progress reference")
			}
			if err != nil {
				return err
			}
			if n != 1 {
				return domain.ErrNotFound
			}
		}
	}
	return nil
}
func (s *Store) SaveState(ctx context.Context, user, topic string, in domain.SaveState) (domain.State, error) {
	if err := domain.ValidateEntries(in.Entries); err != nil {
		return domain.State{}, err
	}
	return mutate(s, ctx, user, topic, "state.save", in.Mutation, in, func(tx *gorm.DB) (domain.State, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.State{}, err
		}
		current, err := loadState(tx, topic)
		if err != nil {
			return current, err
		}
		if current.Revision != in.ExpectedRevision {
			return current, &domain.Conflict{CurrentRevision: current.Revision}
		}
		if err := checkEntries(tx, user, topic, in.Entries, current.Entries); err != nil {
			return domain.State{}, err
		}
		out, err := insertState(tx, topic, current.Revision+1, in.Entries, "user")
		if err != nil {
			return out, err
		}
		if err := tx.Exec(`UPDATE t_work_state SET revision=? WHERE topic_id=?`, out.Revision, topic).Error; err != nil {
			return out, err
		}
		return out, touch(tx, topic)
	})
}
func validateArtifact(in domain.SaveArtifact) error {
	if err := domain.ValidateName(in.Title); err != nil {
		return err
	}
	if len(in.Summary) > 8000 {
		return fmt.Errorf("change summary is too long")
	}
	return in.Body.Validate()
}

type versionRow struct {
	ArtifactID     string
	Revision       int
	Title          string
	BodyJSON       []byte
	Author         string
	Summary        string
	ConversationID *string
	RestoredFrom   int
	CreatedAt      time.Time
}

func (r versionRow) value() (domain.ArtifactVersion, error) {
	v := domain.ArtifactVersion{ArtifactID: r.ArtifactID, Revision: r.Revision, Title: r.Title, Author: r.Author, Summary: r.Summary, RestoredFrom: r.RestoredFrom, CreatedAt: r.CreatedAt}
	if r.ConversationID != nil {
		v.ConversationID = *r.ConversationID
	}
	err := json.Unmarshal(r.BodyJSON, &v.Body)
	return v, err
}
func loadArtifact(tx *gorm.DB, topic, id string) (domain.Artifact, error) {
	var a domain.Artifact
	r := tx.Raw(`SELECT id,topic_id,COALESCE(item_id,'') AS item_id,title,revision,created_at,updated_at FROM t_work_artifact WHERE topic_id=? AND id=?`, topic, id).Scan(&a)
	if r.Error != nil {
		return a, r.Error
	}
	if r.RowsAffected == 0 {
		return a, domain.ErrNotFound
	}
	return a, nil
}
func loadVersion(tx *gorm.DB, id string, revision int) (domain.ArtifactVersion, error) {
	var row versionRow
	r := tx.Raw(`SELECT * FROM t_work_artifact_version WHERE artifact_id=? AND revision=?`, id, revision).Scan(&row)
	if r.Error != nil {
		return domain.ArtifactVersion{}, r.Error
	}
	if r.RowsAffected == 0 {
		return domain.ArtifactVersion{}, domain.ErrNotFound
	}
	return row.value()
}
func insertVersion(tx *gorm.DB, v domain.ArtifactVersion) error {
	body, err := json.Marshal(v.Body)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO t_work_artifact_version(artifact_id,revision,title,body_json,author,summary,conversation_id,restored_from,created_at) VALUES(?,?,?,CAST(? AS jsonb),?,?,?,?,?)`, v.ArtifactID, v.Revision, v.Title, string(body), v.Author, v.Summary, nullable(v.ConversationID), v.RestoredFrom, v.CreatedAt).Error
}
func (s *Store) CreateArtifact(ctx context.Context, user, topic string, in domain.SaveArtifact) (domain.ArtifactDetail, error) {
	in.Title = strings.TrimSpace(in.Title)
	if err := validateArtifact(in); err != nil {
		return domain.ArtifactDetail{}, err
	}
	return mutate(s, ctx, user, topic, "artifact.create", in.Mutation, in, func(tx *gorm.DB) (domain.ArtifactDetail, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.ArtifactDetail{}, err
		}
		if err := checkItem(tx, topic, in.ItemID); err != nil {
			return domain.ArtifactDetail{}, err
		}
		id, err := nextID()
		if err != nil {
			return domain.ArtifactDetail{}, err
		}
		now := time.Now().UTC()
		a := domain.Artifact{ID: id, TopicID: topic, ItemID: in.ItemID, Title: in.Title, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := tx.Exec(`INSERT INTO t_work_artifact(id,topic_id,item_id,title,revision,created_at,updated_at) VALUES(?,?,?,?,1,?,?)`, id, topic, nullable(in.ItemID), in.Title, now, now).Error; err != nil {
			return domain.ArtifactDetail{}, err
		}
		v := domain.ArtifactVersion{ArtifactID: id, Revision: 1, Title: in.Title, Body: in.Body, Author: "user", Summary: in.Summary, CreatedAt: now}
		if err := insertVersion(tx, v); err != nil {
			return domain.ArtifactDetail{}, err
		}
		return domain.ArtifactDetail{Artifact: a, Version: v}, touch(tx, topic)
	})
}
func (s *Store) ListArtifacts(ctx context.Context, user, topic string, page domain.Page) ([]domain.Artifact, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	rows := []domain.Artifact{}
	err := s.db.WithContext(ctx).Raw(`SELECT id,topic_id,COALESCE(item_id,'') AS item_id,title,revision,created_at,updated_at FROM t_work_artifact WHERE topic_id=? ORDER BY updated_at DESC,id DESC LIMIT ? OFFSET ?`, topic, page.Limit, page.Offset).Scan(&rows).Error
	return rows, err
}
func (s *Store) GetArtifact(ctx context.Context, user, topic, id string) (domain.ArtifactDetail, error) {
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return domain.ArtifactDetail{}, err
	}
	a, err := loadArtifact(s.db.WithContext(ctx), topic, id)
	if err != nil {
		return domain.ArtifactDetail{}, err
	}
	v, err := loadVersion(s.db.WithContext(ctx), id, a.Revision)
	return domain.ArtifactDetail{Artifact: a, Version: v}, err
}
func saveVersion(tx *gorm.DB, a domain.Artifact, v domain.ArtifactVersion, item string) (domain.ArtifactDetail, error) {
	if err := insertVersion(tx, v); err != nil {
		return domain.ArtifactDetail{}, err
	}
	a.Revision = v.Revision
	a.Title = v.Title
	a.ItemID = item
	a.UpdatedAt = v.CreatedAt
	if err := tx.Exec(`UPDATE t_work_artifact SET revision=?,title=?,item_id=?,updated_at=? WHERE id=?`, a.Revision, a.Title, nullable(item), a.UpdatedAt, a.ID).Error; err != nil {
		return domain.ArtifactDetail{}, err
	}
	return domain.ArtifactDetail{Artifact: a, Version: v}, touch(tx, a.TopicID)
}
func (s *Store) SaveArtifact(ctx context.Context, user, topic, id string, in domain.SaveArtifact) (domain.ArtifactDetail, error) {
	in.Title = strings.TrimSpace(in.Title)
	if err := validateArtifact(in); err != nil {
		return domain.ArtifactDetail{}, err
	}
	return mutate(s, ctx, user, topic, "artifact.save/"+id, in.Mutation, in, func(tx *gorm.DB) (domain.ArtifactDetail, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.ArtifactDetail{}, err
		}
		if err := checkItem(tx, topic, in.ItemID); err != nil {
			return domain.ArtifactDetail{}, err
		}
		a, err := loadArtifact(tx, topic, id)
		if err != nil {
			return domain.ArtifactDetail{}, err
		}
		if a.Revision != in.ExpectedRevision {
			return domain.ArtifactDetail{}, &domain.Conflict{CurrentRevision: a.Revision}
		}
		return saveVersion(tx, a, domain.ArtifactVersion{ArtifactID: id, Revision: a.Revision + 1, Title: in.Title, Body: in.Body, Author: "user", Summary: in.Summary, CreatedAt: time.Now().UTC()}, in.ItemID)
	})
}
func (s *Store) GetVersion(ctx context.Context, user, topic, id string, revision int) (domain.ArtifactVersion, error) {
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return domain.ArtifactVersion{}, err
	}
	if _, err := loadArtifact(s.db.WithContext(ctx), topic, id); err != nil {
		return domain.ArtifactVersion{}, err
	}
	return loadVersion(s.db.WithContext(ctx), id, revision)
}
func (s *Store) ListVersions(ctx context.Context, user, topic, id string, page domain.Page) ([]domain.ArtifactVersionSummary, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.GetArtifact(ctx, user, topic, id); err != nil {
		return nil, err
	}
	out := []domain.ArtifactVersionSummary{}
	if err := s.db.WithContext(ctx).Raw(`SELECT artifact_id,revision,title,author,summary,COALESCE(conversation_id,'') AS conversation_id,restored_from,created_at FROM t_work_artifact_version WHERE artifact_id=? ORDER BY revision DESC LIMIT ? OFFSET ?`, id, page.Limit, page.Offset).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Store) RestoreArtifact(ctx context.Context, user, topic, id string, in domain.RestoreArtifact) (domain.ArtifactDetail, error) {
	return mutate(s, ctx, user, topic, "artifact.restore/"+id, in.Mutation, in, func(tx *gorm.DB) (domain.ArtifactDetail, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.ArtifactDetail{}, err
		}
		a, err := loadArtifact(tx, topic, id)
		if err != nil {
			return domain.ArtifactDetail{}, err
		}
		if a.Revision != in.ExpectedRevision {
			return domain.ArtifactDetail{}, &domain.Conflict{CurrentRevision: a.Revision}
		}
		v, err := loadVersion(tx, id, in.Revision)
		if err != nil {
			return domain.ArtifactDetail{}, err
		}
		v.RestoredFrom = in.Revision
		v.Revision = a.Revision + 1
		v.Author = "user"
		v.ConversationID = ""
		v.CreatedAt = time.Now().UTC()
		v.Summary = fmt.Sprintf("恢复版本 v%d", in.Revision)
		return saveVersion(tx, a, v, a.ItemID)
	})
}
