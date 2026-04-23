package exception

import "local/rag-project/internal/framework/errorcode"

// AbstractException 抽象项目中三类异常体系，客户端异常、服务端异常以及远程服务调用异常
type AbstractException struct {
	errorCode    string
	errorMessage string
	cause        error
}

// NewAbstractException 创建抽象异常
func NewAbstractException(message string, cause error, errorCode errorcode.IErrorCode) *AbstractException {
	var errMsg string
	if message != "" {
		errMsg = message
	} else {
		errMsg = errorCode.Message()
	}

	return &AbstractException{
		errorCode:    errorCode.Code(),
		errorMessage: errMsg,
		cause:        cause,
	}
}

func (e *AbstractException) Error() string {
	return e.errorMessage
}

func (e *AbstractException) ErrorCode() string {
	return e.errorCode
}

func (e *AbstractException) ErrorMessage() string {
	return e.errorMessage
}

func (e *AbstractException) Cause() error {
	return e.cause
}

func (e *AbstractException) Unwrap() error {
	return e.cause
}
