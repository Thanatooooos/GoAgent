package governance

import (
	"sort"
	"strings"

	"local/rag-project/internal/app/rag/domain"
)

var canonicalMemoryKeyRegistry = map[string]MemoryKeySpec{
	"response.language": {
		CanonicalKey:      "response.language",
		Category:          domain.MemoryCategoryResponse,
		MemoryType:        domain.MemoryTypePreference,
		ValueType:         domain.MemoryValueTypeEnum,
		Cardinality:       MemoryCardinalitySingle,
		DefaultImportance: 100,
		AllowedScopeTypes: []string{domain.MemoryScopeGlobal, domain.MemoryScopeKB},
	},
	"workflow.first_step": {
		CanonicalKey:      "workflow.first_step",
		Category:          domain.MemoryCategoryWorkflow,
		MemoryType:        domain.MemoryTypePreference,
		ValueType:         domain.MemoryValueTypeText,
		Cardinality:       MemoryCardinalitySingle,
		DefaultImportance: 90,
		AllowedScopeTypes: []string{domain.MemoryScopeGlobal, domain.MemoryScopeKB},
	},
	"behavior.avoid": {
		CanonicalKey:      "behavior.avoid",
		Category:          domain.MemoryCategoryBehavior,
		MemoryType:        domain.MemoryTypePreference,
		ValueType:         domain.MemoryValueTypeText,
		Cardinality:       MemoryCardinalityMulti,
		DefaultImportance: 80,
		AllowedScopeTypes: []string{domain.MemoryScopeGlobal, domain.MemoryScopeKB},
	},
	"project.constraint.network": {
		CanonicalKey:      "project.constraint.network",
		Category:          domain.MemoryCategoryProject,
		MemoryType:        domain.MemoryTypeKnowledge,
		ValueType:         domain.MemoryValueTypeText,
		Cardinality:       MemoryCardinalitySingle,
		DefaultImportance: 90,
		AllowedScopeTypes: []string{domain.MemoryScopeGlobal, domain.MemoryScopeKB},
	},
	"project.messaging.main_bus": {
		CanonicalKey:      "project.messaging.main_bus",
		Category:          domain.MemoryCategoryProject,
		MemoryType:        domain.MemoryTypeKnowledge,
		ValueType:         domain.MemoryValueTypeText,
		Cardinality:       MemoryCardinalitySingle,
		DefaultImportance: 90,
		AllowedScopeTypes: []string{domain.MemoryScopeGlobal, domain.MemoryScopeKB},
	},
	"project.fact.dependencies": {
		CanonicalKey:      "project.fact.dependencies",
		Category:          domain.MemoryCategoryProject,
		MemoryType:        domain.MemoryTypeKnowledge,
		ValueType:         domain.MemoryValueTypeText,
		Cardinality:       MemoryCardinalityMulti,
		DefaultImportance: 70,
		AllowedScopeTypes: []string{domain.MemoryScopeGlobal, domain.MemoryScopeKB},
	},
	"project.integrations": {
		CanonicalKey:      "project.integrations",
		Category:          domain.MemoryCategoryProject,
		MemoryType:        domain.MemoryTypeKnowledge,
		ValueType:         domain.MemoryValueTypeText,
		Cardinality:       MemoryCardinalityMulti,
		DefaultImportance: 70,
		AllowedScopeTypes: []string{domain.MemoryScopeGlobal, domain.MemoryScopeKB},
	},
}

func lookupMemoryKeySpec(canonicalKey string) (MemoryKeySpec, bool) {
	spec, ok := canonicalMemoryKeyRegistry[strings.TrimSpace(strings.ToLower(canonicalKey))]
	return spec, ok
}

func allowsMemoryScope(spec MemoryKeySpec, scopeType string) bool {
	scopeType = strings.TrimSpace(scopeType)
	for _, allowed := range spec.AllowedScopeTypes {
		if strings.TrimSpace(allowed) == scopeType {
			return true
		}
	}
	return false
}

func SingleValuedCanonicalKeys() []string {
	result := make([]string, 0, len(canonicalMemoryKeyRegistry))
	for key, spec := range canonicalMemoryKeyRegistry {
		if spec.Cardinality == MemoryCardinalitySingle {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}
