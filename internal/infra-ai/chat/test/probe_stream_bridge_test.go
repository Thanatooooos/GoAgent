package test

import (
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/infra-ai/chat"
)

type bridgeRecorder struct {
	content   []string
	thinking  []string
	completed int
	errs      []error
}

func (b *bridgeRecorder) OnContent(content string) {
	b.content = append(b.content, content)
}

func (b *bridgeRecorder) OnThinking(content string) {
	b.thinking = append(b.thinking, content)
}

func (b *bridgeRecorder) OnComplete() {
	b.completed++
}

func (b *bridgeRecorder) OnError(err error) {
	b.errs = append(b.errs, err)
}

func TestProbeStreamBridgeSuccessCommitsBufferedEvents(t *testing.T) {
	downstream := &bridgeRecorder{}
	bridge := chat.NewProbeStreamBridge(downstream)

	bridge.OnThinking("t1")
	bridge.OnContent("c1")

	result := bridge.AwaitFirstPacket(100 * time.Millisecond)
	if !result.IsSuccess() {
		t.Fatalf("expected success probe result, got %+v", result)
	}
	if len(downstream.thinking) != 1 || downstream.thinking[0] != "t1" {
		t.Fatalf("unexpected thinking events: %+v", downstream.thinking)
	}
	if len(downstream.content) != 1 || downstream.content[0] != "c1" {
		t.Fatalf("unexpected content events: %+v", downstream.content)
	}
}

func TestProbeStreamBridgeErrorBuffersUntilReported(t *testing.T) {
	downstream := &bridgeRecorder{}
	bridge := chat.NewProbeStreamBridge(downstream)
	wantErr := errors.New("boom")

	bridge.OnError(wantErr)

	result := bridge.AwaitFirstPacket(100 * time.Millisecond)
	if result.Type != chat.ProbeResultError {
		t.Fatalf("expected error probe result, got %+v", result)
	}
	if len(downstream.errs) != 0 {
		t.Fatalf("downstream should remain buffered on probe error, got %+v", downstream.errs)
	}
}
