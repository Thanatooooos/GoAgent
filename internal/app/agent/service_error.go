package agent

import "errors"

const (
	ErrorKindInvalidRequest     = "invalid_request"
	ErrorKindNotFound           = "not_found"
	ErrorKindFailedPrecondition = "failed_precondition"
	ErrorKindUnavailable        = "unavailable"
	ErrorKindInternal           = "internal"

	ErrorCodeServiceNotInitialized       = "service_not_initialized"
	ErrorCodeQuestionRequired            = "question_required"
	ErrorCodeSessionStoreNotInitialized  = "session_store_not_initialized"
	ErrorCodeCheckpointIDRequired        = "checkpoint_id_required"
	ErrorCodeApprovalDecisionInvalid     = "approval_decision_invalid"
	ErrorCodeApprovalSessionLoadFailed   = "approval_session_load_failed"
	ErrorCodeApprovalSessionSaveFailed   = "approval_session_save_failed"
	ErrorCodeApprovalSessionDeleteFailed = "approval_session_delete_failed"
	ErrorCodeApprovalSessionNotFound     = "approval_session_not_found"
	ErrorCodeApprovalNotPending          = "approval_not_pending"
	ErrorCodeRuntimeExecutionFailed      = "runtime_execution_failed"
)

type ServiceErrorDescriptor struct {
	Code      string
	Message   string
	Kind      string
	Retryable bool
}

type ServiceError struct {
	Code      string
	Message   string
	Kind      string
	Retryable bool
	Operation string
	Err       error
}

func (e *ServiceError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "agent service error"
}

func (e *ServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func serviceError(code string, message string) error {
	spec := describeServiceErrorCode(code)
	return &ServiceError{
		Code:      code,
		Message:   firstNonEmpty(message, spec.Message),
		Kind:      spec.Kind,
		Retryable: spec.Retryable,
	}
}

func serviceErrorWrap(code string, message string, operation string, err error) error {
	spec := describeServiceErrorCode(code)
	return &ServiceError{
		Code:      code,
		Message:   firstNonEmpty(message, spec.Message),
		Kind:      spec.Kind,
		Retryable: spec.Retryable,
		Operation: operation,
		Err:       err,
	}
}

func ServiceErrorCode(err error) string {
	var target *ServiceError
	if errors.As(err, &target) && target != nil {
		return target.Code
	}
	return ""
}

func DescribeServiceError(err error) ServiceErrorDescriptor {
	if err == nil {
		return ServiceErrorDescriptor{}
	}
	var target *ServiceError
	if errors.As(err, &target) && target != nil {
		return ServiceErrorDescriptor{
			Code:      target.Code,
			Message:   target.Error(),
			Kind:      firstNonEmpty(target.Kind, ErrorKindInternal),
			Retryable: target.Retryable,
		}
	}
	return ServiceErrorDescriptor{
		Message:   err.Error(),
		Kind:      ErrorKindInternal,
		Retryable: false,
	}
}

func describeServiceErrorCode(code string) ServiceErrorDescriptor {
	switch code {
	case ErrorCodeQuestionRequired, ErrorCodeCheckpointIDRequired, ErrorCodeApprovalDecisionInvalid:
		return ServiceErrorDescriptor{Code: code, Kind: ErrorKindInvalidRequest}
	case ErrorCodeApprovalSessionNotFound:
		return ServiceErrorDescriptor{Code: code, Kind: ErrorKindNotFound}
	case ErrorCodeApprovalNotPending:
		return ServiceErrorDescriptor{Code: code, Kind: ErrorKindFailedPrecondition}
	case ErrorCodeApprovalSessionLoadFailed, ErrorCodeApprovalSessionSaveFailed, ErrorCodeApprovalSessionDeleteFailed, ErrorCodeRuntimeExecutionFailed:
		return ServiceErrorDescriptor{Code: code, Kind: ErrorKindUnavailable, Retryable: true}
	case ErrorCodeServiceNotInitialized, ErrorCodeSessionStoreNotInitialized:
		return ServiceErrorDescriptor{Code: code, Kind: ErrorKindInternal}
	default:
		return ServiceErrorDescriptor{Code: code, Kind: ErrorKindInternal}
	}
}
