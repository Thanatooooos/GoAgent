package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	briefdomain "local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/scheduledtask/domain"
)

type ProposedConfig struct {
	DailyBrief        *briefdomain.TaskContract `json:"dailyBrief,omitempty"`
	Name              string                    `json:"name"`
	Prompt            string                    `json:"prompt"`
	Schedule          domain.Schedule           `json:"schedule"`
	ReportMode        domain.ReportMode         `json:"reportMode"`
	ConditionKind     domain.ConditionKind      `json:"conditionKind"`
	KnowledgeBaseIDs  []string                  `json:"knowledgeBaseIds"`
	AllowedWebDomains []string                  `json:"allowedWebDomains"`
	AllowedToolIDs    []string                  `json:"allowedToolIds"`
}

type Draft struct {
	ID              string         `json:"id"`
	TaskID          string         `json:"taskId,omitempty"`
	DuplicateTaskID string         `json:"duplicateTaskId,omitempty"`
	BaseVersion     int            `json:"baseVersion,omitempty"`
	Config          ProposedConfig `json:"config"`
	ExpiresAt       time.Time      `json:"expiresAt"`
}

func (c ProposedConfig) version(taskID string, number int, confirmedAt time.Time) domain.Version {
	return domain.Version{DailyBrief: c.DailyBrief, TaskID: taskID, Number: number, Name: c.Name, Prompt: c.Prompt, Schedule: c.Schedule,
		ReportMode: c.ReportMode, ConditionKind: c.ConditionKind, KnowledgeBaseIDs: c.KnowledgeBaseIDs,
		AllowedWebDomains: c.AllowedWebDomains, AllowedToolIDs: c.AllowedToolIDs, ConfirmedAt: confirmedAt}
}

