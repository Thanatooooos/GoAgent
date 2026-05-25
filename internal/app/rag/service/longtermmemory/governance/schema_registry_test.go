package governance

import (
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestLookupMemoryKeySpecReturnsExpectedProjectIntegrationSpec(t *testing.T) {
	spec, ok := lookupMemoryKeySpec("project.integrations")
	if !ok {
		t.Fatal("expected project.integrations spec to exist")
	}
	if spec.Category != domain.MemoryCategoryProject {
		t.Fatalf("unexpected category: %+v", spec)
	}
	if spec.MemoryType != domain.MemoryTypeKnowledge {
		t.Fatalf("unexpected memory type: %+v", spec)
	}
	if spec.ValueType != domain.MemoryValueTypeText {
		t.Fatalf("unexpected value type: %+v", spec)
	}
	if spec.Cardinality != MemoryCardinalityMulti {
		t.Fatalf("unexpected cardinality: %+v", spec)
	}
	if !allowsMemoryScope(spec, domain.MemoryScopeKB) || !allowsMemoryScope(spec, domain.MemoryScopeGlobal) {
		t.Fatalf("unexpected scope allowance: %+v", spec)
	}
}
