package knowledge

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/knowledge/models"
	sqlcqueries "local/rag-project/internal/adapter/repository/postgres/sqlc"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type KnowledgeDocumentRepository struct {
	db      *gorm.DB
	queries *sqlcqueries.Queries
}

func NewKnowledgeDocumentRepository(db *gorm.DB, queries *sqlcqueries.Queries) *KnowledgeDocumentRepository {
	return &KnowledgeDocumentRepository{
		db:      db,
		queries: queries,
	}
}

func (r *KnowledgeDocumentRepository) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	model := toKnowledgeDocumentModel(document)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.KnowledgeDocument{}, fmt.Errorf("create knowledge document: %w", err)
	}
	return toKnowledgeDocumentDomain(model), nil
}

func (r *KnowledgeDocumentRepository) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	rows, err := r.UpdateWhere(ctx, port.KnowledgeDocumentConditions{ID: document.ID}, port.KnowledgeDocumentPatch{
		Name:            port.ValueOf(document.Name),
		Enabled:         port.ValueOf(document.Enabled),
		ChunkCount:      port.ValueOf(document.ChunkCount),
		FileURL:         port.ValueOf(document.FileURL),
		FileType:        port.ValueOf(document.FileType),
		FileSize:        port.ValueOf(document.FileSize),
		ProcessMode:     port.ValueOf(document.ProcessMode),
		Status:          port.ValueOf(document.Status),
		SourceType:      port.ValueOf(document.SourceType),
		SourceLocation:  port.ValueOf(document.SourceLocation),
		ScheduleEnabled: port.ValueOf(boolPointer(document.ScheduleEnabled)),
		ScheduleCron:    port.ValueOf(document.ScheduleCron),
		ChunkStrategy:   port.ValueOf(document.ChunkStrategy),
		ChunkConfig:     port.ValueOf(document.ChunkConfig),
		PipelineID:      port.ValueOf(document.PipelineID),
		UpdatedBy:       port.ValueOf(document.UpdatedBy),
		UpdatedAt:       port.ValueOf(document.UpdatedAt),
	})
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	if rows == 0 {
		return domain.KnowledgeDocument{}, fmt.Errorf("update knowledge document: no rows affected")
	}
	return document, nil
}

func (r *KnowledgeDocumentRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
	updates := buildKnowledgeDocumentUpdates(patch)
	if len(updates) == 0 {
		return 0, nil
	}
	if !hasKnowledgeDocumentConditions(cond) {
		return 0, conditionalUpdateRequiresConditions("knowledge document")
	}

	query := r.applyKnowledgeDocumentConditions(r.db.WithContext(ctx).Model(&models.KnowledgeDocumentModel{}), cond)
	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update knowledge document with conditions: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *KnowledgeDocumentRepository) UpdateFields(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
	if len(where) == 0 {
		return 0, fmt.Errorf("update knowledge document fields: conditions are required")
	}

	updates, err := buildUpdateAssignments(set, knowledgeDocumentAssignmentColumn, knowledgeDocumentDBValue)
	if err != nil {
		return 0, fmt.Errorf("build knowledge document field updates: %w", err)
	}
	if len(updates) == 0 {
		return 0, nil
	}

	query, err := applyUpdatePredicates(
		r.db.WithContext(ctx).Model(&models.KnowledgeDocumentModel{}),
		where,
		knowledgeDocumentConditionColumn,
		knowledgeDocumentDBValue,
	)
	if err != nil {
		return 0, fmt.Errorf("build knowledge document field conditions: %w", err)
	}

	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update knowledge document fields: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *KnowledgeDocumentRepository) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&models.KnowledgeDocumentModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete knowledge document: %w", err)
	}
	return nil
}

func (r *KnowledgeDocumentRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
	var model models.KnowledgeDocumentModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.KnowledgeDocument{}, nil
	}
	if err != nil {
		return domain.KnowledgeDocument{}, fmt.Errorf("get knowledge document by id: %w", err)
	}
	return toKnowledgeDocumentDomain(model), nil
}

func (r *KnowledgeDocumentRepository) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentModel{}).
		Where("kb_id = ?", knowledgeBaseID).
		Count(&count).
		Error
	if err != nil {
		return 0, fmt.Errorf("count knowledge documents by knowledge base id: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeDocumentRepository) Count(ctx context.Context, filter port.KnowledgeDocumentListFilter) (int, error) {
	query := r.applyKnowledgeDocumentListFilter(r.db.WithContext(ctx).Model(&models.KnowledgeDocumentModel{}), filter)

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count knowledge documents: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeDocumentRepository) CountByKnowledgeBaseIDs(ctx context.Context, knowledgeBaseIDs []string) (map[string]int, error) {
	result := make(map[string]int, len(knowledgeBaseIDs))
	if len(knowledgeBaseIDs) == 0 {
		return result, nil
	}

	type item struct {
		KnowledgeBaseID string `gorm:"column:kb_id"`
		Count           int64  `gorm:"column:document_count"`
	}
	rows := make([]item, 0, len(knowledgeBaseIDs))
	if err := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentModel{}).
		Select("kb_id, COUNT(*) AS document_count").
		Where("kb_id IN ?", knowledgeBaseIDs).
		Group("kb_id").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("count knowledge documents by knowledge base ids: %w", err)
	}

	for _, row := range rows {
		result[row.KnowledgeBaseID] = int(row.Count)
	}
	return result, nil
}

