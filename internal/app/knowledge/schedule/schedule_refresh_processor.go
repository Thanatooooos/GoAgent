package schedule

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
)

// RemoteFileFetchClient 定义远程文件获取接口
// 实现者负责：检查文件变化（ETag/Hash）→ 下载临时文件
type RemoteFileFetchClient interface {
	FetchIfChanged(ctx context.Context, rawURL string, lastETag string, lastModified string, lastContentHash string, fallbackFileName string) (RemoteFetchResult, error)
}

// RefreshedDocumentProcessor 定义刷新后的文档处理接口
// 实现者负责：提交文档处理任务（如分块、向量化）
type RefreshedDocumentProcessor interface {
	ProcessRefreshedDocument(ctx context.Context, document domain.KnowledgeDocument) error
}

// ScheduleRefreshProcessorOptions 调度刷新处理器的配置选项
type ScheduleRefreshProcessorOptions struct {
	ScheduleRepo      port.KnowledgeDocumentScheduleRepository     // 调度配置仓储
	DocumentRepo      port.KnowledgeDocumentRepository             // 文档仓储
	ExecRepo          port.KnowledgeDocumentScheduleExecRepository // 执行记录仓储
	Storage           port.FileStorage                             // 文件存储（S3/OSS）
	LockManager       *ScheduleLockManager                         // 分布式锁管理器
	StateManager      *ScheduleStateManager                        // 状态管理器
	DocumentHelper    *DocumentStatusHelper                        // 文档状态辅助
	RemoteFileFetcher RemoteFileFetchClient                        // 远程文件获取器
	DocumentProcessor RefreshedDocumentProcessor                   // 文档处理器
	Now               func() time.Time                             // 时间函数抽象
	NextID            func() (int64, error)                        // ID 生成函数
}

// ScheduleRefreshProcessor 是调度刷新任务的核心处理器
// 职责：
// 1. 检查远程文件是否变化
// 2. 如果有变化，下载并存储到 S3
// 3. 提交文档处理任务
// 4. 更新调度状态和执行记录
type ScheduleRefreshProcessor struct {
	scheduleRepo      port.KnowledgeDocumentScheduleRepository
	documentRepo      port.KnowledgeDocumentRepository
	execRepo          port.KnowledgeDocumentScheduleExecRepository
	storage           port.FileStorage
	lockManager       *ScheduleLockManager
	stateManager      *ScheduleStateManager
	documentHelper    *DocumentStatusHelper
	remoteFileFetcher RemoteFileFetchClient
	documentProcessor RefreshedDocumentProcessor
	now               func() time.Time
	nextID            func() (int64, error)
}

// scheduleRefreshPhase 定义刷新流程的阶段
// 设计意图：用于异常时的回滚判断
type scheduleRefreshPhase int

const (
	scheduleRefreshPhaseInit         scheduleRefreshPhase = iota // 初始状态
	scheduleRefreshPhaseDocOccupied                              // 文档已标记为 running
	scheduleRefreshPhaseFileStored                               // 文件已存储到 S3
	scheduleRefreshPhaseFileSwitched                             // 文件元数据已切换
)

// scheduleRefreshRunState 记录刷新流程的运行状态
// 设计意图：用于 defer 中的清理和回滚
type scheduleRefreshRunState struct {
	schedule domain.KnowledgeDocumentSchedule             // 调度配置
	document domain.KnowledgeDocument                     // 文档信息
	ctx      domain.KnowledgeDocumentScheduleStateContext // 状态上下文
	fetch    *RemoteFetchResult                           // 远程获取结果
	stored   *StoredFileDTO                               // 存储结果
	phase    scheduleRefreshPhase                         // 当前阶段
}

