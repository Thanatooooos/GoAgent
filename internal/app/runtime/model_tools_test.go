package runtime

import (
	"encoding/json"
	"testing"

	"local/rag-project/internal/app/runtime/capability"
)

func scheduledTaskToolRegistry(t *testing.T) *capability.Registry {
	t.Helper()
	tools := capability.NewRegistry()
	for _, id := range []string{capability.CreateScheduledTaskID, capability.ListScheduledTasksID, capability.PauseScheduledTaskID} {
		if err := tools.Register(capability.Def{
			ID: id, Description: id, JSONSchema: json.RawMessage(`{"type":"object"}`),
			Validate: func(capability.Value) error { return nil },
			Describe: func(v capability.Value, _ capability.Context) (capability.Operation, error) {
				return capability.Operation{Input: v}, nil
			},
			Execute: func(capability.Value, capability.Context) (capability.Result, error) { return capability.Result{}, nil },
		}); err != nil {
			t.Fatal(err)
		}
	}
	return tools
}

// A model must never be offered a scheduled-task tool the runtime will deny:
// both the policy flag and the user's own timezone decide visibility.
func TestModelToolsHideScheduledTasksWithoutPolicyOrTimezone(t *testing.T) {
	runtime := &Runtime{Tools: scheduledTaskToolRegistry(t)}
	for _, test := range []struct {
		name     string
		policy   Policy
		timezone string
		want     int
	}{
		{"permitted", Policy{AllowScheduledTasks: true}, "Asia/Shanghai", 3},
		{"policy denied", Policy{}, "Asia/Shanghai", 0},
		{"missing timezone", Policy{AllowScheduledTasks: true}, "", 0},
		{"unusable timezone", Policy{AllowScheduledTasks: true}, "Mars/Olympus", 0},
	} {
		visible := runtime.modelTools(test.policy, test.timezone, false)
		if len(visible) != test.want {
			t.Fatalf("%s: visible tools = %+v", test.name, visible)
		}
	}
}
