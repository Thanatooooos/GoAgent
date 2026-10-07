package rag

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/stream"
	fwweb "local/rag-project/internal/framework/web"
	"time"
)

func WatchRuntimeStop(ctx context.Context, manager stream.StreamManager, id string, cancel func() bool) {
	stopWatcher(ctx, manager, id, cancel, defaultStopWatcherInterval, 0)
}
func StopRuntimeStream(ctx context.Context, manager stream.StreamManager, id string) error {
	return manager.AppendEvent(ctx, id, stream.StreamEvent{Name: internalStopEventName, Data: json.RawMessage(`{}`), Timestamp: time.Now()})
}
func EndRuntimeStream(ctx context.Context, manager stream.StreamManager, id string) error {
	return manager.AppendEvent(ctx, id, stream.StreamEvent{Name: "done", Data: json.RawMessage(`{}`), Done: true, Timestamp: time.Now()})
}

// Work uses the same wire projection and polling transport as ordinary chat.
// Callers must verify the user/topic/turn before serving or replaying a stream.
func NewRuntimeEventSink(manager stream.StreamManager, id string) conversationruntime.EventSink {
	return newRuntimeStreamSink(manager, id)
}
func SendRuntimeMeta(manager stream.StreamManager, id, conversation string) error {
	return (&streamChatSink{manager: manager, streamID: id}).SendMeta(chatStreamMeta{ConversationID: conversation, TaskID: id})
}
func SendRuntimeFailure(manager stream.StreamManager, id string, err error) {
	s := &streamChatSink{manager: manager, streamID: id}
	_ = s.SendError(err)
	_ = s.SendDone()
}
func ServeRuntimeStream(c *gin.Context, manager stream.StreamManager, id string, offset int) {
	pollLoop(c.Request.Context(), fwweb.NewSseEmitterSender(c), manager, id, offset, defaultStreamPollInterval, 0)
}
