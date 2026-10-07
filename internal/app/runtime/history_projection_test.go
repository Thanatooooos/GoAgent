package runtime

import "testing"

func TestProjectHistoryExcludesThinkingAndRestoresToolProtocol(t *testing.T) {
	t.Parallel()
	entries := []JournalEntry{{EventType: EventAssistantTurn, Detail: `{"content":"I will search.","tool_calls":[{"ID":"call-1","CapabilityID":"retrieve_knowledge","Arguments":{"query":"q"}}]}`}, {EventType: EventToolSettled, ToolCallID: "call-1", Detail: `{"content":"evidence"}`}}
	messages, err := ProjectHistory("question", entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || messages[1].Role != ModelRoleAssistant || len(messages[1].ToolCalls) != 1 || messages[2].Role != ModelRoleTool || messages[2].ToolCallID != "call-1" || messages[2].Content != "evidence" {
		t.Fatalf("messages = %#v", messages)
	}
}
