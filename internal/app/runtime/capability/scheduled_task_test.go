package capability

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type scheduledTaskStub struct {
	created        ScheduledTaskConfig
	createdUser    string
	createdOrigin  string
	createErr      error
	tasks          []ScheduledTaskInfo
	pausedUser     string
	pausedTaskID   string
	pauseErr       error
	pausedResponse ScheduledTaskInfo
}

func (s *scheduledTaskStub) CreateDraft(_ context.Context, userID, originConversationID string, config ScheduledTaskConfig, _ time.Time) (ScheduledTaskDraft, error) {
	s.created, s.createdUser, s.createdOrigin = config, userID, originConversationID
	if s.createErr != nil {
		return ScheduledTaskDraft{}, s.createErr
	}
	return ScheduledTaskDraft{ID: "draft-1", Config: config, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
}

func (s *scheduledTaskStub) ListTasks(context.Context, string) ([]ScheduledTaskInfo, error) {
	return s.tasks, nil
}

func (s *scheduledTaskStub) PauseTask(_ context.Context, userID, taskID string) (ScheduledTaskInfo, error) {
	s.pausedUser, s.pausedTaskID = userID, taskID
	if s.pauseErr != nil {
		return ScheduledTaskInfo{}, s.pauseErr
	}
	return s.pausedResponse, nil
}

func scheduledTaskContext() Context {
	return Context{AllowScheduledTasks: true, Timezone: "Asia/Shanghai", UserID: "u1", ConversationID: "c1"}
}

func TestCreateScheduledTaskUsesRequestTimezoneAndFixedToolScope(t *testing.T) {
	stub := &scheduledTaskStub{}
	def := CreateScheduledTask(stub)
	args := Value(`{"name":"赛程提醒","prompt":"查看今天的赛程并汇报","schedule":{"kind":"daily","localTime":"09:00"},"reportMode":"always"}`)
	if err := def.Validate(args); err != nil {
		t.Fatal(err)
	}
	result, err := def.Execute(args, scheduledTaskContext())
	if err != nil {
		t.Fatal(err)
	}
	if stub.createdUser != "u1" || stub.createdOrigin != "c1" {
		t.Fatalf("draft was not scoped to the conversation's user: %+v", stub)
	}
	if stub.created.Schedule.Timezone != "Asia/Shanghai" || stub.created.Schedule.Kind != "daily" || stub.created.Schedule.LocalTime != "09:00" {
		t.Fatalf("schedule = %+v", stub.created.Schedule)
	}
	if stub.created.ConditionKind != "none" || stub.created.ReportMode != "always" {
		t.Fatalf("report rules = %+v", stub.created)
	}
	if len(stub.created.KnowledgeBaseIDs) != 0 {
		t.Fatalf("chat draft must not gain knowledge scope: %+v", stub.created.KnowledgeBaseIDs)
	}
	if len(stub.created.AllowedToolIDs) != 2 || stub.created.AllowedToolIDs[0] != WebSearchID || stub.created.AllowedToolIDs[1] != WebFetchID {
		t.Fatalf("tool scope = %+v", stub.created.AllowedToolIDs)
	}
	if !strings.Contains(result.Content, "草稿 ID: draft-1") || !strings.Contains(result.Content, "尚未生效") {
		t.Fatalf("content = %q", result.Content)
	}
	if !strings.Contains(string(result.Value), `"draft"`) {
		t.Fatalf("value = %s", result.Value)
	}
}

func TestCreateScheduledTaskResolvesLocalTimesInTheRequestZone(t *testing.T) {
	stub := &scheduledTaskStub{}
	def := CreateScheduledTask(stub)
	args := Value(`{"prompt":"提醒我","schedule":{"kind":"once","atLocal":"2026-10-06 09:00"},"reportMode":"always"}`)
	result, err := def.Execute(args, scheduledTaskContext())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse(time.RFC3339, "2026-10-06T01:00:00Z")
	if !stub.created.Schedule.At.Equal(want) {
		t.Fatalf("at = %s, want %s", stub.created.Schedule.At, want)
	}
	if result.Content == "" {
		t.Fatal("draft summary is empty")
	}
	if _, err := def.Execute(Value(`{"prompt":"提醒我","schedule":{"kind":"once"},"reportMode":"always"}`), scheduledTaskContext()); err == nil {
		t.Fatal("once schedule without atLocal was accepted")
	}
}

func TestCreateScheduledTaskSchemaDeclaresScheduleSpecificFields(t *testing.T) {
	def := CreateScheduledTask(&scheduledTaskStub{})
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(def.JSONSchema, &schema); err != nil {
		t.Fatal(err)
	}
	var schedule struct {
		AdditionalProperties bool              `json:"additionalProperties"`
		AllOf                []json.RawMessage `json:"allOf"`
	}
	if err := json.Unmarshal(schema.Properties["schedule"], &schedule); err != nil {
		t.Fatal(err)
	}
	if schedule.AdditionalProperties || len(schedule.AllOf) != 5 {
		t.Fatalf("schedule schema does not constrain fields by kind: %+v", schedule)
	}
	for _, raw := range schedule.AllOf {
		var rule struct {
			Then struct {
				Required []string `json:"required"`
			} `json:"then"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			t.Fatal(err)
		}
		if len(rule.Then.Required) != 1 {
			t.Fatalf("schedule rule has no required field: %s", raw)
		}
	}
}

func TestScheduledTaskToolsDenyWithoutPermissionOrTimezone(t *testing.T) {
	stub := &scheduledTaskStub{}
	createArgs := Value(`{"prompt":"提醒我","schedule":{"kind":"daily","localTime":"09:00"},"reportMode":"always"}`)
	for _, test := range []struct {
		name    string
		context Context
	}{
		{"policy denied", Context{Timezone: "Asia/Shanghai", UserID: "u1"}},
		{"missing timezone", Context{AllowScheduledTasks: true, UserID: "u1"}},
		{"unusable timezone", Context{AllowScheduledTasks: true, Timezone: "Mars/Olympus", UserID: "u1"}},
	} {
		for _, def := range []Def{CreateScheduledTask(stub), ListScheduledTasks(stub), PauseScheduledTask(stub)} {
			args := createArgs
			if def.ID == PauseScheduledTaskID {
				args = Value(`{"task_id":"task-1"}`)
			}
			if _, err := def.Describe(args, test.context); !IsDenied(err) {
				t.Fatalf("%s: %s Describe error = %v", test.name, def.ID, err)
			}
		}
		if _, err := CreateScheduledTask(stub).Execute(createArgs, test.context); !IsDenied(err) {
			t.Fatalf("%s: execute error = %v", test.name, err)
		}
	}
}

func TestListScheduledTasksRendersServerWording(t *testing.T) {
	stub := &scheduledTaskStub{tasks: []ScheduledTaskInfo{
		{ID: "task-1", Status: "active", Name: "赛程追踪", ScheduleDescription: "每天 09:00", NextDueAt: time.Now().Add(time.Hour)},
		{ID: "task-2", Status: "paused", Name: "周报", ScheduleDescription: "每周一 08:30"},
	}}
	result, err := ListScheduledTasks(stub).Execute(Value(`{}`), scheduledTaskContext())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"共 2 个定时任务", "赛程追踪（ID: task-1）", "计划：每天 09:00", "状态：已暂停"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("missing %q in %q", want, result.Content)
		}
	}
	empty, err := ListScheduledTasks(&scheduledTaskStub{}).Execute(Value(`{}`), scheduledTaskContext())
	if err != nil {
		t.Fatal(err)
	}
	if empty.Content != "当前没有定时任务。" {
		t.Fatalf("empty content = %q", empty.Content)
	}
}

func TestPauseScheduledTaskRequiresTaskID(t *testing.T) {
	def := PauseScheduledTask(&scheduledTaskStub{})
	if err := def.Validate(Value(`{}`)); err == nil {
		t.Fatal("pause accepted an empty task id")
	}
	stub := &scheduledTaskStub{pausedResponse: ScheduledTaskInfo{ID: "task-1", Status: "paused", Name: "赛程追踪"}}
	result, err := PauseScheduledTask(stub).Execute(Value(`{"task_id":"task-1"}`), scheduledTaskContext())
	if err != nil {
		t.Fatal(err)
	}
	if stub.pausedUser != "u1" || stub.pausedTaskID != "task-1" {
		t.Fatalf("pause scope = %q / %q", stub.pausedUser, stub.pausedTaskID)
	}
	if !strings.Contains(result.Content, "已暂停") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestDescribeScheduledTaskScheduleWording(t *testing.T) {
	at := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		schedule ScheduledTaskSchedule
		want     string
	}{
		{ScheduledTaskSchedule{Kind: "once", Timezone: "Asia/Shanghai", At: at}, "一次 · 2026-10-06 09:00"},
		{ScheduledTaskSchedule{Kind: "interval", Timezone: "Asia/Shanghai", At: at, EverySeconds: 3600}, "每 60 分钟 · 首次 2026-10-06 09:00"},
		{ScheduledTaskSchedule{Kind: "daily", Timezone: "Asia/Shanghai", LocalTime: "09:00"}, "每天 09:00"},
		{ScheduledTaskSchedule{Kind: "weekly", Timezone: "Asia/Shanghai", LocalTime: "09:00", Weekday: 3}, "每周三 09:00"},
		{ScheduledTaskSchedule{Kind: "monthly", Timezone: "Asia/Shanghai", LocalTime: "09:00", MonthDay: 5}, "每月 5 日 09:00"},
	} {
		if got := DescribeScheduledTaskSchedule(test.schedule); got != test.want {
			t.Fatalf("describe(%+v) = %q, want %q", test.schedule, got, test.want)
		}
	}
}
