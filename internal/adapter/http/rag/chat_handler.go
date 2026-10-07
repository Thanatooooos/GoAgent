package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/stream"
	fwweb "local/rag-project/internal/framework/web"
)

// Chat 通过「聊天协程写流 + SSE 轮询」输出流式结果。
func (h *Handler) Chat(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	taskID, err := nextTaskID()
	if err != nil {
		_ = c.Error(err)
		return
	}
	if h.runtimeChat == nil {
		_ = c.Error(exception.NewServiceException("conversation runtime is unavailable", nil))
		return
	}
	h.runtimeChatRequest(c, user.UserID, taskID)
}

func nextTaskID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate rag task id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

func (h *Handler) runtimeChatRequest(c *gin.Context, userID, taskID string) {
	req := chatRequest{
		ConversationID:  c.Query("conversationId"),
		Question:        c.Query("question"),
		KnowledgeBaseID: c.Query("knowledgeBaseId"),
		Timezone:        c.Query("timezone"),
	}
	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&req); err != nil {
			_ = c.Error(exception.NewClientException("invalid chat request", err))
			return
		}
	} else if value := c.Query("deepThinking"); value != "" {
		var err error
		req.DeepThinking, err = strconv.ParseBool(value)
		if err != nil {
			_ = c.Error(exception.NewClientException("deepThinking must be a boolean", err))
			return
		}
	}
	conversationID := strings.TrimSpace(req.ConversationID)
	if conversationID == "" {
		conversationID = taskID
	}
	input := conversationruntime.ChatInput{
		TaskID: taskID, ConversationID: conversationID, UserID: userID, Question: strings.TrimSpace(req.Question),
		DeepThinking:     req.DeepThinking,
		KnowledgeBaseIDs: splitCommaValues(req.KnowledgeBaseID), Timezone: strings.TrimSpace(req.Timezone),
		Policy: conversationruntime.Policy{AllowKnowledgeRetrieval: true, AllowMemoryRecall: true, AllowMemoryMutation: hasExplicitMemoryMutationIntent(req.Question), AllowWebSearch: true, AllowConversationHistory: true, AllowEpisodeArchive: true, AllowScheduledTasks: true}, TraceID: taskID,
	}
	input, err := h.runtimeChat.Admit(c.Request.Context(), input)
	if err != nil {
		_ = c.Error(exception.NewClientException("chat admission failed", err))
		return
	}
	sender := fwweb.NewSseEmitterSender(c)
	sink := newRuntimeStreamSink(h.streamManager, taskID)
	_ = sink.stream.SendMeta(chatStreamMeta{ConversationID: conversationID, TaskID: taskID})
	baseCtx := context.WithoutCancel(c.Request.Context())
	go func() {
		if _, err := h.runtimeChat.Chat(baseCtx, input, sink); err != nil && !sink.stream.Terminated() {
			_ = sink.stream.SendError(err)
			_ = sink.stream.SendDone()
		}
	}()
	go stopWatcher(baseCtx, h.streamManager, taskID, func() bool { return h.runtimeChat.CancelTask(taskID) }, defaultStopWatcherInterval, 0)
	pollLoop(c.Request.Context(), sender, h.streamManager, taskID, 0, defaultStreamPollInterval, 0)
}

func hasExplicitMemoryMutationIntent(question string) bool {
	question = strings.TrimSpace(question)
	// Exposing a write tool is itself authority. Keep this gate deliberately
	// narrow: a bare future-tense word such as “以后” must not let the model
	// turn an ordinary task statement into a durable preference.
	for _, marker := range []string{
		"请记住", "帮我记住", "记下来", "保存偏好", "保存这个偏好",
		"忘掉", "删除记忆", "删除偏好", "修改偏好", "更新偏好",
		"我更喜欢", "我偏好", "我希望你", "请始终", "请总是",
		"以后请", "以后都", "以后默认",
	} {
		if strings.Contains(question, marker) {
			return true
		}
	}
	return false
}

