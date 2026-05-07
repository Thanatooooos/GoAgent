package rag

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type RagTraceNodeRepository struct {
	db *gorm.DB
}

func NewRagTraceNodeRepository(db *gorm.DB) *RagTraceNodeRepository {
	return &RagTraceNodeRepository{db: db}
}

func (r *RagTraceNodeRepository) Create(ctx context.Context, node domain.RagTraceNode) (domain.RagTraceNode, error) {
	model := toRagTraceNodeModel(node)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.RagTraceNode{}, fmt.Errorf("create rag trace node: %w", err)
	}
	return toRagTraceNodeDomain(model), nil
}

func (r *RagTraceNodeRepository) UpdateByTraceIDAndNodeID(ctx context.Context, traceID string, nodeID string, node domain.RagTraceNode) error {
	_, err := r.UpdateWhere(ctx, port.RagTraceNodeConditions{TraceID: traceID, NodeID: nodeID}, port.RagTraceNodePatch{
		ParentNodeID: port.ValueOf(node.ParentNodeID),
		Depth:        port.ValueOf(node.Depth),
		NodeType:     port.ValueOf(node.NodeType),
		NodeName:     port.ValueOf(node.NodeName),
		ClassName:    port.ValueOf(node.ClassName),
		MethodName:   port.ValueOf(node.MethodName),
		Status:       port.ValueOf(node.Status),
		ErrorMessage: port.ValueOf(node.ErrorMessage),
		StartTime:    port.ValueOf(node.StartTime),
		EndTime:      port.ValueOf(node.EndTime),
		DurationMs:   port.ValueOf(node.DurationMs),
		ExtraData:    port.ValueOf(node.ExtraData),
		UpdateTime:   port.ValueOf(node.UpdateTime),
	})
	if err != nil {
		return fmt.Errorf("update rag trace node by trace id and node id: %w", err)
	}
	return nil
}

func (r *RagTraceNodeRepository) UpdateWhere(ctx context.Context, cond port.RagTraceNodeConditions, patch port.RagTraceNodePatch) (int64, error) {
	updates := buildRagTraceNodeUpdates(patch)
	if len(updates) == 0 {
		return 0, nil
	}
	if !hasRagTraceNodeConditions(cond) {
		return 0, conditionalUpdateRequiresConditions("rag trace node")
	}

	query := applyRagTraceNodeConditions(r.db.WithContext(ctx).Model(&models.RagTraceNodeModel{}), cond)
	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update rag trace node where: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *RagTraceNodeRepository) ListByTraceID(ctx context.Context, traceID string) ([]domain.RagTraceNode, error) {
	var items []models.RagTraceNodeModel
	if err := r.db.WithContext(ctx).
		Where("trace_id = ?", traceID).
		Order("start_time asc").
		Order("id asc").
		Limit(500).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list rag trace nodes by trace id: %w", err)
	}

	result := make([]domain.RagTraceNode, 0, len(items))
	for _, item := range items {
		result = append(result, toRagTraceNodeDomain(item))
	}
	return result, nil
}

func applyRagTraceNodeConditions(query *gorm.DB, cond port.RagTraceNodeConditions) *gorm.DB {
	if cond.ID != "" {
		query = query.Where("id = ?", cond.ID)
	}
	if cond.TraceID != "" {
		query = query.Where("trace_id = ?", cond.TraceID)
	}
	if cond.NodeID != "" {
		query = query.Where("node_id = ?", cond.NodeID)
	}
	if cond.ParentNodeID != "" {
		query = query.Where("parent_node_id = ?", cond.ParentNodeID)
	}
	if cond.StatusEQ != "" {
		query = query.Where("status = ?", cond.StatusEQ)
	}
	if cond.StatusNE != "" {
		query = query.Where("status <> ?", cond.StatusNE)
	}
	return query
}

func hasRagTraceNodeConditions(cond port.RagTraceNodeConditions) bool {
	return cond.ID != "" ||
		cond.TraceID != "" ||
		cond.NodeID != "" ||
		cond.ParentNodeID != "" ||
		cond.StatusEQ != "" ||
		cond.StatusNE != ""
}

func buildRagTraceNodeUpdates(patch port.RagTraceNodePatch) map[string]any {
	updates := map[string]any{}
	if patch.ParentNodeID.Set {
		updates["parent_node_id"] = patch.ParentNodeID.Value
	}
	if patch.Depth.Set {
		updates["depth"] = patch.Depth.Value
	}
	if patch.NodeType.Set {
		updates["node_type"] = patch.NodeType.Value
	}
	if patch.NodeName.Set {
		updates["node_name"] = patch.NodeName.Value
	}
	if patch.ClassName.Set {
		updates["class_name"] = patch.ClassName.Value
	}
	if patch.MethodName.Set {
		updates["method_name"] = patch.MethodName.Value
	}
	if patch.Status.Set {
		updates["status"] = patch.Status.Value
	}
	if patch.ErrorMessage.Set {
		updates["error_message"] = patch.ErrorMessage.Value
	}
	if patch.StartTime.Set {
		updates["start_time"] = patch.StartTime.Value
	}
	if patch.EndTime.Set {
		updates["end_time"] = patch.EndTime.Value
	}
	if patch.DurationMs.Set {
		updates["duration_ms"] = patch.DurationMs.Value
	}
	if patch.ExtraData.Set {
		updates["extra_data"] = patch.ExtraData.Value
	}
	if patch.UpdateTime.Set {
		updates["update_time"] = patch.UpdateTime.Value
	}
	return updates
}
