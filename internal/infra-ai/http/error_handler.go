package http

import (
	"fmt"
)

// ModelClientException 模型客户端异常
type ModelClientException struct {
	Message    string
	ErrorType  ModelClientErrorType
	StatusCode int
	Cause      error
}

// NewModelClientException 创建模型客户端异常
func NewModelClientException(message string, errorType ModelClientErrorType, statusCode int, cause error) *ModelClientException {
	return &ModelClientException{
		Message:    message,
		ErrorType:  errorType,
		StatusCode: statusCode,
		Cause:      cause,
	}
}

// Error 实现 error 接口
func (e *ModelClientException) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("ModelClientException: %s (type=%s, status=%d, cause=%v)",
			e.Message, e.ErrorType.String(), e.StatusCode, e.Cause)
	}
	return fmt.Sprintf("ModelClientException: %s (type=%s, status=%d)",
		e.Message, e.ErrorType.String(), e.StatusCode)
}

// Unwrap 实现 errors.Is 和 errors.As 支持
func (e *ModelClientException) Unwrap() error {
	return e.Cause
}

// IsModelClientException 检查是否为模型客户端异常
func IsModelClientException(err error) bool {
	_, ok := err.(*ModelClientException)
	return ok
}

// AsModelClientException 将错误转换为模型客户端异常
func AsModelClientException(err error) (*ModelClientException, bool) {
	if e, ok := err.(*ModelClientException); ok {
		return e, true
	}
	return nil, false
}
