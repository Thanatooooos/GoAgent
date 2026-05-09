package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

const (
	defaultPipelinePage     = 1
	defaultPipelinePageSize = 10
	maxPipelinePageSize     = 100
)

// PagePipelinesInput 描述 pipeline 分页查询参数。
type PagePipelinesInput struct {
	Page     int
	PageSize int
	Keyword  string
}

// PipelinePageResult 表示 pipeline 分页结果。
type PipelinePageResult struct {
	Items    []domain.Pipeline
	Total    int
	Page     int
	PageSize int
}

// CreatePipelineInput 描述创建 pipeline 的入参。
type CreatePipelineInput struct {
	Name        string
	Description string
	Nodes       []domain.PipelineNode
	CreatedBy   string
}

// UpdatePipelineInput 描述更新 pipeline 的入参。
type UpdatePipelineInput struct {
	ID          string
	Name        string
	Description string
	Nodes       []domain.PipelineNode
	UpdatedBy   string
}

// PipelineService 负责 pipeline 的管理与校验。
type PipelineService struct {
	repo port.PipelineRepository
	now  func() time.Time
}

// NewPipelineService 创建 pipeline 服务。
func NewPipelineService(repo port.PipelineRepository) *PipelineService {
	return &PipelineService{
		repo: repo,
		now:  time.Now,
	}
}

// Page 分页查询 pipeline 列表。
func (s *PipelineService) Page(ctx context.Context, input PagePipelinesInput) (PipelinePageResult, error) {
	if s == nil || s.repo == nil {
		return PipelinePageResult{}, exception.NewServiceException("ingestion pipeline repository is required", nil)
	}

	page := normalizePipelinePage(input.Page)
	pageSize := normalizePipelinePageSize(input.PageSize)
	filter := port.PipelineListFilter{
		Keyword: strings.TrimSpace(input.Keyword),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return PipelinePageResult{}, exception.NewServiceException("failed to count ingestion pipelines", err)
	}
	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return PipelinePageResult{}, exception.NewServiceException("failed to list ingestion pipelines", err)
	}

	return PipelinePageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Get 查询单个 pipeline。
func (s *PipelineService) Get(ctx context.Context, id string) (domain.Pipeline, error) {
	if s == nil || s.repo == nil {
		return domain.Pipeline{}, exception.NewServiceException("ingestion pipeline repository is required", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Pipeline{}, exception.NewClientException("pipeline id is required", nil)
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Pipeline{}, exception.NewServiceException("failed to load ingestion pipeline", err)
	}
	if strings.TrimSpace(item.ID) == "" {
		return domain.Pipeline{}, exception.NewClientException("ingestion pipeline not found", nil)
	}
	return item, nil
}

// Create 创建一条 pipeline。
func (s *PipelineService) Create(ctx context.Context, input CreatePipelineInput) (domain.Pipeline, error) {
	if s == nil || s.repo == nil {
		return domain.Pipeline{}, exception.NewServiceException("ingestion pipeline repository is required", nil)
	}
	if err := validatePipelineDefinition(strings.TrimSpace(input.Name), input.Nodes); err != nil {
		return domain.Pipeline{}, err
	}

	now := s.now()
	id, err := distributedid.NextID()
	if err != nil {
		return domain.Pipeline{}, exception.NewServiceException("failed to generate ingestion pipeline id", err)
	}
	pipeline := domain.Pipeline{
		ID:          fmt.Sprintf("%d", id),
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Nodes:       clonePipelineNodes(input.Nodes),
		CreatedBy:   strings.TrimSpace(input.CreatedBy),
		UpdatedBy:   strings.TrimSpace(input.CreatedBy),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	item, err := s.repo.Create(ctx, pipeline)
	if err != nil {
		return domain.Pipeline{}, exception.NewServiceException("failed to create ingestion pipeline", err)
	}
	return item, nil
}

// Update 更新一条 pipeline。
func (s *PipelineService) Update(ctx context.Context, input UpdatePipelineInput) (domain.Pipeline, error) {
	if s == nil || s.repo == nil {
		return domain.Pipeline{}, exception.NewServiceException("ingestion pipeline repository is required", nil)
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return domain.Pipeline{}, exception.NewClientException("pipeline id is required", nil)
	}
	if err := validatePipelineDefinition(strings.TrimSpace(input.Name), input.Nodes); err != nil {
		return domain.Pipeline{}, err
	}

	existing, err := s.Get(ctx, id)
	if err != nil {
		return domain.Pipeline{}, err
	}
	existing.Name = strings.TrimSpace(input.Name)
	existing.Description = strings.TrimSpace(input.Description)
	existing.Nodes = clonePipelineNodes(input.Nodes)
	existing.UpdatedBy = strings.TrimSpace(input.UpdatedBy)
	existing.UpdatedAt = s.now()

	item, err := s.repo.Update(ctx, existing)
	if err != nil {
		return domain.Pipeline{}, exception.NewServiceException("failed to update ingestion pipeline", err)
	}
	return item, nil
}

// Delete 删除一条 pipeline。
func (s *PipelineService) Delete(ctx context.Context, id string) error {
	if s == nil || s.repo == nil {
		return exception.NewServiceException("ingestion pipeline repository is required", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return exception.NewClientException("pipeline id is required", nil)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return exception.NewServiceException("failed to delete ingestion pipeline", err)
	}
	return nil
}

// validatePipelineDefinition 校验 pipeline 的最小定义合法性。
func validatePipelineDefinition(name string, nodes []domain.PipelineNode) error {
	if strings.TrimSpace(name) == "" {
		return exception.NewClientException("pipeline name is required", nil)
	}
	if len(nodes) == 0 {
		return exception.NewClientException("pipeline nodes are required", nil)
	}

	nodeIDs := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		nodeType := strings.TrimSpace(node.NodeType)
		if nodeID == "" {
			return exception.NewClientException("pipeline node id is required", nil)
		}
		if nodeType == "" {
			return exception.NewClientException("pipeline node type is required", nil)
		}
		if _, exists := nodeIDs[nodeID]; exists {
			return exception.NewClientException("pipeline node id must be unique", nil)
		}
		nodeIDs[nodeID] = struct{}{}
	}

	for _, node := range nodes {
		nextNodeID := strings.TrimSpace(node.NextNodeID)
		if nextNodeID == "" {
			continue
		}
		if _, exists := nodeIDs[nextNodeID]; !exists {
			return exception.NewClientException("pipeline next node id must reference an existing node", nil)
		}
	}
	return nil
}

// clonePipelineNodes 复制节点定义，避免外部切片被直接复用。
func clonePipelineNodes(nodes []domain.PipelineNode) []domain.PipelineNode {
	if len(nodes) == 0 {
		return nil
	}
	result := make([]domain.PipelineNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, domain.PipelineNode{
			NodeID:     strings.TrimSpace(node.NodeID),
			NodeType:   strings.TrimSpace(node.NodeType),
			Settings:   node.Settings,
			Condition:  node.Condition,
			NextNodeID: strings.TrimSpace(node.NextNodeID),
		})
	}
	return result
}

// normalizePipelinePage 规范化分页页码。
func normalizePipelinePage(page int) int {
	if page <= 0 {
		return defaultPipelinePage
	}
	return page
}

// normalizePipelinePageSize 规范化分页大小。
func normalizePipelinePageSize(pageSize int) int {
	if pageSize <= 0 {
		return defaultPipelinePageSize
	}
	if pageSize > maxPipelinePageSize {
		return maxPipelinePageSize
	}
	return pageSize
}