// NewScheduleRefreshProcessor 创建调度刷新处理器实例
// 设计意图：依赖自动装配，降低使用门槛
func NewScheduleRefreshProcessor(options ScheduleRefreshProcessorOptions) *ScheduleRefreshProcessor {
	// 如果没有提供 LockManager，自动创建默认实现
	lockManager := options.LockManager
	if lockManager == nil && options.ScheduleRepo != nil {
		lockManager = NewScheduleLockManager(options.ScheduleRepo, ScheduleLockOptions{})
	}

	// 如果没有提供 StateManager，自动创建
	stateManager := options.StateManager
	if stateManager == nil {
		stateManager = NewScheduleStateManager(options.ScheduleRepo, options.ExecRepo)
	}

	// 如果没有提供 DocumentHelper，自动创建
	documentHelper := options.DocumentHelper
	if documentHelper == nil && options.DocumentRepo != nil {
		documentHelper = NewDocumentStatusHelper(options.DocumentRepo)
	}

	// 默认使用当前时间和分布式 ID 生成器
	now := options.Now
	if now == nil {
		now = time.Now
	}
	nextID := options.NextID
	if nextID == nil {
		nextID = distributedid.NextID
	}

	return &ScheduleRefreshProcessor{
		scheduleRepo:      options.ScheduleRepo,
		documentRepo:      options.DocumentRepo,
		execRepo:          options.ExecRepo,
		storage:           options.Storage,
		lockManager:       lockManager,
		stateManager:      stateManager,
		documentHelper:    documentHelper,
		remoteFileFetcher: options.RemoteFileFetcher,
		documentProcessor: options.DocumentProcessor,
		now:               now,
		nextID:            nextID,
	}
}

