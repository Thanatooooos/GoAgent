package history

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

func TestValidateStructuredSummaryRejectsMissingCriticalEntity(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    1,
		Goal:             "鎺掓煡瀵煎叆澶辫触",
		EstablishedFacts: []string{"瀵煎叆澶辫触"},
	}
	source := []domain.ConversationMessage{
		{Role: "assistant", Content: "indexer failed: vector store unavailable"},
		{Role: "user", Content: "doc_fail_01 涓轰粈涔堝け璐?"},
	}

	result := ValidateStructuredSummary(summary, source)
	if result.Accepted {
		t.Fatalf("expected validator to reject missing critical entity, got %+v", result)
	}
}

func TestValidateStructuredSummaryAcceptsCriticalEntityCoverage(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    1,
		Goal:             "鎺掓煡瀵煎叆澶辫触",
		EstablishedFacts: []string{"doc_fail_01 鍦?indexer 闃舵鎶ラ敊", "閿欒涓?vector store unavailable"},
	}
	source := []domain.ConversationMessage{
		{Role: "assistant", Content: "indexer failed: vector store unavailable"},
		{Role: "user", Content: "doc_fail_01 涓轰粈涔堝け璐?"},
	}

	result := ValidateStructuredSummary(summary, source)
	if !result.Accepted {
		t.Fatalf("expected validator to accept preserved critical entity, got %+v", result)
	}
}

func TestGenerateStructuredSummaryRepairsBeforeValidation(t *testing.T) {
	llm := &repairAwareLLMStub{
		response: `{"schema_version":1,"goal":"整理结构化摘要","established_facts":["接口方案还没确认"]}`,
	}

	output, err := GenerateStructuredSummary(context.Background(), llm, GenerateStructuredSummaryInput{
		SourceMessages: []domain.ConversationMessage{
			{Role: "user", Content: "请帮我整理结构化摘要"},
			{Role: "assistant", Content: "收到"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateStructuredSummary returned error: %v", err)
	}

	if len(output.Structured.EstablishedFacts) != 0 {
		t.Fatalf("expected established facts to be repaired away, got %#v", output.Structured.EstablishedFacts)
	}
	if got := output.Structured.OpenQuestions; len(got) != 1 || got[0] != "接口方案还没确认" {
		t.Fatalf("expected unresolved item to move to open questions, got %#v", got)
	}
	if output.Validation.Accepted {
		t.Fatalf("expected validation to run on repaired summary and reject missing high-value sections, got %+v", output.Validation)
	}
	if output.Validation.Reason != "missing high-value sections" {
		t.Fatalf("expected repaired summary to fail for missing high-value sections, got %q", output.Validation.Reason)
	}
	if !strings.Contains(output.Rendered, "待确认问题") {
		t.Fatalf("expected rendered summary to use repaired open questions section, got %q", output.Rendered)
	}
	if !strings.Contains(output.Rendered, "接口方案还没确认") {
		t.Fatalf("expected rendered summary to keep the unresolved item, got %q", output.Rendered)
	}
}

type repairAwareLLMStub struct {
	response    string
	lastRequest convention.ChatRequest
}

func (s *repairAwareLLMStub) Chat(_ string) (string, error) { return s.response, nil }

func (s *repairAwareLLMStub) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.lastRequest = request
	return s.response, nil
}

func (s *repairAwareLLMStub) ChatWithModel(_ convention.ChatRequest, _ string) (string, error) {
	return s.response, nil
}

func (s *repairAwareLLMStub) StreamChat(_ string, _ aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *repairAwareLLMStub) StreamChatWithRequest(_ convention.ChatRequest, _ aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

var _ aichat.LLMService = (*repairAwareLLMStub)(nil)




