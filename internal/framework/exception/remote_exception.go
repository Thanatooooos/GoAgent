package exception

import "local/rag-project/internal/framework/errorcode"

// RemoteException represents downstream/third-party invocation errors.
type RemoteException struct {
	*AppError
}

func NewRemoteException(message string, cause error) *RemoteException {
	return NewRemoteCodeException(message, cause, errorcode.RemoteError)
}

func NewRemoteCodeException(message string, cause error, code errorcode.IErrorCode) *RemoteException {
	return &RemoteException{AppError: NewAppError(message, cause, code)}
}
