package capability

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// PreconditionError reports an invocation input contract violation.
type PreconditionError struct {
	Field       string
	Requirement string
}

func (e PreconditionError) Error() string {
	field := strings.TrimSpace(e.Field)
	requirement := strings.TrimSpace(e.Requirement)
	if field == "" && requirement == "" {
		return "capability precondition failed"
	}
	if field == "" {
		return fmt.Sprintf("capability precondition failed: %s", requirement)
	}
	if requirement == "" {
		return fmt.Sprintf("capability precondition failed: %s", field)
	}
	return fmt.Sprintf("capability precondition failed: %s must satisfy %s", field, requirement)
}

func IsPreconditionError(err error) bool {
	var target PreconditionError
	return errors.As(err, &target)
}

// ValidateInput checks the declared input preconditions against the provided invocation input.
func ValidateInput(spec Spec, input any) error {
	if len(spec.Preconditions) == 0 {
		return nil
	}
	value := reflect.ValueOf(input)
	if !value.IsValid() {
		return PreconditionError{}
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return PreconditionError{}
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return fmt.Errorf("capability input has unsupported shape %T for precondition validation", input)
	}
	for _, precondition := range spec.Preconditions {
		if err := validatePrecondition(value, precondition); err != nil {
			return err
		}
	}
	return nil
}

func validatePrecondition(value reflect.Value, precondition Precondition) error {
	fieldValue, ok := findFieldValue(value, precondition.Field)
	if !ok {
		return PreconditionError{Field: precondition.Field, Requirement: precondition.Requirement}
	}
	switch strings.TrimSpace(precondition.Requirement) {
	case PreconditionRequirementNonEmpty:
		if isEmptyValue(fieldValue) {
			return PreconditionError{Field: precondition.Field, Requirement: precondition.Requirement}
		}
	}
	return nil
}

func findFieldValue(value reflect.Value, field string) (reflect.Value, bool) {
	typ := value.Type()
	trimmedField := strings.TrimSpace(field)
	for i := 0; i < typ.NumField(); i++ {
		structField := typ.Field(i)
		if strings.EqualFold(structField.Name, trimmedField) {
			return value.Field(i), true
		}
		jsonTag := strings.Split(structField.Tag.Get("json"), ",")[0]
		if jsonTag != "" && strings.EqualFold(jsonTag, trimmedField) {
			return value.Field(i), true
		}
	}
	return reflect.Value{}, false
}

func isEmptyValue(value reflect.Value) bool {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return true
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.String:
		return strings.TrimSpace(value.String()) == ""
	case reflect.Slice, reflect.Array, reflect.Map:
		return value.Len() == 0
	default:
		zero := reflect.Zero(value.Type())
		return reflect.DeepEqual(value.Interface(), zero.Interface())
	}
}
