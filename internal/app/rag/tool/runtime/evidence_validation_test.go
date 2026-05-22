package runtime

import (
	"testing"

	. "local/rag-project/internal/app/rag/tool/core"
)

func TestValidateHintAgainstEvidenceRejectsInventedNodeID(t *testing.T) {
	ok := validateHintAgainstEvidence([]HintCall{{
		Name: "ingestion_task_node_query",
		Arguments: map[string]any{
			"taskId": "task-1",
			"nodeId": "node_0",
		},
	}}, ObserveInput{
		Question: "why did task-1 fail?",
		RoundResults: []Result{{
			Name: "ingestion_task_query",
			Data: map[string]any{
				"taskId": "task-1",
				"taskNodeSummary": []map[string]any{
					{"nodeId": "indexer", "status": "failed"},
				},
			},
		}},
	})
	if ok {
		t.Fatal("expected invented node id to be rejected")
	}
}

func TestValidateHintAgainstEvidenceAcceptsBackedIDs(t *testing.T) {
	ok := validateHintAgainstEvidence([]HintCall{{
		Name: "ingestion_task_node_query",
		Arguments: map[string]any{
			"taskId": "task-1",
			"nodeId": "indexer",
		},
	}}, ObserveInput{
		Question: "why did task-1 fail?",
		RoundResults: []Result{{
			Name: "ingestion_task_query",
			Data: map[string]any{
				"taskId": "task-1",
				"taskNodeSummary": []map[string]any{
					{"nodeId": "indexer", "status": "failed"},
				},
			},
		}},
	})
	if !ok {
		t.Fatal("expected evidence-backed ids to be accepted")
	}
}

func TestValidateHintAgainstEvidenceAcceptsTraceIDsFromTypedViews(t *testing.T) {
	ok := validateHintAgainstEvidence([]HintCall{{
		Name: "trace_node_query",
		Arguments: map[string]any{
			"traceId": "trace-1",
			"taskId":  "task-1",
			"nodeId":  "retrieve",
		},
	}}, ObserveInput{
		Question: "why did trace-1 fail?",
		RoundResults: []Result{
			{
				Name: "trace_node_query",
				Data: map[string]any{
					"traceId": "trace-1",
					"taskId":  "task-1",
					"nodes": []map[string]any{
						{"nodeId": "rewrite", "status": "success"},
						{"nodeId": "retrieve", "status": "failed"},
					},
				},
			},
			{
				Name: "trace_retrieval_diagnose",
				Data: map[string]any{
					"taskId":       "task-1",
					"latestTaskId": "task-1",
					"latestNodeId": "retrieve",
				},
			},
		},
	})
	if !ok {
		t.Fatal("expected trace ids from typed views to be accepted")
	}
}
