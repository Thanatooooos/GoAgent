package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	CreateScheduledTaskID = "create_scheduled_task"
	ListScheduledTasksID  = "list_scheduled_tasks"
	PauseScheduledTaskID  = "pause_scheduled_task"
)

// ScheduledTaskService is runtime's port to the scheduled-task store. Every
// method is scoped by the caller's user id; schedule scope, timezone, and
// knowledge-base scope are server-owned and never come from model arguments.
type ScheduledTaskService interface {
	// CreateDraft saves a 24-hour preview. It never activates a task.
	CreateDraft(ctx context.Context, userID, originConversationID string, config ScheduledTaskConfig, now time.Time) (ScheduledTaskDraft, error)
	// ListTasks returns the user's tasks, newest first.
	ListTasks(ctx context.Context, userID string) ([]ScheduledTaskInfo, error)
	// PauseTask stops an active task and returns its new state. It must not
	// resume, complete, or otherwise change the task's approved version.
	PauseTask(ctx context.Context, userID, taskID string) (ScheduledTaskInfo, error)
}

// ScheduledTaskConfig mirrors the management-page payload so a draft created
// from chat is confirmed through the existing confirmation endpoint.
type ScheduledTaskConfig struct {
	Name              string                `json:"name,omitempty"`
	Prompt            string                `json:"prompt"`
	Schedule          ScheduledTaskSchedule `json:"schedule"`
	ReportMode        string                `json:"reportMode"`
	ConditionKind     string                `json:"conditionKind"`
	KnowledgeBaseIDs  []string              `json:"knowledgeBaseIds"`
	AllowedWebDomains []string              `json:"allowedWebDomains"`
	AllowedToolIDs    []string              `json:"allowedToolIds"`
}

type ScheduledTaskSchedule struct {
	Kind         string    `json:"kind"`
	Timezone     string    `json:"timezone"`
	At           time.Time `json:"at,omitempty"`
	EverySeconds int64     `json:"everySeconds,omitempty"`
	LocalTime    string    `json:"localTime,omitempty"`
	Weekday      int       `json:"weekday,omitempty"`
	MonthDay     int       `json:"monthDay,omitempty"`
}

type ScheduledTaskDraft struct {
	ID              string              `json:"id"`
	TaskID          string              `json:"taskId,omitempty"`
	DuplicateTaskID string              `json:"duplicateTaskId,omitempty"`
	BaseVersion     int                 `json:"baseVersion,omitempty"`
	Config          ScheduledTaskConfig `json:"config"`
	ExpiresAt       time.Time           `json:"expiresAt"`
}

// ScheduledTaskInfo is the model- and UI-facing projection of one saved task.
type ScheduledTaskInfo struct {
	ID                  string                `json:"id"`
	Status              string                `json:"status"`
	Name                string                `json:"name"`
	Prompt              string                `json:"prompt,omitempty"`
	Schedule            ScheduledTaskSchedule `json:"schedule"`
	ScheduleDescription string                `json:"scheduleDescription"`
	ReportMode          string                `json:"reportMode"`
	ConditionKind       string                `json:"conditionKind"`
	NextDueAt           time.Time             `json:"nextDueAt,omitempty"`
	LastReportedAt      time.Time             `json:"lastReportedAt,omitempty"`
}

const (
	scheduledTaskOnce     = "once"
	scheduledTaskInterval = "interval"
	scheduledTaskDaily    = "daily"
	scheduledTaskWeekly   = "weekly"
	scheduledTaskMonthly  = "monthly"

	scheduledTaskReportAlways      = "always"
	scheduledTaskReportOnCondition = "on_condition"

	scheduledTaskListLimit = 100
)

