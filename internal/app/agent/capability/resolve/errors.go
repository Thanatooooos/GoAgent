package resolve

import "fmt"

type NotFoundError struct {
	Message string
}

func (e NotFoundError) Error() string {
	if e.Message == "" {
		return "capability selection could not be resolved"
	}
	return e.Message
}

type AmbiguousError struct {
	Message string
}

func (e AmbiguousError) Error() string {
	if e.Message == "" {
		return "capability selection resolved to multiple capabilities"
	}
	return e.Message
}

type InvalidInputError struct {
	Name string
	Err  error
}

func (e InvalidInputError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("capability %q input is invalid", e.Name)
	}
	return fmt.Sprintf("capability %q input is invalid: %v", e.Name, e.Err)
}

func (e InvalidInputError) Unwrap() error {
	return e.Err
}
