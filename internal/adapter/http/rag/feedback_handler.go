package rag

import (
	"github.com/gin-gonic/gin"

	ragservice "local/rag-project/internal/app/rag/service"
)

type feedbackRequest struct {
	Vote int `json:"vote"`
}

// SubmitFeedback 保存一条 assistant 消息反馈。
func (h *Handler) SubmitFeedback(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req feedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	if err := h.feedbackService.Submit(c.Request.Context(), ragservice.SubmitMessageFeedbackInput{
		MessageID: c.Param("messageId"),
		UserID:    user.UserID,
		Vote:      req.Vote,
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}
