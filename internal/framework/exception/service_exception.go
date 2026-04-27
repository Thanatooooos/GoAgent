package exception

import "local/rag-project/internal/framework/errorcode"

// ServiceException represents internal service execution errors.
type ServiceException struct {
	*AppError
}

func NewServiceException(message string, cause error) *ServiceException {
	return NewServiceCodeException(message, cause, errorcode.ServiceError)
}

func NewServiceCodeException(message string, cause error, code errorcode.IErrorCode) *ServiceException {
	return &ServiceException{AppError: NewAppError(message, cause, code)}
}
