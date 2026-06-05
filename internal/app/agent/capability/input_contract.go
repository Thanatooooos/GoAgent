package capability

import "fmt"

// ResolvePreconditionFallback allows a capability to opt out of resolver-side
// rejection for specific precondition failures and handle them during Invoke,
// for example by returning a skipped result instead of a degraded one.
type ResolvePreconditionFallback interface {
	ResolveOnPreconditionFailure(err error) bool
}

// ResolverInputNormalizer allows resolver-specific input normalization without
// changing the capability's general NormalizeInput behavior used by other
// runtime paths.
type ResolverInputNormalizer interface {
	NormalizeResolverInput(raw any) (any, error)
}

// DecodeInput normalizes generic structured input into the typed capability
// input contract without applying declared preconditions.
func DecodeInput[T any](raw any, requiredMessage string, inputLabel string) (T, error) {
	var zero T

	input, err := DecodeStructuredInput[T](raw, requiredMessage)
	if err != nil {
		if inputLabel == "" {
			inputLabel = "capability input"
		}
		return zero, fmt.Errorf("%s has unexpected type %T: %w", inputLabel, raw, err)
	}
	return input, nil
}

// DecodeAndValidateInput normalizes generic structured input into the typed
// input contract for a capability and then validates the declared
// preconditions against the typed value.
func DecodeAndValidateInput[T any](spec Spec, raw any, requiredMessage string, inputLabel string) (T, error) {
	input, err := DecodeInput[T](raw, requiredMessage, inputLabel)
	if err != nil {
		var zero T
		return zero, err
	}
	if err := ValidateInput(spec, input); err != nil {
		var zero T
		return zero, err
	}
	return input, nil
}
