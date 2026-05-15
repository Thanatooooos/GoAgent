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
	// defaultScheduleLockSeconds 分布式锁默认超时时间（15 分钟）
	// 设计意图：足够长以容纳大多数任务执行，避免频繁续租
	defaultScheduleLockSeconds = int64(900)
	// minScheduleLockSeconds 分布式锁最小超时时间（1 分钟）
	// 设计意图：防止配置过短导致锁频繁过期
	minScheduleLockSeconds = int64(60)
)

// ScheduleLockOptions 分布式锁管理器配置选项
type ScheduleLockOptions struct {
	LockSeconds    int64            // 锁超时时间（秒），默认 900 秒
	InstancePrefix string           // 实例前缀，用于生成锁 token，默认随机生成
	Now            func() time.Time // 时间函数抽象，便于单元测试
	TokenSuffix    func() string    // Token 后缀生成函数，默认使用随机 hex
}

// ScheduleLockManager 是调度任务的分布式锁管理器
// 核心功能：
// 1. TryAcquire：尝试获取锁（基于数据库乐观锁）
// 2. Renew：续租锁（心跳机制）
// 3. Release：释放锁
// 4. StartHeartbeat：启动心跳线程，定期续租
type ScheduleLockManager struct {
	scheduleRepo   port.KnowledgeDocumentScheduleRepository // 调度仓储，用于数据库层面的锁操作
	lockSeconds    int64                                    // 锁超时时间（秒）
	instancePrefix string                                   // 实例前缀，用于标识当前实例
	now            func() time.Time                         // 时间函数
	tokenSuffix    func() string                            // Token 后缀生成函数

	ctx    context.Context    // 管理器生命周期 context
	cancel context.CancelFunc // 取消函数，用于关闭管理器
	wg     sync.WaitGroup     // 等待组，用于等待所有心跳线程退出
}

// NewScheduleLockManager 创建分布式锁管理器实例
func NewScheduleLockManager(scheduleRepo port.KnowledgeDocumentScheduleRepository, options ScheduleLockOptions) *ScheduleLockManager {
	// 创建带取消功能的 context，用于控制管理器生命周期
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

// TryAcquire 尝试获取分布式锁
// 流程：生成唯一 token → 调用数据库乐观锁 → 返回租约
// 设计意图：同一调度任务只能被一个实例执行
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

	// 生成唯一锁 token（实例前缀 + 随机后缀）
	// 设计意图：token 用于标识锁的持有者，释放和续租时必须匹配
	lease := domain.NewKnowledgeDocumentScheduleLockLease(scheduleID, m.nextLockToken())
	// 数据库乐观锁：只有当 LockUntil <= now 时才能获取
	ok, err := m.scheduleRepo.TryAcquireLock(ctx, lease, m.computeLockUntil(now), now)
	if err != nil || !ok {
		return domain.KnowledgeDocumentScheduleLockLease{}, false, err
	}
	return lease, true, nil
}

// Renew 续租锁（延长锁的超时时间）
// 使用场景：心跳线程定期调用，防止长任务执行过程中锁过期
func (m *ScheduleLockManager) Renew(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	if m == nil || m.scheduleRepo == nil || !validLease(lease) {
		return false, nil
	}
	return m.scheduleRepo.RenewLock(ctx, lease, m.ComputeLockUntil())
}

// Release 释放锁
// 使用场景：任务完成/失败/服务关闭时调用
func (m *ScheduleLockManager) Release(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	if m == nil || m.scheduleRepo == nil || !validLease(lease) {
		return false, nil
	}
	return m.scheduleRepo.ReleaseLock(ctx, lease)
}

