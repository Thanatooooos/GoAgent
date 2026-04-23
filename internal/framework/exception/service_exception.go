package exception

import (
	"fmt"
	"local/rag-project/internal/framework/errorcode"
)

// ServiceException 服务端运行异常
// 请求运行过程中出现的不符合业务预期的异常
type ServiceException struct {
	*AbstractException
}

// NewServiceException 创建服务异常
func NewServiceException(args ...interface{}) *ServiceException {
	var message string
	var cause error
	var errorCode errorcode.IErrorCode = errorcode.ServiceError

	// 解析参数
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			message = v
		case error:
			cause = v
		case errorcode.IErrorCode:
			errorCode = v
		}
	}

	// 如果 message 为空，使用 errorCode 的 message
	if message == "" {
		message = errorCode.Message()
	}

	return &ServiceException{
		AbstractException: NewAbstractException(message, cause, errorCode),
	}
}

func (e *ServiceException) Error() string {
	return fmt.Sprintf("ServiceException{code='%s', message='%s'}", e.errorCode, e.errorMessage)
}

func (e *ServiceException) String() string {
	return e.Error()
}
