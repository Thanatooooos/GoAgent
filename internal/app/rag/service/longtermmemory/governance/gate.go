package governance

import (
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/exception"
)

func EvaluateExplicitMemoryGate(input NormalizedSaveInput) (GateDecision, error) {
	if strings.TrimSpace(input.UserID) == "" {
		return GateDecision{}, exception.NewClientException("user id is required", nil)
	}
	if strings.TrimSpace(input.Content) == "" {
		return GateDecision{}, exception.NewClientException("memory content is required", nil)
	}
	if !isSupportedMemoryScopeType(input.ScopeType) {
		return GateDecision{}, exception.NewClientException("memory scope type must be global or kb", nil)
	}
	if input.ScopeType == domain.MemoryScopeKB && strings.TrimSpace(input.ScopeID) == "" {
		return GateDecision{}, exception.NewClientException("scope id is required for kb-scoped memory", nil)
	}
	if !isSupportedMemoryType(input.MemoryType) {
		return GateDecision{}, exception.NewClientException("memory type must be preference, knowledge, or feedback", nil)
	}
	if !isSupportedMemoryValueType(normalizeValueType(input.ValueType)) {
		return GateDecision{}, exception.NewClientException("memory value type must be text, enum, boolean, or json", nil)
	}
	if !isSupportedMemoryExtractionMethod(input.ExtractionMethod) {
		return GateDecision{}, exception.NewClientException("memory extraction method must be manual, explicit_rule, or explicit_llm", nil)
	}

	if strings.TrimSpace(input.CanonicalKey) == "" {
		return GateDecision{
			Action: GateDecisionCreate,
			Input:  input,
		}, nil
	}

	spec, ok := lookupMemoryKeySpec(input.CanonicalKey)
	if !ok {
		return GateDecision{}, exception.NewClientException("canonical key is not supported", nil)
	}
	if strings.TrimSpace(input.Category) != strings.TrimSpace(spec.Category) {
		return GateDecision{}, exception.NewClientException("memory category does not match canonical key", nil)
	}
	if strings.TrimSpace(input.MemoryType) != strings.TrimSpace(spec.MemoryType) {
		return GateDecision{}, exception.NewClientException("memory type does not match canonical key", nil)
	}
	if normalizeValueType(input.ValueType) != strings.TrimSpace(spec.ValueType) {
		return GateDecision{}, exception.NewClientException("memory value type does not match canonical key", nil)
	}
	if !allowsMemoryScope(spec, input.ScopeType) {
		return GateDecision{}, exception.NewClientException("memory scope type is not allowed for canonical key", nil)
	}
	return GateDecision{
		Action: GateDecisionCreate,
		Input:  input,
		Spec:   &spec,
	}, nil
}
