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

type RemoteFileFetchClient interface {
	FetchIfChanged(ctx context.Context, rawURL string, lastETag string, lastModified string, lastContentHash string, fallbackFileName string) (RemoteFetchResult, error)
}

type RefreshedDocumentProcessor interface {
	ProcessRefreshedDocument(ctx context.Context, document domain.KnowledgeDocument) error
}

type ScheduleRefreshProcessorOptions struct {
	ScheduleRepo      port.KnowledgeDocumentScheduleRepository
	DocumentRepo      port.KnowledgeDocumentRepository
	ExecRepo          port.KnowledgeDocumentScheduleExecRepository
	Storage           port.FileStorage
	LockManager       *ScheduleLockManager
	StateManager      *ScheduleStateManager
	DocumentHelper    *DocumentStatusHelper
	RemoteFileFetcher RemoteFileFetchClient
	DocumentProcessor RefreshedDocumentProcessor
	Now               func() time.Time
	NextID            func() (int64, error)
}

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

type scheduleRefreshPhase int

const (
	scheduleRefreshPhaseInit scheduleRefreshPhase = iota
	scheduleRefreshPhaseDocOccupied
	scheduleRefreshPhaseFileStored
	scheduleRefreshPhaseFileSwitched
)

type scheduleRefreshRunState struct {
	schedule domain.KnowledgeDocumentSchedule
	document domain.KnowledgeDocument
	ctx      domain.KnowledgeDocumentScheduleStateContext
	fetch    *RemoteFetchResult
	stored   *StoredFileDTO
	phase    scheduleRefreshPhase
}

func NewScheduleRefreshProcessor(options ScheduleRefreshProcessorOptions) *ScheduleRefreshProcessor {
	lockManager := options.LockManager
	if lockManager == nil && options.ScheduleRepo != nil {
		lockManager = NewScheduleLockManager(options.ScheduleRepo, ScheduleLockOptions{})
	}

	stateManager := options.StateManager
	if stateManager == nil {
		stateManager = NewScheduleStateManager(options.ScheduleRepo, options.ExecRepo)
	}

	documentHelper := options.DocumentHelper
	if documentHelper == nil && options.DocumentRepo != nil {
		documentHelper = NewDocumentStatusHelper(options.DocumentRepo)
	}

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

func (p *ScheduleRefreshProcessor) Process(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error {
	if !validLease(lease) {
		return nil
	}
	if err := p.validateDependencies(); err != nil {
		return err
	}

	state := &scheduleRefreshRunState{}
	startTime := p.now()
	if p.shouldAbortForLeaseLoss(ctx, lease, nil, "task start") {
		return nil
	}

	heartbeat := p.lockManager.StartHeartbeat(ctx, lease)
	defer heartbeat.Close()
	defer p.cleanupAfterProcess(ctx, lease, state, heartbeat)

	schedule, err := p.scheduleRepo.GetByID(ctx, lease.ScheduleID)
	if err != nil {
		return exception.NewServiceException("failed to get knowledge document schedule", err)
	}
	if schedule.ID == "" {
		return nil
	}
	state.schedule = schedule

	document, err := p.documentRepo.GetByID(ctx, schedule.DocumentID)
	if err != nil {
		return exception.NewServiceException("failed to get scheduled knowledge document", err)
	}
	state.document = document
	if document.ID == "" {
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "document not found or deleted", "disable missing document")
		return nil
	}
	if !document.Enabled {
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "document disabled", "disable disabled document")
		return nil
	}

	cron := strings.TrimSpace(document.ScheduleCron)
	enabled := document.ScheduleEnabled
	if cron == "" || !strings.EqualFold(document.SourceType, domain.KnowledgeDocumentSourceURL) {
		enabled = false
	}
	if !enabled {
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "schedule disabled", "disable schedule")
		return nil
	}

	nextRunTime, err := NextRunTime(cron, startTime)
	if err != nil {
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "invalid cron expression", "disable invalid cron")
		return nil
	}
	if nextRunTime == nil {
		p.disableIfOwnedOrMarkLeaseLost(ctx, lease, state, "failed to compute next run time", "disable empty next run")
		return nil
	}

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

	fetchResult, err := p.remoteFileFetcher.FetchIfChanged(ctx, document.SourceLocation, schedule.LastETag, schedule.LastModified, schedule.LastContentHash, document.Name)
	if err != nil {
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "fetch remote file")
		return err
	}
	state.fetch = &fetchResult
	defer fetchResult.Close()

	if !fetchResult.Changed {
		p.markSkippedFetchIfOwnedOrMarkLeaseLost(ctx, lease, state, fetchResult, "remote file unchanged")
		return nil
	}

	if document.Status == domain.KnowledgeDocumentStatusRunning {
		p.markSkippedIfOwnedOrMarkLeaseLost(ctx, lease, state, "document is running, skip schedule", "document occupied")
		return nil
	}

	if p.shouldAbortForLeaseLoss(ctx, lease, heartbeat, "claim document") {
		_ = p.stateManager.MarkLeaseLost(ctx, state.ctx, "claim document")
		return nil
	}

	occupied, err := p.documentHelper.TryMarkRunning(ctx, document.ID)
	if err != nil {
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "claim document failed")
		return err
	}
	if !occupied {
		p.markSkippedIfOwnedOrMarkLeaseLost(ctx, lease, state, "document is running, skip schedule", "document claim missed")
		return nil
	}
	state.phase = scheduleRefreshPhaseDocOccupied

	stored, err := p.storeFetchedFile(ctx, document, fetchResult)
	if err != nil {
		_ = p.documentHelper.MarkFailedIfRunning(ctx, document.ID)
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "store remote file")
		return err
	}
	state.stored = &stored
	state.phase = scheduleRefreshPhaseFileStored

	refreshedDoc := document
	refreshedDoc.Name = stored.OriginFileName
	refreshedDoc.FileURL = stored.Url
	refreshedDoc.FileType = stored.DetectedType
	refreshedDoc.FileSize = stored.Size
	refreshedDoc.UpdatedBy = systemUser
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

	if err := p.documentHelper.ApplyRefreshedFileMetadata(ctx, document.ID, stored); err != nil {
		_ = p.documentHelper.MarkFailedIfRunning(ctx, document.ID)
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "switch refreshed file")
		return err
	}
	if err := p.documentHelper.MarkSuccessIfRunning(ctx, document.ID); err != nil {
		p.markFailedIfOwnedOrMarkLeaseLost(ctx, lease, state, err.Error(), "mark document success")
		return err
	}
	state.phase = scheduleRefreshPhaseFileSwitched

	if !p.markSuccessIfOwnedOrMarkLeaseLost(ctx, lease, state, fetchResult, "write success state") {
		_ = p.stateManager.MarkSuccessExecOnly(ctx, state.ctx, state.stored, fetchResult.ContentHash, fetchResult.ETag, fetchResult.LastModified, "refresh success; schedule state write failed")
	}
	return nil
}

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

