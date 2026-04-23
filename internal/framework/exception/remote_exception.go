package exception

import (
	"fmt"
	"local/rag-project/internal/framework/errorcode"
)

// RemoteException 远程服务调用异常
// 比如订单调用支付失败，向上抛出的异常应该是远程服务调用异常
type RemoteException struct {
	*AbstractException
}

// NewRemoteException 创建远程服务异常
func NewRemoteException(args ...interface{}) *RemoteException {
	var message string
	var cause error
	var errorCode errorcode.IErrorCode = errorcode.RemoteError

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

	return &RemoteException{
		AbstractException: NewAbstractException(message, cause, errorCode),
	}
}

func (e *RemoteException) Error() string {
	return fmt.Sprintf("RemoteException{code='%s', message='%s'}", e.errorCode, e.errorMessage)
}

func (e *RemoteException) String() string {
	return e.Error()
}
