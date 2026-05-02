package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSseEmitterSenderSendEventWritesPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/stream", nil)

	emitter := NewSseEmitterSender(ctx)
	emitter.writeTimeout = time.Second

	if err := emitter.SendEvent("message", gin.H{"hello": "world"}); err != nil {
		t.Fatalf("SendEvent returned error: %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: message") {
		t.Fatalf("expected event line, got %q", body)
	}
	if !strings.Contains(body, "\"hello\":\"world\"") {
		t.Fatalf("expected json payload, got %q", body)
	}
}

func TestBuildSSEPayloadDefaultEvent(t *testing.T) {
	payload, err := buildSSEPayload("", "hello")
	if err != nil {
		t.Fatalf("buildSSEPayload returned error: %v", err)
	}
	if payload != "data: hello\n\n" {
		t.Fatalf("unexpected payload: %q", payload)
	}
}
