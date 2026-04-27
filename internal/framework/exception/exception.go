package exception

import "local/rag-project/internal/framework/errorcode"

// AppError is the unified application error type.
type AppError struct {
	code    string
	message string
	cause   error
}

func NewAppError(message string, cause error, errorCode errorcode.IErrorCode) *AppError {
	if message == "" {
		message = errorCode.Message()
	}

	return &AppError{
		code:    errorCode.Code(),
		message: message,
		cause:   cause,
	}
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *AppError) ErrorCode() string {
	if e == nil {
		return ""
	}
	return e.code
}

func (e *AppError) ErrorMessage() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
