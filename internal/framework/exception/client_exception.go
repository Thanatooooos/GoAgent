package exception

import (
	"fmt"
	"local/rag-project/internal/framework/errorcode"
)

// ClientException 客户端异常
// 用户发起调用请求后因客户端提交参数或其他客户端问题导致的异常
type ClientException struct {
	*AbstractException
}

// NewClientException 创建客户端异常
func NewClientException(args ...interface{}) *ClientException {
	var message string
	var cause error
	var errorCode errorcode.IErrorCode = errorcode.ClientError

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

	return &ClientException{
		AbstractException: NewAbstractException(message, cause, errorCode),
	}
}

func (e *ClientException) Error() string {
	return fmt.Sprintf("ClientException{code='%s', message='%s'}", e.errorCode, e.errorMessage)
}

func (e *ClientException) String() string {
	return e.Error()
}