// CreateScheduledTask drafts a task the user must still confirm. Activation is
// always a separate, explicit user action in the interface.
func CreateScheduledTask(service ScheduledTaskService) Def {
	return Def{
		ID:          CreateScheduledTaskID,
		Description: createScheduledTaskDescription,
		JSONSchema:  json.RawMessage(createScheduledTaskSchema),
		Validate: func(v Value) error {
			_, err := parseScheduledTaskArgs(v, time.UTC)
			return err
		},
		Describe: func(v Value, c Context) (Operation, error) {
			location, err := scheduledTaskLocation(c)
			if err != nil {
				return Operation{}, err
			}
			config, err := parseScheduledTaskArgs(v, location)
			if err != nil {
				return Operation{}, err
			}
			return Operation{ID: CreateScheduledTaskID, Summary: DescribeScheduledTaskSchedule(config.Schedule), Input: v}, nil
		},
		Execute: func(v Value, c Context) (Result, error) {
			location, err := scheduledTaskLocation(c)
			if err != nil {
				return Result{}, err
			}
			config, err := parseScheduledTaskArgs(v, location)
			if err != nil {
				return Result{}, err
			}
			draft, err := service.CreateDraft(c.Context, c.UserID, c.ConversationID, config, time.Now())
			if err != nil {
				return Result{}, err
			}
			description := DescribeScheduledTaskSchedule(draft.Config.Schedule)
			payload, err := json.Marshal(struct {
				Status              string             `json:"status"`
				Draft               ScheduledTaskDraft `json:"draft"`
				ScheduleDescription string             `json:"scheduleDescription"`
			}{"draft", draft, description})
			if err != nil {
				return Result{}, err
			}
			content := fmt.Sprintf("已生成定时任务草稿（草稿 ID: %s），计划：%s。任务尚未生效，需要用户在界面上确认后才会开始运行。", draft.ID, description)
			if draft.DuplicateTaskID != "" {
				content += fmt.Sprintf("注意：已存在一个配置相同的任务（任务 ID: %s），确认时用户需要选择是否重复创建。", draft.DuplicateTaskID)
			}
			return Result{Content: content, Value: payload}, nil
		},
	}
}

// ListScheduledTasks is read-only. It exists so pause can address a task by an
// identifier the server actually returned instead of one the model invented.
func ListScheduledTasks(service ScheduledTaskService) Def {
	return Def{
		ID:          ListScheduledTasksID,
		Description: listScheduledTasksDescription,
		JSONSchema:  json.RawMessage(listScheduledTasksSchema),
		Validate:    func(Value) error { return nil },
		Describe: func(_ Value, c Context) (Operation, error) {
			if _, err := scheduledTaskLocation(c); err != nil {
				return Operation{}, err
			}
			return Operation{ID: ListScheduledTasksID}, nil
		},
		Execute: func(_ Value, c Context) (Result, error) {
			if _, err := scheduledTaskLocation(c); err != nil {
				return Result{}, err
			}
			tasks, err := service.ListTasks(c.Context, c.UserID)
			if err != nil {
				return Result{}, err
			}
			truncated := len(tasks) > scheduledTaskListLimit
			if truncated {
				tasks = tasks[:scheduledTaskListLimit]
			}
			payload, err := json.Marshal(struct {
				Tasks     []ScheduledTaskInfo `json:"tasks"`
				Truncated bool                `json:"truncated,omitempty"`
			}{tasks, truncated})
			if err != nil {
				return Result{}, err
			}
			if len(tasks) == 0 {
				return Result{Content: "当前没有定时任务。", Value: payload}, nil
			}
			var builder strings.Builder
			fmt.Fprintf(&builder, "共 %d 个定时任务：", len(tasks))
			for _, task := range tasks {
				fmt.Fprintf(&builder, "\n- %s（ID: %s）｜计划：%s｜状态：%s", task.Name, task.ID, task.ScheduleDescription, scheduledTaskStatusLabel(task.Status))
				if !task.NextDueAt.IsZero() {
					fmt.Fprintf(&builder, "｜下次执行：%s", task.NextDueAt.In(scheduledTaskLocationOrUTC(task.Schedule.Timezone)).Format("2006-01-02 15:04"))
				}
			}
			if truncated {
				builder.WriteString("\n（任务过多，仅显示最近 100 个。）")
			}
			return Result{Content: builder.String(), Value: payload}, nil
		},
	}
}

