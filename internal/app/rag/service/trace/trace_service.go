package trace

import (
	"context"
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	userport "local/rag-project/internal/app/user/port"
	"local/rag-project/internal/framework/exception"
)

const (
	defaultTracePage     = 1
	defaultTracePageSize = 10
	maxTracePageSize     = 100
)

// PageTraceRunsInput 描述 trace run 分页查询参数。
type PageTraceRunsInput struct {
	Page           int
	PageSize       int
	TraceID        string
	ConversationID string
	TaskID         string
	Status         string
}

// TraceRunPageResult 表示 trace run 分页结果。
type TraceRunPageResult struct {
	Items    []domain.RagTraceRun
	Total    int
	Page     int
	PageSize int
}

// TraceDetail 表示单条 trace 详情与节点列表。
type TraceDetail struct {
	Run   domain.RagTraceRun
	Nodes []domain.RagTraceNode
}

// Service 负责查询 RAG trace 运行记录与节点详情。
type Service struct {
	traceRunRepo  port.RagTraceRunRepository
	traceNodeRepo port.RagTraceNodeRepository
	userRepo      userport.UserRepository
}

// NewService 创建 trace 查询服务。
func NewService(
	traceRunRepo port.RagTraceRunRepository,
	traceNodeRepo port.RagTraceNodeRepository,
	userRepo userport.UserRepository,
) *Service {
	return &Service{
		traceRunRepo:  traceRunRepo,
		traceNodeRepo: traceNodeRepo,
		userRepo:      userRepo,
	}
}

// PageRuns 分页查询 trace run 列表。
func (s *Service) PageRuns(ctx context.Context, input PageTraceRunsInput) (TraceRunPageResult, error) {
	if s == nil || s.traceRunRepo == nil {
		return TraceRunPageResult{}, exception.NewServiceException("rag trace run repository is required", nil)
	}

	page := normalizeTracePage(input.Page)
	pageSize := normalizeTracePageSize(input.PageSize)
	filter := port.RagTraceRunListFilter{
		TraceID:        strings.TrimSpace(input.TraceID),
		ConversationID: strings.TrimSpace(input.ConversationID),
		TaskID:         strings.TrimSpace(input.TaskID),
		Status:         strings.TrimSpace(input.Status),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	}

	total, err := s.traceRunRepo.Count(ctx, filter)
	if err != nil {
		return TraceRunPageResult{}, exception.NewServiceException("failed to count rag trace runs", err)
	}

	items, err := s.traceRunRepo.List(ctx, filter)
	if err != nil {
		return TraceRunPageResult{}, exception.NewServiceException("failed to list rag trace runs", err)
	}

	return TraceRunPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetDetail 查询单条 trace 及其节点详情。
func (s *Service) GetDetail(ctx context.Context, traceID string) (TraceDetail, error) {
	if s == nil || s.traceRunRepo == nil || s.traceNodeRepo == nil {
		return TraceDetail{}, exception.NewServiceException("rag trace repositories are required", nil)
	}

	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return TraceDetail{}, exception.NewClientException("trace id is required", nil)
	}

	run, err := s.traceRunRepo.GetByTraceID(ctx, traceID)
	if err != nil {
		return TraceDetail{}, exception.NewServiceException("failed to load rag trace run", err)
	}
	if strings.TrimSpace(run.ID) == "" {
		return TraceDetail{}, exception.NewClientException("rag trace run not found", nil)
	}

	nodes, err := s.traceNodeRepo.ListByTraceID(ctx, traceID)
	if err != nil {
		return TraceDetail{}, exception.NewServiceException("failed to list rag trace nodes", err)
	}

	return TraceDetail{
		Run:   run,
		Nodes: nodes,
	}, nil
}

// ListNodes 查询单条 trace 下的所有节点。
func (s *Service) ListNodes(ctx context.Context, traceID string) ([]domain.RagTraceNode, error) {
	detail, err := s.GetDetail(ctx, traceID)
	if err != nil {
		return nil, err
	}
	return detail.Nodes, nil
}

// ResolveUserName 按用户 ID 解析展示用用户名。
func (s *Service) ResolveUserName(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || s == nil || s.userRepo == nil {
		return "", nil
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", exception.NewServiceException("failed to load trace user", err)
	}
	return strings.TrimSpace(user.Username), nil
}

func normalizeTracePage(page int) int {
	if page <= 0 {
		return defaultTracePage
	}
	return page
}

func normalizeTracePageSize(pageSize int) int {
	if pageSize <= 0 {
		return defaultTracePageSize
	}
	if pageSize > maxTracePageSize {
		return maxTracePageSize
	}
	return pageSize
}
