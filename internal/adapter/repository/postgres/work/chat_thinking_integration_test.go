package work_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	raghttp "local/rag-project/internal/adapter/http/rag"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/stream"
	"local/rag-project/internal/infra-ai/chat"
)

func TestChatHTTPThinkingSwitchReachesProviderAndSSEAcrossToolTurns(t *testing.T) {
	for _, mode := range []string{"true", "false", "omitted"} {
		t.Run(mode, func(t *testing.T) {
			db := testDB(t)
			user, _ := testIdentity(t)
			f := newPublicationFixture(t, db, user, publicationID(t))
			wantThinking := mode == "true"
			var calls atomic.Int32
			settings := make(chan bool, 2)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				thinking, ok := request["enable_thinking"].(bool)
				if !ok {
					t.Error("provider request lacks explicit thinking mode")
				}
				settings <- thinking
				w.Header().Set("Content-Type", "text/event-stream")
				// Always send reasoning to prove the parser drops it when disabled.
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"思考片段\"}}]}\n\n")
				if calls.Add(1) == 1 {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"先查资料。\\n\\n\"}}]}\n\n")
					fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"search","function":{"name":"web_search","arguments":"{\"query\":\"evidence\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"最终答案。\"},\"finish_reason\":\"stop\"}]}\n\n")
				}
			}))
			defer provider.Close()
			f.kernel.Model = runtimeadapter.NewSiliconFlowDeepSeekV4Flash(
				chat.NewOpenAIStyleChatClient("siliconflow", provider.Client()),
				config.ProviderConfig{Url: provider.URL, ApiKey: "test-key", Endpoints: map[string]string{"chat": "/v1/chat/completions"}}, false,
			)
			manager := stream.NewMemoryStreamManager()
			handler := raghttp.NewHandler(f.conversations, f.messages, nil, nil, nil, manager)
			handler.SetRuntimeChat(f.chat)
			router := gin.New()
			router.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user}); c.Next() })
			router.POST("/chat", handler.Chat)
			body := fmt.Sprintf(`{"conversationId":%q,"question":"question"`, f.id)
			if mode != "omitted" {
				body += `,"deepThinking":` + mode
			}
			body += "}"
			req := httptest.NewRequest("POST", "/chat", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			wire := recorder.Body.String()
			if recorder.Code != 200 || !strings.Contains(wire, "event: finish") || !strings.Contains(wire, "event: done") {
				t.Fatalf("HTTP=%d wire=%s", recorder.Code, wire)
			}
			if strings.Contains(wire, `"type":"think"`) != wantThinking {
				t.Fatalf("thinking=%t wire=%s", wantThinking, wire)
			}
			if calls.Load() != 2 {
				t.Fatalf("provider calls=%d", calls.Load())
			}
			for i := 0; i < 2; i++ {
				if got := <-settings; got != wantThinking {
					t.Fatalf("turn %d thinking=%t want=%t", i+1, got, wantThinking)
				}
			}
			var completion struct{ MessageID, Content string }
			for _, line := range strings.Split(wire, "\n") {
				if strings.HasPrefix(line, "data:") && strings.Contains(line, `"messageId"`) {
					if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &completion); err != nil {
						t.Fatal(err)
					}
				}
			}
			const wantAnswer = "先查资料。\n\n最终答案。"
			message, err := postgresrag.NewConversationMessageRepository(db).GetByID(context.Background(), completion.MessageID)
			if err != nil || message.DisplayContent() != wantAnswer || completion.Content != wantAnswer {
				t.Fatalf("message=%+v completion=%+v err=%v", message, completion, err)
			}
			var sessionID string
			if err := db.Raw(`SELECT runtime_session_id FROM t_runtime_chat_publication WHERE assistant_message_id=?`, completion.MessageID).Scan(&sessionID).Error; err != nil {
				t.Fatal(err)
			}
			journal, err := postgresruntime.NewStore(db).ListJournal(context.Background(), sessionID)
			if err != nil {
				t.Fatal(err)
			}
			thinkingCount := 0
			for _, entry := range journal {
				if entry.EventType == convruntime.EventThinkingDelta {
					thinkingCount++
				}
			}
			if (thinkingCount == 2) != wantThinking || (!wantThinking && thinkingCount != 0) {
				t.Fatalf("thinking journal entries=%d mode=%s", thinkingCount, mode)
			}
		})
	}
}
