package governance

import (
	"strings"

	"local/rag-project/internal/app/rag/domain"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
	"local/rag-project/internal/framework/log"
)

func NormalizeSaveExplicitMemoryInput(input memorytypes.SaveExplicitMemoryInput) NormalizedSaveInput {
	canonicalKey := normalizeCanonicalKey(input.CanonicalKey)
	spec, hasSpec := lookupMemoryKeySpec(canonicalKey)

	scopeType := normalizeMemoryScopeType(input.ScopeType)
	scopeID := strings.TrimSpace(input.ScopeID)
	memoryType := strings.ToLower(strings.TrimSpace(input.MemoryType))
	if memoryType == "" {
		if hasSpec {
			memoryType = spec.MemoryType
		} else {
			memoryType = domain.MemoryTypeKnowledge
		}
	}

	category := strings.ToLower(strings.TrimSpace(input.Category))
	if category == "" {
		if hasSpec {
			category = spec.Category
		} else {
			category = defaultMemoryCategory(memoryType)
		}
	}

	valueType := strings.ToLower(strings.TrimSpace(input.ValueType))
	if valueType == "" {
		if hasSpec {
			valueType = spec.ValueType
		} else {
			valueType = domain.MemoryValueTypeText
		}
	}

	importance := input.Importance
	if importance <= 0 && hasSpec {
		importance = spec.DefaultImportance
	}

	summary := strings.TrimSpace(input.Summary)
	content := strings.TrimSpace(input.Content)
	displayValue := strings.TrimSpace(input.DisplayValue)
	if displayValue == "" {
		if summary != "" {
			displayValue = summary
		} else {
			displayValue = summarizeMemoryText(content, memorytypes.DefaultMemorySummaryRunes)
		}
	}

	valueJSON := strings.TrimSpace(input.ValueJSON)
	if valueJSON == "" {
		if normalizeValueType(valueType) == domain.MemoryValueTypeJSON {
			if canonical, ok := canonicalizeJSONObject(content); ok {
				valueJSON = canonical
			} else {
				log.Warnf(
					"long-term memory normalized json value missing valueJSON: userID=%s canonicalKey=%s scopeType=%s scopeID=%s",
					strings.TrimSpace(input.UserID),
					canonicalKey,
					scopeType,
					scopeID,
				)
			}
		} else {
			valueJSON = content
		}
	}

	extractionMethod := strings.ToLower(strings.TrimSpace(input.ExtractionMethod))
	if extractionMethod == "" {
		extractionMethod = domain.MemoryExtractionMethodManual
	}

	namespace := normalizeMemoryNamespace(input.Namespace, scopeType, scopeID)

	return NormalizedSaveInput{
		UserID:           strings.TrimSpace(input.UserID),
		ScopeType:        scopeType,
		ScopeID:          scopeID,
		Namespace:        namespace,
		MemoryType:       memoryType,
		Category:         category,
		CanonicalKey:     canonicalKey,
		ValueType:        valueType,
		ValueJSON:        valueJSON,
		DisplayValue:     displayValue,
		SourceMessageID:  strings.TrimSpace(input.SourceMessageID),
		Content:          content,
		Summary:          summary,
		Importance:       importance,
		ExtractionMethod: extractionMethod,
		ExpiresAt:        input.ExpiresAt,
	}
}

func normalizeMemoryScopeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domain.MemoryScopeGlobal
	}
	return value
}

func normalizeMemoryType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domain.MemoryTypeKnowledge
	}
	return value
}

func normalizeMemoryNamespace(value string, scopeType string, scopeID string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	if strings.TrimSpace(scopeType) == domain.MemoryScopeKB && strings.TrimSpace(scopeID) != "" {
		return scopeType + ":" + scopeID
	}
	return scopeType + ":global"
}

func normalizeCanonicalKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeValueType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domain.MemoryValueTypeText
	}
	return value
}

func defaultMemoryCategory(memoryType string) string {
	switch strings.TrimSpace(memoryType) {
	case domain.MemoryTypePreference:
		return domain.MemoryCategoryBehavior
	case domain.MemoryTypeFeedback:
		return domain.MemoryCategoryFeedback
	default:
		return domain.MemoryCategoryGeneral
	}
}

func isSupportedMemoryScopeType(value string) bool {
	return value == domain.MemoryScopeGlobal || value == domain.MemoryScopeKB
}

func isSupportedMemoryType(value string) bool {
	return value == domain.MemoryTypePreference || value == domain.MemoryTypeKnowledge || value == domain.MemoryTypeFeedback
}

func isSupportedMemoryValueType(value string) bool {
	switch strings.TrimSpace(value) {
	case domain.MemoryValueTypeText, domain.MemoryValueTypeEnum, domain.MemoryValueTypeBoolean, domain.MemoryValueTypeJSON:
		return true
	default:
		return false
	}
}

func isSupportedMemoryExtractionMethod(value string) bool {
	switch strings.TrimSpace(value) {
	case domain.MemoryExtractionMethodManual, domain.MemoryExtractionMethodRule, domain.MemoryExtractionMethodLLM:
		return true
	default:
		return false
	}
}
