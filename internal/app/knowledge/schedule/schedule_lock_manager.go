package schedule

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

const (
	defaultScheduleLockSeconds = int64(900)
	minScheduleLockSeconds     = int64(60)
)

type ScheduleLockOptions struct {
	LockSeconds    int64
	InstancePrefix string
	Now            func() time.Time
	TokenSuffix    func() string
}

type ScheduleLockManager struct {
	scheduleRepo   port.KnowledgeDocumentScheduleRepository
	lockSeconds    int64
	instancePrefix string
	now            func() time.Time
	tokenSuffix    func() string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewScheduleLockManager(scheduleRepo port.KnowledgeDocumentScheduleRepository, options ScheduleLockOptions) *ScheduleLockManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ScheduleLockManager{
		scheduleRepo:   scheduleRepo,
		lockSeconds:    effectiveScheduleLockSeconds(options.LockSeconds),
		instancePrefix: normalizeInstancePrefix(options.InstancePrefix),
		now:            normalizeClock(options.Now),
		tokenSuffix:    normalizeTokenSuffix(options.TokenSuffix),
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (m *ScheduleLockManager) TryAcquire(ctx context.Context, scheduleID string, now time.Time) (domain.KnowledgeDocumentScheduleLockLease, bool, error) {
	if m == nil || m.scheduleRepo == nil {
		return domain.KnowledgeDocumentScheduleLockLease{}, false, nil
	}
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return domain.KnowledgeDocumentScheduleLockLease{}, false, nil
	}
	if now.IsZero() {
		now = m.now()
	}

	lease := domain.NewKnowledgeDocumentScheduleLockLease(scheduleID, m.nextLockToken())
	ok, err := m.scheduleRepo.TryAcquireLock(ctx, lease, m.computeLockUntil(now), now)
	if err != nil || !ok {
		return domain.KnowledgeDocumentScheduleLockLease{}, false, err
	}
	return lease, true, nil
}

func (m *ScheduleLockManager) Renew(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	if m == nil || m.scheduleRepo == nil || !validLease(lease) {
		return false, nil
	}
	return m.scheduleRepo.RenewLock(ctx, lease, m.ComputeLockUntil())
}

func (m *ScheduleLockManager) Release(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	if m == nil || m.scheduleRepo == nil || !validLease(lease) {
		return false, nil
	}
	return m.scheduleRepo.ReleaseLock(ctx, lease)
}

func (m *ScheduleLockManager) StartHeartbeat(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) *ScheduleLockHeartbeat {
	startAt := m.now()
	heartbeat := newScheduleLockHeartbeat(lease, startAt, m.lockTTL())
	heartbeat.bindCancel(func() {})

	if m == nil || m.scheduleRepo == nil || !validLease(lease) {
		heartbeat.markLost()
		return heartbeat
	}

	heartbeatCtx, cancel := context.WithCancel(ctx)
	heartbeat.bindCancel(cancel)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				heartbeat.markLost()
			}
		}()

		ticker := time.NewTicker(m.heartbeatInterval())
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				heartbeat.closeWithoutCancel()
				return
			case <-heartbeatCtx.Done():
				heartbeat.closeWithoutCancel()
				return
			case <-ticker.C:
				m.doHeartbeat(heartbeat)
			}
		}
	}()

	return heartbeat
}

func (m *ScheduleLockManager) ComputeLockUntil() time.Time {
	return m.computeLockUntil(m.now())
}

func (m *ScheduleLockManager) Close() {
	if m == nil {
		return
	}
	m.cancel()
	m.wg.Wait()
}

func (m *ScheduleLockManager) doHeartbeat(heartbeat *ScheduleLockHeartbeat) {
	if heartbeat == nil || heartbeat.IsClosed() || heartbeat.IsLost() {
		return
	}

	ok, err := m.Renew(m.ctx, heartbeat.Lease())
	if err == nil && ok {
		heartbeat.markRenewed(m.now())
		return
	}
	if err == nil || heartbeat.isExpiredWithoutConfirmation(m.now()) {
		heartbeat.markLost()
	}
}