type chatRequest struct {
	ConversationID  string `json:"conversationId"`
	Question        string `json:"question"`
	DeepThinking    bool   `json:"deepThinking"`
	KnowledgeBaseID string `json:"knowledgeBaseId"`
	// Timezone is the browser-reported IANA zone. It is only ever read by the
	// runtime's scheduled-task capabilities, never forwarded to the model as an
	// argument it can choose.
	Timezone string `json:"timezone"`
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
	if !h.authorizeChatTask(c, user.UserID, taskID) {
		return
	}
	offset, err := continueOffset(c.Query("offset"))
	if err != nil {
		_ = c.Error(exception.NewClientException("invalid stream offset", err))
		return
	}
	sender := fwweb.NewSseEmitterSender(c)
	events, _, err := h.streamManager.GetEvents(c.Request.Context(), taskID, 0)
	if err != nil {
		log.Printf("rag chat continue read %q: %v", taskID, err)
		sender.Complete()
		return
	}
	if len(events) == 0 && h.runtimeReplay != nil {
		found, replayErr := h.runtimeReplay.Replay(c.Request.Context(), user.UserID, taskID, newRuntimeStreamSink(h.streamManager, taskID))
		if replayErr != nil {
			log.Printf("rag runtime continue replay %q: %v", taskID, replayErr)
			sender.Complete()
			return
		}
		if found {
			events, _, err = h.streamManager.GetEvents(c.Request.Context(), taskID, 0)
			if err != nil {
				log.Printf("rag chat continue reload %q: %v", taskID, err)
				sender.Complete()
				return
			}
		}
	}
	if len(events) > 0 {
		if recoverer, ok := h.runtimeChat.(interface {
			RecoverPublication(context.Context, string, string) (conversationruntime.JournalEntry, error)
		}); ok {
			if err := RecoverRuntimePublication(c.Request.Context(), h.streamManager, taskID, func(ctx context.Context) (conversationruntime.JournalEntry, error) {
				return recoverer.RecoverPublication(ctx, user.UserID, taskID)
			}); err != nil {
				log.Printf("rag publication recovery %q: %v", taskID, err)
				sender.Complete()
				return
			}
		}
	}
	if len(events) == 0 {
		sender.Complete()
		return
	}
	events, _, err = h.streamManager.GetEvents(c.Request.Context(), taskID, 0)
	if err != nil {
		sender.Complete()
		return
	}
	pollLoop(c.Request.Context(), sender, h.streamManager, taskID, PublicationResumeOffset(events, offset), defaultStreamPollInterval, 0)
}

func continueOffset(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("offset must be a non-negative integer")
	}
	return offset, nil
}

// StopChat 双通道取消：本地 taskRegistry 快速路径 + 写入 stop 控制事件（跨节点）。
func (h *Handler) StopChat(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	taskID := strings.TrimSpace(c.Query("taskId"))
	if taskID == "" {
		_ = c.Error(exception.NewClientException("task id is required", nil))
		return
	}
	if !h.authorizeChatTask(c, user.UserID, taskID) {
		return
	}
	if canceller, ok := h.runtimeChat.(interface {
		CancelPublication(context.Context, string, string) error
	}); ok {
		if err := canceller.CancelPublication(c.Request.Context(), user.UserID, taskID); err != nil {
			_ = c.Error(exception.NewServiceException("failed to cancel pending answer", err))
			return
		}
	}
	if h.runtimeChat != nil {
		_ = h.runtimeChat.CancelTask(taskID)
	}
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

func (h *Handler) authorizeChatTask(c *gin.Context, userID, taskID string) bool {
	if h.runtimeChat == nil {
		_ = c.Error(exception.NewServiceException("conversation runtime is unavailable", nil))
		return false
	}
	allowed, err := h.runtimeChat.AuthorizeTask(c.Request.Context(), userID, taskID)
	if err != nil {
		_ = c.Error(exception.NewServiceException("failed to authorize chat execution", err))
		return false
	}
	if !allowed {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": "CHAT_EXECUTION_NOT_FOUND", "message": "执行不存在或不可访问"})
		return false
	}
	return true
}
