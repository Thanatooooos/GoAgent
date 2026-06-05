package capability

import agentstate "local/rag-project/internal/app/agent/state"

func ValidationFailureResult(spec Spec, actionSummary string, err error) InvocationResult {
	return failureResult(spec, actionSummary, err, ErrorClassValidation)
}

func DependencyFailureResult(spec Spec, actionSummary string, err error) InvocationResult {
	return failureResult(spec, actionSummary, err, ErrorClassDependency)
}

func ExternalFailureResult(spec Spec, actionSummary string, err error) InvocationResult {
	return failureResult(spec, actionSummary, err, ErrorClassExternal)
}

func failureResult(spec Spec, actionSummary string, err error, errorClass string) InvocationResult {
	return InvocationResult{
		Action: ActionRecord{
			Name:    spec.Name,
			Summary: actionSummary,
		},
		Observation: ObservationRecord{
			Summary:    errorMessage(err),
			Degraded:   true,
			ErrorClass: errorClass,
		},
		Delta:      agentstate.StateDelta{},
		Status:     StatusDegraded,
		ErrorClass: errorClass,
	}
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
