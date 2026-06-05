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

// ServiceError is the canonical outward service error contract for agent
// service boundaries.
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
		return ServiceErrorDescriptor{
			Code:      code,
			Message:   defaultServiceErrorMessage(code),
			Kind:      ErrorKindInvalidRequest,
			Retryable: false,
		}
	case ErrorCodeApprovalSessionNotFound:
		return ServiceErrorDescriptor{
			Code:      code,
			Message:   defaultServiceErrorMessage(code),
			Kind:      ErrorKindNotFound,
			Retryable: false,
		}
	case ErrorCodeApprovalNotPending:
		return ServiceErrorDescriptor{
			Code:      code,
			Message:   defaultServiceErrorMessage(code),
			Kind:      ErrorKindFailedPrecondition,
			Retryable: false,
		}
	case ErrorCodeApprovalSessionLoadFailed, ErrorCodeApprovalSessionSaveFailed, ErrorCodeApprovalSessionDeleteFailed, ErrorCodeRuntimeExecutionFailed:
		return ServiceErrorDescriptor{
			Code:      code,
			Message:   defaultServiceErrorMessage(code),
			Kind:      ErrorKindUnavailable,
			Retryable: true,
		}
	case ErrorCodeServiceNotInitialized, ErrorCodeSessionStoreNotInitialized:
		return ServiceErrorDescriptor{
			Code:      code,
			Message:   defaultServiceErrorMessage(code),
			Kind:      ErrorKindInternal,
			Retryable: false,
		}
	default:
		return ServiceErrorDescriptor{
			Code:      code,
			Message:   defaultServiceErrorMessage(code),
			Kind:      ErrorKindInternal,
			Retryable: false,
		}
	}
}

func defaultServiceErrorMessage(code string) string {
	switch code {
	case ErrorCodeServiceNotInitialized:
		return "agent service is not initialized"
	case ErrorCodeQuestionRequired:
		return "question is required"
	case ErrorCodeSessionStoreNotInitialized:
		return "agent service session store is not initialized"
	case ErrorCodeCheckpointIDRequired:
		return "checkpoint id is required"
	case ErrorCodeApprovalDecisionInvalid:
		return `approval decision must be one of "approved" or "rejected"`
	case ErrorCodeApprovalSessionLoadFailed:
		return "failed to load approval session"
	case ErrorCodeApprovalSessionSaveFailed:
		return "failed to persist approval session"
	case ErrorCodeApprovalSessionDeleteFailed:
		return "failed to delete pending approval session"
	case ErrorCodeApprovalSessionNotFound:
		return "approval session not found"
	case ErrorCodeApprovalNotPending:
		return "approval session is not awaiting approval"
	case ErrorCodeRuntimeExecutionFailed:
		return "agent runtime execution failed"
	default:
		return ""
	}
}