func (m *ScheduleLockManager) computeLockUntil(now time.Time) time.Time {
	return now.Add(m.lockTTL())
}

func (m *ScheduleLockManager) heartbeatInterval() time.Duration {
	effectiveSeconds := m.lockSeconds
	intervalSeconds := min(max(effectiveSeconds/3, 5), 60)
	return time.Duration(intervalSeconds) * time.Second
}

func (m *ScheduleLockManager) lockTTL() time.Duration {
	return time.Duration(m.lockSeconds) * time.Second
}

func (m *ScheduleLockManager) nextLockToken() string {
	return m.instancePrefix + ":" + m.tokenSuffix()
}

type ScheduleLockHeartbeat struct {
	lease           domain.KnowledgeDocumentScheduleLockLease
	lockTTL         time.Duration
	lost            atomic.Bool
	closed          atomic.Bool
	lastConfirmedAt atomic.Int64

	cancelMu sync.Mutex
	cancel   func()
}

func newScheduleLockHeartbeat(lease domain.KnowledgeDocumentScheduleLockLease, startAt time.Time, lockTTL time.Duration) *ScheduleLockHeartbeat {
	heartbeat := &ScheduleLockHeartbeat{
		lease:   lease,
		lockTTL: lockTTL,
	}
	heartbeat.lastConfirmedAt.Store(startAt.UnixMilli())
	return heartbeat
}

func (h *ScheduleLockHeartbeat) Lease() domain.KnowledgeDocumentScheduleLockLease {
	if h == nil {
		return domain.KnowledgeDocumentScheduleLockLease{}
	}
	return h.lease
}

func (h *ScheduleLockHeartbeat) IsLost() bool {
	return h != nil && h.lost.Load()
}

func (h *ScheduleLockHeartbeat) IsClosed() bool {
	return h == nil || h.closed.Load()
}

func (h *ScheduleLockHeartbeat) Close() {
	if h == nil || !h.closed.CompareAndSwap(false, true) {
		return
	}
	h.cancelMu.Lock()
	cancel := h.cancel
	h.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (h *ScheduleLockHeartbeat) bindCancel(cancel func()) {
	h.cancelMu.Lock()
	defer h.cancelMu.Unlock()
	h.cancel = cancel
}

func (h *ScheduleLockHeartbeat) markRenewed(now time.Time) {
	h.lastConfirmedAt.Store(now.UnixMilli())
}

func (h *ScheduleLockHeartbeat) markLost() {
	if h.lost.CompareAndSwap(false, true) {
		h.Close()
	}
}

func (h *ScheduleLockHeartbeat) closeWithoutCancel() {
	h.closed.Store(true)
}

func (h *ScheduleLockHeartbeat) isExpiredWithoutConfirmation(now time.Time) bool {
	lastConfirmedAt := time.UnixMilli(h.lastConfirmedAt.Load())
	return now.Sub(lastConfirmedAt) >= h.lockTTL
}

func validLease(lease domain.KnowledgeDocumentScheduleLockLease) bool {
	return strings.TrimSpace(lease.ScheduleID) != "" && strings.TrimSpace(lease.LockToken) != ""
}

func effectiveScheduleLockSeconds(seconds int64) int64 {
	if seconds <= 0 {
		seconds = defaultScheduleLockSeconds
	}
	return max(seconds, minScheduleLockSeconds)
}

func normalizeClock(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return time.Now
}

func normalizeInstancePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" {
		return prefix
	}
	return "kb-schedule-" + randomHex(8)
}

func normalizeTokenSuffix(generator func() string) func() string {
	if generator != nil {
		return generator
	}
	return func() string {
		return randomHex(16)
	}
}

func randomHex(byteCount int) string {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}
