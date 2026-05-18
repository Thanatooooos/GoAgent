package service

import (
	"fmt"
	"reflect"
	"strings"
)

func evaluateWorkflowCondition(condition map[string]any, state ExecutionState) (bool, error) {
	if len(condition) == 0 {
		return true, nil
	}
	if items, ok := readConditionItems(condition, "all"); ok {
		for _, item := range items {
			matched, err := evaluateWorkflowCondition(item, state)
			if err != nil {
				return false, err
			}
			if !matched {
				return false, nil
			}
		}
		return true, nil
	}
	if items, ok := readConditionItems(condition, "any"); ok {
		for _, item := range items {
			matched, err := evaluateWorkflowCondition(item, state)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	}

	path := readStringSetting(condition, "path")
	op := strings.ToLower(readStringSetting(condition, "op"))
	if path == "" || op == "" {
		return false, fmt.Errorf("condition requires path and op")
	}

	actual, exists := resolveConditionPath(path, state)
	switch op {
	case "exists":
		return exists, nil
	case "eq":
		return compareConditionValues(actual, condition["value"]) == 0, nil
	case "ne":
		return compareConditionValues(actual, condition["value"]) != 0, nil
	case "gt":
		return compareConditionValues(actual, condition["value"]) > 0, nil
	case "gte":
		return compareConditionValues(actual, condition["value"]) >= 0, nil
	case "lt":
		return compareConditionValues(actual, condition["value"]) < 0, nil
	case "lte":
		return compareConditionValues(actual, condition["value"]) <= 0, nil
	case "in":
		return conditionValueIn(actual, condition["value"]), nil
	default:
		return false, fmt.Errorf("unsupported condition op: %s", op)
	}
}

func readConditionItems(condition map[string]any, key string) ([]map[string]any, bool) {
	raw, ok := condition[key]
	if !ok || raw == nil {
		return nil, false
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	result := make([]map[string]any, 0, len(list))
	for _, item := range list {
		mapped, ok := item.(map[string]any)
		if ok && len(mapped) > 0 {
			result = append(result, mapped)
		}
	}
	return result, true
}

func resolveConditionPath(path string, state ExecutionState) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return nil, false
	}
	switch segments[0] {
	case "task":
		if len(segments) >= 2 && segments[1] == "metadata" {
			return readNestedValue(state.Task.Metadata, segments[2:])
		}
	case "nodeOutputs":
		if len(segments) < 2 {
			return nil, false
		}
		nodeOutput, ok := state.NodeOutputs[segments[1]]
		if !ok {
			return nil, false
		}
		return readNestedValue(nodeOutput, segments[2:])
	case "artifacts":
		return readNestedValue(state.Artifacts, segments[1:])
	}
	return nil, false
}

func readNestedValue(current any, segments []string) (any, bool) {
	if len(segments) == 0 {
		return current, true
	}
	mapped, ok := current.(map[string]any)
	if !ok {
		return nil, false
	}
	next, exists := mapped[segments[0]]
	if !exists {
		return nil, false
	}
	return readNestedValue(next, segments[1:])
}

func compareConditionValues(left any, right any) int {
	if leftFloat, ok := asFloat64(left); ok {
		if rightFloat, ok := asFloat64(right); ok {
			switch {
			case leftFloat < rightFloat:
				return -1
			case leftFloat > rightFloat:
				return 1
			default:
				return 0
			}
		}
	}
	leftString := strings.TrimSpace(fmt.Sprint(left))
	rightString := strings.TrimSpace(fmt.Sprint(right))
	switch {
	case leftString < rightString:
		return -1
	case leftString > rightString:
		return 1
	default:
		return 0
	}
}

func conditionValueIn(actual any, raw any) bool {
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if compareConditionValues(actual, item) == 0 {
				return true
			}
		}
	case []string:
		for _, item := range typed {
			if compareConditionValues(actual, item) == 0 {
				return true
			}
		}
	default:
		value := reflect.ValueOf(raw)
		if value.IsValid() && value.Kind() == reflect.Slice {
			for i := 0; i < value.Len(); i++ {
				if compareConditionValues(actual, value.Index(i).Interface()) == 0 {
					return true
				}
			}
		}
	}
	return false
}

func asFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}
