package pipeline

import (
	ingestionrunner "local/rag-project/internal/app/ingestion/service/runner"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"

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
	Definition  domain.PipelineDefinition
	Nodes       []domain.PipelineNode
	CreatedBy   string
}

// UpdatePipelineInput 描述更新 pipeline 的入参。
type UpdatePipelineInput struct {
	ID          string
	Name        string
	Description string
	Definition  domain.PipelineDefinition
	Nodes       []domain.PipelineNode
	UpdatedBy   string
}

// PipelineService 负责 pipeline 的管理与校验。
type PipelineService struct {
	repo        port.PipelineRepository
	nodeRunners *ingestionrunner.NodeRunnerRegistry
	now         func() time.Time
}

// NewPipelineService 创建 pipeline 服务。
func NewPipelineService(repo port.PipelineRepository, nodeRunners ...*ingestionrunner.NodeRunnerRegistry) *PipelineService {
	var registry *ingestionrunner.NodeRunnerRegistry
	if len(nodeRunners) > 0 {
		registry = nodeRunners[0]
	}
	return &PipelineService{
		repo:        repo,
		nodeRunners: registry,
		now:         time.Now,
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
	definition, err := s.normalizeDefinition(strings.TrimSpace(input.Name), input.Definition, input.Nodes)
	if err != nil {
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
		Definition:  definition,
		Nodes:       domain.ClonePipelineNodes(definition.Nodes),
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
	definition, err := s.normalizeDefinition(strings.TrimSpace(input.Name), input.Definition, input.Nodes)
	if err != nil {
		return domain.Pipeline{}, err
	}

	existing, err := s.Get(ctx, id)
	if err != nil {
		return domain.Pipeline{}, err
	}
	existing.Name = strings.TrimSpace(input.Name)
	existing.Description = strings.TrimSpace(input.Description)
	existing.Definition = definition
	existing.Nodes = domain.ClonePipelineNodes(definition.Nodes)
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

func (s *PipelineService) normalizeDefinition(name string, definition domain.PipelineDefinition, nodes []domain.PipelineNode) (domain.PipelineDefinition, error) {
	normalized := domain.NormalizePipelineDefinition(definition, nodes)
	if err := s.validatePipelineDefinition(name, normalized); err != nil {
		return domain.PipelineDefinition{}, err
	}
	return normalized, nil
}

// validatePipelineDefinition validates a DAG-style pipeline definition.
func (s *PipelineService) validatePipelineDefinition(name string, definition domain.PipelineDefinition) error {
	if strings.TrimSpace(name) == "" {
		return exception.NewClientException("pipeline name is required", nil)
	}
	if len(definition.Nodes) == 0 {
		return exception.NewClientException("pipeline definition nodes are required", nil)
	}
	if len(definition.EntryNodeIDs) == 0 {
		return exception.NewClientException("pipeline entry node ids are required", nil)
	}

	nodeIDs := make(map[string]struct{}, len(definition.Nodes))
	nodesByID := make(map[string]domain.PipelineNode, len(definition.Nodes))
	inDegree := make(map[string]int, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		nodeType := strings.TrimSpace(node.NodeType)
		if nodeID == "" {
			return exception.NewClientException("pipeline node id is required", nil)
		}
		if nodeID == compose.START || nodeID == compose.END {
			return exception.NewClientException("pipeline node id is reserved: "+nodeID, nil)
		}
		if nodeType == "" {
			return exception.NewClientException("pipeline node type is required", nil)
		}
		if _, exists := nodeIDs[nodeID]; exists {
			return exception.NewClientException("pipeline node id must be unique", nil)
		}
		nodeIDs[nodeID] = struct{}{}
		nodesByID[nodeID] = node
		inDegree[nodeID] = 0
		if s != nil && s.nodeRunners != nil {
			if _, ok := s.nodeRunners.Get(nodeType); !ok {
				return exception.NewClientException("pipeline node type is not executable: "+nodeType, nil)
			}
		}
	}

	edgeIDs := make(map[string]struct{}, len(definition.Edges))
	adjacency := make(map[string][]string, len(definition.Nodes))
	for _, edge := range definition.Edges {
		edgeID := strings.TrimSpace(edge.EdgeID)
		if edgeID == "" {
			return exception.NewClientException("pipeline edge id is required", nil)
		}
		if _, exists := edgeIDs[edgeID]; exists {
			return exception.NewClientException("pipeline edge id must be unique", nil)
		}
		edgeIDs[edgeID] = struct{}{}
		fromNodeID := strings.TrimSpace(edge.FromNodeID)
		toNodeID := strings.TrimSpace(edge.ToNodeID)
		if _, exists := nodeIDs[fromNodeID]; !exists {
			return exception.NewClientException("pipeline edge from node id must reference an existing node", nil)
		}
		if _, exists := nodeIDs[toNodeID]; !exists {
			return exception.NewClientException("pipeline edge to node id must reference an existing node", nil)
		}
		adjacency[fromNodeID] = append(adjacency[fromNodeID], toNodeID)
		inDegree[toNodeID]++
	}

	for _, entryNodeID := range definition.EntryNodeIDs {
		entryNodeID = strings.TrimSpace(entryNodeID)
		if _, exists := nodeIDs[entryNodeID]; !exists {
			return exception.NewClientException("pipeline entry node id must reference an existing node", nil)
		}
	}

	visited := make(map[string]bool, len(definition.Nodes))
	var walk func(nodeID string)
	walk = func(nodeID string) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true
		for _, nextNodeID := range adjacency[nodeID] {
			walk(nextNodeID)
		}
	}
	for _, entryNodeID := range definition.EntryNodeIDs {
		walk(strings.TrimSpace(entryNodeID))
	}
	if len(visited) != len(definition.Nodes) {
		return exception.NewClientException("pipeline graph contains unreachable nodes", nil)
	}

	queue := make([]string, 0, len(definition.Nodes))
	inDegreeCopy := make(map[string]int, len(inDegree))
	for nodeID, degree := range inDegree {
		inDegreeCopy[nodeID] = degree
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}
	visitedCount := 0
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		visitedCount++
		for _, nextNodeID := range adjacency[nodeID] {
			inDegreeCopy[nextNodeID]--
			if inDegreeCopy[nextNodeID] == 0 {
				queue = append(queue, nextNodeID)
			}
		}
	}
	if visitedCount != len(definition.Nodes) {
		return exception.NewClientException("pipeline graph must be acyclic", nil)
	}
	if err := s.validateNodeContracts(definition, nodesByID, adjacency); err != nil {
		return err
	}
	return nil
}

func (s *PipelineService) validateNodeContracts(
	definition domain.PipelineDefinition,
	nodesByID map[string]domain.PipelineNode,
	adjacency map[string][]string,
) error {
	if len(definition.Nodes) == 0 {
		return nil
	}

	inDegree := make(map[string]int, len(definition.Nodes))
	predecessors := make(map[string][]string, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		inDegree[nodeID] = 0
	}
	for fromNodeID, nextNodeIDs := range adjacency {
		for _, toNodeID := range nextNodeIDs {
			inDegree[toNodeID]++
			predecessors[toNodeID] = append(predecessors[toNodeID], fromNodeID)
		}
	}

	entryNodeIDs := ingestionworkflow.ArtifactSetFromNames(definition.EntryNodeIDs...)
	baseArtifacts := ingestionworkflow.ArtifactSetFromNames("task")
	availableAfter := make(map[string]map[string]struct{}, len(definition.Nodes))

	queue := make([]string, 0, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		if inDegree[nodeID] == 0 {
			queue = append(queue, nodeID)
		}
	}

	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]

		before := ingestionworkflow.ArtifactSetFromNames()
		if _, isEntry := entryNodeIDs[nodeID]; isEntry {
			before = ingestionworkflow.MergeArtifactSets(before, baseArtifacts)
		}
		for _, predecessorNodeID := range predecessors[nodeID] {
			before = ingestionworkflow.MergeArtifactSets(before, availableAfter[predecessorNodeID])
		}
		node := nodesByID[nodeID]
		contract, ok := ingestionworkflow.GetNodeIOContract(node.NodeType)
		if ok {
			if _, isEntry := entryNodeIDs[nodeID]; isEntry && !contract.SupportsEntry {
				return exception.NewClientException("pipeline entry node type does not support entry position: "+node.NodeType, nil)
			}
			for _, requirement := range contract.Requires {
				if ingestionworkflow.ArtifactSetContainsAny(before, requirement.AnyOf) {
					continue
				}
				return exception.NewClientException(
					fmt.Sprintf(
						"pipeline node %s (%s) requires input artifact [%s], available artifacts are [%s]",
						nodeID,
						node.NodeType,
						strings.Join(requirement.AnyOf, " or "),
						strings.Join(ingestionworkflow.ArtifactSetNames(before), ", "),
					),
					nil,
				)
			}
		}

		after := ingestionworkflow.MergeArtifactSets(before, ingestionworkflow.ArtifactSetFromNames(contract.Produces...))
		availableAfter[nodeID] = after

		for _, nextNodeID := range adjacency[nodeID] {
			inDegree[nextNodeID]--
			if inDegree[nextNodeID] == 0 {
				queue = append(queue, nextNodeID)
			}
		}
	}
	return nil
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