// StartHeartbeat 启动心跳线程，定期续租锁
// 设计意图：防止长任务执行过程中锁过期，导致其他实例误认为任务失败
// 心跳间隔：锁超时时间的 1/3（默认 5 分钟）
func (m *ScheduleLockManager) StartHeartbeat(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) *ScheduleLockHeartbeat {
	startAt := m.now()
	heartbeat := newScheduleLockHeartbeat(lease, startAt, m.lockTTL())
	heartbeat.bindCancel(func() {})

	if m == nil || m.scheduleRepo == nil || !validLease(lease) {
		heartbeat.markLost()
		return heartbeat
	}

	// 创建心跳专用的 context，用于控制心跳线程生命周期
	heartbeatCtx, cancel := context.WithCancel(ctx)
	heartbeat.bindCancel(cancel)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		// panic 恢复：避免心跳线程崩溃影响主流程
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
				// 管理器关闭，停止心跳
				heartbeat.closeWithoutCancel()
				return
			case <-heartbeatCtx.Done():
				// 心跳主动停止（任务完成或锁已释放）
				heartbeat.closeWithoutCancel()
				return
			case <-ticker.C:
				// 定时触发续租
				m.doHeartbeat(heartbeat)
			}
		}
	}()

	return heartbeat
}

// ComputeLockUntil 计算锁的到期时间（当前时间 + 锁超时时间）
func (m *ScheduleLockManager) ComputeLockUntil() time.Time {
	return m.computeLockUntil(m.now())
}

// Close 关闭锁管理器，停止所有心跳线程
func (m *ScheduleLockManager) Close() {
	if m == nil {
		return
	}
	m.cancel()
	m.wg.Wait()
}

// doHeartbeat 执行一次心跳续租
// 设计意图：如果续租失败，标记心跳为 lost，触发后续处理
func (m *ScheduleLockManager) doHeartbeat(heartbeat *ScheduleLockHeartbeat) {
	if heartbeat == nil || heartbeat.IsClosed() || heartbeat.IsLost() {
		return
	}

	ok, err := m.Renew(m.ctx, heartbeat.Lease())
	if err == nil && ok {
		// 续租成功，更新最后确认时间
		heartbeat.markRenewed(m.now())
		return
	}
	// 续租失败：如果已经超过锁超时时间仍未确认，标记为 lost
	if err == nil || heartbeat.isExpiredWithoutConfirmation(m.now()) {
		heartbeat.markLost()
	}
}

// computeLockUntil 计算锁的到期时间
func (m *ScheduleLockManager) computeLockUntil(now time.Time) time.Time {
	return now.Add(m.lockTTL())
}

// heartbeatInterval 计算心跳间隔
// 设计意图：锁超时时间的 1/3，但限制在 5-60 秒之间
// 例如：锁超时 900 秒 → 心跳间隔 300 秒（5 分钟）
func (m *ScheduleLockManager) heartbeatInterval() time.Duration {
	effectiveSeconds := m.lockSeconds
	intervalSeconds := min(max(effectiveSeconds/3, 5), 60)
	return time.Duration(intervalSeconds) * time.Second
}

// lockTTL 返回锁的存活时间（TTL）
func (m *ScheduleLockManager) lockTTL() time.Duration {
	return time.Duration(m.lockSeconds) * time.Second
}

// nextLockToken 生成下一个锁 token
// 格式：实例前缀 + ":" + 随机 hex
// 示例：kb-schedule-a1b2c3d4:1234567890abcdef
func (m *ScheduleLockManager) nextLockToken() string {
	return m.instancePrefix + ":" + m.tokenSuffix()
}

// ScheduleLockHeartbeat 心跳状态对象
// 职责：
// 1. 跟踪锁的最后确认时间
// 2. 检测心跳是否丢失（续租失败）
// 3. 管理心跳线程的生命周期
type ScheduleLockHeartbeat struct {
	lease           domain.KnowledgeDocumentScheduleLockLease // 锁租约信息
	lockTTL         time.Duration                             // 锁超时时间
	lost            atomic.Bool                               // 心跳是否丢失
	closed          atomic.Bool                               // 心跳是否已关闭
	lastConfirmedAt atomic.Int64                              // 最后确认时间（毫秒时间戳）

	cancelMu sync.Mutex // 保护 cancel 函数的互斥锁
	cancel   func()     // 心跳 context 的取消函数
}