// PauseScheduledTask takes effect immediately on the user's explicit request.
func PauseScheduledTask(service ScheduledTaskService) Def {
	return Def{
		ID:          PauseScheduledTaskID,
		Description: pauseScheduledTaskDescription,
		JSONSchema:  json.RawMessage(pauseScheduledTaskSchema),
		Validate: func(v Value) error {
			var args struct {
				TaskID string `json:"task_id"`
			}
			if err := json.Unmarshal(v, &args); err != nil || strings.TrimSpace(args.TaskID) == "" {
				return fmt.Errorf("task_id is required")
			}
			return nil
		},
		Describe: func(v Value, c Context) (Operation, error) {
			if _, err := scheduledTaskLocation(c); err != nil {
				return Operation{}, err
			}
			var args struct {
				TaskID string `json:"task_id"`
			}
			if err := json.Unmarshal(v, &args); err != nil {
				return Operation{}, err
			}
			return Operation{ID: PauseScheduledTaskID, Input: v}, nil
		},
		Execute: func(v Value, c Context) (Result, error) {
			if _, err := scheduledTaskLocation(c); err != nil {
				return Result{}, err
			}
			var args struct {
				TaskID string `json:"task_id"`
			}
			if err := json.Unmarshal(v, &args); err != nil {
				return Result{}, err
			}
			task, err := service.PauseTask(c.Context, c.UserID, strings.TrimSpace(args.TaskID))
			if err != nil {
				return Result{}, err
			}
			payload, err := json.Marshal(struct {
				Status string            `json:"status"`
				Task   ScheduledTaskInfo `json:"task"`
			}{"paused", task})
			if err != nil {
				return Result{}, err
			}
			return Result{Content: fmt.Sprintf("任务「%s」（ID: %s）已暂停，不会再按计划运行。", task.Name, task.ID), Value: payload}, nil
		},
	}
}

// DescribeScheduledTaskSchedule renders the server's own wording for a plan so
// chat, the confirmation card, and the management page never disagree.
func DescribeScheduledTaskSchedule(schedule ScheduledTaskSchedule) string {
	location := scheduledTaskLocationOrUTC(schedule.Timezone)
	switch schedule.Kind {
	case scheduledTaskOnce:
		return "一次 · " + schedule.At.In(location).Format("2006-01-02 15:04")
	case scheduledTaskInterval:
		return fmt.Sprintf("每 %d 分钟 · 首次 %s", schedule.EverySeconds/60, schedule.At.In(location).Format("2006-01-02 15:04"))
	case scheduledTaskDaily:
		return "每天 " + schedule.LocalTime
	case scheduledTaskWeekly:
		names := []string{"日", "一", "二", "三", "四", "五", "六"}
		weekday := 0
		if schedule.Weekday >= 0 && schedule.Weekday < len(names) {
			weekday = schedule.Weekday
		}
		return "每周" + names[weekday] + " " + schedule.LocalTime
	case scheduledTaskMonthly:
		return fmt.Sprintf("每月 %d 日 %s", schedule.MonthDay, schedule.LocalTime)
	default:
		return schedule.Kind
	}
}

func scheduledTaskStatusLabel(status string) string {
	switch status {
	case "active":
		return "进行中"
	case "paused":
		return "已暂停"
	case "completed":
		return "已完成"
	default:
		return status
	}
}

// scheduledTaskLocation resolves the browser-reported timezone the request
// carried. It is the shared gate for all three scheduled-task tools: a model
// must never be invited to call a tool the runtime will deny, so an unusable
// timezone is a denial rather than a silent UTC fallback.
func scheduledTaskLocation(c Context) (*time.Location, error) {
	if !c.AllowScheduledTasks {
		return nil, Deny("scheduled tasks are not permitted")
	}
	zone := strings.TrimSpace(c.Timezone)
	if zone == "" {
		return nil, Deny("the request carries no timezone")
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, Deny("the request carries no usable timezone")
	}
	return location, nil
}

func scheduledTaskLocationOrUTC(zone string) *time.Location {
	location, err := time.LoadLocation(strings.TrimSpace(zone))
	if err != nil {
		return time.UTC
	}
	return location
}

