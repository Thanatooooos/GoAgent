package rag

import (
	"strings"

	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
)

func toConversationModel(item domain.Conversation) models.ConversationModel {
	return models.ConversationModel{
		ID:             item.ID,
		ConversationID: item.ConversationID,
		UserID:         item.UserID,
		Title:          item.Title,
		LastTime:       item.LastTime,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func toConversationDomain(item models.ConversationModel) domain.Conversation {
	return domain.Conversation{
		ID:             item.ID,
		ConversationID: item.ConversationID,
		UserID:         item.UserID,
		Title:          item.Title,
		LastTime:       item.LastTime,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func toConversationMessageModel(item domain.ConversationMessage) models.ConversationMessageModel {
	return models.ConversationMessageModel{
		ID:               item.ID,
		ConversationID:   item.ConversationID,
		UserID:           item.UserID,
		Role:             item.Role,
		Content:          item.Content,
		RawContent:       item.RawContent,
		ContentSummary:   item.ContentSummary,
		IsSummarized:     item.IsSummarized,
		ThinkingContent:  item.ThinkingContent,
		ThinkingDuration: item.ThinkingDuration,
		CreateTime:       item.CreateTime,
		UpdateTime:       item.UpdateTime,
	}
}

func toConversationMessageDomain(item models.ConversationMessageModel) domain.ConversationMessage {
	return domain.ConversationMessage{
		ID:               item.ID,
		ConversationID:   item.ConversationID,
		UserID:           item.UserID,
		Role:             item.Role,
		Content:          item.Content,
		RawContent:       item.RawContent,
		ContentSummary:   item.ContentSummary,
		IsSummarized:     item.IsSummarized,
		ThinkingContent:  item.ThinkingContent,
		ThinkingDuration: item.ThinkingDuration,
		CreateTime:       item.CreateTime,
		UpdateTime:       item.UpdateTime,
	}
}

func toConversationSummaryModel(item domain.ConversationSummary) models.ConversationSummaryModel {
	summaryVersion := item.SummaryVersion
	if summaryVersion <= 0 {
		summaryVersion = domain.SummaryVersionV1
	}
	qualityStatus := strings.TrimSpace(item.QualityStatus)
	if qualityStatus == "" {
		qualityStatus = domain.SummaryQualityUnchecked
	}
	return models.ConversationSummaryModel{
		ID:                    item.ID,
		ConversationID:        item.ConversationID,
		UserID:                item.UserID,
		LastMessageID:         item.LastMessageID,
		Content:               item.Content,
		StructuredSummaryJSON: strings.TrimSpace(item.StructuredSummaryJSON),
		SummaryVersion:        summaryVersion,
		CoveredFromMessageID:  item.CoveredFromMessageID,
		CoveredToMessageID:    item.CoveredToMessageID,
		SourceMessageCount:    item.SourceMessageCount,
		QualityStatus:         qualityStatus,
		LastRebuildReason:     item.LastRebuildReason,
		CreateTime:            item.CreateTime,
		UpdateTime:            item.UpdateTime,
	}
}

func toConversationSummaryDomain(item models.ConversationSummaryModel) domain.ConversationSummary {
	return domain.ConversationSummary{
		ID:                    item.ID,
		ConversationID:        item.ConversationID,
		UserID:                item.UserID,
		LastMessageID:         item.LastMessageID,
		Content:               item.Content,
		StructuredSummaryJSON: item.StructuredSummaryJSON,
		SummaryVersion:        item.SummaryVersion,
		CoveredFromMessageID:  item.CoveredFromMessageID,
		CoveredToMessageID:    item.CoveredToMessageID,
		SourceMessageCount:    item.SourceMessageCount,
		QualityStatus:         item.QualityStatus,
		LastRebuildReason:     item.LastRebuildReason,
		CreateTime:            item.CreateTime,
		UpdateTime:            item.UpdateTime,
	}
}

func toMemoryItemModel(item domain.MemoryItem) models.MemoryItemModel {
	return models.MemoryItemModel{
		ID:               item.ID,
		UserID:           item.UserID,
		ScopeType:        item.ScopeType,
		ScopeID:          item.ScopeID,
		Namespace:        item.Namespace,
		MemoryType:       item.MemoryType,
		Category:         item.Category,
		CanonicalKey:     item.CanonicalKey,
		ValueType:        item.ValueType,
		ValueJSON:        item.ValueJSON,
		DisplayValue:     item.DisplayValue,
		SourceMessageID:  item.SourceMessageID,
		Content:          item.Content,
		Summary:          item.Summary,
		Confidence:       item.Confidence,
		Importance:       item.Importance,
		Status:           item.Status,
		LastConfirmedAt:  item.LastConfirmedAt,
		LastUsedAt:       item.LastUsedAt,
		ExpiresAt:        item.ExpiresAt,
		SupersedesID:     item.SupersedesID,
		ExtractionMethod: item.ExtractionMethod,
		CreatedBy:        item.CreatedBy,
		UpdatedBy:        item.UpdatedBy,
		CreateTime:       item.CreateTime,
		UpdateTime:       item.UpdateTime,
	}
}

func toMemoryItemDomain(item models.MemoryItemModel) domain.MemoryItem {
	return domain.MemoryItem{
		ID:               item.ID,
		UserID:           item.UserID,
		ScopeType:        item.ScopeType,
		ScopeID:          item.ScopeID,
		Namespace:        item.Namespace,
		MemoryType:       item.MemoryType,
		Category:         item.Category,
		CanonicalKey:     item.CanonicalKey,
		ValueType:        item.ValueType,
		ValueJSON:        item.ValueJSON,
		DisplayValue:     item.DisplayValue,
		SourceMessageID:  item.SourceMessageID,
		Content:          item.Content,
		Summary:          item.Summary,
		Confidence:       item.Confidence,
		Importance:       item.Importance,
		Status:           item.Status,
		LastConfirmedAt:  item.LastConfirmedAt,
		LastUsedAt:       item.LastUsedAt,
		ExpiresAt:        item.ExpiresAt,
		SupersedesID:     item.SupersedesID,
		ExtractionMethod: item.ExtractionMethod,
		CreatedBy:        item.CreatedBy,
		UpdatedBy:        item.UpdatedBy,
		CreateTime:       item.CreateTime,
		UpdateTime:       item.UpdateTime,
	}
}

func toSessionChunkModel(item domain.SessionChunk) models.SessionChunkModel {
	return models.SessionChunkModel{
		ID:             item.ID,
		ConversationID: item.ConversationID,
		MessageID:      item.MessageID,
		UserID:         item.UserID,
		ChunkIndex:     item.ChunkIndex,
		Content:        item.Content,
		ContentSummary: item.ContentSummary,
		TokenEstimate:  item.TokenEstimate,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func toSessionChunkDomain(item models.SessionChunkModel) domain.SessionChunk {
	return domain.SessionChunk{
		ID:             item.ID,
		ConversationID: item.ConversationID,
		MessageID:      item.MessageID,
		UserID:         item.UserID,
		ChunkIndex:     item.ChunkIndex,
		Content:        item.Content,
		ContentSummary: item.ContentSummary,
		TokenEstimate:  item.TokenEstimate,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func toMessageFeedbackModel(item domain.MessageFeedback) models.MessageFeedbackModel {
	return models.MessageFeedbackModel{
		ID:             item.ID,
		MessageID:      item.MessageID,
		ConversationID: item.ConversationID,
		UserID:         item.UserID,
		Vote:           int16(item.Vote),
		Reason:         item.Reason,
		Comment:        item.Comment,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func toMessageFeedbackDomain(item models.MessageFeedbackModel) domain.MessageFeedback {
	return domain.MessageFeedback{
		ID:             item.ID,
		MessageID:      item.MessageID,
		ConversationID: item.ConversationID,
		UserID:         item.UserID,
		Vote:           int(item.Vote),
		Reason:         item.Reason,
		Comment:        item.Comment,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func toRagTraceRunModel(item domain.RagTraceRun) models.RagTraceRunModel {
	return models.RagTraceRunModel{
		ID:             item.ID,
		TraceID:        item.TraceID,
		TraceName:      item.TraceName,
		EntryMethod:    item.EntryMethod,
		ConversationID: item.ConversationID,
		TaskID:         item.TaskID,
		UserID:         item.UserID,
		Status:         item.Status,
		ErrorMessage:   item.ErrorMessage,
		StartTime:      item.StartTime,
		EndTime:        item.EndTime,
		DurationMs:     item.DurationMs,
		ExtraData:      item.ExtraData,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func toRagTraceRunDomain(item models.RagTraceRunModel) domain.RagTraceRun {
	return domain.RagTraceRun{
		ID:             item.ID,
		TraceID:        item.TraceID,
		TraceName:      item.TraceName,
		EntryMethod:    item.EntryMethod,
		ConversationID: item.ConversationID,
		TaskID:         item.TaskID,
		UserID:         item.UserID,
		Status:         item.Status,
		ErrorMessage:   item.ErrorMessage,
		StartTime:      item.StartTime,
		EndTime:        item.EndTime,
		DurationMs:     item.DurationMs,
		ExtraData:      item.ExtraData,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func toRagTraceNodeModel(item domain.RagTraceNode) models.RagTraceNodeModel {
	return models.RagTraceNodeModel{
		ID:           item.ID,
		TraceID:      item.TraceID,
		NodeID:       item.NodeID,
		ParentNodeID: item.ParentNodeID,
		Depth:        item.Depth,
		NodeType:     item.NodeType,
		NodeName:     item.NodeName,
		ClassName:    item.ClassName,
		MethodName:   item.MethodName,
		Status:       item.Status,
		ErrorMessage: item.ErrorMessage,
		StartTime:    item.StartTime,
		EndTime:      item.EndTime,
		DurationMs:   item.DurationMs,
		ExtraData:    item.ExtraData,
		CreateTime:   item.CreateTime,
		UpdateTime:   item.UpdateTime,
	}
}

func toRagTraceNodeDomain(item models.RagTraceNodeModel) domain.RagTraceNode {
	return domain.RagTraceNode{
		ID:           item.ID,
		TraceID:      item.TraceID,
		NodeID:       item.NodeID,
		ParentNodeID: item.ParentNodeID,
		Depth:        item.Depth,
		NodeType:     item.NodeType,
		NodeName:     item.NodeName,
		ClassName:    item.ClassName,
		MethodName:   item.MethodName,
		Status:       item.Status,
		ErrorMessage: item.ErrorMessage,
		StartTime:    item.StartTime,
		EndTime:      item.EndTime,
		DurationMs:   item.DurationMs,
		ExtraData:    item.ExtraData,
		CreateTime:   item.CreateTime,
		UpdateTime:   item.UpdateTime,
	}
}
