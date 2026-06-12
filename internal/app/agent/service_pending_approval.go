package agent

import (
	"context"
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
)

func (s *Service) GetPendingApproval(ctx context.Context, req PendingApprovalLookupRequest) (*ApprovalPending, bool, error) {
	if s == nil || s.sessionStore == nil || s.pendingStore == nil {
		return nil, false, nil
	}

	conversationID := strings.TrimSpace(req.ConversationID)
	userID := strings.TrimSpace(req.UserID)
	if conversationID == "" || userID == "" {
		return nil, false, nil
	}

	ref, ok, err := s.pendingStore.Get(ctx, conversationID, userID)
	if err != nil {
		return nil, false, err
	}
	if !ok || strings.TrimSpace(ref.CheckpointID) == "" {
		return nil, false, nil
	}

	session, sessionFound, err := s.sessionStore.Get(ctx, ref.CheckpointID)
	if err != nil {
		return nil, false, err
	}
	if !sessionFound || session == nil || !s.pendingApprovalBelongsToLookup(session, conversationID, userID) || !approvalCheckpointMatchesRequest(session, ref.CheckpointID) || !s.isAwaitingApproval(session) {
		s.deletePendingApprovalLookup(ctx, session, conversationID, userID)
		return nil, false, nil
	}

	pending := s.approvalPendingFromSession(session, ref.CheckpointID)
	if pending == nil {
		s.deletePendingApprovalLookup(ctx, session, conversationID, userID)
		return nil, false, nil
	}
	return pending, true, nil
}

func (s *Service) putPendingApprovalLookup(ctx context.Context, checkpointID string, session *agentruntime.RuntimeSession) {
	if s == nil || s.pendingStore == nil || session == nil {
		return
	}
	conversationID := strings.TrimSpace(firstNonEmpty(session.Request.ConversationID, session.Snapshot.Request.ConversationID))
	userID := strings.TrimSpace(firstNonEmpty(session.Request.UserID, session.Snapshot.Request.UserID))
	if conversationID == "" || userID == "" || strings.TrimSpace(checkpointID) == "" {
		return
	}
	_ = s.pendingStore.Put(ctx, conversationID, userID, agentruntime.PendingApprovalRef{
		CheckpointID: strings.TrimSpace(checkpointID),
		SessionID:    strings.TrimSpace(session.SessionID),
		RequestedAt:  session.Snapshot.Approval.RequestedAt,
	})
}

func (s *Service) deletePendingApprovalLookup(ctx context.Context, session *agentruntime.RuntimeSession, fallbackConversationID string, fallbackUserID string) {
	if s == nil || s.pendingStore == nil {
		return
	}
	conversationID := strings.TrimSpace(fallbackConversationID)
	userID := strings.TrimSpace(fallbackUserID)
	if session != nil {
		conversationID = strings.TrimSpace(firstNonEmpty(session.Request.ConversationID, session.Snapshot.Request.ConversationID, conversationID))
		userID = strings.TrimSpace(firstNonEmpty(session.Request.UserID, session.Snapshot.Request.UserID, userID))
	}
	if conversationID == "" || userID == "" {
		return
	}
	_ = s.pendingStore.Delete(ctx, conversationID, userID)
}

func (s *Service) pendingApprovalBelongsToLookup(session *agentruntime.RuntimeSession, conversationID string, userID string) bool {
	if session == nil {
		return false
	}
	return strings.TrimSpace(firstNonEmpty(session.Request.ConversationID, session.Snapshot.Request.ConversationID)) == strings.TrimSpace(conversationID) &&
		strings.TrimSpace(firstNonEmpty(session.Request.UserID, session.Snapshot.Request.UserID)) == strings.TrimSpace(userID)
}
