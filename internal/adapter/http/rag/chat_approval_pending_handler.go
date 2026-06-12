package rag

import (
	"github.com/gin-gonic/gin"

	ragservice "local/rag-project/internal/app/rag/service"
)

type approvalPendingLookupVO struct {
	Pending  bool                                      `json:"pending"`
	Approval *ragservice.RagChatApprovalPendingPayload `json:"approval,omitempty"`
}

// GetPendingApproval returns the latest approval-pending runtime state for one conversation.
func (h *Handler) GetPendingApproval(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	approval, err := h.chatService.GetPendingApproval(c.Request.Context(), ragservice.RagChatApprovalPendingQueryInput{
		ConversationID: c.Query("conversationId"),
		UserID:         user.UserID,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, approvalPendingLookupVO{
		Pending:  approval != nil,
		Approval: approval,
	})
}
