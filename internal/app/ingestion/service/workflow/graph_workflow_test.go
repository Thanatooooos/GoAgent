package workflow

import (
	"testing"

	"local/rag-project/internal/app/ingestion/domain"
)

func TestEvaluateWorkflowCondition(t *testing.T) {
	t.Parallel()

	state := ExecutionState{
		Task: domain.Task{
			ID:     "task-1",
			Status: domain.TaskStatusRunning,
			Metadata: map[string]any{
				"source": "manual",
			},
		},
		Artifacts: map[string]any{
			"chunks": map[string]any{
				"count": 3,
			},
		},
		NodeOutputs: map[string]map[string]any{
			"fetch": {
				"success": true,
				"mime":    "text/plain",
			},
		},
	}

	testCases := []struct {
		name      string
		condition map[string]any
		want      bool
	}{
		{
			name: "task metadata eq",
			condition: map[string]any{
				"path":  "task.metadata.source",
				"op":    "eq",
				"value": "manual",
			},
			want: true,
		},
		{
			name: "node output exists",
			condition: map[string]any{
				"path": "nodeOutputs.fetch.mime",
				"op":   "exists",
			},
			want: true,
		},
		{
			name: "artifact numeric comparison",
			condition: map[string]any{
				"path":  "artifacts.chunks.count",
				"op":    "gte",
				"value": 2,
			},
			want: true,
		},
		{
			name: "all condition",
			condition: map[string]any{
				"all": []any{
					map[string]any{"path": "task.metadata.source", "op": "eq", "value": "manual"},
					map[string]any{"path": "nodeOutputs.fetch.success", "op": "eq", "value": true},
				},
			},
			want: true,
		},
		{
			name: "any condition",
			condition: map[string]any{
				"any": []any{
					map[string]any{"path": "task.metadata.source", "op": "eq", "value": "batch"},
					map[string]any{"path": "nodeOutputs.fetch.mime", "op": "in", "value": []any{"application/pdf", "text/plain"}},
				},
			},
			want: true,
		},
		{
			name: "false condition",
			condition: map[string]any{
				"path":  "artifacts.chunks.count",
				"op":    "lt",
				"value": 1,
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EvaluateWorkflowCondition(tc.condition, state)
			if err != nil {
				t.Fatalf("EvaluateWorkflowCondition() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("EvaluateWorkflowCondition() = %v, want %v", got, tc.want)
			}
		})
	}
}
