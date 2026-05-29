package rag

import (
	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	ragservice "local/rag-project/internal/app/rag/service"
)

type renameConversationRequest struct {
	Title string `json:"title"`
}

// ListConversations 返回当前登录用户的会话列表。
func (h *Handler) ListConversations(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	items, err := h.conversationService.List(c.Request.Context(), ragservice.ListConversationsInput{
		UserID: user.UserID,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	result := make([]conversationVO, 0, len(items))
	for _, item := range items {
		result = append(result, toConversationVO(item))
	}
	writeSuccess(c, result)
}

// ListMessages 返回当前会话的消息列表。
func (h *Handler) ListMessages(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	items, err := h.messageService.ListMessages(c.Request.Context(), ragservice.ListConversationMessagesInput{
		ConversationID: c.Param("conversationId"),
		UserID:         user.UserID,
		Order:          port.ConversationMessageOrderAsc,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	result := make([]messageVO, 0, len(items))
	for _, item := range items {
		result = append(result, toMessageVO(item))
	}
	writeSuccess(c, result)
}

// RenameConversation 修改会话标题。
func (h *Handler) RenameConversation(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req renameConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	if err := h.conversationService.Rename(c.Request.Context(), ragservice.RenameConversationInput{
		ConversationID: c.Param("conversationId"),
		UserID:         user.UserID,
		Title:          req.Title,
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

// DeleteConversation 删除一个会话及其消息。
func (h *Handler) DeleteConversation(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	if err := h.conversationService.Delete(c.Request.Context(), ragservice.DeleteConversationInput{
		ConversationID: c.Param("conversationId"),
		UserID:         user.UserID,
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

// toConversationVO 转换会话出参。
func toConversationVO(item domain.Conversation) conversationVO {
	return conversationVO{
		ConversationID: item.ConversationID,
		Title:          item.Title,
		LastTime:       item.LastTime,
	}
}

// toMessageVO 转换消息出参。
func toMessageVO(item ragservice.ConversationMessageView) messageVO {
	return messageVO{
		ID:               item.ID,
		ConversationID:   item.ConversationID,
		Role:             string(item.Role),
		Content:          item.Content,
		RawContent:       item.RawContent,
		ContentSummary:   item.ContentSummary,
		IsSummarized:     item.IsSummarized,
		ThinkingContent:  item.ThinkingContent,
		ThinkingDuration: item.ThinkingDuration,
		Vote:             item.Vote,
		CreateTime:       timePointer(item.CreateTime),
	}
}
