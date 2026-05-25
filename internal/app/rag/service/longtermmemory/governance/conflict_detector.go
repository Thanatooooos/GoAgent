package governance

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"
	"unicode"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/exception"
)

func DetectMemoryConflict(
	ctx context.Context,
	repo port.MemoryItemRepository,
	now func() time.Time,
	decision GateDecision,
	candidate domain.MemoryItem,
) (ConflictResolution, error) {
	if repo == nil {
		return ConflictResolution{}, exception.NewServiceException("memory item repository is required", nil)
	}
	if decision.Spec == nil || strings.TrimSpace(decision.Input.CanonicalKey) == "" {
		return ConflictResolution{
			Action:          GateDecisionCreate,
			CreateCandidate: &candidate,
		}, nil
	}

	active, err := repo.ListActiveByCanonicalKey(
		ctx,
		strings.TrimSpace(decision.Input.UserID),
		decision.Input.ScopeType,
		decision.Input.ScopeID,
		decision.Input.CanonicalKey,
	)
	if err != nil {
		return ConflictResolution{}, exception.NewServiceException("failed to load active memory items", err)
	}

	if decision.Input.MemoryType == domain.MemoryTypeFeedback {
		for idx := range active {
			if MemoryItemsEquivalent(active[idx], candidate) {
				return ConflictResolution{
					Action:   GateDecisionIgnore,
					Existing: &active[idx],
				}, nil
			}
		}
		return ConflictResolution{
			Action:          GateDecisionCreate,
			CreateCandidate: &candidate,
		}, nil
	}

	if decision.Spec.Cardinality == MemoryCardinalitySingle {
		if len(active) > 1 {
			return ConflictResolution{}, exception.NewServiceException(
				"multiple active memory items detected for single-valued canonical key",
				nil,
			)
		}
		if len(active) == 0 {
			return ConflictResolution{
				Action:          GateDecisionCreate,
				CreateCandidate: &candidate,
			}, nil
		}
		existing := active[0]
		if MemoryItemsEquivalent(existing, candidate) {
			updated := refreshExistingMemory(existing, candidate, now())
			return ConflictResolution{
				Action:          GateDecisionUpdate,
				Existing:        &existing,
				UpdatedExisting: &updated,
			}, nil
		}
		superseded := markMemorySuperseded(existing, candidate.UpdatedBy, now())
		created := candidate
		created.SupersedesID = existing.ID
		return ConflictResolution{
			Action:          GateDecisionCreate,
			Existing:        &existing,
			UpdatedExisting: &superseded,
			CreateCandidate: &created,
		}, nil
	}

	for idx := range active {
		if MemoryItemsEquivalent(active[idx], candidate) {
			updated := refreshExistingMemory(active[idx], candidate, now())
			return ConflictResolution{
				Action:          GateDecisionMerge,
				Existing:        &active[idx],
				UpdatedExisting: &updated,
			}, nil
		}
	}
	return ConflictResolution{
		Action:          GateDecisionCreate,
		CreateCandidate: &candidate,
	}, nil
}

func MemoryItemsEquivalent(left domain.MemoryItem, right domain.MemoryItem) bool {
	if normalizeValueType(left.ValueType) == domain.MemoryValueTypeJSON && normalizeValueType(right.ValueType) == domain.MemoryValueTypeJSON {
		if strings.TrimSpace(left.ValueJSON) != "" && strings.TrimSpace(right.ValueJSON) != "" && jsonValuesEquivalent(left.ValueJSON, right.ValueJSON) {
			return true
		}
	}
	if comparableMemoryValue(left.ValueType, left.ValueJSON) != "" && comparableMemoryValue(right.ValueType, right.ValueJSON) != "" {
		return comparableMemoryValue(left.ValueType, left.ValueJSON) == comparableMemoryValue(right.ValueType, right.ValueJSON)
	}
	if comparableDisplayValue(left.Content) != "" && comparableDisplayValue(right.Content) != "" {
		return comparableDisplayValue(left.Content) == comparableDisplayValue(right.Content)
	}
	if comparableDisplayValue(left.DisplayValue) != "" && comparableDisplayValue(right.DisplayValue) != "" {
		return comparableDisplayValue(left.DisplayValue) == comparableDisplayValue(right.DisplayValue)
	}
	return false
}

func comparableMemoryValue(valueType string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch normalizeValueType(valueType) {
	case domain.MemoryValueTypeEnum, domain.MemoryValueTypeBoolean:
		return comparableDisplayValue(strings.ToLower(value))
	case domain.MemoryValueTypeJSON:
		if canonical, ok := canonicalizeJSONObject(value); ok {
			return canonical
		}
		return collapseInnerWhitespace(value)
	default:
		return comparableDisplayValue(value)
	}
}

func canonicalizeJSONObject(value string) (string, bool) {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return "", false
	}
	normalized := normalizeJSONValue(decoded)
	bytes, err := json.Marshal(normalized)
	if err != nil {
		return "", false
	}
	return string(bytes), true
}

func normalizeJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[key] = normalizeJSONValue(item)
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, normalizeJSONValue(item))
		}
		return normalized
	default:
		return typed
	}
}

func jsonValuesEquivalent(left string, right string) bool {
	leftCanonical, leftOK := canonicalizeJSONObject(left)
	rightCanonical, rightOK := canonicalizeJSONObject(right)
	if leftOK && rightOK {
		return leftCanonical == rightCanonical
	}

	var leftDecoded any
	if err := json.Unmarshal([]byte(left), &leftDecoded); err != nil {
		return false
	}
	var rightDecoded any
	if err := json.Unmarshal([]byte(right), &rightDecoded); err != nil {
		return false
	}
	return reflect.DeepEqual(normalizeJSONValue(leftDecoded), normalizeJSONValue(rightDecoded))
}

func comparableDisplayValue(value string) string {
	return strings.ToLower(collapseInnerWhitespace(value))
}

func collapseInnerWhitespace(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r)
	}), " ")
}
