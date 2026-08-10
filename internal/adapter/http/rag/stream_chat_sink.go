package rag

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	ragservice "local/rag-project/internal/app/rag/service"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/stream"
)

// streamChatSink 把 RagChatEventSink 事件写入 StreamManager，由 SSE 轮询消费。
type streamChatSink struct {
	manager  stream.StreamManager
	streamID string
	seq      int64
}

var _ ragservice.RagChatEventSink = (*streamChatSink)(nil)

func (s *streamChatSink) append(name string, payload interface{}, done bool) error {
	if s == nil || s.manager == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.seq++
	if err := s.manager.AppendEvent(context.Background(), s.streamID, stream.StreamEvent{
		ID:        strconv.FormatInt(s.seq, 10),
		Name:      name,
		Data:      data,
		Done:      done,
		Timestamp: time.Now(),
	}); err != nil {
		log.Printf("stream chat sink append %q: %v", s.streamID, err)
		return err
	}
	return nil
}

func (s *streamChatSink) SendMeta(meta ragservice.RagChatMeta) error {
	return s.append("meta", meta, false)
}

func (s *streamChatSink) SendFallback(reason string) error {
	return s.append("fallback", gin.H{"reason": reason}, false)
}

func (s *streamChatSink) SendAgentThink(message string) error {
	return s.append("agent_think", gin.H{"message": message}, false)
}

func (s *streamChatSink) SendAgentOutcome(payload ragservice.RagChatAgentOutcomePayload) error {
	if err := s.append("agent_outcome", payload, false); err != nil {
		return err
	}
	return s.append("agent_status", newAgentOutcomeStatusEventPayload(payload), false)
}

func (s *streamChatSink) SendApprovalPending(payload ragservice.RagChatApprovalPendingPayload) error {
	if err := s.append("approval_pending", payload, false); err != nil {
		return err
	}
	return s.append("agent_status", newAgentApprovalStatusEventPayload(payload), false)
}

func (s *streamChatSink) SendAgentServiceError(payload ragservice.RagChatAgentServiceErrorPayload) error {
	if err := s.append("agent_service_error", payload, false); err != nil {
		return err
	}
	return s.append("agent_status", newAgentServiceErrorStatusEventPayload(payload), false)
}

func (s *streamChatSink) SendMemoryStored(payload ragservice.RagChatMemoryStoredPayload) error {
	return s.append("memory_stored", payload, false)
}

func (s *streamChatSink) SendSessionRecall(payload ragservice.RagChatSessionRecallPayload) error {
	return s.append("session_recall", payload, false)
}

func (s *streamChatSink) SendThinking(delta string) error {
	return s.append("message", gin.H{"type": "think", "delta": delta}, false)
}

func (s *streamChatSink) SendMessage(delta string) error {
	return s.append("message", gin.H{"type": "response", "delta": delta}, false)
}

func (s *streamChatSink) SendToolStart(payload ragtool.ToolCallEvent) error {
	return s.append("tool_start", payload, false)
}

func (s *streamChatSink) SendToolResult(payload ragtool.ToolCallEvent) error {
	return s.append("tool_result", payload, false)
}

func (s *streamChatSink) SendTool(name string, status string, summary string) error {
	return s.append("tool", gin.H{"name": name, "status": status, "summary": summary}, false)
}

func (s *streamChatSink) SendTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return nil
	}
	return s.append("title", gin.H{"title": title}, false)
}

func (s *streamChatSink) SendFinish(payload ragservice.RagChatFinishPayload) error {
	return s.append("finish", gin.H{"messageId": payload.MessageID, "title": payload.Title}, false)
}

func (s *streamChatSink) SendCancel(payload ragservice.RagChatFinishPayload) error {
	return s.append("cancel", gin.H{"messageId": payload.MessageID, "title": payload.Title}, true)
}

func (s *streamChatSink) SendError(err error) error {
	if err == nil {
		return nil
	}
	return s.append("error", gin.H{"error": err.Error()}, true)
}

func (s *streamChatSink) SendDone() error {
	return s.append("done", gin.H{}, true)
}