// Process 是调度刷新任务的核心方法
// 流程：
// 1. 前置校验 → 2. 加载配置 → 3. 检查文件变化 → 4. 下载存储 → 5. 提交处理 → 6. 更新状态
// 设计意图：
// - 心跳机制保证长任务执行过程中锁不过期
// - 阶段状态机支持异常回滚
// - 锁丢失检测保证状态一致性
func (p *ScheduleRefreshProcessor) Process(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error {
	// 1. 前置校验
	if !validLease(lease) {
		return nil
	}
	if err := p.validateDependencies(); err != nil {
		return err
	}

	// 2. 初始化运行状态
	state := &scheduleRefreshRunState{}
	startTime := p.now()
	// 检查锁是否仍然有效
	if p.shouldAbortForLeaseLoss(ctx, lease, nil, "task start") {
		return nil
	}

	// 3. 启动心跳线程，定期续租锁
	heartbeat := p.lockManager.StartHeartbeat(ctx, lease)
	defer heartbeat.Close()
	// defer 清理：异常时回滚文档状态和删除临时文件
	defer p.cleanupAfterProcess(ctx, lease, state, heartbeat)

	// 4. 加载调度配置
	schedule, err := p.scheduleRepo.GetByID(ctx, lease.ScheduleID)
	if err != nil {
		return exception.NewServiceException("failed to get knowledge document schedule", err)
	}
	if schedule.ID == "" {
		return nil
	}
	state.schedule = schedule

	// 5. 加载文档前再次检查锁
	if p.shouldAbortForLeaseLoss(ctx, lease, heartbeat, "load document") {
		return nil
	}

	// 6. 加载文档
	document, err := p.documentRepo.GetByID(ctx, schedule.DocumentID)
	if err != nil {
		return exception.NewServiceException("failed to get scheduled knowledge document", err)
	}
	state.document = document
	if document.ID == "" {
		// 文档不存在，禁用调度
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "document not found or deleted", "disable missing document")
		return nil
	}
	if !document.Enabled {
		// 文档已禁用，禁用调度
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "document disabled", "disable disabled document")
		return nil
	}

	// 7. 校验调度配置
	cron := strings.TrimSpace(document.ScheduleCron)
	enabled := document.ScheduleEnabled
	// 只有 URL 类型的文档才支持定时刷新
	if cron == "" || !strings.EqualFold(document.SourceType, domain.KnowledgeDocumentSourceURL) {
		enabled = false
	}
	if !enabled {
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "schedule disabled", "disable schedule")
		return nil
	}

	// 8. 计算下次执行时间
	nextRunTime, err := NextRunTime(cron, startTime)
	if err != nil {
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "invalid cron expression", "disable invalid cron")
		return nil
	}
	if nextRunTime == nil {
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "failed to compute next run time", "disable empty next run")
		return nil
	}

	// 9. 创建执行记录
	exec, err := p.createExec(ctx, schedule, document, startTime)
	if err != nil {
		return err
	}
	state.ctx = domain.KnowledgeDocumentScheduleStateContext{
		ScheduleID:  schedule.ID,
		ExecID:      exec.ID,
		CronExpr:    cron,
		StartTime:   startTime,
		NextRunTime: nextRunTime,
	}

	// 10. 检查远程文件是否变化
	fetchResult, err := p.remoteFileFetcher.FetchIfChanged(ctx, document.SourceLocation, schedule.LastETag, schedule.LastModified, schedule.LastContentHash, document.Name)
	if err != nil {
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "fetch remote file")
		return err
	}
	state.fetch = &fetchResult
	defer fetchResult.Close()

	// 11. 文件未变化，跳过处理
	if !fetchResult.Changed {
		p.markSkippedFetchIfOwnedOrMarkLeaseLost(ctx, lease, state, fetchResult, "remote file unchanged")
		return nil
	}

	// 12. 文档正在处理中，跳过
	if document.Status == domain.KnowledgeDocumentStatusRunning {
		p.markSkippedIfOwnedOrMarkLeaseLost(ctx, lease, state, "document is running, skip schedule", "document occupied")
		return nil
	}

	// 13. 标记文档为 running 前检查锁
	if p.shouldAbortForLeaseLoss(ctx, lease, heartbeat, "claim document") {
		_ = p.stateManager.MarkLeaseLost(ctx, state.ctx, "claim document")
		return nil
	}

	// 14. 标记文档为 running（占用文档）
	occupied, err := p.documentHelper.TryMarkRunning(ctx, document.ID)
	if err != nil {
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "claim document failed")
		return err
	}
	if !occupied {
		// 标记失败（可能已被其他实例占用）
		p.markSkippedIfOwnedOrMarkLeaseLost(ctx, lease, state, "document is running, skip schedule", "document claim missed")
		return nil
	}
	state.phase = scheduleRefreshPhaseDocOccupied

	// 15. 下载文件并存储到 S3
	stored, err := p.storeFetchedFile(ctx, document, fetchResult)
	if err != nil {
		_ = p.documentHelper.MarkFailedIfRunning(ctx, document.ID)
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "store remote file")
		return err
	}
	state.stored = &stored
	state.phase = scheduleRefreshPhaseFileStored

	// 16. 构建刷新后的文档对象
	refreshedDoc := document
	refreshedDoc.Name = stored.OriginFileName
	refreshedDoc.FileURL = stored.Url
	refreshedDoc.FileType = stored.DetectedType
	refreshedDoc.FileSize = stored.Size
	refreshedDoc.UpdatedBy = systemUser

	// 17. 提交文档处理任务（分块、向量化）
	if p.documentProcessor != nil {
		if p.shouldAbortForLeaseLoss(ctx, lease, heartbeat, "process document") {
			_ = p.stateManager.MarkLeaseLost(ctx, state.ctx, "process document")
			return nil
		}
		if err := p.documentProcessor.ProcessRefreshedDocument(ctx, refreshedDoc); err != nil {
			_ = p.documentHelper.MarkFailedIfRunning(ctx, document.ID)
			p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "process document")
			return err
		}
	}

	// 18. 更新文档元数据（指向新文件）
	if err := p.documentHelper.ApplyRefreshedFileMetadata(ctx, document.ID, stored); err != nil {
		_ = p.documentHelper.MarkFailedIfRunning(ctx, document.ID)
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "switch refreshed file")
		return err
	}

	// 19. 标记文档处理成功
	if err := p.documentHelper.MarkSuccessIfRunning(ctx, document.ID); err != nil {
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "mark document success")
		return err
	}
	state.phase = scheduleRefreshPhaseFileSwitched

	// 20. 更新调度状态为成功
	if !p.markSuccessIfOwnedOrMarkLeaseLost(ctx, lease, state, fetchResult, "write success state") {
		// 锁已丢失，只更新执行记录
		_ = p.stateManager.MarkSuccessExecOnly(ctx, state.ctx, state.stored, fetchResult.ContentHash, fetchResult.ETag, fetchResult.LastModified, "refresh success; schedule state write failed")
	}
	return nil
}

