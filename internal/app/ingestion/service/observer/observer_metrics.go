package observer

import (
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
)

// MetricsSnapshot 描述 ingestion 执行器当前可观测指标快照。
type MetricsSnapshot struct {
	RunningTasks  int                      `json:"runningTasks"`
	MaxConcurrent int                      `json:"maxConcurrent"`
	UsedSlots     int                      `json:"usedSlots"`
	Totals        MetricsTotalsSnapshot    `json:"totals"`
	Rates         MetricsRatesSnapshot     `json:"rates"`
	Reconcile     ReconcileMetricsSnapshot `json:"reconcile"`
	Nodes         []NodeMetricsSnapshot    `json:"nodes"`
}

// MetricsTotalsSnapshot 汇总 task 与 retry 相关指标。
type MetricsTotalsSnapshot struct {
	Submitted int64 `json:"submitted"`
	Started   int64 `json:"started"`
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	Canceled  int64 `json:"canceled"`
	Retries   int64 `json:"retries"`
}

// MetricsRatesSnapshot 汇总成功率与失败率。
type MetricsRatesSnapshot struct {
	SuccessRate float64 `json:"successRate"`
	FailureRate float64 `json:"failureRate"`
}

type ReconcileMetricsSnapshot struct {
	Attempts        int64                     `json:"attempts"`
	Skipped         int64                     `json:"skipped"`
	DocumentUpdated int64                     `json:"documentUpdated"`
	ChunkLogUpdated int64                     `json:"chunkLogUpdated"`
	ChunkLogCreated int64                     `json:"chunkLogCreated"`
	Failures        int64                     `json:"failures"`
	LastFailure     *ReconcileFailureSnapshot `json:"lastFailure,omitempty"`
}

type ReconcileFailureSnapshot struct {
	Source       string    `json:"source"`
	TaskID       string    `json:"taskId"`
	DocumentID   string    `json:"documentId"`
	ErrorMessage string    `json:"errorMessage"`
	OccurredAt   time.Time `json:"occurredAt"`
}

type ReconcileMetricsEvent struct {
	Source          string
	TaskID          string
	DocumentID      string
	Skipped         bool
	DocumentUpdated bool
	ChunkLogUpdated bool
	ChunkLogCreated bool
	ErrorMessage    string
}

// NodeMetricsSnapshot 描述单类节点的聚合指标。
type NodeMetricsSnapshot struct {
	NodeType      string `json:"nodeType"`
	Runs          int64  `json:"runs"`
	Successes     int64  `json:"successes"`
	Failures      int64  `json:"failures"`
	Retries       int64  `json:"retries"`
	AvgDurationMs int64  `json:"avgDurationMs"`
	MaxDurationMs int64  `json:"maxDurationMs"`
}

type nodeMetrics struct {
	runs            int64
	successes       int64
	failures        int64
	retries         int64
	totalDurationMs int64
	maxDurationMs   int64
}

// MetricsService 提供进程内实时指标聚合与查询。
type MetricsService struct {
	mu            sync.RWMutex
	maxConcurrent int
	runningTasks  map[string]struct{}
	totals        MetricsTotalsSnapshot
	reconcile     ReconcileMetricsSnapshot
	nodes         map[string]*nodeMetrics
}

// NewMetricsService 创建 ingestion 指标服务。
func NewMetricsService(maxConcurrent int) *MetricsService {
	return &MetricsService{
		maxConcurrent: maxConcurrent,
		runningTasks:  make(map[string]struct{}),
		nodes:         make(map[string]*nodeMetrics),
	}
}

// SetMaxConcurrent 同步执行器当前最大并发配置。
func (s *MetricsService) SetMaxConcurrent(maxConcurrent int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxConcurrent = maxConcurrent
}

