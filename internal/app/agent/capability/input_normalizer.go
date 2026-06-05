package capability

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// InputNormalizer allows a capability to accept generic structured input,
// such as LLM-produced JSON objects, and normalize it into the capability's
// typed invocation input.
type InputNormalizer interface {
	NormalizeInput(raw any) (any, error)
}

// DecodeStructuredInput converts a generic JSON-like payload into a typed
// capability input while still accepting already-typed values directly.
func DecodeStructuredInput[T any](raw any, requiredMessage string) (T, error) {
	var zero T
	if raw == nil {
		if requiredMessage == "" {
			requiredMessage = "capability input is required"
		}
		return zero, fmt.Errorf("%s", requiredMessage)
	}
	value := reflect.ValueOf(raw)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			if requiredMessage == "" {
				requiredMessage = "capability input is required"
			}
			return zero, fmt.Errorf("%s", requiredMessage)
		}
		value = value.Elem()
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return zero, fmt.Errorf("marshal capability input: %w", err)
	}
	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return zero, fmt.Errorf("decode capability input: %w", err)
	}
	return decoded, nil
}
