package runtime

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

type ScheduleDecision string

const (
	ScheduleDecisionExecute      ScheduleDecision = "execute"
	ScheduleDecisionWaitApproval ScheduleDecision = "wait_approval"
	ScheduleDecisionSkip         ScheduleDecision = "skip"
	ScheduleDecisionRetry        ScheduleDecision = "retry"
	ScheduleDecisionDegrade      ScheduleDecision = "degrade"
	ScheduleDecisionFail         ScheduleDecision = "fail"
)

type CapabilityScheduleInput struct {
	RuntimeOptions      agentstate.RuntimeOptions
	Snapshot            agentstate.StateSnapshot
	PatternAction       string
	Session             *RuntimeSession
	Spec                agentcapability.Spec
	Input               any
	SkipInputValidation bool
}

type CapabilityScheduleResult struct {
	Decision         ScheduleDecision `json:"decision,omitempty"`
	Reason           string           `json:"reason,omitempty"`
	PatternAction    string           `json:"pattern_action,omitempty"`
	ErrorClass       string           `json:"error_class,omitempty"`
	RiskLevel        string           `json:"risk_level,omitempty"`
	Idempotency      string           `json:"idempotency,omitempty"`
	SupportsParallel bool             `json:"supports_parallel,omitempty"`
	SupportsResume   bool             `json:"supports_resume,omitempty"`
}

// CapabilityScheduleBatch is the deterministic scheduling group produced for a
// batch of requested capability calls.
type CapabilityScheduleBatch struct {
	Decision ScheduleDecision           `json:"decision,omitempty"`
	Parallel bool                       `json:"parallel,omitempty"`
	Results  []CapabilityScheduleResult `json:"results,omitempty"`
}

func EvaluateCapabilitySchedule(input CapabilityScheduleInput) CapabilityScheduleResult {
	result := CapabilityScheduleResult{
		Decision:         ScheduleDecisionExecute,
		PatternAction:    strings.TrimSpace(input.PatternAction),
		RiskLevel:        strings.TrimSpace(input.Spec.RiskLevel),
		Idempotency:      strings.TrimSpace(input.Spec.Idempotency),
		SupportsParallel: input.Spec.SupportsParallel,
		SupportsResume:   input.Spec.SupportsResume,
	}

	if !input.SkipInputValidation {
		if err := agentcapability.ValidateInput(input.Spec, input.Input); err != nil {
			result.Decision = ScheduleDecisionFail
			result.Reason = "precondition_failed"
			result.ErrorClass = ErrorClassValidation
			return result
		}
	}

	if resumedRuntime(input.Session) && !input.Spec.SupportsResume {
		result.Decision = ScheduleDecisionDegrade
		result.Reason = "resume_not_supported"
		return result
	}

	if capabilityNeedsApproval(input, input.Spec) {
		result.Decision = ScheduleDecisionWaitApproval
		result.Reason = "approval_required"
		return result
	}

	return result
}

func BuildCapabilityScheduleBatches(inputs []CapabilityScheduleInput) []CapabilityScheduleBatch {
	if len(inputs) == 0 {
		return nil
	}

	batches := make([]CapabilityScheduleBatch, 0, len(inputs))
	for _, input := range inputs {
		result := EvaluateCapabilitySchedule(input)
		if result.Decision == ScheduleDecisionExecute && result.SupportsParallel {
			if len(batches) > 0 && batches[len(batches)-1].Decision == ScheduleDecisionExecute && batches[len(batches)-1].Parallel {
				batches[len(batches)-1].Results = append(batches[len(batches)-1].Results, result)
				continue
			}
			batches = append(batches, CapabilityScheduleBatch{
				Decision: result.Decision,
				Parallel: true,
				Results:  []CapabilityScheduleResult{result},
			})
			continue
		}

		batches = append(batches, CapabilityScheduleBatch{
			Decision: result.Decision,
			Parallel: false,
			Results:  []CapabilityScheduleResult{result},
		})
	}
	return batches
}

func capabilityNeedsApproval(input CapabilityScheduleInput, spec agentcapability.Spec) bool {
	if spec.RequiresApproval {
		return true
	}
	if input.RuntimeOptions.RequireApproval {
		return true
	}
	if input.Snapshot.Request.RuntimeOptions.RequireApproval {
		return true
	}
	session := input.Session
	if session == nil {
		return false
	}
	return session.Snapshot.Request.RuntimeOptions.RequireApproval || session.Request.Options.RequireApproval
}

func resumedRuntime(session *RuntimeSession) bool {
	return session != nil && session.Metadata.ResumeCount > 0
}
