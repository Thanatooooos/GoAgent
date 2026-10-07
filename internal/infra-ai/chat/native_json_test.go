package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/infra-ai/model"
)

func TestNativeJSONRequestKeepsAllSystemInstructions(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"signal\\\":\\\"no_report\\\"}\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer server.Close()
	client := NewOpenAIStyleChatClient("siliconflow", server.Client())
	callback := &jsonCallback{}
	err := client.StreamNative(context.Background(), NativeRequest{JSONMode: true, System: []string{"fixed task prompt", "strict JSON only", "event baseline"}, Messages: []NativeMessage{{Role: "user", Content: "run now"}}}, model.ModelTarget{Candidate: config.ModelCandidate{Model: "test"}, Provider: config.ProviderConfig{Url: server.URL, ApiKey: "test-key", Endpoints: map[string]string{"chat": "/v1/chat/completions"}}}, callback)
	if err != nil {
		t.Fatal(err)
	}
	format, ok := body["response_format"].(map[string]any)
	if !ok || format["type"] != "json_object" {
		t.Fatalf("format = %+v", body["response_format"])
	}
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %+v", messages)
	}
	system := messages[0].(map[string]any)["content"].(string)
	for _, required := range []string{"fixed task prompt", "strict JSON only", "event baseline"} {
		if !strings.Contains(system, required) {
			t.Fatalf("lost %q", required)
		}
	}
	if callback.content != `{"signal":"no_report"}` || !callback.completed {
		t.Fatalf("callback = %+v", callback)
	}
}

type jsonCallback struct {
	content   string
	completed bool
}

func (c *jsonCallback) OnContent(s string) error     { c.content += s; return nil }
func (*jsonCallback) OnThinking(string) error        { return nil }
func (*jsonCallback) OnToolCall(ToolCallDelta) error { return nil }
func (c *jsonCallback) OnComplete(string) error      { c.completed = true; return nil }