// newScheduleLockHeartbeat 创建心跳状态对象
func newScheduleLockHeartbeat(lease domain.KnowledgeDocumentScheduleLockLease, startAt time.Time, lockTTL time.Duration) *ScheduleLockHeartbeat {
	heartbeat := &ScheduleLockHeartbeat{
		lease:   lease,
		lockTTL: lockTTL,
	}
	heartbeat.lastConfirmedAt.Store(startAt.UnixMilli())
	return heartbeat
}

// Lease 返回锁租约信息
func (h *ScheduleLockHeartbeat) Lease() domain.KnowledgeDocumentScheduleLockLease {
	if h == nil {
		return domain.KnowledgeDocumentScheduleLockLease{}
	}
	return h.lease
}

// IsLost 返回心跳是否丢失
// 设计意图：心跳丢失意味着锁可能已被其他实例获取
func (h *ScheduleLockHeartbeat) IsLost() bool {
	return h != nil && h.lost.Load()
}

// IsClosed 返回心跳是否已关闭
func (h *ScheduleLockHeartbeat) IsClosed() bool {
	return h == nil || h.closed.Load()
}

// Close 关闭心跳
// 设计意图：使用 CompareAndSwap 保证只关闭一次
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

// markRenewed 标记心跳已续租（更新最后确认时间）
func (h *ScheduleLockHeartbeat) markRenewed(now time.Time) {
	h.lastConfirmedAt.Store(now.UnixMilli())
}

// markLost 标记心跳丢失
// 设计意图：心跳丢失时自动关闭，防止僵尸心跳
func (h *ScheduleLockHeartbeat) markLost() {
	if h.lost.CompareAndSwap(false, true) {
		h.Close()
	}
}

// closeWithoutCancel 关闭心跳但不触发 cancel
// 使用场景：心跳线程退出时调用，避免重复 cancel
func (h *ScheduleLockHeartbeat) closeWithoutCancel() {
	h.closed.Store(true)
}

// isExpiredWithoutConfirmation 检查是否超过锁超时时间仍未确认
// 设计意图：如果最后一次续租确认已经超过锁超时时间，认为心跳丢失
func (h *ScheduleLockHeartbeat) isExpiredWithoutConfirmation(now time.Time) bool {
	lastConfirmedAt := time.UnixMilli(h.lastConfirmedAt.Load())
	return now.Sub(lastConfirmedAt) >= h.lockTTL
}

// validLease 检查租约是否有效
func validLease(lease domain.KnowledgeDocumentScheduleLockLease) bool {
	return strings.TrimSpace(lease.ScheduleID) != "" && strings.TrimSpace(lease.LockToken) != ""
}

// effectiveScheduleLockSeconds 计算有效的锁超时时间
// 设计意图：确保锁超时时间不小于最小值
func effectiveScheduleLockSeconds(seconds int64) int64 {
	if seconds <= 0 {
		seconds = defaultScheduleLockSeconds
	}
	return max(seconds, minScheduleLockSeconds)
}

// normalizeClock 规范化时间函数
func normalizeClock(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return time.Now
}

// normalizeInstancePrefix 规范化实例前缀
// 设计意图：如果没有指定前缀，自动生成一个随机前缀用于标识实例
func normalizeInstancePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" {
		return prefix
	}
	return "kb-schedule-" + randomHex(8)
}

// normalizeTokenSuffix 规范化 Token 后缀生成函数
func normalizeTokenSuffix(generator func() string) func() string {
	if generator != nil {
		return generator
	}
	return func() string {
		return randomHex(16)
	}
}

// randomHex 生成随机 hex 字符串
// 设计意图：用于生成唯一的锁 token 后缀
func randomHex(byteCount int) string {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		// 随机数生成失败时，使用时间戳作为降级方案
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}
