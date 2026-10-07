package capability

import "errors"

// DeniedError marks a call rejected by runtime policy rather than a broken
// tool, malformed model argument, or infrastructure failure.
type DeniedError struct{ Reason string }

func (e *DeniedError) Error() string { return e.Reason }

func Deny(reason string) error { return &DeniedError{Reason: reason} }

func IsDenied(err error) bool {
	var denied *DeniedError
	return errors.As(err, &denied)
}
