package observability

import (
	"context"
	"strings"

	"local/rag-project/internal/framework/log"
)

const subsystemLongTermMemory = "long_term_memory"

func LogWritebackStarted(ctx context.Context, userID string, sourceMessageID string, messageLength int) {
	log.FromContext(ctx).Infow(
		"long-term memory writeback started",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"source_message_id", strings.TrimSpace(sourceMessageID),
			"message_length", messageLength,
		)...,
	)
}

func LogWritebackSkipped(ctx context.Context, userID string, sourceMessageID string, skipReason string) {
	log.FromContext(ctx).Infow(
		"long-term memory writeback skipped",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"source_message_id", strings.TrimSpace(sourceMessageID),
			"skip_reason", strings.TrimSpace(skipReason),
		)...,
	)
}

func LogWritebackExtracted(ctx context.Context, userID string, sourceMessageID string, canonicalKey string, confidence float64) {
	log.FromContext(ctx).Infow(
		"long-term memory candidate extracted",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"source_message_id", strings.TrimSpace(sourceMessageID),
			"canonical_key", strings.TrimSpace(canonicalKey),
			"confidence", confidence,
		)...,
	)
}

func LogWritebackRejected(ctx context.Context, userID string, sourceMessageID string, canonicalKey string, rejectionReason string, confidence float64) {
	log.FromContext(ctx).Infow(
		"long-term memory candidate rejected",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"source_message_id", strings.TrimSpace(sourceMessageID),
			"canonical_key", strings.TrimSpace(canonicalKey),
			"rejection_reason", strings.TrimSpace(rejectionReason),
			"confidence", confidence,
		)...,
	)
}

func LogWritebackFailed(ctx context.Context, userID string, sourceMessageID string, failureReason string) {
	log.FromContext(ctx).Warnw(
		"long-term memory writeback failed",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"source_message_id", strings.TrimSpace(sourceMessageID),
			"failure_reason", strings.TrimSpace(failureReason),
		)...,
	)
}

func LogWritebackPersisted(ctx context.Context, userID string, sourceMessageID string, candidateID string, canonicalKey string, confidence float64) {
	log.FromContext(ctx).Infow(
		"long-term memory candidate persisted",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"source_message_id", strings.TrimSpace(sourceMessageID),
			"candidate_id", strings.TrimSpace(candidateID),
			"canonical_key", strings.TrimSpace(canonicalKey),
			"confidence", confidence,
			"status", "pending",
		)...,
	)
}

func LogCandidatePersisted(ctx context.Context, userID string, candidateID string, canonicalKey string, confidence float64) {
	log.FromContext(ctx).Infow(
		"long-term memory candidate stored as pending",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"candidate_id", strings.TrimSpace(candidateID),
			"canonical_key", strings.TrimSpace(canonicalKey),
			"confidence", confidence,
			"status", "pending",
		)...,
	)
}

func LogCandidateConfirmationRequested(ctx context.Context, userID string, candidateID string) {
	log.FromContext(ctx).Infow(
		"long-term memory candidate confirmation requested",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"candidate_id", strings.TrimSpace(candidateID),
		)...,
	)
}

func LogCandidateConfirmed(ctx context.Context, userID string, candidateID string, canonicalKey string, fromStatus string, toStatus string) {
	log.FromContext(ctx).Infow(
		"long-term memory candidate confirmed",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"candidate_id", strings.TrimSpace(candidateID),
			"canonical_key", strings.TrimSpace(canonicalKey),
			"status_from", strings.TrimSpace(fromStatus),
			"status_to", strings.TrimSpace(toStatus),
		)...,
	)
}

func LogCandidateConfirmationRejected(ctx context.Context, userID string, candidateID string, reason string) {
	log.FromContext(ctx).Warnw(
		"long-term memory candidate confirmation rejected",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"candidate_id", strings.TrimSpace(candidateID),
			"reason", strings.TrimSpace(reason),
		)...,
	)
}

func LogCandidateRejectionRequested(ctx context.Context, userID string, candidateID string) {
	log.FromContext(ctx).Infow(
		"long-term memory candidate rejection requested",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"candidate_id", strings.TrimSpace(candidateID),
		)...,
	)
}

func LogCandidateRejected(ctx context.Context, userID string, candidateID string, canonicalKey string, fromStatus string, toStatus string) {
	log.FromContext(ctx).Infow(
		"long-term memory candidate rejected by user",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"candidate_id", strings.TrimSpace(candidateID),
			"canonical_key", strings.TrimSpace(canonicalKey),
			"status_from", strings.TrimSpace(fromStatus),
			"status_to", strings.TrimSpace(toStatus),
		)...,
	)
}

func LogCandidateRejectionFailed(ctx context.Context, userID string, candidateID string, reason string) {
	log.FromContext(ctx).Warnw(
		"long-term memory candidate rejection failed",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"candidate_id", strings.TrimSpace(candidateID),
			"reason", strings.TrimSpace(reason),
		)...,
	)
}

func LogRecallStarted(ctx context.Context, userID string, query string, scopeTypes []string, memoryTypes []string, statuses []string, knowledgeBaseCount int) {
	log.FromContext(ctx).Infow(
		"long-term memory recall started",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"query_length", len(strings.TrimSpace(query)),
			"scope_types", trimStrings(scopeTypes),
			"memory_types", trimStrings(memoryTypes),
			"statuses", trimStrings(statuses),
			"knowledge_base_count", knowledgeBaseCount,
		)...,
	)
}

func LogRecallCompleted(
	ctx context.Context,
	userID string,
	candidateCount int,
	selectedCount int,
	ruleCount int,
	factSelectedCount int,
	ruleCacheLayer string,
	factCacheLayer string,
	embeddingCacheLayer string,
	truncated bool,
) {
	log.FromContext(ctx).Infow(
		"long-term memory recall completed",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"candidate_count", candidateCount,
			"selected_count", selectedCount,
			"rule_count", ruleCount,
			"fact_selected_count", factSelectedCount,
			"rule_cache_layer", strings.TrimSpace(ruleCacheLayer),
			"fact_cache_layer", strings.TrimSpace(factCacheLayer),
			"embedding_cache_layer", strings.TrimSpace(embeddingCacheLayer),
			"truncated", truncated,
		)...,
	)
}

func LogRecallFailed(ctx context.Context, userID string, reason string, err error) {
	log.FromContext(ctx).Warnw(
		"long-term memory recall failed",
		baseFields(
			"user_id", strings.TrimSpace(userID),
			"reason", strings.TrimSpace(reason),
			"error", err,
		)...,
	)
}

func LogRecallOverriddenByCurrentTurn(ctx context.Context, canonicalKey string, historicalValue string, currentValue string) {
	log.FromContext(ctx).Infow(
		"rag chat preference recall overridden by current turn input",
		baseFields(
			"canonical_key", strings.TrimSpace(canonicalKey),
			"historical_value", strings.TrimSpace(historicalValue),
			"current_value", strings.TrimSpace(currentValue),
		)...,
	)
}

func baseFields(fields ...interface{}) []interface{} {
	return append([]interface{}{"subsystem", subsystemLongTermMemory}, fields...)
}

func trimStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}
	return trimmed
}