// validateDependencies 校验所有依赖是否已初始化
func (p *ScheduleRefreshProcessor) validateDependencies() error {
	if p == nil {
		return exception.NewServiceException("schedule refresh processor is required", nil)
	}
	if p.scheduleRepo == nil {
		return exception.NewServiceException("knowledge document schedule repository is required", nil)
	}
	if p.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	if p.execRepo == nil {
		return exception.NewServiceException("knowledge document schedule exec repository is required", nil)
	}
	if p.storage == nil {
		return exception.NewServiceException("file storage is required", nil)
	}
	if p.lockManager == nil {
		return exception.NewServiceException("schedule lock manager is required", nil)
	}
	if p.stateManager == nil {
		return exception.NewServiceException("schedule state manager is required", nil)
	}
	if p.documentHelper == nil {
		return exception.NewServiceException("document status helper is required", nil)
	}
	if p.remoteFileFetcher == nil {
		return exception.NewServiceException("remote file fetcher is required", nil)
	}
	if p.now == nil || p.nextID == nil {
		return exception.NewServiceException("schedule refresh processor clock/id generator is required", nil)
	}
	return nil
}

// createExec 创建执行记录
func (p *ScheduleRefreshProcessor) createExec(
	ctx context.Context,
	schedule domain.KnowledgeDocumentSchedule,
	document domain.KnowledgeDocument,
	startTime time.Time,
) (domain.KnowledgeDocumentScheduleExec, error) {
	id, err := p.nextID()
	if err != nil {
		return domain.KnowledgeDocumentScheduleExec{}, exception.NewServiceException("failed to generate schedule exec id", err)
	}
	exec := domain.NewKnowledgeDocumentScheduleExec(fmt.Sprintf("%d", id), schedule.ID, document.ID, document.KnowledgeBaseID, startTime)
	created, err := p.execRepo.Create(ctx, exec)
	if err != nil {
		return domain.KnowledgeDocumentScheduleExec{}, exception.NewServiceException("failed to create schedule exec", err)
	}
	return created, nil
}

// storeFetchedFile 将获取的文件上传到存储
// 流程：打开临时文件 → 生成存储 Key → 上传 → 返回存储信息
func (p *ScheduleRefreshProcessor) storeFetchedFile(ctx context.Context, document domain.KnowledgeDocument, fetchResult RemoteFetchResult) (StoredFileDTO, error) {
	if strings.TrimSpace(fetchResult.TempFile) == "" {
		return StoredFileDTO{}, exception.NewServiceException("remote fetch temp file is required", nil)
	}
	file, err := os.Open(fetchResult.TempFile)
	if err != nil {
		return StoredFileDTO{}, exception.NewServiceException("open remote fetch temp file failed", err)
	}
	defer file.Close()

	// 生成存储 Key：knowledge/{documentId}/schedule/{timestamp}/{fileName}
	key := scheduleRefreshStorageKey(document, fetchResult.FileName, p.now())
	stored, err := p.storage.Upload(ctx, port.FileUpload{
		Key:         key,
		FileName:    firstText(fetchResult.FileName, document.Name, "remote-file"),
		ContentType: fetchResult.ContentType,
		Size:        fetchResult.Size,
		Body:        file,
	})
	if err != nil {
		return StoredFileDTO{}, exception.NewServiceException("upload refreshed remote file failed", err)
	}
	return StoredFileDTO{
		Url:            stored.Key,
		DetectedType:   stored.ContentType,
		Size:           stored.Size,
		OriginFileName: stored.FileName,
	}, nil
}

