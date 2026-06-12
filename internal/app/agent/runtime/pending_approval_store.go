package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PendingApprovalStore indexes the latest approval-pending runtime session by
// conversation and user so callers can rediscover resumable checkpoints.
type PendingApprovalStore interface {
	Put(ctx context.Context, conversationID string, userID string, ref PendingApprovalRef) error
	Get(ctx context.Context, conversationID string, userID string) (PendingApprovalRef, bool, error)
	Delete(ctx context.Context, conversationID string, userID string) error
}

// PendingApprovalRef is the persisted lookup value for one approval-pending run.
type PendingApprovalRef struct {
	CheckpointID string    `json:"checkpoint_id"`
	SessionID    string    `json:"session_id,omitempty"`
	RequestedAt  time.Time `json:"requested_at,omitempty"`
}

func pendingApprovalLookupKey(conversationID string, userID string) (string, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return "", fmt.Errorf("conversation id is required")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user id is required")
	}
	return conversationID + "::" + userID, nil
}