func (p *ScheduleRefreshProcessor) storeFetchedFile(ctx context.Context, document domain.KnowledgeDocument, fetchResult RemoteFetchResult) (StoredFileDTO, error) {
	if strings.TrimSpace(fetchResult.TempFile) == "" {
		return StoredFileDTO{}, exception.NewServiceException("remote fetch temp file is required", nil)
	}
	file, err := os.Open(fetchResult.TempFile)
	if err != nil {
		return StoredFileDTO{}, exception.NewServiceException("open remote fetch temp file failed", err)
	}
	defer file.Close()

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

func (p *ScheduleRefreshProcessor) shouldAbortForLeaseLoss(
	ctx context.Context,
	lease domain.KnowledgeDocumentScheduleLockLease,
	heartbeat *ScheduleLockHeartbeat,
	stage string,
) bool {
	if heartbeat != nil && heartbeat.IsLost() {
		log.Warnf("schedule refresh lock lost: scheduleId=%s stage=%s lockToken=%s", lease.ScheduleID, stage, lease.LockToken)
		return true
	}
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

func (p *ScheduleRefreshProcessor) cleanupAfterProcess(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, state *scheduleRefreshRunState, heartbeat *ScheduleLockHeartbeat) {
	if state == nil {
		return
	}
	if heartbeat != nil && heartbeat.IsLost() && state.phase == scheduleRefreshPhaseDocOccupied && state.document.ID != "" {
		_ = p.documentHelper.MarkFailedIfRunning(ctx, state.document.ID)
	}
	if state.stored != nil && state.phase < scheduleRefreshPhaseFileSwitched {
		_ = p.storage.Delete(ctx, state.stored.Url)
	}
}

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

func scheduleRefreshStorageKey(document domain.KnowledgeDocument, fileName string, now time.Time) string {
	cleanName := path.Base(firstText(fileName, document.Name, "remote-file"))
	return fmt.Sprintf("knowledge/%s/schedule/%d/%s", document.ID, now.UnixNano(), cleanName)
}
