package extraction

import (
	"fmt"
	"strings"
	"testing"

	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type stubLLMService struct {
	response string
	err      error
	requests []convention.ChatRequest
}

func (s *stubLLMService) Chat(prompt string) (string, error) {
	return s.response, s.err
}

func (s *stubLLMService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.requests = append(s.requests, request)
	return s.response, s.err
}

func (s *stubLLMService) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	s.requests = append(s.requests, request)
	return s.response, s.err
}

func (s *stubLLMService) StreamChat(prompt string, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *stubLLMService) StreamChatWithRequest(request convention.ChatRequest, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

var _ aichat.LLMService = (*stubLLMService)(nil)

func TestLLMPreferenceExtractorExtractStructuredCandidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		message             string
		llmResponse         string
		wantCandidate       *StructuredPreferenceCandidate
		wantRejected        bool
		wantRejectionReason string
		wantFailed          bool
		wantFailureReason   string
	}{
		{
			name:        "language preference maps to response language",
			message:     "以后默认用中文回答",
			llmResponse: `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"以后默认用中文回答","content":"以后默认用中文回答","confidence":0.94}`,
			wantCandidate: &StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "response.language",
				Summary:      "以后默认用中文回答",
				Content:      "以后默认用中文回答",
				Confidence:   0.94,
			},
		},
		{
			name:        "troubleshooting preference maps to narrowed workflow key",
			message:     "以后遇到报错先判断是不是环境问题",
			llmResponse: `{"scope_type":"global","memory_type":"preference","canonical_key":"workflow.troubleshooting.first_step","summary":"以后遇到报错先判断是不是环境问题","content":"先判断是不是环境问题","confidence":0.91}`,
			wantCandidate: &StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "workflow.troubleshooting.first_step",
				Summary:      "以后遇到报错先判断是不是环境问题",
				Content:      "先判断是不是环境问题",
				Confidence:   0.91,
			},
		},
		{
			name:        "avoidance preference maps to behavior avoid",
			message:     "以后不要一上来就大改代码",
			llmResponse: `{"scope_type":"global","memory_type":"preference","canonical_key":"behavior.avoid","summary":"以后不要一上来就大改代码","content":"不要一上来就大改代码","confidence":0.89}`,
			wantCandidate: &StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "behavior.avoid",
				Summary:      "以后不要一上来就大改代码",
				Content:      "不要一上来就大改代码",
				Confidence:   0.89,
			},
		},
		{
			name:                "knowledge candidate is rejected as invalid",
			message:             "我们项目长期使用 PostgreSQL",
			llmResponse:         `{"scope_type":"global","memory_type":"knowledge","canonical_key":"project.constraint.database","summary":"项目长期使用 PostgreSQL","content":"项目长期使用 PostgreSQL","confidence":0.85}`,
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidMemoryType,
		},
		{
			name:                "deprecated workflow first step is rejected as invalid",
			message:             "以后遇到报错先判断是不是环境问题",
			llmResponse:         `{"scope_type":"global","memory_type":"preference","canonical_key":"workflow.first_step","summary":"以后遇到报错先判断是不是环境问题","content":"先判断是不是环境问题","confidence":0.91}`,
			wantRejected:        true,
			wantRejectionReason: RejectionReasonDeprecatedWorkflowKey,
		},
		{
			name:                "non allowlist key is rejected as invalid",
			message:             "以后默认把答案控制在 3 行内",
			llmResponse:         `{"scope_type":"global","memory_type":"preference","canonical_key":"response.length","summary":"以后默认把答案控制在 3 行内","content":"答案控制在 3 行内","confidence":0.8}`,
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidCanonicalKey,
		},
		{
			name:                "invalid scope type is rejected",
			message:             "以后默认用中文回答",
			llmResponse:         `{"scope_type":"kb","memory_type":"preference","canonical_key":"response.language","summary":"以后默认用中文回答","content":"中文","confidence":0.9}`,
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidScopeType,
		},
		{
			name:                "negative confidence is rejected",
			message:             "以后默认用中文回答",
			llmResponse:         `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"以后默认用中文回答","content":"中文","confidence":-0.1}`,
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidConfidence,
		},
		{
			name:                "confidence above one is rejected",
			message:             "以后默认用中文回答",
			llmResponse:         `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"以后默认用中文回答","content":"中文","confidence":1.1}`,
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidConfidence,
		},
		{
			name:              "invalid json fails extraction without candidate",
			message:           "以后默认用中文回答",
			llmResponse:       `not json`,
			wantFailed:        true,
			wantFailureReason: FailureReasonInvalidJSON,
		},
		{
			name:                "missing fields is rejected as invalid",
			message:             "以后默认用中文回答",
			llmResponse:         `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"","content":"以后默认用中文回答","confidence":0.9}`,
			wantRejected:        true,
			wantRejectionReason: RejectionReasonMissingField,
		},
		{
			name:              "llm call failure fails open without candidate",
			message:           "以后默认用中文回答",
			wantFailed:        true,
			wantFailureReason: FailureReasonLLMCall,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			llm := &stubLLMService{response: tc.llmResponse}
			if tc.wantFailureReason == FailureReasonLLMCall {
				llm.err = fmt.Errorf("boom")
			}
			extractor := NewLLMPreferenceExtractor(llm)

			result := extractor.Extract(ExtractInput{Message: tc.message})

			if tc.wantCandidate != nil {
				if result.Candidate == nil {
					t.Fatalf("expected candidate, got result=%+v", result)
				}
				if *result.Candidate != *tc.wantCandidate {
					t.Fatalf("candidate = %+v, want %+v", *result.Candidate, *tc.wantCandidate)
				}
			} else if result.Candidate != nil {
				t.Fatalf("expected no candidate, got %+v", *result.Candidate)
			}
			if result.Rejected != tc.wantRejected {
				t.Fatalf("Rejected = %v, want %v, result=%+v", result.Rejected, tc.wantRejected, result)
			}
			if result.RejectionReason != tc.wantRejectionReason {
				t.Fatalf("RejectionReason = %q, want %q, result=%+v", result.RejectionReason, tc.wantRejectionReason, result)
			}
			if result.Failed != tc.wantFailed {
				t.Fatalf("Failed = %v, want %v, result=%+v", result.Failed, tc.wantFailed, result)
			}
			if result.FailureReason != tc.wantFailureReason {
				t.Fatalf("FailureReason = %q, want %q, result=%+v", result.FailureReason, tc.wantFailureReason, result)
			}
		})
	}
}

