package runtime

import (
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestEvaluateCapabilitySchedule_RequiresApprovalYieldsWaitApproval(t *testing.T) {
	result := EvaluateCapabilitySchedule(CapabilityScheduleInput{
		Session: &RuntimeSession{
			Snapshot: agentstate.StateSnapshot{
				Request: agentstate.RequestState{
					RuntimeOptions: agentstate.RuntimeOptions{},
				},
			},
		},
		Spec: agentcapability.Spec{
			Name:             agentcapability.NameWebFetch,
			RequiresApproval: true,
			RiskLevel:        agentcapability.RiskLevelHigh,
		},
	})

	if result.Decision != ScheduleDecisionWaitApproval {
		t.Fatalf("expected wait approval decision, got %+v", result)
	}
	if result.Reason != "approval_required" {
		t.Fatalf("expected approval reason, got %+v", result)
	}
	if result.RiskLevel != agentcapability.RiskLevelHigh {
		t.Fatalf("expected risk level passthrough, got %+v", result)
	}
}

func TestEvaluateCapabilitySchedule_RuntimeOptionRequireApprovalYieldsWaitApproval(t *testing.T) {
	result := EvaluateCapabilitySchedule(CapabilityScheduleInput{
		Session: &RuntimeSession{
			Snapshot: agentstate.StateSnapshot{
				Request: agentstate.RequestState{
					RuntimeOptions: agentstate.RuntimeOptions{
						RequireApproval: true,
					},
				},
			},
		},
		Spec: agentcapability.Spec{
			Name: agentcapability.NameWebSearch,
		},
	})

	if result.Decision != ScheduleDecisionWaitApproval {
		t.Fatalf("expected runtime require-approval to gate execution, got %+v", result)
	}
}

func TestEvaluateCapabilitySchedule_PreconditionFailureYieldsFail(t *testing.T) {
	result := EvaluateCapabilitySchedule(CapabilityScheduleInput{
		Spec: agentcapability.Spec{
			Name: agentcapability.NameWebSearch,
			Preconditions: []agentcapability.Precondition{
				{Field: "query", Requirement: agentcapability.PreconditionRequirementNonEmpty},
			},
		},
		Input: struct {
			Query string `json:"query"`
		}{},
	})

	if result.Decision != ScheduleDecisionFail {
		t.Fatalf("expected fail decision, got %+v", result)
	}
	if result.ErrorClass != ErrorClassValidation {
		t.Fatalf("expected validation error class, got %+v", result)
	}
}

func TestEvaluateCapabilitySchedule_NonResumableCapabilityAfterResumeYieldsDegrade(t *testing.T) {
	result := EvaluateCapabilitySchedule(CapabilityScheduleInput{
		Session: &RuntimeSession{
			Metadata: SessionMetadata{
				ResumeCount: 1,
			},
		},
		Spec: agentcapability.Spec{
			Name:           agentcapability.NameWebFetch,
			SupportsResume: false,
		},
	})

	if result.Decision != ScheduleDecisionDegrade {
		t.Fatalf("expected degrade decision, got %+v", result)
	}
	if result.Reason != "resume_not_supported" {
		t.Fatalf("expected non-resumable reason, got %+v", result)
	}
}

func TestEvaluateCapabilitySchedule_ExposesParallelCapabilityMetadata(t *testing.T) {
	result := EvaluateCapabilitySchedule(CapabilityScheduleInput{
		PatternAction: "reactive_fetch",
		Spec: agentcapability.Spec{
			Name:             agentcapability.NameWebSearch,
			SupportsParallel: true,
			SupportsResume:   true,
			Idempotency:      agentcapability.IdempotencyIdempotent,
		},
	})

	if result.Decision != ScheduleDecisionExecute {
		t.Fatalf("expected execute decision, got %+v", result)
	}
	if result.PatternAction != "reactive_fetch" {
		t.Fatalf("expected pattern action passthrough, got %+v", result)
	}
	if !result.SupportsParallel || !result.SupportsResume {
		t.Fatalf("expected policy metadata passthrough, got %+v", result)
	}
	if result.Idempotency != agentcapability.IdempotencyIdempotent {
		t.Fatalf("expected idempotency passthrough, got %+v", result)
	}
}

func TestEvaluateCapabilitySchedule_UsesExplicitRuntimeOptionsWithoutSession(t *testing.T) {
	result := EvaluateCapabilitySchedule(CapabilityScheduleInput{
		RuntimeOptions: agentstate.RuntimeOptions{
			RequireApproval: true,
		},
		PatternAction: "plan_execute_gate",
		Spec: agentcapability.Spec{
			Name: agentcapability.NameWebFetch,
		},
	})

	if result.Decision != ScheduleDecisionWaitApproval {
		t.Fatalf("expected explicit runtime options to gate execution, got %+v", result)
	}
	if result.Reason != "approval_required" {
		t.Fatalf("expected approval_required reason, got %+v", result)
	}
	if result.PatternAction != "plan_execute_gate" {
		t.Fatalf("expected pattern action passthrough, got %+v", result)
	}
}

func TestBuildCapabilityScheduleBatches_GroupsOnlyParallelSafeExecutions(t *testing.T) {
	batches := BuildCapabilityScheduleBatches([]CapabilityScheduleInput{
		{
			PatternAction: "parallel_search_a",
			Spec: agentcapability.Spec{
				Name:             "parallel_a",
				SupportsParallel: true,
			},
		},
		{
			PatternAction: "parallel_search_b",
			Spec: agentcapability.Spec{
				Name:             "parallel_b",
				SupportsParallel: true,
			},
		},
		{
			PatternAction: "serial_fetch",
			Spec: agentcapability.Spec{
				Name:             "serial_fetch",
				SupportsParallel: false,
			},
		},
		{
			RuntimeOptions: agentstate.RuntimeOptions{
				RequireApproval: true,
			},
			PatternAction: "approval_gate",
			Spec: agentcapability.Spec{
				Name:             "approval_fetch",
				SupportsParallel: true,
			},
		},
	})

	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %+v", batches)
	}
	if !batches[0].Parallel || len(batches[0].Results) != 2 {
		t.Fatalf("expected first batch to group two parallel-safe executions, got %+v", batches[0])
	}
	if batches[1].Parallel || len(batches[1].Results) != 1 || batches[1].Results[0].PatternAction != "serial_fetch" {
		t.Fatalf("expected second batch to keep serial execution isolated, got %+v", batches[1])
	}
	if batches[2].Decision != ScheduleDecisionWaitApproval || batches[2].Parallel {
		t.Fatalf("expected approval-gated execution to remain isolated, got %+v", batches[2])
	}
}
