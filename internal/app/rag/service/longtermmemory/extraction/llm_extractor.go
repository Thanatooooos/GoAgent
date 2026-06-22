package extraction

import (
	"encoding/json"
	"strings"

	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	FailureReasonNone        = ""
	FailureReasonLLMCall     = "llm_call_failed"
	FailureReasonInvalidJSON = "invalid_json"

	RejectionReasonNone                  = ""
	RejectionReasonMissingField          = "missing_required_field"
	RejectionReasonInvalidScopeType      = "invalid_scope_type"
	RejectionReasonInvalidMemoryType     = "invalid_memory_type"
	RejectionReasonInvalidCanonicalKey   = "invalid_canonical_key"
	RejectionReasonDeprecatedWorkflowKey = "deprecated_workflow_key"
	RejectionReasonInvalidConfidence     = "invalid_confidence"
)

var phase1ExtractionCanonicalKeys = map[string]struct{}{
	"response.language":                   {},
	"workflow.troubleshooting.first_step": {},
	"behavior.avoid":                      {},
}

type StructuredPreferenceCandidate struct {
	ScopeType    string  `json:"scope_type"`
	MemoryType   string  `json:"memory_type"`
	CanonicalKey string  `json:"canonical_key"`
	Summary      string  `json:"summary"`
	Content      string  `json:"content"`
	Confidence   float64 `json:"confidence"`
}

type ExtractInput struct {
	Message string
}

type ExtractResult struct {
	Candidate       *StructuredPreferenceCandidate
	Rejected        bool
	RejectionReason string
	Failed          bool
	FailureReason   string
	RawResponse     string
}

type LLMPreferenceExtractor struct {
	chatService aichat.LLMService
}

type llmStructuredPreferenceResponse struct {
	ScopeType    string   `json:"scope_type"`
	MemoryType   string   `json:"memory_type"`
	CanonicalKey string   `json:"canonical_key"`
	Summary      string   `json:"summary"`
	Content      string   `json:"content"`
	Confidence   *float64 `json:"confidence"`
}

func NewLLMPreferenceExtractor(chatService aichat.LLMService) *LLMPreferenceExtractor {
	return &LLMPreferenceExtractor{chatService: chatService}
}

func (e *LLMPreferenceExtractor) Extract(input ExtractInput) ExtractResult {
	if e == nil || e.chatService == nil {
		return ExtractResult{
			Failed:        true,
			FailureReason: FailureReasonLLMCall,
		}
	}

	request := buildPreferenceExtractionRequest(input.Message)
	raw, err := e.chatService.ChatWithRequest(request)
	if err != nil {
		return ExtractResult{
			Failed:        true,
			FailureReason: FailureReasonLLMCall,
		}
	}

	parsed, parseErr := parseStructuredPreferenceResponse(raw)
	if parseErr != nil {
		return ExtractResult{
			Failed:        true,
			FailureReason: FailureReasonInvalidJSON,
			RawResponse:   raw,
		}
	}

	candidate, rejectionReason := validateStructuredPreferenceCandidate(parsed)
	if rejectionReason != RejectionReasonNone {
		return ExtractResult{
			Rejected:        true,
			RejectionReason: rejectionReason,
			RawResponse:     raw,
		}
	}

	return ExtractResult{
		Candidate:   &candidate,
		RawResponse: raw,
	}
}

func parseStructuredPreferenceResponse(raw string) (llmStructuredPreferenceResponse, error) {
	raw = strings.TrimSpace(raw)
	if extracted := extractJSONBlock(raw); extracted != "" {
		raw = extracted
	}
	var parsed llmStructuredPreferenceResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return llmStructuredPreferenceResponse{}, err
	}
	return parsed, nil
}

func validateStructuredPreferenceCandidate(raw llmStructuredPreferenceResponse) (StructuredPreferenceCandidate, string) {
	scopeType := normalizeLower(raw.ScopeType)
	memoryType := normalizeLower(raw.MemoryType)
	canonicalKey := normalizeLower(raw.CanonicalKey)
	summary := strings.TrimSpace(raw.Summary)
	content := strings.TrimSpace(raw.Content)

	if scopeType == "" || memoryType == "" || canonicalKey == "" || summary == "" || content == "" || raw.Confidence == nil {
		return StructuredPreferenceCandidate{}, RejectionReasonMissingField
	}
	if scopeType != "global" {
		return StructuredPreferenceCandidate{}, RejectionReasonInvalidScopeType
	}
	if memoryType != "preference" {
		return StructuredPreferenceCandidate{}, RejectionReasonInvalidMemoryType
	}
	if canonicalKey == "workflow.first_step" {
		return StructuredPreferenceCandidate{}, RejectionReasonDeprecatedWorkflowKey
	}
	if _, ok := phase1ExtractionCanonicalKeys[canonicalKey]; !ok {
		return StructuredPreferenceCandidate{}, RejectionReasonInvalidCanonicalKey
	}
	if *raw.Confidence < 0 || *raw.Confidence > 1 {
		return StructuredPreferenceCandidate{}, RejectionReasonInvalidConfidence
	}

	return StructuredPreferenceCandidate{
		ScopeType:    scopeType,
		MemoryType:   memoryType,
		CanonicalKey: canonicalKey,
		Summary:      summary,
		Content:      content,
		Confidence:   *raw.Confidence,
	}, RejectionReasonNone
}

func extractJSONBlock(raw string) string {
	markerStart := strings.Index(raw, "```json")
	if markerStart == -1 {
		markerStart = strings.Index(raw, "```")
	}
	if markerStart == -1 {
		return ""
	}
	contentStart := strings.IndexByte(raw[markerStart:], '\n')
	if contentStart == -1 {
		return ""
	}
	contentStart += markerStart + 1

	end := strings.Index(raw[contentStart:], "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(raw[contentStart : contentStart+end])
}

func normalizeLower(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