func (r *KnowledgeDocumentRepository) CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	if r.queries == nil {
		return 0, fmt.Errorf("sqlc queries is required")
	}

	count, err := r.queries.CountChunkedDocumentsByKnowledgeBaseID(ctx, knowledgeBaseID)
	if err != nil {
		return 0, fmt.Errorf("count chunked knowledge documents by knowledge base id: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeDocumentRepository) List(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
	query := r.applyKnowledgeDocumentListFilter(r.db.WithContext(ctx).Model(&models.KnowledgeDocumentModel{}), filter).
		Order("create_time desc")

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.KnowledgeDocumentModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list knowledge documents: %w", err)
	}

	result := make([]domain.KnowledgeDocument, 0, len(items))
	for _, item := range items {
		result = append(result, toKnowledgeDocumentDomain(item))
	}
	return result, nil
}

func (r *KnowledgeDocumentRepository) applyKnowledgeDocumentListFilter(query *gorm.DB, filter port.KnowledgeDocumentListFilter) *gorm.DB {
	if filter.KnowledgeBaseID != "" {
		query = query.Where("kb_id = ?", filter.KnowledgeBaseID)
	}
	if filter.SourceType != "" {
		query = query.Where("source_type = ?", filter.SourceType)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Enabled != nil {
		if *filter.Enabled {
			query = query.Where("enabled = 1")
		} else {
			query = query.Where("enabled = 0")
		}
	}
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		query = query.Where("(doc_name ILIKE ? OR source_location ILIKE ?)", like, like)
	}
	return query
}

func (r *KnowledgeDocumentRepository) applyKnowledgeDocumentConditions(query *gorm.DB, cond port.KnowledgeDocumentConditions) *gorm.DB {
	if cond.ID != "" {
		query = query.Where("id = ?", cond.ID)
	}
	if cond.KnowledgeBaseID != "" {
		query = query.Where("kb_id = ?", cond.KnowledgeBaseID)
	}
	if cond.StatusEQ != "" {
		query = query.Where("status = ?", cond.StatusEQ)
	}
	if cond.StatusNE != "" {
		query = query.Where("status <> ?", cond.StatusNE)
	}
	if cond.SourceTypeEQ != "" {
		query = query.Where("source_type = ?", cond.SourceTypeEQ)
	}
	if cond.Enabled != nil {
		query = query.Where("enabled = ?", boolToFlag(*cond.Enabled))
	}
	if cond.Deleted != nil {
		query = query.Where("deleted = ?", boolToDeletedFlag(*cond.Deleted))
	}
	return query
}

func hasKnowledgeDocumentConditions(cond port.KnowledgeDocumentConditions) bool {
	return cond.ID != "" ||
		cond.KnowledgeBaseID != "" ||
		cond.StatusEQ != "" ||
		cond.StatusNE != "" ||
		cond.SourceTypeEQ != "" ||
		cond.Enabled != nil ||
		cond.Deleted != nil
}

func buildKnowledgeDocumentUpdates(patch port.KnowledgeDocumentPatch) map[string]any {
	updates := map[string]any{}
	if patch.Name.Set {
		updates["doc_name"] = patch.Name.Value
	}
	if patch.Enabled.Set {
		updates["enabled"] = boolToFlag(patch.Enabled.Value)
	}
	if patch.ChunkCount.Set {
		updates["chunk_count"] = patch.ChunkCount.Value
	}
	if patch.FileURL.Set {
		updates["file_url"] = patch.FileURL.Value
	}
	if patch.FileType.Set {
		updates["file_type"] = patch.FileType.Value
	}
	if patch.FileSize.Set {
		updates["file_size"] = patch.FileSize.Value
	}
	if patch.ProcessMode.Set {
		updates["process_mode"] = patch.ProcessMode.Value
	}
	if patch.Status.Set {
		updates["status"] = patch.Status.Value
	}
	if patch.SourceType.Set {
		updates["source_type"] = patch.SourceType.Value
	}
	if patch.SourceLocation.Set {
		updates["source_location"] = patch.SourceLocation.Value
	}
	if patch.ScheduleEnabled.Set {
		if patch.ScheduleEnabled.Value == nil {
			updates["schedule_enabled"] = nil
		} else {
			updates["schedule_enabled"] = boolToFlag(*patch.ScheduleEnabled.Value)
		}
	}
	if patch.ScheduleCron.Set {
		updates["schedule_cron"] = patch.ScheduleCron.Value
	}
	if patch.ChunkStrategy.Set {
		updates["chunk_strategy"] = patch.ChunkStrategy.Value
	}
	if patch.ChunkConfig.Set {
		updates["chunk_config"] = patch.ChunkConfig.Value
	}
	if patch.PipelineID.Set {
		updates["pipeline_id"] = patch.PipelineID.Value
	}
	if patch.UpdatedBy.Set {
		updates["updated_by"] = patch.UpdatedBy.Value
	}
	if patch.UpdatedAt.Set {
		updates["update_time"] = patch.UpdatedAt.Value
	}
	return updates
}

