package state

import "testing"

func TestSnapshotDomainContracts_DocumentsOwners(t *testing.T) {
	contracts := SnapshotDomainContracts()
	if len(contracts) != 8 {
		t.Fatalf("expected 8 state domain contracts, got %d", len(contracts))
	}

	owners := map[string]StateOwner{}
	for _, contract := range contracts {
		owners[contract.Domain] = contract.Owner
	}

	expected := map[string]StateOwner{
		"request":   StateOwnerRuntime,
		"context":   StateOwnerPattern,
		"plan":      StateOwnerPattern,
		"evidence":  StateOwnerCapability,
		"approval":  StateOwnerRuntime,
		"execution": StateOwnerRuntime,
		"answer":    StateOwnerAnswerSynthesizer,
		"pattern":   StateOwnerPattern,
	}
	for domain, owner := range expected {
		if owners[domain] != owner {
			t.Fatalf("expected domain %q owner %q, got %q", domain, owner, owners[domain])
		}
	}
}

func TestDefaultReducerApply_RejectsUnknownApprovalStatus(t *testing.T) {
	reducer := DefaultReducer{}
	status := "mystery"

	_, err := reducer.Apply(StateSnapshot{}, StateDelta{
		Approval: &ApprovalDelta{
			Status: &status,
		},
	})
	if err == nil {
		t.Fatal("expected unknown approval status to fail")
	}
}

func TestDefaultReducerApply_RejectsDraftAnswerThatAlsoCarriesDegradeReason(t *testing.T) {
	reducer := DefaultReducer{}
	degradeReason := "insufficient_evidence"

	_, err := reducer.Apply(StateSnapshot{
		Answer: AnswerState{
			Draft: "partial answer",
		},
	}, StateDelta{
		Answer: &AnswerDelta{
			DegradeReason: &degradeReason,
		},
	})
	if err == nil {
		t.Fatal("expected answer draft with degrade reason to fail")
	}
}

func TestDefaultReducerApply_RejectsPlanCurrentStepIndexOutsideStepBounds(t *testing.T) {
	reducer := DefaultReducer{}

	_, err := reducer.Apply(StateSnapshot{}, StateDelta{
		Plan: &PlanDelta{
			Replace: &PlanState{
				Goal:             "investigate runtime",
				CurrentStepIndex: 1,
				Steps: []PlanStep{
					{StepID: "step-1", Title: "Search", Status: PlanStepStatusPending},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected out-of-bounds current step index to fail")
	}
}

func TestDefaultReducerApply_RejectsExecutionStatusThatConflictsWithInterruptState(t *testing.T) {
	reducer := DefaultReducer{}
	status := ExecutionStatusCompleted
	interrupted := true

	_, err := reducer.Apply(StateSnapshot{}, StateDelta{
		Execution: &ExecutionDelta{
			Status:      &status,
			Interrupted: &interrupted,
		},
	})
	if err == nil {
		t.Fatal("expected completed execution status with interrupted=true to fail")
	}
}

func TestMergeStateDeltas_PlanReplaceIsLastWriteWins(t *testing.T) {
	first := PlanState{
		Goal:             "first plan",
		CurrentStepIndex: 0,
		Steps: []PlanStep{
			{StepID: "step-1", Title: "Search", Status: PlanStepStatusPending},
		},
	}
	second := PlanState{
		Goal:             "second plan",
		CurrentStepIndex: 1,
		Steps: []PlanStep{
			{StepID: "step-1", Title: "Search", Status: PlanStepStatusCompleted},
			{StepID: "step-2", Title: "Fetch", Status: PlanStepStatusPending},
		},
	}

	merged := MergeStateDeltas(
		StateDelta{Plan: &PlanDelta{Replace: &first}},
		StateDelta{Plan: &PlanDelta{Replace: &second}},
	)

	if merged.Plan == nil || merged.Plan.Replace == nil {
		t.Fatalf("expected merged plan replace payload, got %+v", merged.Plan)
	}
	if merged.Plan.Replace.Goal != "second plan" || len(merged.Plan.Replace.Steps) != 2 {
		t.Fatalf("expected last plan replace to win, got %+v", merged.Plan.Replace)
	}
}

func TestMergeStateDeltas_EvidencePreservesUniqueRules(t *testing.T) {
	merged := MergeStateDeltas(
		StateDelta{
			Evidence: &EvidenceDelta{
				AddItems:      []EvidenceItem{{ID: "e1", SourceRef: "url-1", Content: "same"}},
				OpenQuestions: []string{"what changed"},
			},
		},
		StateDelta{
			Evidence: &EvidenceDelta{
				AddItems: []EvidenceItem{
					{ID: "e1", SourceRef: "url-1", Content: "same"},
					{ID: "e2", SourceRef: "url-2", Content: "new"},
				},
				OpenQuestions: []string{"what changed", "what is missing"},
			},
		},
	)

	if merged.Evidence == nil {
		t.Fatal("expected merged evidence delta")
	}
	if len(merged.Evidence.AddItems) != 2 {
		t.Fatalf("expected deduplicated evidence merge, got %+v", merged.Evidence.AddItems)
	}
	if len(merged.Evidence.OpenQuestions) != 2 {
		t.Fatalf("expected deduplicated open question merge, got %+v", merged.Evidence.OpenQuestions)
	}
}
