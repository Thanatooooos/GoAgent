package rag

import (
	"context"

	ragchat "local/rag-project/internal/app/rag/service/chat"
	ltmwriteback "local/rag-project/internal/app/rag/service/longtermmemory/writeback"
)

type chatLongTermMemoryWritebackAdapter struct {
	service *ltmwriteback.Service
}

func adaptLongTermMemoryWriteback(service *ltmwriteback.Service) ragchat.LongTermMemoryWriteback {
	if service == nil {
		return nil
	}
	return &chatLongTermMemoryWritebackAdapter{service: service}
}

func (a *chatLongTermMemoryWritebackAdapter) CapturePreferenceCandidate(ctx context.Context, input ragchat.LongTermMemoryWritebackInput) {
	if a == nil || a.service == nil {
		return
	}
	a.service.CapturePreferenceCandidate(ctx, ltmwriteback.Input{
		UserID:          input.UserID,
		Message:         input.Message,
		SourceMessageID: input.SourceMessageID,
	})
}

var _ ragchat.LongTermMemoryWriteback = (*chatLongTermMemoryWritebackAdapter)(nil)
