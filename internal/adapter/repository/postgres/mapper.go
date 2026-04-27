package postgres

import (
	"local/rag-project/internal/adapter/repository/postgres/models"
	"local/rag-project/internal/app/knowledge/domain"
)

func toKnowledgeBaseModel(kb domain.KnowledgeBase) models.KnowledgeBaseModel {
	return models.KnowledgeBaseModel{
		ID:             kb.ID,
		Name:           kb.Name,
		EmbeddingModel: kb.EmbeddingModel,
		CollectionName: kb.CollectionName,
		CreatedBy:      kb.CreatedBy,
		UpdatedBy:      kb.UpdatedBy,
		CreateTime:     kb.CreatedAt,
		UpdateTime:     kb.UpdatedAt,
	}
}

func toKnowledgeBaseDomain(model models.KnowledgeBaseModel) domain.KnowledgeBase {
	return domain.KnowledgeBase{
		ID:             model.ID,
		Name:           model.Name,
		EmbeddingModel: model.EmbeddingModel,
		CollectionName: model.CollectionName,
		CreatedBy:      model.CreatedBy,
		UpdatedBy:      model.UpdatedBy,
		CreatedAt:      model.CreateTime,
		UpdatedAt:      model.UpdateTime,
	}
}

func toKnowledgeDocumentModel(doc domain.KnowledgeDocument) models.KnowledgeDocumentModel {
	enabled := int16(0)
	if doc.Enabled {
		enabled = 1
	}

	var scheduleEnabled *int16
	if doc.ScheduleEnabled {
		v := int16(1)
		scheduleEnabled = &v
	}

	return models.KnowledgeDocumentModel{
		ID:              doc.ID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		DocName:         doc.Name,
		Enabled:         enabled,
		ChunkCount:      doc.ChunkCount,
		FileURL:         doc.FileURL,
		FileType:        doc.FileType,
		FileSize:        doc.FileSize,
		ProcessMode:     doc.ProcessMode,
		Status:          doc.Status,
		SourceType:      doc.SourceType,
		SourceLocation:  doc.SourceLocation,
		ScheduleEnabled: scheduleEnabled,
		ScheduleCron:    doc.ScheduleCron,
		ChunkStrategy:   doc.ChunkStrategy,
		ChunkConfig:     doc.ChunkConfig,
		PipelineID:      doc.PipelineID,
		CreatedBy:       doc.CreatedBy,
		UpdatedBy:       doc.UpdatedBy,
		CreateTime:      doc.CreatedAt,
		UpdateTime:      doc.UpdatedAt,
	}
}

func toKnowledgeDocumentDomain(model models.KnowledgeDocumentModel) domain.KnowledgeDocument {
	scheduleEnabled := false
	if model.ScheduleEnabled != nil && *model.ScheduleEnabled == 1 {
		scheduleEnabled = true
	}

	return domain.KnowledgeDocument{
		ID:              model.ID,
		KnowledgeBaseID: model.KnowledgeBaseID,
		Name:            model.DocName,
		Enabled:         model.Enabled == 1,
		ChunkCount:      model.ChunkCount,
		FileURL:         model.FileURL,
		FileType:        model.FileType,
		FileSize:        model.FileSize,
		ProcessMode:     model.ProcessMode,
		Status:          model.Status,
		SourceType:      model.SourceType,
		SourceLocation:  model.SourceLocation,
		ScheduleEnabled: scheduleEnabled,
		ScheduleCron:    model.ScheduleCron,
		ChunkStrategy:   model.ChunkStrategy,
		ChunkConfig:     model.ChunkConfig,
		PipelineID:      model.PipelineID,
		CreatedBy:       model.CreatedBy,
		UpdatedBy:       model.UpdatedBy,
		CreatedAt:       model.CreateTime,
		UpdatedAt:       model.UpdateTime,
	}
}
