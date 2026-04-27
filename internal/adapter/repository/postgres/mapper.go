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

func toKnowledgeDocumentScheduleModel(schedule domain.KnowledgeDocumentSchedule) models.KnowledgeDocumentScheduleModel {
	enabled := int16(0)
	if schedule.Enabled {
		enabled = 1
	}

	return models.KnowledgeDocumentScheduleModel{
		ID:              schedule.ID,
		DocumentID:      schedule.DocumentID,
		KnowledgeBaseID: schedule.KnowledgeBaseID,
		CronExpr:        schedule.CronExpr,
		Enabled:         enabled,
		NextRunTime:     schedule.NextRunTime,
		LastRunTime:     schedule.LastRunTime,
		LastSuccessTime: schedule.LastSuccessTime,
		LastStatus:      schedule.LastStatus,
		LastError:       schedule.LastError,
		LastETag:        schedule.LastETag,
		LastModified:    schedule.LastModified,
		LastContentHash: schedule.LastContentHash,
		LockOwner:       schedule.LockOwner,
		LockUntil:       schedule.LockUntil,
		CreateTime:      schedule.CreatedAt,
		UpdateTime:      schedule.UpdatedAt,
	}
}

func toKnowledgeDocumentScheduleDomain(model models.KnowledgeDocumentScheduleModel) domain.KnowledgeDocumentSchedule {
	return domain.KnowledgeDocumentSchedule{
		ID:              model.ID,
		DocumentID:      model.DocumentID,
		KnowledgeBaseID: model.KnowledgeBaseID,
		CronExpr:        model.CronExpr,
		Enabled:         model.Enabled == 1,
		NextRunTime:     model.NextRunTime,
		LastRunTime:     model.LastRunTime,
		LastSuccessTime: model.LastSuccessTime,
		LastStatus:      model.LastStatus,
		LastError:       model.LastError,
		LastETag:        model.LastETag,
		LastModified:    model.LastModified,
		LastContentHash: model.LastContentHash,
		LockOwner:       model.LockOwner,
		LockUntil:       model.LockUntil,
		CreatedAt:       model.CreateTime,
		UpdatedAt:       model.UpdateTime,
	}
}

func toKnowledgeDocumentScheduleExecModel(exec domain.KnowledgeDocumentScheduleExec) models.KnowledgeDocumentScheduleExecModel {
	return models.KnowledgeDocumentScheduleExecModel{
		ID:              exec.ID,
		ScheduleID:      exec.ScheduleID,
		DocumentID:      exec.DocumentID,
		KnowledgeBaseID: exec.KnowledgeBaseID,
		Status:          exec.Status,
		Message:         exec.Message,
		StartTime:       exec.StartTime,
		EndTime:         exec.EndTime,
		FileName:        exec.FileName,
		FileSize:        exec.FileSize,
		ContentHash:     exec.ContentHash,
		ETag:            exec.ETag,
		LastModified:    exec.LastModified,
		CreateTime:      exec.CreatedAt,
		UpdateTime:      exec.UpdatedAt,
	}
}

func toKnowledgeDocumentScheduleExecDomain(model models.KnowledgeDocumentScheduleExecModel) domain.KnowledgeDocumentScheduleExec {
	return domain.KnowledgeDocumentScheduleExec{
		ID:              model.ID,
		ScheduleID:      model.ScheduleID,
		DocumentID:      model.DocumentID,
		KnowledgeBaseID: model.KnowledgeBaseID,
		Status:          model.Status,
		Message:         model.Message,
		StartTime:       model.StartTime,
		EndTime:         model.EndTime,
		FileName:        model.FileName,
		FileSize:        model.FileSize,
		ContentHash:     model.ContentHash,
		ETag:            model.ETag,
		LastModified:    model.LastModified,
		CreatedAt:       model.CreateTime,
		UpdatedAt:       model.UpdateTime,
	}
}
