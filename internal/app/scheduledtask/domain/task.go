package domain

import (
	"fmt"
	"strings"
	"time"

	briefdomain "local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/runtime/capability"
)

type TaskStatus string

const (
	TaskActive    TaskStatus = "active"
	TaskPaused    TaskStatus = "paused"
	TaskCompleted TaskStatus = "completed"
)

type ReportMode string

const (
	ReportAlways      ReportMode = "always"       // Reminders and periodic summaries.
	ReportOnCondition ReportMode = "on_condition" // Watches may return no_report.
)

type ConditionKind string

const (
	ConditionNone  ConditionKind = "none"
	ConditionState ConditionKind = "state"
	ConditionEvent ConditionKind = "event"
)

type Task struct {
	ID              string     `json:"id"`
	UserID          string     `json:"-"`
	Status          TaskStatus `json:"status"`
	CurrentVersion  int        `json:"currentVersion"`
	ConversationID  string     `json:"conversationId,omitempty"`
	ConfirmedAt     time.Time  `json:"confirmedAt"`
	NextDueAt       time.Time  `json:"nextDueAt,omitempty"`
	LastReportedAt  time.Time  `json:"lastReportedAt,omitempty"`
	LastAttemptedAt time.Time  `json:"lastAttemptedAt,omitempty"`
}

// Version is immutable after explicit confirmation. The runtime cannot widen
// these source and tool scopes by editing prompt text.
type Version struct {
	DailyBrief        *briefdomain.TaskContract `json:"dailyBrief,omitempty"`
	TaskID            string                    `json:"taskId"`
	Number            int                       `json:"number"`
	Name              string                    `json:"name"`
	Prompt            string                    `json:"prompt"`
	Schedule          Schedule                  `json:"schedule"`
	ReportMode        ReportMode                `json:"reportMode"`
	ConditionKind     ConditionKind             `json:"conditionKind"`
	KnowledgeBaseIDs  []string                  `json:"knowledgeBaseIds"`
	AllowedWebDomains []string                  `json:"allowedWebDomains"`
	AllowedToolIDs    []string                  `json:"allowedToolIds"`
	ConfirmedAt       time.Time                 `json:"confirmedAt"`
}

func (v Version) Validate() error {
	if v.DailyBrief != nil {
		if v.Schedule.Kind != ScheduleDaily || v.ReportMode != ReportAlways || len(v.DailyBrief.Topics) == 0 || v.DailyBrief.MaxItems < 1 || v.DailyBrief.MaxItemsPerTopic < 0 {
			return fmt.Errorf("DailyBrief requires a daily always-report schedule and a valid artifact contract")
		}
		for _, topic := range v.DailyBrief.Topics {
			if !briefdomain.IsTopicKeySupported(topic) {
				return fmt.Errorf("unsupported DailyBrief topic %q", topic)
			}
		}
		for _, source := range v.DailyBrief.Sources {
			if !briefdomain.IsSourceKeySupported(source) {
				return fmt.Errorf("unsupported DailyBrief source %q", source)
			}
		}
	}
	if len([]rune(strings.TrimSpace(v.Name))) > 60 {
		return fmt.Errorf("task name must be at most 60 characters")
	}
	if strings.TrimSpace(v.TaskID) == "" || v.Number < 1 || v.ConfirmedAt.IsZero() {
		return fmt.Errorf("confirmed task version requires task id, positive version, and confirmation time")
	}
	if strings.TrimSpace(v.Prompt) == "" {
		return fmt.Errorf("task prompt is required")
	}
	if err := v.Schedule.Validate(); err != nil {
		return err
	}
	if v.ReportMode != ReportAlways && v.ReportMode != ReportOnCondition {
		return fmt.Errorf("invalid report mode %q", v.ReportMode)
	}
	if v.ReportMode == ReportOnCondition {
		if v.ConditionKind != ConditionState && v.ConditionKind != ConditionEvent {
			return fmt.Errorf("conditional report requires state or event kind")
		}
		if v.ConditionKind == ConditionEvent && v.ConfirmedAt.IsZero() {
			return fmt.Errorf("event condition requires confirmation time")
		}
	} else if v.ConditionKind != ConditionNone && v.ConditionKind != "" {
		return fmt.Errorf("always-report task cannot have a condition kind")
	}
	approved := map[string]bool{
		capability.WebSearchID:         true,
		capability.WebFetchID:          true,
		capability.RetrieveKnowledgeID: true,
	}
	for _, toolID := range v.AllowedToolIDs {
		if !approved[toolID] {
			return fmt.Errorf("tool %q is not approved for scheduled tasks", toolID)
		}
	}
	if len(v.KnowledgeBaseIDs) > 0 && !contains(v.AllowedToolIDs, capability.RetrieveKnowledgeID) {
		return fmt.Errorf("knowledge base scope requires retrieve_knowledge tool")
	}
	if len(v.AllowedWebDomains) > 0 && !contains(v.AllowedToolIDs, capability.WebSearchID) && !contains(v.AllowedToolIDs, capability.WebFetchID) {
		return fmt.Errorf("web domain scope requires a web tool")
	}
	return nil
}

// DisplayName gives older tasks without a saved name a compact label. It never
// exposes the execution prompt as the task's title or changes saved rules.
func (v Version) DisplayName() string {
	if name := strings.TrimSpace(v.Name); name != "" {
		return name
	}
	label := "定期汇总"
	if v.ReportMode == ReportOnCondition {
		label = "状态监测"
		if v.ConditionKind == ConditionEvent {
			label = "事件监测"
		}
	} else if v.Schedule.Kind == ScheduleOnce {
		label = "提醒事项"
	}
	if len(v.AllowedWebDomains) > 0 {
		label = v.AllowedWebDomains[0] + " · " + label
	}
	return label
}

func contains(values []string, wanted string) bool {
	for _, item := range values {
		if item == wanted {
			return true
		}
	}
	return false
}