// shouldAbortForLeaseLoss 检查是否应该因为锁丢失而中止
// 设计意图：在关键步骤前检查锁是否仍然有效
func (p *ScheduleRefreshProcessor) shouldAbortForLeaseLoss(
	ctx context.Context,
	lease domain.KnowledgeDocumentScheduleLockLease,
	heartbeat *ScheduleLockHeartbeat,
	stage string,
) bool {
	// 心跳已丢失，直接中止
	if heartbeat != nil && heartbeat.IsLost() {
		log.Warnf("schedule refresh lock lost: scheduleId=%s stage=%s lockToken=%s", lease.ScheduleID, stage, lease.LockToken)
		return true
	}
	// 尝试续租
	renewed, err := p.lockManager.Renew(ctx, lease)
	if err != nil {
		log.Warnf("schedule refresh lock renew failed: scheduleId=%s stage=%s lockToken=%s err=%v", lease.ScheduleID, stage, lease.LockToken, err)
		return true
	}
	if !renewed {
		log.Warnf("schedule refresh lock renew missed: scheduleId=%s stage=%s lockToken=%s", lease.ScheduleID, stage, lease.LockToken)
	}
	return !renewed
}

// cleanupAfterProcess 清理资源
// 设计意图：defer 调用，异常时回滚文档状态和删除临时文件
func (p *ScheduleRefreshProcessor) cleanupAfterProcess(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, state *scheduleRefreshRunState, heartbeat *ScheduleLockHeartbeat) {
	if state == nil {
		return
	}
	// 创建独立的 context 用于清理操作
	cleanupCtx, cleanupCancel := newBackgroundTaskContext(ctx, 5*time.Second)
	defer cleanupCancel()

	// 如果文档状态不一致，回滚为 failed
	if p.shouldRollbackDocumentState(ctx, state, heartbeat) {
		_ = p.documentHelper.MarkFailedIfRunning(cleanupCtx, state.document.ID)
	}
	// 如果文件已存储但未切换，删除临时文件
	if state.stored != nil && state.phase < scheduleRefreshPhaseFileSwitched {
		_ = p.storage.Delete(cleanupCtx, state.stored.Url)
	}
}

// shouldRollbackDocumentState 判断是否需要回滚文档状态
// 设计意图：
// - phase < DocOccupied：文档还未被占用，不需要回滚
// - phase >= FileSwitched：文件已切换完成，不需要回滚
// - DocOccupied <= phase < FileSwitched：文档状态不一致，需要回滚
func (p *ScheduleRefreshProcessor) shouldRollbackDocumentState(ctx context.Context, state *scheduleRefreshRunState, heartbeat *ScheduleLockHeartbeat) bool {
	if state == nil || state.document.ID == "" {
		return false
	}
	if state.phase < scheduleRefreshPhaseDocOccupied || state.phase >= scheduleRefreshPhaseFileSwitched {
		return false
	}
	// 心跳丢失或 context 取消，需要回滚
	if heartbeat != nil && heartbeat.IsLost() {
		return true
	}
	return ctx != nil && ctx.Err() != nil
}

// disableIfOwnedOrMarkLeaseLost 禁用调度（如果锁仍有效）
func (p *ScheduleRefreshProcessor) disableIfOwnedOrMarkLeaseLost(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, state *scheduleRefreshRunState, reason string, stage string) {
	updated, err := p.stateManager.DisableIfOwned(ctx, lease, reason)
	if err != nil {
		log.Warnf("disable schedule failed: scheduleId=%s stage=%s err=%v", lease.ScheduleID, stage, err)
		return
	}
	if !updated {
		log.Warnf("schedule state write skipped because lock is lost: scheduleId=%s stage=%s lockToken=%s", lease.ScheduleID, stage, lease.LockToken)
	}
}

