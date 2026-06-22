package chat

import (
	"strings"

	"local/rag-project/internal/app/rag/domain"
)

func (s *RagChatService) emitPreparedObservabilityEvents(prepared ragChatPreparedState, question string, sink RagChatEventSink) {
	if s == nil || sink == nil {
		return
	}
	if payload, ok := buildMemoryStoredPayload(prepared.userMessage); ok {
		_ = sink.SendMemoryStored(payload)
	}
	if payload, ok := buildSessionRecallPayload(prepared.sessionRecall, prepared.rewriteResult.RewrittenQuestion, question); ok {
		_ = sink.SendSessionRecall(payload)
	}
}

func buildMemoryStoredPayload(message domain.ConversationMessage) (RagChatMemoryStoredPayload, bool) {
	if !message.IsSummarized {
		return RagChatMemoryStoredPayload{}, false
	}
	return RagChatMemoryStoredPayload{
		ConversationID:   strings.TrimSpace(message.ConversationID),
		MessageID:        strings.TrimSpace(message.ID),
		IsSummarized:     message.IsSummarized,
		ContentSummary:   strings.TrimSpace(message.ContentSummary),
		RawContentLength: len([]rune(strings.TrimSpace(message.RawContent))),
	}, true
}

func buildSessionRecallPayload(result SessionRecallResult, rewrittenQuestion string, fallbackQuestion string) (RagChatSessionRecallPayload, bool) {
	if !result.Used || len(result.Hits) == 0 {
		return RagChatSessionRecallPayload{}, false
	}

	query := strings.TrimSpace(rewrittenQuestion)
	if query == "" {
		query = strings.TrimSpace(fallbackQuestion)
	}

	hits := make([]RagChatSessionRecallHitPayload, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hits = append(hits, RagChatSessionRecallHitPayload{
			MessageID:     strings.TrimSpace(hit.MessageID),
			ChunkIndex:    hit.ChunkIndex,
			Score:         hit.Score,
			Summary:       strings.TrimSpace(hit.Summary),
			Excerpt:       strings.TrimSpace(hit.Excerpt),
			SourceChunkID: strings.TrimSpace(hit.SourceChunkID),
		})
	}

	return RagChatSessionRecallPayload{
		Query:          query,
		Used:           result.Used,
		HitCount:       len(hits),
		TopScore:       result.TopScore,
		TruncatedBy:    strings.TrimSpace(result.TruncatedBy),
		CandidateCount: result.CandidateCount,
		Hits:           hits,
	}, true
}
