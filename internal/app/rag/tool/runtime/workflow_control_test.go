package runtime

import (
	"context"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	. "local/rag-project/internal/app/rag/tool/core"
)

type workflowControlTestInvoker struct{}

func (workflowControlTestInvoker) Invoke(_ context.Context, _ Call) (Result, error) {
	return Result{}, nil
}

func TestDeriveWorkflowControlWithRegistryInfersSpecWithoutGlobals(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterModule(ToolModule{
		Name:    "document_query",
		Invoker: workflowControlTestInvoker{},
		Spec: ToolSpec{
			Definition:          Definition{Name: "document_query", ReadOnly: true},
			Capability:          CapabilityDiagnosis,
			EvidenceSources:     []string{EvidenceSourceSystemRecords},
			ExecutionMode:       ExecutionModeReadOnly,
			RiskLevel:           RiskLevelLow,
			ApprovalRequirement: ApprovalRequirementNone,
			ReadOnly:            true,
			Family:              "system",
		},
	}.Normalize())

	control := deriveWorkflowControlWithRegistry(WorkflowInput{
		Control: WorkflowControl{
			Capability:          CapabilityGeneral,
			ExecutionMode:       ExecutionModeReadOnly,
			RiskLevel:           RiskLevelLow,
			ApprovalRequirement: ApprovalRequirementNone,
		},
	}, []Result{{
		Name:   "document_query",
		Status: CallStatusSuccess,
	}}, registry)

	if control.Capability != CapabilityDiagnosis {
		t.Fatalf("expected capability %q, got %q", CapabilityDiagnosis, control.Capability)
	}
	if control.ExecutionMode != ExecutionModeReadOnly {
		t.Fatalf("expected execution mode %q, got %q", ExecutionModeReadOnly, control.ExecutionMode)
	}
	if control.RiskLevel != RiskLevelLow {
		t.Fatalf("expected risk level %q, got %q", RiskLevelLow, control.RiskLevel)
	}
	if control.ApprovalRequirement != ApprovalRequirementNone {
		t.Fatalf("expected approval requirement %q, got %q", ApprovalRequirementNone, control.ApprovalRequirement)
	}
}

func TestBuildWorkflowTraceMetaWithRegistryInfersEvidenceSourcesWithoutGlobals(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterModule(ToolModule{
		Name:    "document_query",
		Invoker: workflowControlTestInvoker{},
		Spec: ToolSpec{
			Definition:          Definition{Name: "document_query", ReadOnly: true},
			Capability:          CapabilityDiagnosis,
			EvidenceSources:     []string{EvidenceSourceSystemRecords},
			ExecutionMode:       ExecutionModeReadOnly,
			RiskLevel:           RiskLevelLow,
			ApprovalRequirement: ApprovalRequirementNone,
			ReadOnly:            true,
			Family:              "system",
		},
	}.Normalize())

	traceMeta := buildWorkflowTraceMetaWithRegistry(
		WorkflowControl{Capability: CapabilityDiagnosis, ExecutionMode: ExecutionModeReadOnly, RiskLevel: RiskLevelLow, ApprovalRequirement: ApprovalRequirementNone},
		ragretrieve.Result{},
		[]Result{{Name: "document_query", Status: CallStatusSuccess}},
		registry,
	)

	if len(traceMeta.EvidenceSources) != 1 || traceMeta.EvidenceSources[0] != EvidenceSourceSystemRecords {
		t.Fatalf("expected evidence sources [%q], got %#v", EvidenceSourceSystemRecords, traceMeta.EvidenceSources)
	}
}