// CreateDraft saves a preview. Only ConfirmDraft can change an effective task.
func (s *Store) CreateDraft(ctx context.Context, userID, originConversationID, taskID string, baseVersion int,
	config ProposedConfig, now time.Time) (Draft, error) {
	if s == nil || s.db == nil || userID == "" || now.IsZero() {
		return Draft{}, fmt.Errorf("invalid task draft request")
	}
	if taskID == "" && baseVersion != 0 || taskID != "" && baseVersion < 1 {
		return Draft{}, fmt.Errorf("invalid draft base version")
	}
	if taskID == "" && config.DailyBrief != nil {
		return Draft{}, fmt.Errorf("DailyBrief requires an explicit subscription handoff")
	}
	if taskID != "" {
		_, current, err := s.Get(ctx, userID, taskID)
		if err != nil {
			return Draft{}, err
		}
		config.DailyBrief = current.DailyBrief // The caller cannot remove/replace the page contract.
	}
	if err := config.version("preview", 1, now).Validate(); err != nil {
		return Draft{}, err
	}
	if err := validateKnowledgeBaseScope(s.db.WithContext(ctx), config.KnowledgeBaseIDs); err != nil {
		return Draft{}, err
	}
	next, err := config.Schedule.Next(now)
	if err != nil || next.IsZero() {
		return Draft{}, fmt.Errorf("task has no future occurrence")
	}
	if taskID != "" {
		task, _, err := s.Get(ctx, userID, taskID)
		if err != nil {
			return Draft{}, err
		}
		if task.CurrentVersion != baseVersion {
			return Draft{}, fmt.Errorf("task version conflict")
		}
	}
	if originConversationID != "" {
		var owned int
		if err := s.db.WithContext(ctx).Raw(`SELECT 1 FROM t_conversation WHERE conversation_id = ? AND user_id = ? AND deleted = 0`,
			originConversationID, userID).Scan(&owned).Error; err != nil {
			return Draft{}, err
		}
		if owned != 1 {
			return Draft{}, gorm.ErrRecordNotFound
		}
	}
	id, err := nextID()
	if err != nil {
		return Draft{}, err
	}
	draft := Draft{ID: id, TaskID: taskID, BaseVersion: baseVersion, Config: config, ExpiresAt: now.Add(24 * time.Hour)}
	draft.DuplicateTaskID, err = findDuplicate(s.db.WithContext(ctx), userID, taskID, config)
	if err != nil {
		return Draft{}, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return Draft{}, err
	}
	var taskValue any
	var baseValue any
	if taskID != "" {
		taskValue, baseValue = taskID, baseVersion
	}
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO t_scheduled_task_draft
		(id, user_id, originating_conversation_id, task_id, base_version, proposed_config_json, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, userID, nullable(originConversationID), taskValue, baseValue,
		string(raw), draft.ExpiresAt).Error; err != nil {
		return Draft{}, err
	}
	return draft, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// ListPendingDrafts reports the previews this conversation is still waiting on,
// so a chat page that lost its tool events can offer them again. Only the
// owner's unexpired pending drafts of that conversation are ever returned.
func (s *Store) ListPendingDrafts(ctx context.Context, userID, conversationID string, now time.Time) ([]Draft, error) {
	if s == nil || s.db == nil || userID == "" || conversationID == "" || now.IsZero() {
		return nil, fmt.Errorf("invalid draft query")
	}
	var owned int
	if err := s.db.WithContext(ctx).Raw(`SELECT 1 FROM t_conversation WHERE conversation_id = ? AND user_id = ? AND deleted = 0`,
		conversationID, userID).Scan(&owned).Error; err != nil {
		return nil, err
	}
	if owned != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	var rows []struct {
		ID                 string
		TaskID             *string
		BaseVersion        *int
		ProposedConfigJSON []byte
		ExpiresAt          time.Time
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT id, task_id, base_version, proposed_config_json, expires_at
		FROM t_scheduled_task_draft WHERE user_id = ? AND originating_conversation_id = ?
		AND status = 'pending' AND expires_at > ? ORDER BY create_time, id`,
		userID, conversationID, now).Scan(&rows).Error; err != nil {
		return nil, err
	}
	drafts := make([]Draft, 0, len(rows))
	for _, row := range rows {
		var config ProposedConfig
		if err := json.Unmarshal(row.ProposedConfigJSON, &config); err != nil {
			return nil, err
		}
		draft := Draft{ID: row.ID, Config: config, ExpiresAt: row.ExpiresAt}
		if row.TaskID != nil {
			draft.TaskID = *row.TaskID
		}
		if row.BaseVersion != nil {
			draft.BaseVersion = *row.BaseVersion
		}
		// Duplicates are resolved again because the column is not persisted: a
		// restored card must warn exactly like the preview that produced it,
		// even if a matching task appeared in the meantime.
		duplicate, err := findDuplicate(s.db.WithContext(ctx), userID, draft.TaskID, config)
		if err != nil {
			return nil, err
		}
		draft.DuplicateTaskID = duplicate
		drafts = append(drafts, draft)
	}
	return drafts, nil
}

// ConfirmDraft consumes the exact saved preview and is idempotent for repeats.
func (s *Store) ConfirmDraft(ctx context.Context, userID, draftID string, now time.Time, allowDuplicate bool) (domain.Task, domain.Version, error) {
	if s == nil || s.db == nil || userID == "" || draftID == "" || now.IsZero() {
		return domain.Task{}, domain.Version{}, fmt.Errorf("invalid task confirmation")
	}
	var task domain.Task
	var version domain.Version
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			Status             string
			TaskID             *string
			BaseVersion        *int
			ProposedConfigJSON []byte
			ExpiresAt          time.Time
		}
		loaded := tx.Raw(`SELECT status, task_id, base_version, proposed_config_json, expires_at
			FROM t_scheduled_task_draft WHERE id = ? AND user_id = ? FOR UPDATE`, draftID, userID).Scan(&row)
		if loaded.Error != nil {
			return loaded.Error
		}
		if loaded.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if row.Status == "confirmed" && row.TaskID != nil {
			var existing taskRow
			if err := tx.Raw(`SELECT id, user_id, status, current_version, conversation_id, confirmed_at,
				next_due_at, last_reported_at, last_attempted_at FROM t_scheduled_task WHERE id = ? AND user_id = ?`,
				*row.TaskID, userID).Scan(&existing).Error; err != nil {
				return err
			}
			task = existing.toDomain()
			var err error
			version, err = loadVersion(tx, task.ID, func() int {
				if row.BaseVersion == nil {
					return 1
				}
				return *row.BaseVersion + 1
			}())
			return err
		}
		if row.Status != "pending" || !now.Before(row.ExpiresAt) {
			return fmt.Errorf("task draft expired or already consumed")
		}
		var config ProposedConfig
		if err := json.Unmarshal(row.ProposedConfigJSON, &config); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, "scheduled-task:"+userID).Error; err != nil {
			return err
		}
		excludeID := ""
		if row.TaskID != nil {
			excludeID = *row.TaskID
		}
		duplicateID, err := findDuplicate(tx, userID, excludeID, config)
		if err != nil {
			return err
		}
		if duplicateID != "" && !allowDuplicate {
			return fmt.Errorf("similar task already exists: %s", duplicateID)
		}
		id := ""
		number := 1
		if row.TaskID == nil {
			var err error
			id, err = nextID()
			if err != nil {
				return err
			}
		} else {
			id = *row.TaskID
			if row.BaseVersion == nil {
				return fmt.Errorf("edit draft lacks base version")
			}
			number = *row.BaseVersion + 1
		}
		version = config.version(id, number, now)
		if err := version.Validate(); err != nil {
			return err
		}
		if err := validateKnowledgeBaseScope(tx, version.KnowledgeBaseIDs); err != nil {
			return err
		}
		next, err := version.Schedule.Next(now)
		if err != nil || next.IsZero() {
			return fmt.Errorf("task has no future occurrence")
		}
		if row.TaskID == nil {
			if err := tx.Exec(`INSERT INTO t_scheduled_task (id, user_id, status, current_version, confirmed_at, next_due_at)
				VALUES (?, ?, 'active', 1, ?, ?)`, id, userID, now, next).Error; err != nil {
				return err
			}
			task = domain.Task{ID: id, UserID: userID, Status: domain.TaskActive, CurrentVersion: 1,
				ConfirmedAt: now, NextDueAt: next}
		} else {
			var existing taskRow
			loaded := tx.Raw(`SELECT id, user_id, status, current_version, conversation_id, confirmed_at,
				next_due_at, last_reported_at, last_attempted_at FROM t_scheduled_task
				WHERE id = ? AND user_id = ? AND deleted_at IS NULL FOR UPDATE`, id, userID).Scan(&existing)
			if loaded.Error != nil {
				return loaded.Error
			}
			if loaded.RowsAffected != 1 || existing.CurrentVersion != *row.BaseVersion {
				return fmt.Errorf("task version conflict")
			}
			var due any
			if existing.Status == string(domain.TaskActive) {
				due = next
			}
			if err := tx.Exec(`UPDATE t_scheduled_task SET current_version = ?, confirmed_at = ?, next_due_at = ?, update_time = CURRENT_TIMESTAMP
				WHERE id = ?`, number, now, due, id).Error; err != nil {
				return err
			}
			task = existing.toDomain()
			task.CurrentVersion = number
			task.ConfirmedAt = now
			if due != nil {
				task.NextDueAt = next
			} else {
				task.NextDueAt = time.Time{}
			}
		}
		if err := insertVersion(tx, version); err != nil {
			return err
		}
		return tx.Exec(`UPDATE t_scheduled_task_draft SET status = 'confirmed', task_id = ?,
			update_time = CURRENT_TIMESTAMP WHERE id = ?`, id, draftID).Error
	})
	if err != nil {
		return domain.Task{}, domain.Version{}, err
	}
	return task, version, nil
}

func findDuplicate(db *gorm.DB, userID, excludeID string, config ProposedConfig) (string, error) {
	schedule, err := json.Marshal(config.Schedule)
	if err != nil {
		return "", err
	}
	kb, err := json.Marshal(config.KnowledgeBaseIDs)
	if err != nil {
		return "", err
	}
	domains, err := json.Marshal(config.AllowedWebDomains)
	if err != nil {
		return "", err
	}
	tools, err := json.Marshal(config.AllowedToolIDs)
	if err != nil {
		return "", err
	}
	var id string
	err = db.Raw(`SELECT t.id FROM t_scheduled_task t JOIN t_scheduled_task_version v
		ON v.task_id = t.id AND v.version = t.current_version
		WHERE t.user_id = ? AND t.deleted_at IS NULL AND t.id <> ?
		AND v.prompt = ? AND v.schedule_json = ?::jsonb AND v.report_mode = ? AND v.condition_kind = ?
		AND v.knowledge_base_ids_json = ?::jsonb AND v.allowed_web_domains_json = ?::jsonb
		AND v.allowed_tool_ids_json = ?::jsonb LIMIT 1`,
		userID, excludeID, config.Prompt, string(schedule), config.ReportMode, config.ConditionKind,
		string(kb), string(domains), string(tools)).Scan(&id).Error
	return id, err
}

func validateKnowledgeBaseScope(db *gorm.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			return fmt.Errorf("invalid knowledge base selection")
		}
		seen[id] = true
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM t_knowledge_base WHERE id IN ? AND deleted = 0`, ids).Scan(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("knowledge base selection is no longer available")
	}
	return nil
}
