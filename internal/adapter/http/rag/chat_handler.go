package rag

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/stream"
	fwweb "local/rag-project/internal/framework/web"
)

type resumeApprovalRequest struct {
	ConversationID string `json:"conversationId"`
	Question       string `json:"question"`
	CheckpointID   string `json:"checkpointId"`
	Decision       string `json:"decision"`
	DecisionNote   string `json:"decisionNote"`
}

// Chat 通过「聊天协程写流 + SSE 轮询」输出流式结果。
func (h *Handler) Chat(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	taskID, err := ragservice.NextTaskID()
	if err != nil {
		_ = c.Error(err)
		return
	}
	sender := fwweb.NewSseEmitterSender(c)
	sink := &streamChatSink{manager: h.streamManager, streamID: taskID}
	baseCtx := context.WithoutCancel(c.Request.Context())
	go func() {
		if err := h.chatService.Chat(baseCtx, ragservice.RagChatInput{
			ConversationID:   strings.TrimSpace(c.Query("conversationId")),
			UserID:           user.UserID,
			Question:         strings.TrimSpace(c.Query("question")),
			KnowledgeBaseIDs: splitCommaValues(c.Query("knowledgeBaseId")),
			DeepThinking:     parseBool(c.Query("deepThinking")),
			RequireApproval:  parseBool(c.Query("requireApproval")),
			TaskID:           taskID,
		}, sink); err != nil {
			if !sink.Terminated() {
				_ = sink.SendError(err)
				_ = sink.SendDone()
			}
			log.Printf("rag chat stream error: %v", err)
		}
	}()
	go stopWatcher(baseCtx, h.streamManager, taskID, func() bool { return h.chatService.CancelTask(taskID) }, defaultStopWatcherInterval, 0)
	pollLoop(c.Request.Context(), sender, h.streamManager, taskID, 0, defaultStreamPollInterval, 0)
}

// ResumeAfterApproval 恢复审批后的 agent 流，同样走流传输。
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
	taskID, err := ragservice.NextTaskID()
	if err != nil {
		_ = c.Error(err)
		return
	}
	sender := fwweb.NewSseEmitterSender(c)
	sink := &streamChatSink{manager: h.streamManager, streamID: taskID}
	baseCtx := context.WithoutCancel(c.Request.Context())
	go func() {
		if err := h.chatService.ResumeAfterApproval(baseCtx, ragservice.RagChatApprovalResumeInput{
			ConversationID: strings.TrimSpace(req.ConversationID),
			UserID:         user.UserID,
			Question:       strings.TrimSpace(req.Question),
			CheckpointID:   strings.TrimSpace(req.CheckpointID),
			Decision:       strings.TrimSpace(req.Decision),
			DecisionNote:   strings.TrimSpace(req.DecisionNote),
			TaskID:         taskID,
		}, sink); err != nil {
			if !sink.Terminated() {
				_ = sink.SendError(err)
				_ = sink.SendDone()
			}
			log.Printf("rag chat stream error: %v", err)
		}
	}()
	go stopWatcher(baseCtx, h.streamManager, taskID, func() bool { return h.chatService.CancelTask(taskID) }, defaultStopWatcherInterval, 0)
	pollLoop(c.Request.Context(), sender, h.streamManager, taskID, 0, defaultStreamPollInterval, 0)
}

// ContinueChat 断线重连：回放已有流并继续轮询。
func (h *Handler) ContinueChat(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	taskID := strings.TrimSpace(c.Query("taskId"))
	if taskID == "" {
		_ = c.Error(exception.NewClientException("task id is required", nil))
		return
	}
	sender := fwweb.NewSseEmitterSender(c)
	events, _, err := h.streamManager.GetEvents(c.Request.Context(), taskID, 0)
	if err != nil {
		log.Printf("rag chat continue read %q: %v", taskID, err)
		sender.Complete()
		return
	}
	if len(events) == 0 {
		sender.Complete()
		return
	}
	pollLoop(c.Request.Context(), sender, h.streamManager, taskID, 0, defaultStreamPollInterval, 0)
}

// StopChat 双通道取消：本地 taskRegistry 快速路径 + 写入 stop 控制事件（跨节点）。
func (h *Handler) StopChat(c *gin.Context) {
	taskID := strings.TrimSpace(c.Query("taskId"))
	if taskID == "" {
		_ = c.Error(exception.NewClientException("task id is required", nil))
		return
	}
	_ = h.chatService.CancelTask(taskID)
	stopData, _ := json.Marshal(gin.H{})
	if err := h.streamManager.AppendEvent(c.Request.Context(), taskID, stream.StreamEvent{
		Name:      internalStopEventName,
		Data:      stopData,
		Done:      true,
		Timestamp: time.Now(),
	}); err != nil {
		log.Printf("rag chat stop event %q: %v", taskID, err)
	}
	writeSuccess[any](c, nil)
}
