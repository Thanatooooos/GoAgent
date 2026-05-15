package schedule

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/log"
)

const (
	// defaultScheduleBatchSize 每次扫描的调度任务数量上限
	// 设计意图：避免一次性加载过多任务导致内存压力和长事务
	defaultScheduleBatchSize = 20
	// defaultRunningDocumentTimeoutMinutes 文档处于 running 状态的超时时间
	// 超过此时间仍未完成，视为卡住的任务，需要恢复为 failed
	defaultRunningDocumentTimeoutMinutes = int64(30)
)

// ScheduleLeaseProcessor 定义调度任务的实际处理逻辑
// 实现者负责：获取调度配置 → 检查文件变化 → 下载 → 提交处理任务 → 更新状态
type ScheduleLeaseProcessor interface {
	Process(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error
}

// ScheduleTaskDispatcher 定义任务分发机制
// 设计意图：解耦并发策略，允许替换为带限流的 dispatcher
type ScheduleTaskDispatcher interface {
	Submit(task func()) error
}

// KnowledgeDocumentScheduleJobOptions 调度任务的配置选项
type KnowledgeDocumentScheduleJobOptions struct {
	LockManager           *ScheduleLockManager   // 分布式锁管理器，未提供时自动创建
	Processor             ScheduleLeaseProcessor // 任务处理器，负责实际业务逻辑
	Dispatcher            ScheduleTaskDispatcher // 任务分发器，未提供时使用默认的 goroutine 池
	BatchSize             int                    // 批量扫描大小，默认 20
	RunningTimeoutMinutes int64                  // 超时文档恢复时间，默认 30 分钟
	Now                   func() time.Time       // 时间函数抽象，便于单元测试
}

// KnowledgeDocumentScheduleJob 是调度任务的核心入口
// 职责：扫描到期的调度任务 → 获取分布式锁 → 分发到异步执行器
type KnowledgeDocumentScheduleJob struct {
	scheduleRepo                 port.KnowledgeDocumentScheduleRepository // 调度配置仓储
	lockManager                  *ScheduleLockManager                     // 分布式锁管理器
	processor                    ScheduleLeaseProcessor                   // 任务处理器
	dispatcher                   ScheduleTaskDispatcher                   // 任务分发器
	documentStatusHelper         DocumentStatusHelper                     // 文档状态辅助（恢复卡住的任务）
	batchSize                    int                                      // 批量扫描大小
	runningDocumentTimeoutMinute int64                                    // 超时时间
	now                          func() time.Time                         // 时间函数
	dispatcherCancel             context.CancelFunc                       // 分发器的取消函数，用于优雅关闭
	dispatcherWG                 *sync.WaitGroup                          // 等待组，用于等待所有已提交任务完成
}

// NewKnowledgeDocumentScheduleJob 创建调度任务实例（使用默认配置）
func NewKnowledgeDocumentScheduleJob(scheduleRepo port.KnowledgeDocumentScheduleRepository, documentHelper DocumentStatusHelper) *KnowledgeDocumentScheduleJob {
	return NewKnowledgeDocumentScheduleJobWithOptions(scheduleRepo, documentHelper, KnowledgeDocumentScheduleJobOptions{})
}

// NewKnowledgeDocumentScheduleJobWithOptions 创建调度任务实例（支持自定义配置）
// 设计意图：依赖自动装配，降低使用门槛
func NewKnowledgeDocumentScheduleJobWithOptions(
	scheduleRepo port.KnowledgeDocumentScheduleRepository,
	documentHelper DocumentStatusHelper,
	options KnowledgeDocumentScheduleJobOptions,
) *KnowledgeDocumentScheduleJob {
	// 如果没有提供 LockManager，自动创建默认实现
	lockManager := options.LockManager
	if lockManager == nil && scheduleRepo != nil {
		lockManager = NewScheduleLockManager(scheduleRepo, ScheduleLockOptions{})
	}

	// 如果没有提供 Dispatcher，创建默认的 managed dispatcher（带 context 和 WaitGroup）
	dispatcher, cancel, wg := normalizeScheduleTaskDispatcher(options.Dispatcher)

	return &KnowledgeDocumentScheduleJob{
		scheduleRepo:                 scheduleRepo,
		lockManager:                  lockManager,
		processor:                    options.Processor,
		dispatcher:                   dispatcher,
		documentStatusHelper:         documentHelper,
		batchSize:                    normalizePositiveInt(options.BatchSize, defaultScheduleBatchSize),
		runningDocumentTimeoutMinute: normalizePositiveInt64(options.RunningTimeoutMinutes, defaultRunningDocumentTimeoutMinutes),
		now:                          normalizeClock(options.Now),
		dispatcherCancel:             cancel,
		dispatcherWG:                 wg,
	}
}

// ListDue 查询到期的调度任务
// 用途：供外部调度器判断是否有任务需要执行
func (j *KnowledgeDocumentScheduleJob) ListDue(ctx context.Context, now time.Time, limit int) error {
	if j == nil || j.scheduleRepo == nil {
		return nil
	}
	_, err := j.scheduleRepo.ListDue(ctx, now, limit)
	return err
}

// RecoverStuckRunningDocuments 恢复超时未完成的文档
// 场景：进程崩溃导致文档状态卡在 running，需要手动恢复为 failed
func (j *KnowledgeDocumentScheduleJob) RecoverStuckRunningDocuments(ctx context.Context) error {
	if j == nil {
		return nil
	}
	result, err := j.documentStatusHelper.RecoverStuckRunning(ctx, j.runningDocumentTimeoutMinute)
	if result > 0 {
		log.Warnf("recover %d stuck running knowledge documents to failed", result)
	}
	return err
}

// Scan 是调度任务的核心方法
// 流程：查询到期任务 → 尝试获取锁 → 分发到异步执行器
// 设计意图：错误收集模式，单个任务失败不影响其他任务
func (j *KnowledgeDocumentScheduleJob) Scan(ctx context.Context) error {
	if j == nil || j.scheduleRepo == nil {
		return nil
	}
	if j.lockManager == nil {
		return fmt.Errorf("knowledge document schedule lock manager is required")
	}

	// 查询到期的调度任务（批量查询，避免一次性加载过多）
	now := j.now()
	schedules, err := j.scheduleRepo.ListDue(ctx, now, j.batchSize)
	if err != nil {
		return err
	}
	// 错误收集：不因单个失败中断整个扫描
	scanErrors := make([]error, 0)

	for _, item := range schedules {
		// 检查 context 是否取消，支持优雅退出
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if item.ID == "" {
			continue
		}

		// 尝试获取分布式锁（同一任务只能被一个实例执行）
		lease, acquired, err := j.lockManager.TryAcquire(ctx, item.ID, now)
		if err != nil {
			// 获取锁失败，记录错误但继续处理下一个任务
			log.Warnf("acquire schedule lock failed: scheduleId=%s err=%v", item.ID, err)
			scanErrors = append(scanErrors, fmt.Errorf("acquire schedule %s lock: %w", item.ID, err))
			continue
		}
		if !acquired {
			// 被其他实例锁定，跳过
			continue
		}

		// 分发任务到异步执行器
		if err := j.dispatchLease(ctx, lease); err != nil {
			// 分发失败，立即释放锁（避免锁死 15 分钟）
			releaseCtx, releaseCancel := newBackgroundTaskContext(ctx, 5*time.Second)
			if _, releaseErr := j.lockManager.Release(releaseCtx, lease); releaseErr != nil {
				log.Warnf("release schedule lock after dispatch failure failed: scheduleId=%s lockToken=%s err=%v",
					lease.ScheduleID, lease.LockToken, releaseErr)
			}
			releaseCancel()
			log.Warnf("dispatch schedule task failed: scheduleId=%s lockToken=%s err=%v", lease.ScheduleID, lease.LockToken, err)
			scanErrors = append(scanErrors, fmt.Errorf("dispatch schedule %s: %w", lease.ScheduleID, err))
			continue
		}
	}

	// 汇总所有错误返回
	return errors.Join(scanErrors...)
}

func (j *KnowledgeDocumentScheduleJob) scan(ctx context.Context) error {
	return j.Scan(ctx)
}

// Close 停止默认任务调度器并等待已提交任务退出
// 流程：取消 context（拒绝新任务）→ WaitGroup.Wait（等待已有任务完成）
func (j *KnowledgeDocumentScheduleJob) Close() {
	if j == nil {
		return
	}
	if j.dispatcherCancel != nil {
		j.dispatcherCancel()
	}
	if j.dispatcherWG != nil {
		j.dispatcherWG.Wait()
	}
}

// dispatchLease 将获取到的锁分发给处理器执行
// 设计意图：defer 保证锁一定被释放，即使任务 panic
func (j *KnowledgeDocumentScheduleJob) dispatchLease(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error {
	// 如果没有处理器，直接释放锁
	if j.processor == nil {
		_, err := j.lockManager.Release(ctx, lease)
		return err
	}

	// 提交任务到 dispatcher 异步执行
	return j.dispatcher.Submit(func() {
		// defer 保证无论成功/失败/panic，锁都会被释放
		defer func() {
			// 创建独立的 context 用于释放锁
			// 设计意图：即使原 ctx 已取消（如服务关闭），也要完成锁释放
			releaseCtx, releaseCancel := newBackgroundTaskContext(ctx, 5*time.Second)
			defer releaseCancel()
			if _, err := j.lockManager.Release(releaseCtx, lease); err != nil {
				log.Warnf("release schedule lock after processing failed: scheduleId=%s lockToken=%s err=%v",
					lease.ScheduleID, lease.LockToken, err)
			}
		}()

		// 使用 dispatcher 的 context 而不是 Scan 的 ctx
		// 设计意图：Scan 的 ctx 可能绑定 HTTP 请求，超时后会被取消
		// 但任务本身需要继续执行，不受 HTTP 超时影响
		processCtx := ctx
		if managed, ok := j.dispatcher.(*managedScheduleTaskDispatcher); ok && managed.ctx != nil {
			processCtx = managed.ctx
		}
		if err := j.processor.Process(processCtx, lease); err != nil {
			log.Errorf("process knowledge document schedule failed: scheduleId=%s lockToken=%s err=%v",
				lease.ScheduleID, lease.LockToken, err)
		}
	})
}

// managedScheduleTaskDispatcher 是默认的任务分发器实现
// 特点：无限制 goroutine 池，带 context 生命周期管理和 panic 恢复
type managedScheduleTaskDispatcher struct {
	ctx context.Context
	wg  *sync.WaitGroup
}

// Submit 提交任务到 goroutine 执行
// 设计意图：双重 context 检查 + panic 恢复
func (d *managedScheduleTaskDispatcher) Submit(task func()) error {
	if d == nil || d.ctx == nil || d.wg == nil {
		return fmt.Errorf("managed schedule task dispatcher is not initialized")
	}
	// 第一次检查：提交时检查 context 是否已取消
	if d.ctx.Err() != nil {
		return d.ctx.Err()
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		// panic 恢复：避免单个任务崩溃影响其他任务
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("schedule task dispatch panic recovered: %v", recovered)
			}
		}()
		// 第二次检查：执行前再次检查 context
		// 设计意图：避免在 Submit 和 goroutine 执行之间 context 被取消
		if d.ctx.Err() != nil {
			return
		}
		task()
	}()
	return nil
}

// normalizeScheduleTaskDispatcher 规范化任务分发器
// 如果未提供自定义 dispatcher，创建默认的 managed dispatcher
func normalizeScheduleTaskDispatcher(dispatcher ScheduleTaskDispatcher) (ScheduleTaskDispatcher, context.CancelFunc, *sync.WaitGroup) {
	if dispatcher != nil {
		return dispatcher, nil, nil
	}
	// 创建带取消功能的 context，用于控制 dispatcher 生命周期
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	return &managedScheduleTaskDispatcher{
		ctx: ctx,
		wg:  wg,
	}, cancel, wg
}

func normalizePositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func normalizePositiveInt64(value int64, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

// newBackgroundTaskContext 创建用于后台任务的 context
// 设计意图：不继承原 context 的取消状态，但继承其值（如 trace ID）
// 使用场景：释放锁时，即使原 ctx 已取消，也要完成释放操作
func newBackgroundTaskContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		// WithoutCancel：继承值，不继承取消状态
		base = context.WithoutCancel(ctx)
	}
	// 添加超时限制，避免无限等待
	return context.WithTimeout(base, timeout)
}
