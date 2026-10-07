package domain

import (
	"testing"
	"time"

	"local/rag-project/internal/app/runtime/capability"
)

func TestVersionRequiresReadOnlyToolsAndMatchingReportMode(t *testing.T) {
	v := Version{TaskID: "task-1", Number: 1, Prompt: "Check a source", Schedule: Schedule{Kind: ScheduleDaily, Timezone: "UTC", LocalTime: "08:00"}, ReportMode: ReportOnCondition, ConditionKind: ConditionEvent, ConfirmedAt: time.Now(), AllowedToolIDs: []string{capability.WebSearchID}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	v.AllowedToolIDs = []string{capability.MemoryAddID}
	if err := v.Validate(); err == nil {
		t.Fatal("write tool must be rejected")
	}
	v.AllowedToolIDs = nil
	v.ReportMode = ReportAlways
	if err := v.Validate(); err == nil {
		t.Fatal("always-report condition mismatch must be rejected")
	}
}
