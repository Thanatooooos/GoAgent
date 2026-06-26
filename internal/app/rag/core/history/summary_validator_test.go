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
		Goal:             "整理结构化摘要",
		EstablishedFacts: []string{"缺失信息"},
	}
	source := []domain.ConversationMessage{
		{Role: "assistant", Content: "indexer failed: vector store unavailable"},
		{Role: "user", Content: "doc_fail_01 为什么失败了？"},
	}

	result := ValidateStructuredSummary(summary, source)
	if result.Accepted {
		t.Fatalf("expected validator to reject missing critical entity, got %+v", result)
	}
}

func TestValidateStructuredSummaryAcceptsCriticalEntityCoverage(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    1,
		Goal:             "整理结构化摘要",
		EstablishedFacts: []string{"doc_fail_01 在 indexer 阶段报错", "错误是 vector store unavailable"},
	}
	source := []domain.ConversationMessage{
		{Role: "assistant", Content: "indexer failed: vector store unavailable"},
		{Role: "user", Content: "doc_fail_01 为什么失败了？"},
	}

	result := ValidateStructuredSummary(summary, source)
	if !result.Accepted {
		t.Fatalf("expected validator to accept preserved critical entity, got %+v", result)
	}
}

func TestValidateStructuredSummaryIgnoresCodeSnippetFragmentsWhenCheckingCriticalEntities(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    1,
		Goal:             "???????",
		EstablishedFacts: []string{"?????????????????"},
	}
	source := []domain.ConversationMessage{
		{Role: "assistant", Content: "???RRF(d, k) = sum_{i=1}^{n} 1 / (k + rank_i(d))"},
		{Role: "assistant", Content: "for rank, (doc_id, score) in enumerate(result, start=1): sorted(rrf_scores.items(), key=lambda x: x[1], reverse=True)"},
		{Role: "assistant", Content: `input_ids = tokenizer.encode("rewrite", return_tensors="pt"); tokenizer.decode(outputs[0], skip_special_tokens=True)`},
	}

	result := ValidateStructuredSummary(summary, source)
	if !result.Accepted {
		t.Fatalf("expected validator to ignore code snippet fragments, got %+v", result)
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
	if !output.Validation.Accepted {
		t.Fatalf("expected validation to accept repaired summary after active-priority backfill, got %+v", output.Validation)
	}
	if len(llm.lastRequest.Messages) != 2 {
		t.Fatalf("expected two chat messages, got %#v", llm.lastRequest.Messages)
	}
	if llm.lastRequest.Messages[1].Content != "现在请直接返回结构化工作记忆 JSON。" {
		t.Fatalf("unexpected user prompt: %q", llm.lastRequest.Messages[1].Content)
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