// parseScheduledTaskArgs validates model arguments and converts them into the
// server-owned configuration: the timezone always comes from the request, and
// tool scope is fixed by the runtime rather than chosen by the model.
func parseScheduledTaskArgs(v Value, location *time.Location) (ScheduledTaskConfig, error) {
	var args struct {
		Name     string `json:"name"`
		Prompt   string `json:"prompt"`
		Schedule struct {
			Kind         string `json:"kind"`
			AtLocal      string `json:"atLocal"`
			EveryMinutes int64  `json:"everyMinutes"`
			LocalTime    string `json:"localTime"`
			Weekday      int    `json:"weekday"`
			MonthDay     int    `json:"monthDay"`
		} `json:"schedule"`
		ReportMode    string `json:"reportMode"`
		ConditionKind string `json:"conditionKind"`
	}
	if err := json.Unmarshal(v, &args); err != nil {
		return ScheduledTaskConfig{}, fmt.Errorf("invalid arguments: %w", err)
	}
	prompt := strings.TrimSpace(args.Prompt)
	if prompt == "" {
		return ScheduledTaskConfig{}, fmt.Errorf("prompt is required")
	}
	name := strings.TrimSpace(args.Name)
	if len([]rune(name)) > 60 {
		return ScheduledTaskConfig{}, fmt.Errorf("task name must be at most 60 characters")
	}
	schedule, err := parseScheduledTaskSchedule(args.Schedule.Kind, args.Schedule.AtLocal, args.Schedule.EveryMinutes,
		args.Schedule.LocalTime, args.Schedule.Weekday, args.Schedule.MonthDay, location)
	if err != nil {
		return ScheduledTaskConfig{}, err
	}
	reportMode := strings.TrimSpace(args.ReportMode)
	conditionKind := strings.TrimSpace(args.ConditionKind)
	switch reportMode {
	case scheduledTaskReportAlways:
		if conditionKind != "" && conditionKind != "none" {
			return ScheduledTaskConfig{}, fmt.Errorf("always-report task cannot have a condition kind")
		}
		conditionKind = "none"
	case scheduledTaskReportOnCondition:
		if conditionKind != "event" && conditionKind != "state" {
			return ScheduledTaskConfig{}, fmt.Errorf("conditional report requires conditionKind event or state")
		}
	default:
		return ScheduledTaskConfig{}, fmt.Errorf("reportMode must be always or on_condition")
	}
	return ScheduledTaskConfig{Name: name, Prompt: prompt, Schedule: schedule, ReportMode: reportMode,
		ConditionKind: conditionKind, AllowedToolIDs: []string{WebSearchID, WebFetchID}}, nil
}

func parseScheduledTaskSchedule(kind, atLocal string, everyMinutes int64, localTime string, weekday, monthDay int, location *time.Location) (ScheduledTaskSchedule, error) {
	schedule := ScheduledTaskSchedule{Kind: strings.TrimSpace(kind), Timezone: location.String()}
	switch schedule.Kind {
	case scheduledTaskOnce:
		at, err := parseScheduledTaskLocalInstant(atLocal, location)
		if err != nil {
			return ScheduledTaskSchedule{}, err
		}
		schedule.At = at
	case scheduledTaskInterval:
		if everyMinutes < 1 {
			return ScheduledTaskSchedule{}, fmt.Errorf("interval schedule requires everyMinutes of at least 1")
		}
		schedule.EverySeconds = everyMinutes * 60
		schedule.At = time.Now()
	case scheduledTaskDaily, scheduledTaskWeekly, scheduledTaskMonthly:
		if _, err := time.Parse("15:04", strings.TrimSpace(localTime)); err != nil {
			return ScheduledTaskSchedule{}, fmt.Errorf("schedule requires localTime in HH:MM form")
		}
		schedule.LocalTime = strings.TrimSpace(localTime)
		if schedule.Kind == scheduledTaskWeekly {
			if weekday < 0 || weekday > 6 {
				return ScheduledTaskSchedule{}, fmt.Errorf("weekly schedule requires weekday between 0 and 6")
			}
			schedule.Weekday = weekday
		}
		if schedule.Kind == scheduledTaskMonthly {
			if monthDay < 1 || monthDay > 31 {
				return ScheduledTaskSchedule{}, fmt.Errorf("monthly schedule requires monthDay between 1 and 31")
			}
			schedule.MonthDay = monthDay
		}
	default:
		return ScheduledTaskSchedule{}, fmt.Errorf("unsupported schedule kind %q", schedule.Kind)
	}
	return schedule, nil
}

func parseScheduledTaskLocalInstant(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("schedule requires atLocal in YYYY-MM-DD HH:MM form")
}
