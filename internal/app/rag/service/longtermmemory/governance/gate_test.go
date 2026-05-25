package governance

import (
	"testing"

	"local/rag-project/internal/app/rag/domain"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
)

func TestEvaluateExplicitMemoryGateRejectsUnknownCanonicalKey(t *testing.T) {
	_, err := EvaluateExplicitMemoryGate(NormalizedSaveInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		Namespace:    "global:global",
		MemoryType:   domain.MemoryTypeKnowledge,
		Category:     domain.MemoryCategoryGeneral,
		CanonicalKey: "unknown.key",
		ValueType:    domain.MemoryValueTypeText,
		Content:      "content",
	})
	if err == nil {
		t.Fatal("expected error for unknown canonical key")
	}
}

func TestEvaluateExplicitMemoryGateAcceptsCanonicalKeySpecDefaults(t *testing.T) {
	normalized := NormalizeSaveExplicitMemoryInput(memorytypes.SaveExplicitMemoryInput{
		UserID:       "user-1",
		CanonicalKey: "response.language",
		ValueJSON:    "zh-CN",
		DisplayValue: "中文",
		Content:      "以后都用中文回答",
	})
	decision, err := EvaluateExplicitMemoryGate(normalized)
	if err != nil {
		t.Fatalf("evaluateExplicitMemoryGate returned error: %v", err)
	}
	if decision.Spec == nil || decision.Spec.CanonicalKey != "response.language" {
		t.Fatalf("expected key spec to be resolved, got %+v", decision)
	}
	if decision.Input.MemoryType != domain.MemoryTypePreference {
		t.Fatalf("expected memory type to follow spec, got %+v", decision.Input)
	}
	if decision.Input.Category != domain.MemoryCategoryResponse {
		t.Fatalf("expected category to follow spec, got %+v", decision.Input)
	}
	if decision.Input.ValueType != domain.MemoryValueTypeEnum {
		t.Fatalf("expected value type to follow spec, got %+v", decision.Input)
	}
}
