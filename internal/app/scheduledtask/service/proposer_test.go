package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/scheduledtask/domain"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type proposalModelStub struct {
	answer  string
	request convention.ChatRequest
}

func TestConfigurationReviewHasExplicitModeAndStillRequiresCompletePreview(t *testing.T) {
	now := time.Date(2026, 10, 2, 6, 0, 0, 0, time.UTC)
	config := domain.Version{Name: "回归提醒", Prompt: "提醒整理结果", Schedule: domain.Schedule{Kind: "once", Timezone: "Asia/Shanghai", At: now.Add(time.Hour)}, ReportMode: "always", ConditionKind: "none"}
	model := &proposalModelStub{answer: `{"status":"draft","taskId":"preview","prompt":"提醒整理结果","schedule":{"kind":"once","timezone":"Asia/Shanghai","at":"2026-10-02T07:00:00Z"},"reportMode":"always","conditionKind":"none","knowledgeBaseIds":[],"allowedWebDomains":[],"allowedToolIds":[]}`}
	p := Proposer{Model: model}
	proposal, err := p.ReviewConfig(context.Background(), config, now)
	if err != nil || proposal.TaskID != "preview" || proposal.Name != config.Name {
		t.Fatalf("review = %+v, %v", proposal, err)
	}
	if !strings.Contains(model.request.Messages[0].Content, "management-UI configuration review") {
		t.Fatal("configuration review does not state its explicit mode")
	}
	if model.request.JSONMode == nil || !*model.request.JSONMode || model.request.EnableTools == nil || *model.request.EnableTools {
		t.Fatalf("unexpected review model policy: %+v", model.request)
	}
	model.answer = `{"status":"none"}`
	if _, err := p.ReviewConfig(context.Background(), config, now); err == nil {
		t.Fatal("incomplete review bypassed preview validation")
	}
}

func TestConfigurationReviewPreservesTaskTimezoneAndRejectsKBExpansion(t *testing.T) {
	now := time.Date(2026, 10, 2, 6, 0, 0, 0, time.UTC)
	model := &proposalModelStub{answer: `{"status":"draft","taskId":"preview","prompt":"Check weekly","schedule":{"kind":"weekly","timezone":"Asia/Shanghai","localTime":"09:00","weekday":5},"reportMode":"always","conditionKind":"none","knowledgeBaseIds":["approved"],"allowedToolIds":["retrieve_knowledge"]}`}
	config := domain.Version{Name: "周报", Prompt: "Check weekly", Schedule: domain.Schedule{Kind: "weekly", Timezone: "Asia/Shanghai", LocalTime: "09:00", Weekday: 5},
		ReportMode: "always", ConditionKind: "none", KnowledgeBaseIDs: []string{"approved"}, AllowedToolIDs: []string{"retrieve_knowledge"}}
	proposal, err := (Proposer{Model: model}).ReviewConfig(context.Background(), config, now)
	if err != nil || proposal.Schedule.Timezone != "Asia/Shanghai" || proposal.TaskID != "preview" {
		t.Fatalf("review = %+v, %v", proposal, err)
	}
	model.answer = `{"status":"draft","taskId":"preview","prompt":"Check weekly","schedule":{"kind":"weekly","timezone":"Asia/Shanghai","localTime":"09:00","weekday":5},"reportMode":"always","conditionKind":"none","knowledgeBaseIds":["other"],"allowedToolIds":["retrieve_knowledge"]}`
	if _, err := (Proposer{Model: model}).ReviewConfig(context.Background(), config, now); err == nil {
		t.Fatal("configuration review accepted expanded knowledge base scope")
	}
}

func TestConfigurationReviewRejectsIncompleteDraft(t *testing.T) {
	now := time.Date(2026, 10, 2, 6, 0, 0, 0, time.UTC)
	config := domain.Version{Name: "赛事追踪", Prompt: "有消息告诉我", Schedule: domain.Schedule{Kind: "interval", Timezone: "Asia/Shanghai", At: now.Add(time.Hour), EverySeconds: 3600},
		ReportMode: "on_condition", ConditionKind: "event"}
	model := &proposalModelStub{answer: `{"status":"draft","taskId":"preview","prompt":"Watch the event","schedule":{"kind":"interval","timezone":"Asia/Shanghai","everySeconds":3600},"reportMode":"on_condition","conditionKind":"event"}`}
	if _, err := (Proposer{Model: model}).ReviewConfig(context.Background(), config, now); err == nil {
		t.Fatal("expected incomplete schedule to be rejected")
	}
}

func (s *proposalModelStub) Chat(string) (string, error) { return s.answer, nil }
func (s *proposalModelStub) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.request = request
	return s.answer, nil
}
func (s *proposalModelStub) ChatWithModel(request convention.ChatRequest, _ string) (string, error) {
	return s.ChatWithRequest(request)
}
func (s *proposalModelStub) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
func (s *proposalModelStub) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
