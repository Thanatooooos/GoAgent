package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	briefdomain "local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/scheduledtask/domain"
)

const taskBriefArtifact = `{"headline":"今日简报","topSummary":"覆盖说明","sections":[{"key":"tech.dev","title":"开发","items":[{"title":"更新","summary":"摘要","whyItMatters":"影响","url":"https://example.org/news","source":"官网","topic":"tech.dev"}]}]}`

func TestDailyBriefExecutorPreservesContractAndUsesTaskRuntime(t *testing.T) {
	stub := &runtimeStub{answer: `{"signal":"report","body":"untrusted separate text","sources":["other"],"artifact":` + taskBriefArtifact + `}`}
	input := executionInput()
	input.Version.Schedule.Kind = domain.ScheduleDaily
	input.Version.Schedule.LocalTime = "09:00"
	input.Version.ReportMode, input.Version.ConditionKind = domain.ReportAlways, domain.ConditionNone
	input.Version.DailyBrief = &briefdomain.TaskContract{Topics: []string{"tech.dev"}, Sources: []string{briefdomain.SourceKeyHackerNews}, MaxItems: 5}
	result, err := (Executor{Runtime: stub}).Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Outcome.Body, "今日简报") || strings.Contains(result.Outcome.Body, "untrusted") || len(result.Outcome.Sources) != 1 || result.Outcome.Sources[0] != "https://example.org/news" {
		t.Fatalf("views diverge: %+v", result.Outcome)
	}
	if stub.request.TaskType != "scheduled" || stub.request.TaskID != input.AttemptID || len(stub.request.Sources) != 1 || stub.request.Sources[0].Key != "scheduled-task/history" {
		t.Fatalf("unexpected precollection path: %+v", stub.request)
	}
	if !strings.Contains(strings.Join(stub.request.System, "\n"), `signal must be exactly "report"`) {
		t.Fatal("DailyBrief-specific instructions omitted the result signal contract")
	}
	for _, answer := range []string{
		`{"signal":"no_report"}`,
		`{"signal":"normal","body":"text","artifact":` + taskBriefArtifact + `}`,
		`{"signal":"report","body":"text"}`,
		`{"signal":"report","body":"text","artifact":` + strings.ReplaceAll(taskBriefArtifact, "tech.dev", "tech.ai.models") + `}`,
		`{"signal":"report","body":"text","artifact":` + strings.ReplaceAll(taskBriefArtifact, "https://example.org/news", "javascript:alert(1)") + `}`,
	} {
		stub.answer = answer
		if _, err := (Executor{Runtime: stub}).Run(context.Background(), input); err == nil {
			t.Fatalf("invalid output accepted: %s", answer)
		}
	}
	stub.answer = `{"signal":"uncertain","reason":"所有来源不可用"}`
	if result, err := (Executor{Runtime: stub}).Run(context.Background(), input); err != nil || result.Outcome.Signal != domain.SignalUncertain {
		t.Fatalf("uncertain: %+v %v", result, err)
	}
}

func TestDailyBriefExecutorTruncatesToContractQuota(t *testing.T) {
	var artifact briefdomain.BriefArtifact
	if err := json.Unmarshal([]byte(taskBriefArtifact), &artifact); err != nil {
		t.Fatal(err)
	}
	item := artifact.Sections[0].Items[0]
	artifact.Sections[0].Items = nil
	for _, title := range []string{"one", "two", "three"} {
		item.Title, item.URL = title, "https://example.org/"+title
		artifact.Sections[0].Items = append(artifact.Sections[0].Items, item)
	}
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	stub := &runtimeStub{answer: "{\"signal\":\"report\",\"body\":\"ignored\",\"artifact\":" + string(raw) + "}"}
	input := executionInput()
	input.Version.Schedule.Kind = domain.ScheduleDaily
	input.Version.ReportMode, input.Version.ConditionKind = domain.ReportAlways, domain.ConditionNone
	input.Version.DailyBrief = &briefdomain.TaskContract{Topics: []string{"tech.dev"}, MaxItems: 2, MaxItemsPerTopic: 2}
	result, err := (Executor{Runtime: stub}).Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Outcome.Body, "one") || strings.Contains(result.Outcome.Body, "three") {
		t.Fatalf("expected deterministic quota truncation, got %s", result.Outcome.Body)
	}
	if len(result.Outcome.Sources) != 2 || strings.Contains(string(result.Outcome.Artifact), "three") {
		t.Fatalf("persisted artifact and sources exceed quota: %+v", result.Outcome)
	}
	normalized, _, err := NormalizeDailyBrief(result.Outcome, *input.Version.DailyBrief)
	if err != nil || normalized.Body != result.Outcome.Body || string(normalized.Artifact) != string(result.Outcome.Artifact) {
		t.Fatalf("publication recovery changed the result: %+v %v", normalized, err)
	}
	// Invalid output must still be rejected even if the bad item is over quota.
	stub.answer = strings.ReplaceAll(stub.answer, "https://example.org/three", "javascript:alert(1)")
	if _, err := (Executor{Runtime: stub}).Run(context.Background(), input); err == nil {
		t.Fatal("invalid discarded item accepted")
	}
}
