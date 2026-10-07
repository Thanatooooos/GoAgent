package runtimeadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	"local/rag-project/internal/app/runtime/capability"
	scheduledtaskdomain "local/rag-project/internal/app/scheduledtask/domain"
)

// ScheduledTasks adapts the scheduled-task store to the capability port. It is
// the only place that knows both the chat-facing config shape and the domain
// types; the port stays free of persistence details.
type ScheduledTasks struct{ store *storepkg.Store }

func NewScheduledTasks(store *storepkg.Store) *ScheduledTasks { return &ScheduledTasks{store: store} }

// draftScanLimit bounds the per-task version reads a single list call performs.
// The tool only ever reports the most recent 100 tasks.
const draftScanLimit = 100

func (a *ScheduledTasks) CreateDraft(ctx context.Context, userID, originConversationID string, config capability.ScheduledTaskConfig, now time.Time) (capability.ScheduledTaskDraft, error) {
	if a == nil || a.store == nil {
		return capability.ScheduledTaskDraft{}, fmt.Errorf("scheduled task store is not configured")
	}
	proposed, err := proposedConfig(config)
	if err != nil {
		return capability.ScheduledTaskDraft{}, err
	}
	draft, err := a.store.CreateDraft(ctx, userID, originConversationID, "", 0, proposed, now)
	if err != nil {
		return capability.ScheduledTaskDraft{}, err
	}
	return scheduledTaskDraft(draft)
}

func (a *ScheduledTasks) ListTasks(ctx context.Context, userID string) ([]capability.ScheduledTaskInfo, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("scheduled task store is not configured")
	}
	tasks, err := a.store.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(tasks) > draftScanLimit {
		tasks = tasks[:draftScanLimit]
	}
	infos := make([]capability.ScheduledTaskInfo, 0, len(tasks))
	for _, task := range tasks {
		_, version, err := a.store.Get(ctx, userID, task.ID)
		if err != nil {
			return nil, err
		}
		infos = append(infos, scheduledTaskInfo(task, version))
	}
	return infos, nil
}

func (a *ScheduledTasks) PauseTask(ctx context.Context, userID, taskID string) (capability.ScheduledTaskInfo, error) {
	if a == nil || a.store == nil {
		return capability.ScheduledTaskInfo{}, fmt.Errorf("scheduled task store is not configured")
	}
	task, version, err := a.store.Get(ctx, userID, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return capability.ScheduledTaskInfo{}, fmt.Errorf("no scheduled task with id %q belongs to this user", taskID)
		}
		return capability.ScheduledTaskInfo{}, err
	}
	switch task.Status {
	case scheduledtaskdomain.TaskPaused:
		return scheduledTaskInfo(task, version), nil
	case scheduledtaskdomain.TaskActive:
	case scheduledtaskdomain.TaskCompleted:
		return capability.ScheduledTaskInfo{}, fmt.Errorf("task is already completed and cannot be paused; resuming it requires the user to confirm a schedule again")
	default:
		return capability.ScheduledTaskInfo{}, fmt.Errorf("task status %q cannot be paused", task.Status)
	}
	if err := a.store.SetStatus(ctx, userID, taskID, scheduledtaskdomain.TaskPaused, time.Time{}); err != nil {
		return capability.ScheduledTaskInfo{}, err
	}
	task.Status = scheduledtaskdomain.TaskPaused
	task.NextDueAt = time.Time{}
	return scheduledTaskInfo(task, version), nil
}

func proposedConfig(config capability.ScheduledTaskConfig) (storepkg.ProposedConfig, error) {
	schedule, err := domainSchedule(config.Schedule)
	if err != nil {
		return storepkg.ProposedConfig{}, err
	}
	return storepkg.ProposedConfig{Name: config.Name, Prompt: config.Prompt, Schedule: schedule,
		ReportMode: scheduledtaskdomain.ReportMode(config.ReportMode), ConditionKind: scheduledtaskdomain.ConditionKind(config.ConditionKind),
		KnowledgeBaseIDs: config.KnowledgeBaseIDs, AllowedWebDomains: config.AllowedWebDomains, AllowedToolIDs: config.AllowedToolIDs}, nil
}

func scheduledTaskDraft(draft storepkg.Draft) (capability.ScheduledTaskDraft, error) {
	config, err := scheduledTaskConfig(draft.Config)
	if err != nil {
		return capability.ScheduledTaskDraft{}, err
	}
	return capability.ScheduledTaskDraft{ID: draft.ID, TaskID: draft.TaskID, DuplicateTaskID: draft.DuplicateTaskID,
		BaseVersion: draft.BaseVersion, Config: config, ExpiresAt: draft.ExpiresAt}, nil
}

func scheduledTaskConfig(config storepkg.ProposedConfig) (capability.ScheduledTaskConfig, error) {
	schedule, err := capabilitySchedule(config.Schedule)
	if err != nil {
		return capability.ScheduledTaskConfig{}, err
	}
	return capability.ScheduledTaskConfig{Name: config.Name, Prompt: config.Prompt, Schedule: schedule,
		ReportMode: string(config.ReportMode), ConditionKind: string(config.ConditionKind),
		KnowledgeBaseIDs: config.KnowledgeBaseIDs, AllowedWebDomains: config.AllowedWebDomains, AllowedToolIDs: config.AllowedToolIDs}, nil
}

func scheduledTaskInfo(task scheduledtaskdomain.Task, version scheduledtaskdomain.Version) capability.ScheduledTaskInfo {
	schedule, err := capabilitySchedule(version.Schedule)
	if err != nil {
		schedule = capability.ScheduledTaskSchedule{Kind: string(version.Schedule.Kind), Timezone: version.Schedule.Timezone}
	}
	return capability.ScheduledTaskInfo{ID: task.ID, Status: string(task.Status), Name: version.DisplayName(), Prompt: version.Prompt,
		Schedule: schedule, ScheduleDescription: capability.DescribeScheduledTaskSchedule(schedule),
		ReportMode: string(version.ReportMode), ConditionKind: string(version.ConditionKind),
		NextDueAt: task.NextDueAt, LastReportedAt: task.LastReportedAt}
}

// domainSchedule and capabilitySchedule convert through JSON so the two
// structurally identical schedule shapes cannot drift field by field.
func domainSchedule(schedule capability.ScheduledTaskSchedule) (scheduledtaskdomain.Schedule, error) {
	raw, err := json.Marshal(schedule)
	if err != nil {
		return scheduledtaskdomain.Schedule{}, err
	}
	var result scheduledtaskdomain.Schedule
	if err := json.Unmarshal(raw, &result); err != nil {
		return scheduledtaskdomain.Schedule{}, err
	}
	return result, nil
}

func capabilitySchedule(schedule scheduledtaskdomain.Schedule) (capability.ScheduledTaskSchedule, error) {
	raw, err := json.Marshal(schedule)
	if err != nil {
		return capability.ScheduledTaskSchedule{}, err
	}
	var result capability.ScheduledTaskSchedule
	if err := json.Unmarshal(raw, &result); err != nil {
		return capability.ScheduledTaskSchedule{}, err
	}
	return result, nil
}