func knowledgeDocumentConditionColumn(field port.FieldKey) (string, bool) {
	column, ok := knowledgeDocumentConditionColumns[field]
	return column, ok
}

func knowledgeDocumentAssignmentColumn(field port.FieldKey) (string, bool) {
	column, ok := knowledgeDocumentAssignmentColumns[field]
	return column, ok
}

func knowledgeDocumentDBValue(field port.FieldKey, value any) (any, error) {
	switch field {
	case port.KnowledgeDocument.Enabled.Key:
		boolValue, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("field %s expects bool value", field)
		}
		return boolToFlag(boolValue), nil
	case port.KnowledgeDocument.Deleted.Key:
		boolValue, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("field %s expects bool value", field)
		}
		return boolToDeletedFlag(boolValue), nil
	case port.KnowledgeDocument.ScheduleEnabled.Key:
		if value == nil {
			return nil, nil
		}
		boolValue, ok := value.(*bool)
		if !ok {
			return nil, fmt.Errorf("field %s expects *bool value", field)
		}
		if boolValue == nil {
			return nil, nil
		}
		return boolToFlag(*boolValue), nil
	default:
		return value, nil
	}
}

var knowledgeDocumentConditionColumns = map[port.FieldKey]string{
	port.KnowledgeDocument.ID.Key:              "id",
	port.KnowledgeDocument.KnowledgeBaseID.Key: "kb_id",
	port.KnowledgeDocument.Name.Key:            "doc_name",
	port.KnowledgeDocument.Enabled.Key:         "enabled",
	port.KnowledgeDocument.ChunkCount.Key:      "chunk_count",
	port.KnowledgeDocument.FileURL.Key:         "file_url",
	port.KnowledgeDocument.FileType.Key:        "file_type",
	port.KnowledgeDocument.FileSize.Key:        "file_size",
	port.KnowledgeDocument.ProcessMode.Key:     "process_mode",
	port.KnowledgeDocument.Status.Key:          "status",
	port.KnowledgeDocument.SourceType.Key:      "source_type",
	port.KnowledgeDocument.SourceLocation.Key:  "source_location",
	port.KnowledgeDocument.ScheduleEnabled.Key: "schedule_enabled",
	port.KnowledgeDocument.ScheduleCron.Key:    "schedule_cron",
	port.KnowledgeDocument.ChunkStrategy.Key:   "chunk_strategy",
	port.KnowledgeDocument.ChunkConfig.Key:     "chunk_config",
	port.KnowledgeDocument.PipelineID.Key:      "pipeline_id",
	port.KnowledgeDocument.UpdatedBy.Key:       "updated_by",
	port.KnowledgeDocument.UpdatedAt.Key:       "update_time",
	port.KnowledgeDocument.Deleted.Key:         "deleted",
}

var knowledgeDocumentAssignmentColumns = map[port.FieldKey]string{
	port.KnowledgeDocument.Name.Key:            "doc_name",
	port.KnowledgeDocument.Enabled.Key:         "enabled",
	port.KnowledgeDocument.ChunkCount.Key:      "chunk_count",
	port.KnowledgeDocument.FileURL.Key:         "file_url",
	port.KnowledgeDocument.FileType.Key:        "file_type",
	port.KnowledgeDocument.FileSize.Key:        "file_size",
	port.KnowledgeDocument.ProcessMode.Key:     "process_mode",
	port.KnowledgeDocument.Status.Key:          "status",
	port.KnowledgeDocument.SourceType.Key:      "source_type",
	port.KnowledgeDocument.SourceLocation.Key:  "source_location",
	port.KnowledgeDocument.ScheduleEnabled.Key: "schedule_enabled",
	port.KnowledgeDocument.ScheduleCron.Key:    "schedule_cron",
	port.KnowledgeDocument.ChunkStrategy.Key:   "chunk_strategy",
	port.KnowledgeDocument.ChunkConfig.Key:     "chunk_config",
	port.KnowledgeDocument.PipelineID.Key:      "pipeline_id",
	port.KnowledgeDocument.UpdatedBy.Key:       "updated_by",
	port.KnowledgeDocument.UpdatedAt.Key:       "update_time",
}