// markSkippedFetchIfOwnedOrMarkLeaseLost 标记调度为跳过（文件未变化）
func (p *ScheduleRefreshProcessor) markSkippedFetchIfOwnedOrMarkLeaseLost(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, state *scheduleRefreshRunState, fetchResult RemoteFetchResult, stage string) {
	updated, err := p.stateManager.MarkSkippedFetchIfOwned(ctx, lease, state.ctx, ScheduleFetchResult{
		Message:      fetchResult.Message,
		ETag:         fetchResult.ETag,
		LastModified: fetchResult.LastModified,
		ContentHash:  fetchResult.ContentHash,
	})
	if err != nil {
		log.Warnf("mark schedule skipped failed: scheduleId=%s stage=%s err=%v", lease.ScheduleID, stage, err)
		return
	}
	if !updated {
		log.Warnf("schedule skipped state write skipped because lock is lost: scheduleId=%s stage=%s lockToken=%s", lease.ScheduleID, stage, lease.LockToken)
	}
}

// markSkippedIfOwnedOrMarkLeaseLost 标记调度为跳过（通用）
func (p *ScheduleRefreshProcessor) markSkippedIfOwnedOrMarkLeaseLost(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, state *scheduleRefreshRunState, message string, stage string) {
	updated, err := p.stateManager.MarkSkippedIfOwned(ctx, lease, state.ctx, message)
	if err != nil {
		log.Warnf("mark schedule skipped failed: scheduleId=%s stage=%s err=%v", lease.ScheduleID, stage, err)
		return
	}
	if !updated {
		log.Warnf("schedule skipped state write skipped because lock is lost: scheduleId=%s stage=%s lockToken=%s", lease.ScheduleID, stage, lease.LockToken)
	}
}

// markFailedIfOwnedOrMarkLeaseLost 标记调度为失败
func (p *ScheduleRefreshProcessor) markFailedIfOwnedOrMarkLeaseLost(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, state *scheduleRefreshRunState, message string, stage string) {
	if state == nil || strings.TrimSpace(state.ctx.ExecID) == "" {
		return
	}
	updated, err := p.stateManager.MarkFailedIfOwned(ctx, lease, state.ctx, message)
	if err != nil {
		log.Warnf("mark schedule failed failed: scheduleId=%s stage=%s err=%v", lease.ScheduleID, stage, err)
		return
	}
	if !updated {
		log.Warnf("schedule failed state write skipped because lock is lost: scheduleId=%s stage=%s lockToken=%s", lease.ScheduleID, stage, lease.LockToken)
	}
}

// markSuccessIfOwnedOrMarkLeaseLost 标记调度为成功
// 返回值表示调度配置是否更新成功
func (p *ScheduleRefreshProcessor) markSuccessIfOwnedOrMarkLeaseLost(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, state *scheduleRefreshRunState, fetchResult RemoteFetchResult, stage string) bool {
	updated, err := p.stateManager.MarkSuccessIfOwned(ctx, lease, state.ctx, ScheduleFetchResult{
		Message:      fetchResult.Message,
		ETag:         fetchResult.ETag,
		LastModified: fetchResult.LastModified,
		ContentHash:  fetchResult.ContentHash,
	}, *state.stored)
	if err != nil {
		log.Warnf("mark schedule success failed: scheduleId=%s stage=%s err=%v", lease.ScheduleID, stage, err)
		return false
	}
	if !updated {
		log.Warnf("schedule success state write skipped because lock is lost: scheduleId=%s stage=%s lockToken=%s", lease.ScheduleID, stage, lease.LockToken)
	}
	return updated
}

// scheduleRefreshStorageKey 生成存储 Key
// 格式：knowledge/{documentId}/schedule/{nanosecond_timestamp}/{fileName}
func scheduleRefreshStorageKey(document domain.KnowledgeDocument, fileName string, now time.Time) string {
	cleanName := path.Base(firstText(fileName, document.Name, "remote-file"))
	return fmt.Sprintf("knowledge/%s/schedule/%d/%s", document.ID, now.UnixNano(), cleanName)
}
