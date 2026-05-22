package runtime

import (
	"testing"

	. "local/rag-project/internal/app/rag/tool/core"
)

func TestPlanWithBaseRulesRoutesSpecificDocumentToDiagnosis(t *testing.T) {
	calls := PlanWithBaseRules(WorkflowInput{
		Question: "document doc_run_01 现在还在运行吗？",
	}, DefaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "document_root_cause_diagnosis" {
		t.Fatalf("expected document_root_cause_diagnosis, got %q", calls[0].Name)
	}
}

func TestPlanWithBaseRulesRoutesOpenEndedTaskList(t *testing.T) {
	calls := PlanWithBaseRules(WorkflowInput{
		Question: "哪些ingestion任务还在运行中？",
	}, DefaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "task_list" {
		t.Fatalf("expected task_list, got %q", calls[0].Name)
	}
	if calls[0].Arguments["status"] != "running" {
		t.Fatalf("expected status=running, got %#v", calls[0].Arguments["status"])
	}
}
