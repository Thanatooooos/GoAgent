package rag

import (
	"strings"

	"github.com/gin-gonic/gin"

	ragservice "local/rag-project/internal/app/rag/service"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/exception"
	fwweb "local/rag-project/internal/framework/web"
)

type resumeApprovalRequest struct {
	ConversationID string `json:"conversationId"`
	Question       string `json:"question"`
	CheckpointID   string `json:"checkpointId"`
	Decision       string `json:"decision"`
	DecisionNote   string `json:"decisionNote"`
}

// Chat 通过 SSE 输出最小聊天闭环的流式结果。
func (h *Handler) Chat(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}

	sender := fwweb.NewSseEmitterSender(c)
	sink := &sseChatSink{sender: sender}
	if err := h.chatService.Chat(c.Request.Context(), ragservice.RagChatInput{
		ConversationID:   strings.TrimSpace(c.Query("conversationId")),
		UserID:           user.UserID,
		Question:         strings.TrimSpace(c.Query("question")),
		KnowledgeBaseIDs: splitCommaValues(c.Query("knowledgeBaseId")),
		DeepThinking:     parseBool(c.Query("deepThinking")),
		RequireApproval:  parseBool(c.Query("requireApproval")),
	}, sink); err != nil {
		return
	}
}

// ResumeAfterApproval resumes an agent-runtime chat flow after an approval decision.
func (h *Handler) ResumeAfterApproval(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}

	var req resumeApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	sender := fwweb.NewSseEmitterSender(c)
	sink := &sseChatSink{sender: sender}
	if err := h.chatService.ResumeAfterApproval(c.Request.Context(), ragservice.RagChatApprovalResumeInput{
		ConversationID: strings.TrimSpace(req.ConversationID),
		UserID:         user.UserID,
		Question:       strings.TrimSpace(req.Question),
		CheckpointID:   strings.TrimSpace(req.CheckpointID),
		Decision:       strings.TrimSpace(req.Decision),
		DecisionNote:   strings.TrimSpace(req.DecisionNote),
	}, sink); err != nil {
		return
	}
}

// StopChat 取消一个正在执行的聊天任务。
func (h *Handler) StopChat(c *gin.Context) {
	taskID := strings.TrimSpace(c.Query("taskId"))
	if taskID == "" {
		_ = c.Error(exception.NewClientException("task id is required", nil))
		return
	}
	if !h.chatService.CancelTask(taskID) {
		_ = c.Error(exception.NewClientException("chat task not found", nil))
		return
	}
	writeSuccess[any](c, nil)
}

type sseChatSink struct {
	sender *fwweb.SseEmitterSender
}

// SendMeta 发送 SSE meta 事件。
func (s *sseChatSink) SendMeta(meta ragservice.RagChatMeta) error {
	return s.sender.SendEvent("meta", meta)
}

// SendFallback 发送检索回退通知事件。
func (s *sseChatSink) SendFallback(reason string) error {
	return s.sender.SendEvent("fallback", gin.H{"reason": reason})
}

// SendAgentThink 发送 agent 观察/继续规划事件。
func (s *sseChatSink) SendAgentThink(message string) error {
	return s.sender.SendEvent("agent_think", gin.H{"message": message})
}

func (s *sseChatSink) SendAgentOutcome(payload ragservice.RagChatAgentOutcomePayload) error {
	if err := s.sender.SendEvent("agent_outcome", payload); err != nil {
		return err
	}
	return s.sender.SendEvent("agent_status", newAgentOutcomeStatusEventPayload(payload))
}

func (s *sseChatSink) SendApprovalPending(payload ragservice.RagChatApprovalPendingPayload) error {
	if err := s.sender.SendEvent("approval_pending", payload); err != nil {
		return err
	}
	return s.sender.SendEvent("agent_status", newAgentApprovalStatusEventPayload(payload))
}

func (s *sseChatSink) SendAgentServiceError(payload ragservice.RagChatAgentServiceErrorPayload) error {
	if err := s.sender.SendEvent("agent_service_error", payload); err != nil {
		return err
	}
	return s.sender.SendEvent("agent_status", newAgentServiceErrorStatusEventPayload(payload))
}

func (s *sseChatSink) SendMemoryStored(payload ragservice.RagChatMemoryStoredPayload) error {
	return s.sender.SendEvent("memory_stored", payload)
}

func (s *sseChatSink) SendSessionRecall(payload ragservice.RagChatSessionRecallPayload) error {
	return s.sender.SendEvent("session_recall", payload)
}

// SendThinking 发送 SSE thinking 事件。
func (s *sseChatSink) SendThinking(delta string) error {
	return s.sender.SendEvent("message", gin.H{
		"type":  "think",
		"delta": delta,
	})
}

// SendMessage 发送 SSE response 事件。
func (s *sseChatSink) SendMessage(delta string) error {
	return s.sender.SendEvent("message", gin.H{
		"type":  "response",
		"delta": delta,
	})
}

// SendToolStart 发送单个 tool 开始事件。
func (s *sseChatSink) SendToolStart(payload ragtool.ToolCallEvent) error {
	return s.sender.SendEvent("tool_start", payload)
}

// SendToolResult 发送单个 tool 结果事件。
func (s *sseChatSink) SendToolResult(payload ragtool.ToolCallEvent) error {
	return s.sender.SendEvent("tool_result", payload)
}

// SendTool 发送单个 tool 调用摘要事件。
func (s *sseChatSink) SendTool(name string, status string, summary string) error {
	return s.sender.SendEvent("tool", gin.H{
		"name":    name,
		"status":  status,
		"summary": summary,
	})
}

// SendTitle 发送会话标题事件。
func (s *sseChatSink) SendTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return nil
	}
	return s.sender.SendEvent("title", gin.H{"title": title})
}

// SendFinish 发送完成事件。
func (s *sseChatSink) SendFinish(payload ragservice.RagChatFinishPayload) error {
	return s.sender.SendEvent("finish", gin.H{
		"messageId": payload.MessageID,
		"title":     payload.Title,
	})
}

// SendCancel 发送取消事件。
func (s *sseChatSink) SendCancel(payload ragservice.RagChatFinishPayload) error {
	return s.sender.SendEvent("cancel", gin.H{
		"messageId": payload.MessageID,
		"title":     payload.Title,
	})
}

// SendError 发送错误事件。
func (s *sseChatSink) SendError(err error) error {
	if err == nil {
		return nil
	}
	return s.sender.SendEvent("error", gin.H{"error": err.Error()})
}

// SendDone 发送 done 事件并关闭 SSE。
func (s *sseChatSink) SendDone() error {
	if err := s.sender.SendEvent("done", gin.H{}); err != nil {
		return err
	}
	s.sender.Complete()
	return nil
}
