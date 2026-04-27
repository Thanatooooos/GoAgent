package exception

import "local/rag-project/internal/framework/errorcode"

// ClientException represents client-side request errors.
type ClientException struct {
	*AppError
}

func NewClientException(message string, cause error) *ClientException {
	return NewClientCodeException(message, cause, errorcode.ClientError)
}

func NewClientCodeException(message string, cause error, code errorcode.IErrorCode) *ClientException {
	return &ClientException{AppError: NewAppError(message, cause, code)}
}