func TestLLMPreferenceExtractorRequestsStrictJSONPrompt(t *testing.T) {
	t.Parallel()

	llm := &stubLLMService{
		response: `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"以后默认用中文回答","content":"以后默认用中文回答","confidence":0.94}`,
	}
	extractor := NewLLMPreferenceExtractor(llm)

	_ = extractor.Extract(ExtractInput{Message: "以后默认用中文回答"})

	if len(llm.requests) != 1 {
		t.Fatalf("expected one llm request, got %d", len(llm.requests))
	}
	request := llm.requests[0]
	if request.JSONMode == nil || !*request.JSONMode {
		t.Fatalf("expected JSONMode request, got %+v", request)
	}
	if len(request.Messages) != 2 {
		t.Fatalf("expected system and user messages, got %+v", request.Messages)
	}
	systemPrompt := request.Messages[0].Content
	if !strings.Contains(systemPrompt, "response.language") ||
		!strings.Contains(systemPrompt, "workflow.troubleshooting.first_step") ||
		!strings.Contains(systemPrompt, "behavior.avoid") {
		t.Fatalf("expected allowlist keys in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "workflow.first_step") {
		t.Fatalf("expected deprecated workflow key guidance in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "不要输出") || !strings.Contains(systemPrompt, "先分析一下") {
		t.Fatalf("expected troubleshooting content quality guidance in system prompt, got %q", systemPrompt)
	}
}
