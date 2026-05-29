package knowledge

import (
	"math"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
)

func (h *KnowledgeDocumentHandler) ChunkLogs(c *gin.Context) {
	result, err := h.service.PageChunkLogs(c.Request.Context(), service.KnowledgeDocumentChunkLogPageInput{
		DocumentID: c.Param("docId"),
		Page:       parsePositiveInt(c.Query("current"), 1),
		PageSize:   parsePositiveInt(c.Query("size"), 10),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	records := make([]knowledgeDocumentChunkLogVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toKnowledgeDocumentChunkLogVO(item))
	}
	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	writeSuccess(c, pageResult[knowledgeDocumentChunkLogVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func (h *KnowledgeDocumentHandler) ScheduleExecs(c *gin.Context) {
	result, err := h.service.PageScheduleExecs(c.Request.Context(), service.PageKnowledgeDocumentScheduleExecInput{
		DocumentID: c.Param("docId"),
		Page:       parsePositiveInt(c.Query("current"), 1),
		PageSize:   parsePositiveInt(c.Query("size"), 10),
		Status:     c.Query("status"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	records := make([]knowledgeDocumentScheduleExecVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toKnowledgeDocumentScheduleExecVO(item))
	}
	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	writeSuccess(c, pageResult[knowledgeDocumentScheduleExecVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func toKnowledgeDocumentChunkLogVO(item service.KnowledgeDocumentChunkLogItem) knowledgeDocumentChunkLogVO {
	logItem := item.Log
	var ingestionTask *knowledgeDocumentIngestionTaskVO
	if item.IngestionTask != nil {
		ingestionTask = &knowledgeDocumentIngestionTaskVO{
			ID:             item.IngestionTask.ID,
			PipelineID:     item.IngestionTask.PipelineID,
			SourceType:     item.IngestionTask.SourceType,
			SourceLocation: item.IngestionTask.SourceLocation,
			SourceFileName: item.IngestionTask.SourceFileName,
			Status:         item.IngestionTask.Status,
			ChunkCount:     item.IngestionTask.ChunkCount,
			ErrorMessage:   item.IngestionTask.ErrorMessage,
			Metadata:       item.IngestionTask.Metadata,
			StartedAt:      item.IngestionTask.StartedAt,
			CompletedAt:    item.IngestionTask.CompletedAt,
			CreateTime:     timePointer(item.IngestionTask.CreatedAt),
			UpdateTime:     timePointer(item.IngestionTask.UpdatedAt),
		}
	}
	nodes := make([]knowledgeDocumentIngestionNodeVO, 0, len(item.IngestionNodes))
	for _, node := range item.IngestionNodes {
		nodes = append(nodes, knowledgeDocumentIngestionNodeVO{
			ID:           node.ID,
			TaskID:       node.TaskID,
			NodeID:       node.NodeID,
			NodeType:     node.NodeType,
			NodeOrder:    node.NodeOrder,
			Status:       node.Status,
			DurationMs:   node.DurationMs,
			Message:      node.Message,
			ErrorMessage: node.ErrorMessage,
			Output:       node.Output,
			CreateTime:   timePointer(node.CreatedAt),
			UpdateTime:   timePointer(node.UpdatedAt),
		})
	}
	return knowledgeDocumentChunkLogVO{
		ID:              logItem.ID,
		DocumentID:      logItem.DocumentID,
		Status:          logItem.Status,
		ProcessMode:     logItem.ProcessMode,
		ChunkStrategy:   logItem.ChunkStrategy,
		PipelineID:      logItem.PipelineID,
		ExtractDuration: logItem.ExtractDuration,
		ChunkDuration:   logItem.ChunkDuration,
		EmbedDuration:   logItem.EmbedDuration,
		PersistDuration: logItem.PersistDuration,
		TotalDuration:   logItem.TotalDuration,
		ChunkCount:      logItem.ChunkCount,
		ErrorMessage:    logItem.ErrorMessage,
		StartTime:       logItem.StartTime,
		EndTime:         logItem.EndTime,
		CreateTime:      timePointer(logItem.CreatedAt),
		IngestionTask:   ingestionTask,
		IngestionNodes:  nodes,
	}
}

func toKnowledgeDocumentScheduleExecVO(item domain.KnowledgeDocumentScheduleExec) knowledgeDocumentScheduleExecVO {
	return knowledgeDocumentScheduleExecVO{
		ID:              item.ID,
		ScheduleID:      item.ScheduleID,
		DocumentID:      item.DocumentID,
		KnowledgeBaseID: item.KnowledgeBaseID,
		Status:          item.Status,
		Message:         item.Message,
		FileName:        item.FileName,
		FileSize:        item.FileSize,
		ContentHash:     item.ContentHash,
		ETag:            item.ETag,
		LastModified:    item.LastModified,
		StartTime:       item.StartTime,
		EndTime:         item.EndTime,
		CreateTime:      timePointer(item.CreatedAt),
		UpdateTime:      timePointer(item.UpdatedAt),
	}
}
