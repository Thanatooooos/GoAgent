package work

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	"local/rag-project/internal/app/runtime/persistence"
	"local/rag-project/internal/app/work/domain"
)

type turnRow struct {
	ID, TopicID, UserID, ConversationID, UserMessageID, Question, Action, Status, Error string
	ItemID, ArtifactID, AssistantMessageID                                              *string
	ArtifactRevision, StateRevision                                                     int
	ContextJSON                                                                         []byte
	DeadlineAt, CreatedAt                                                               time.Time
}

func (r turnRow) value() (domain.Turn, error) {
	v := domain.Turn{ID: r.ID, TopicID: r.TopicID, UserID: r.UserID, ConversationID: r.ConversationID, UserMessageID: r.UserMessageID, Question: r.Question, Action: r.Action, Status: r.Status, Error: r.Error, ArtifactRevision: r.ArtifactRevision, StateRevision: r.StateRevision, DeadlineAt: r.DeadlineAt, CreatedAt: r.CreatedAt}
	if r.ItemID != nil {
		v.ItemID = *r.ItemID
	}
	if r.ArtifactID != nil {
		v.ArtifactID = *r.ArtifactID
	}
	if r.AssistantMessageID != nil {
		v.AssistantMessageID = *r.AssistantMessageID
	}
	err := json.Unmarshal(r.ContextJSON, &v.Snapshot)
	return v, err
}
func (s *Store) GetTurn(ctx context.Context, user, topic, id string) (domain.Turn, error) {
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return domain.Turn{}, err
	}
	// Expired unfinished turns are read as interrupted, never silently rerun.
	if _, err := postgresruntime.NewStore(s.db).RecoverChatExecution(ctx, user, id); err != nil {
		return domain.Turn{}, err
	}
	if err := s.db.WithContext(ctx).Exec(`UPDATE t_work_turn SET status='interrupted',error='执行已中断，可查看已保存结果后重新发起',updated_at=CURRENT_TIMESTAMP WHERE id=? AND topic_id=? AND user_id=? AND status IN ('accepted','running') AND deadline_at<CURRENT_TIMESTAMP AND NOT EXISTS(SELECT 1 FROM t_runtime_chat_execution e WHERE e.task_id=t_work_turn.id) AND NOT EXISTS(SELECT 1 FROM t_runtime_session r JOIN t_runtime_chat_publication p ON p.runtime_session_id=r.id WHERE r.conversation_id=t_work_turn.conversation_id AND r.user_message_id=t_work_turn.user_message_id)`, id, topic, user).Error; err != nil {
		return domain.Turn{}, err
	}
	var row turnRow
	r := s.db.WithContext(ctx).Raw(`SELECT * FROM t_work_turn WHERE id=? AND topic_id=? AND user_id=?`, id, topic, user).Scan(&row)
	if r.Error != nil {
		return domain.Turn{}, r.Error
	}
	if r.RowsAffected == 0 {
		return domain.Turn{}, domain.ErrNotFound
	}
	turn, err := row.value()
	if err != nil {
		return turn, err
	}
	if err := checkConversation(s.db.WithContext(ctx), user, topic, turn.ConversationID); err != nil {
		return domain.Turn{}, err
	}
	turn.Outputs, err = s.turnOutputs(ctx, user, topic, id)
	return turn, err
}
func (s *Store) turnOutputs(ctx context.Context, user, topic, id string) ([]domain.TurnOutput, error) {
	out := []domain.TurnOutput{}
	err := s.db.WithContext(ctx).Raw(`SELECT o.kind,COALESCE(o.result_json#>>'{artifact,id}',o.result_json#>>'{document,artifactId}','') AS artifact_id,COALESCE((o.result_json#>>'{version,revision}')::integer,0) AS revision,COALESCE(o.result_json#>>'{artifact,title}',o.result_json#>>'{document,title}','') AS title,COALESCE(o.result_json#>>'{version,summary}',o.result_json#>>'{document,summary}','') AS summary,CASE WHEN o.kind IN ('progress','document_suggestion') THEN COALESCE(o.result_json->>'id','') ELSE '' END AS proposal_id FROM t_work_turn_operation o JOIN t_work_turn t ON t.id=o.turn_id WHERE t.id=? AND t.topic_id=? AND t.user_id=? AND (o.kind NOT IN ('progress','document_suggestion') OR o.result_json->>'turnId'=t.id) ORDER BY o.created_at`, id, topic, user).Scan(&out).Error
	return out, err
}
func (s *Store) ListTurns(ctx context.Context, user, topic, conversation string, p domain.Page) ([]domain.Turn, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	if err := checkConversation(s.db.WithContext(ctx), user, topic, conversation); err != nil {
		return nil, err
	}
	if conversation == "" {
		return []domain.Turn{}, nil
	}
	rows := []turnRow{}
	if err := s.db.WithContext(ctx).Raw(`SELECT * FROM t_work_turn WHERE topic_id=? AND user_id=? AND conversation_id=? ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, topic, user, conversation, p.Limit, p.Offset).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := []domain.Turn{}
	for _, row := range rows {
		v, err := s.GetTurn(ctx, user, topic, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) AcceptTurn(ctx context.Context, user, topic string, in domain.ChatRequest) (domain.Turn, error) {
	if user == "" {
		return domain.Turn{}, domain.ErrNotFound
	}
	in.Question = strings.TrimSpace(in.Question)
	if in.Question == "" || len(in.Question) > 60000 {
		return domain.Turn{}, fmt.Errorf("question must contain 1..60000 bytes")
	}
	if in.Action == "" {
		in.Action = "discuss"
	}
	switch in.Action {
	case "discuss", "create_document", "edit_document", "rewrite_document":
	default:
		return domain.Turn{}, fmt.Errorf("unsupported Work action")
	}
	if err := (domain.Mutation{RequestID: in.RequestID}).Validate(); err != nil {
		return domain.Turn{}, err
	}
	body, _ := json.Marshal(in)
	digest := sha256.Sum256(body)
	hash := hex.EncodeToString(digest[:])
	var out domain.Turn
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, user+"/turn/"+in.RequestID).Error; err != nil {
			return err
		}
		t, err := activeTopic(tx, user, topic)
		if err != nil {
			return err
		}
		var existing struct {
			ID        string
			InputHash string
			TopicID   string
		}
		r := tx.Raw(`SELECT id,input_hash,topic_id FROM t_work_turn WHERE user_id=? AND client_request_id=?`, user, in.RequestID).Scan(&existing)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			if existing.InputHash != hash || existing.TopicID != topic {
				return domain.ErrRequestReused
			}
			var row turnRow
			if err := tx.Raw(`SELECT * FROM t_work_turn WHERE id=?`, existing.ID).Scan(&row).Error; err != nil {
				return err
			}
			out, err = row.value()
			return err
		}
		if err := checkItem(tx, topic, in.ItemID); err != nil {
			return err
		}
		if in.ArtifactID != "" {
			a, err := loadArtifact(tx, topic, in.ArtifactID)
			if err != nil {
				return err
			}
			if in.Action == "edit_document" || in.Action == "rewrite_document" {
				if a.Revision != in.ArtifactRevision {
					return &domain.Conflict{CurrentRevision: a.Revision}
				}
			}
			in.ArtifactRevision = a.Revision
		} else if in.Action == "edit_document" || in.Action == "rewrite_document" {
			return fmt.Errorf("a focused document and its revision are required")
		}
		if err := checkConversation(tx, user, topic, in.ContinueFrom); err != nil {
			return err
		}
		// Legacy chat tables use timestamp without time zone, matching the shared
		// message repository's local wall clock. Work tables use timestamptz.
		now := time.Now()
		conversation := in.ConversationID
		if conversation == "" {
			conversation, err = nextID()
			if err != nil {
				return err
			}
			title := []rune(in.Question)
			if len(title) > 30 {
				title = title[:30]
			}
			if err := tx.Exec(`INSERT INTO t_conversation(id,conversation_id,user_id,title,last_time,create_time,update_time,deleted) VALUES(?,?,?,?,?,?,?,0)`, conversation, conversation, user, string(title), now, now, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO t_work_conversation(conversation_id,topic_id,user_id,item_id,continue_from,created_at) VALUES(?,?,?,?,?,?)`, conversation, topic, user, nullable(in.ItemID), nullable(in.ContinueFrom), now).Error; err != nil {
				return err
			}
		} else {
			if err := checkConversation(tx, user, topic, conversation); err != nil {
				return err
			}
		}
		var pending int64
		if len(in.SourceIDs) > 20 {
			return fmt.Errorf("too many attachments")
		}
		for _, sourceID := range in.SourceIDs {
			r := tx.Exec(`UPDATE t_work_source SET conversation_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND topic_id=? AND user_id=? AND promoted=false AND (conversation_id IS NULL OR conversation_id=?) AND status='ready'`, conversation, sourceID, topic, user, conversation)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return domain.ErrNotFound
			}
		}
		if err := tx.Raw(`SELECT COUNT(*) FROM t_work_turn WHERE conversation_id=? AND user_id=? AND status IN ('accepted','running') AND deadline_at>CURRENT_TIMESTAMP`, conversation, user).Scan(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return fmt.Errorf("this conversation already has an active turn")
		}
		state, err := loadState(tx, topic)
		if err != nil {
			return err
		}
		selected := []domain.StateEntry{}
		omitted := 0
		size := len(t.Description)
		for _, e := range state.Entries {
			if e.ItemID != "" && e.ItemID != in.ItemID {
				continue
			}
			size += len(e.Text)
			if size > 24000 {
				omitted++
				continue
			}
			selected = append(selected, e)
		}
		state.Entries = selected
		artifacts := []domain.Artifact{}
		if err := tx.Raw(`SELECT id,topic_id,COALESCE(item_id,'') AS item_id,title,revision,created_at,updated_at FROM t_work_artifact WHERE topic_id=? AND (item_id IS NULL OR item_id=? OR id=?) ORDER BY updated_at DESC LIMIT 30`, topic, in.ItemID, in.ArtifactID).Scan(&artifacts).Error; err != nil {
			return err
		}
		id, err := nextID()
		if err != nil {
			return err
		}
		message, err := nextID()
		if err != nil {
			return err
		}
		out = domain.Turn{ID: id, TopicID: topic, UserID: user, ConversationID: conversation, ItemID: in.ItemID, ArtifactID: in.ArtifactID, ArtifactRevision: in.ArtifactRevision, StateRevision: state.Revision, UserMessageID: message, Question: in.Question, Action: in.Action, Snapshot: domain.TurnSnapshot{TopicName: t.Name, Description: t.Description, State: state, Artifacts: artifacts, OmittedEntries: omitted}, Status: "accepted", DeadlineAt: now.Add(10 * time.Minute), CreatedAt: now}
		snapshot, _ := json.Marshal(out.Snapshot)
		if err := tx.Exec(`INSERT INTO t_message(id,conversation_id,user_id,role,content,create_time,update_time,deleted) VALUES(?,?,?,'user',?,?,?,0)`, message, conversation, user, in.Question, now, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_work_turn(id,topic_id,user_id,client_request_id,conversation_id,item_id,artifact_id,artifact_revision,state_revision,user_message_id,question,action,input_hash,context_json,deadline_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,CAST(? AS jsonb),?,?)`, id, topic, user, in.RequestID, conversation, nullable(in.ItemID), nullable(in.ArtifactID), in.ArtifactRevision, state.Revision, message, in.Question, in.Action, hash, string(snapshot), out.DeadlineAt, now).Error; err != nil {
			return err
		}
		if err := postgresruntime.ReserveChatExecution(ctx, tx, persistence.Session{TraceID: id, ConversationID: conversation, UserMessageID: message, UserID: user}); err != nil {
			return err
		}
		return touch(tx, topic)
	})
	return out, err
}
func (s *Store) StartTurn(ctx context.Context, user, topic, id string) (bool, error) {
	if _, err := postgresruntime.NewStore(s.db).RecoverChatExecution(ctx, user, id); err != nil {
		return false, err
	}
	started := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return err
		}
		r := tx.Exec(`UPDATE t_work_turn SET status='running',updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=? AND topic_id=? AND status='accepted' AND deadline_at>clock_timestamp() AND (NOT EXISTS(SELECT 1 FROM t_runtime_chat_execution e WHERE e.task_id=t_work_turn.id) OR EXISTS(SELECT 1 FROM t_runtime_chat_execution e WHERE e.task_id=t_work_turn.id AND e.state='pending' AND e.lease_until>clock_timestamp()))`, id, user, topic)
		started = r.RowsAffected == 1
		return r.Error
	})
	return started, err
}
func (s *Store) FinishTurn(ctx context.Context, user, topic, id, status, message string) error {
	if status != "completed" && status != "failed" && status != "cancelled" {
		return fmt.Errorf("invalid terminal turn status")
	}
	return s.db.WithContext(ctx).Exec(`UPDATE t_work_turn SET status=?,assistant_message_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=? AND topic_id=? AND status='running'`, status, nullable(message), id, user, topic).Error
}
func (s *Store) CancelTurn(ctx context.Context, user, topic, id string) error {
	if _, err := s.GetTurn(ctx, user, topic, id); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := getTopic(tx, user, topic, true); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE t_work_turn SET status='cancelled',updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=? AND topic_id=? AND status IN ('accepted','running')`, id, user, topic).Error; err != nil {
			return err
		}
		return postgresruntime.NewStore(tx).CancelChatExecution(ctx, user, id)
	})
}
func (s *Store) ListMessages(ctx context.Context, user, topic, conversation string, page domain.Page) ([]domain.Message, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	if conversation == "" {
		return nil, domain.ErrNotFound
	}
	if err := checkConversation(s.db.WithContext(ctx), user, topic, conversation); err != nil {
		return nil, err
	}
	messages := []domain.Message{}
	err := s.db.WithContext(ctx).Raw(`SELECT id,conversation_id,role,CASE WHEN COALESCE(BTRIM(raw_content),'') <> '' THEN raw_content ELSE content END AS content,create_time AS created_at FROM t_message WHERE conversation_id=? AND user_id=? AND deleted=0 ORDER BY id DESC LIMIT ? OFFSET ?`, conversation, user, page.Limit, page.Offset).Scan(&messages).Error
	return messages, err
}
