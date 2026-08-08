package schedule

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type LockManager struct {
	subscriptionRepo port.SubscriptionRepository
	lockSeconds      int64
	now              func() time.Time
}

func NewLockManager(subscriptionRepo port.SubscriptionRepository, lockSeconds int64, now func() time.Time) *LockManager {
	if now == nil {
		now = time.Now
	}
	if lockSeconds <= 0 {
		lockSeconds = 900
	}
	return &LockManager{
		subscriptionRepo: subscriptionRepo,
		lockSeconds:      lockSeconds,
		now:              now,
	}
}

func (m *LockManager) TryAcquire(ctx context.Context, userID string) (domain.SubscriptionLockLease, bool, error) {
	if m == nil || m.subscriptionRepo == nil {
		return domain.SubscriptionLockLease{}, false, nil
	}
	now := m.now()
	lease := domain.SubscriptionLockLease{
		UserID:    strings.TrimSpace(userID),
		LockOwner: m.nextToken(),
	}
	ok, err := m.subscriptionRepo.TryAcquireLock(ctx, lease, now.Add(time.Duration(m.lockSeconds)*time.Second), now)
	if err != nil || !ok {
		return domain.SubscriptionLockLease{}, false, err
	}
	return lease, true, nil
}

func (m *LockManager) Release(ctx context.Context, lease domain.SubscriptionLockLease) error {
	if m == nil || m.subscriptionRepo == nil {
		return nil
	}
	_, err := m.subscriptionRepo.ReleaseLock(ctx, lease)
	return err
}

func (m *LockManager) nextToken() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("daily-brief-%d", m.now().UnixNano())
	}
	return "daily-brief-" + hex.EncodeToString(buf)
}