// RecordTaskSubmitted 记录任务进入执行器。
func (s *MetricsService) RecordTaskSubmitted(task domain.Task) {
	if s == nil || strings.TrimSpace(task.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totals.Submitted++
}

// Snapshot 返回当前指标快照。
func (s *MetricsService) Snapshot() MetricsSnapshot {
	if s == nil {
		return MetricsSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := MetricsSnapshot{
		RunningTasks:  len(s.runningTasks),
		MaxConcurrent: s.maxConcurrent,
		UsedSlots:     len(s.runningTasks),
		Totals:        s.totals,
		Reconcile:     s.reconcile,
	}
	if s.reconcile.LastFailure != nil {
		lastFailure := *s.reconcile.LastFailure
		snapshot.Reconcile.LastFailure = &lastFailure
	}
	completed := s.totals.Succeeded + s.totals.Failed + s.totals.Canceled
	if completed > 0 {
		snapshot.Rates.SuccessRate = float64(s.totals.Succeeded) / float64(completed)
		snapshot.Rates.FailureRate = float64(s.totals.Failed) / float64(completed)
	}
	if len(s.nodes) > 0 {
		snapshot.Nodes = make([]NodeMetricsSnapshot, 0, len(s.nodes))
		for nodeType, metrics := range s.nodes {
			item := NodeMetricsSnapshot{
				NodeType:      nodeType,
				Runs:          metrics.runs,
				Successes:     metrics.successes,
				Failures:      metrics.failures,
				Retries:       metrics.retries,
				MaxDurationMs: metrics.maxDurationMs,
			}
			if metrics.runs > 0 {
				item.AvgDurationMs = metrics.totalDurationMs / metrics.runs
			}
			snapshot.Nodes = append(snapshot.Nodes, item)
		}
		sort.Slice(snapshot.Nodes, func(i, j int) bool {
			return snapshot.Nodes[i].NodeType < snapshot.Nodes[j].NodeType
		})
	}
	return snapshot
}

func (s *MetricsService) RecordReconcileEvent(event ReconcileMetricsEvent) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reconcile.Attempts++
	if event.Skipped {
		s.reconcile.Skipped++
	}
	if event.DocumentUpdated {
		s.reconcile.DocumentUpdated++
	}
	if event.ChunkLogUpdated {
		s.reconcile.ChunkLogUpdated++
	}
	if event.ChunkLogCreated {
		s.reconcile.ChunkLogCreated++
	}
	if strings.TrimSpace(event.ErrorMessage) != "" {
		s.reconcile.Failures++
		s.reconcile.LastFailure = &ReconcileFailureSnapshot{
			Source:       strings.TrimSpace(event.Source),
			TaskID:       strings.TrimSpace(event.TaskID),
			DocumentID:   strings.TrimSpace(event.DocumentID),
			ErrorMessage: strings.TrimSpace(event.ErrorMessage),
			OccurredAt:   time.Now(),
		}
	}
}

// MetricsObserver 把执行事件聚合为实时指标。
type MetricsObserver struct {
	service *MetricsService
}

// NewMetricsObserver 创建 metrics observer。
func NewMetricsObserver(service *MetricsService) *MetricsObserver {
	return &MetricsObserver{service: service}
}

// OnTaskStarted 记录 task 开始执行。
func (o *MetricsObserver) OnTaskStarted(ctx context.Context, task domain.Task) error {
	_ = ctx
	if o == nil || o.service == nil {
		return nil
	}
	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		return nil
	}
	o.service.mu.Lock()
	defer o.service.mu.Unlock()
	if _, exists := o.service.runningTasks[taskID]; !exists {
		o.service.runningTasks[taskID] = struct{}{}
		o.service.totals.Started++
	}
	return nil
}

// OnTaskCompleted 记录 task 结束状态。
func (o *MetricsObserver) OnTaskCompleted(ctx context.Context, task domain.Task, state ingestionworkflow.ExecutionState, execErr error) error {
	_ = ctx
	_ = state
	if o == nil || o.service == nil {
		return nil
	}
	taskID := strings.TrimSpace(task.ID)
	o.service.mu.Lock()
	defer o.service.mu.Unlock()
	delete(o.service.runningTasks, taskID)
	switch {
	case execErr == nil:
		o.service.totals.Succeeded++
	case errors.Is(execErr, context.Canceled):
		o.service.totals.Canceled++
	default:
		o.service.totals.Failed++
	}
	return nil
}

// OnNodeStarted 记录节点执行次数。
func (o *MetricsObserver) OnNodeStarted(ctx context.Context, task domain.Task, node ingestionworkflow.WorkflowNodeSpec) error {
	_ = ctx
	_ = task
	if o == nil || o.service == nil {
		return nil
	}
	metrics := o.service.getOrCreateNodeMetrics(node.Node.NodeType)
	o.service.mu.Lock()
	defer o.service.mu.Unlock()
	metrics.runs++
	return nil
}

// OnNodeRetry 记录节点重试次数。
func (o *MetricsObserver) OnNodeRetry(ctx context.Context, task domain.Task, node ingestionworkflow.WorkflowNodeSpec, attempt int, backoff time.Duration, execErr error) error {
	_ = ctx
	_ = task
	_ = attempt
	_ = backoff
	_ = execErr
	if o == nil || o.service == nil {
		return nil
	}
	metrics := o.service.getOrCreateNodeMetrics(node.Node.NodeType)
	o.service.mu.Lock()
	defer o.service.mu.Unlock()
	metrics.retries++
	o.service.totals.Retries++
	return nil
}

// OnNodeCompleted 记录节点耗时与成功失败。
func (o *MetricsObserver) OnNodeCompleted(ctx context.Context, task domain.Task, node ingestionworkflow.WorkflowNodeSpec, output map[string]any, duration time.Duration, execErr error) error {
	_ = ctx
	_ = task
	_ = output
	if o == nil || o.service == nil {
		return nil
	}
	metrics := o.service.getOrCreateNodeMetrics(node.Node.NodeType)
	durationMs := duration.Milliseconds()
	o.service.mu.Lock()
	defer o.service.mu.Unlock()
	metrics.totalDurationMs += durationMs
	if durationMs > metrics.maxDurationMs {
		metrics.maxDurationMs = durationMs
	}
	if execErr == nil {
		metrics.successes++
	} else {
		metrics.failures++
	}
	return nil
}

func (s *MetricsService) getOrCreateNodeMetrics(nodeType string) *nodeMetrics {
	nodeType = strings.TrimSpace(nodeType)
	if nodeType == "" {
		nodeType = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	metrics := s.nodes[nodeType]
	if metrics == nil {
		metrics = &nodeMetrics{}
		s.nodes[nodeType] = metrics
	}
	return metrics
}
