package model

import (
	"sync"
	"time"

	"local/rag-project/internal/framework/config"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

type modelHealth struct {
	mu                  sync.Mutex
	consecutiveFailures int
	openUntil           time.Time
	halfOpenInFlight    bool
	state               State
}

type ModelHealthStore struct {
	healthByID sync.Map
}

func NewModelHealthStore() *ModelHealthStore {
	return &ModelHealthStore{}
}

func (m *ModelHealthStore) getPoint(id string) *modelHealth {
	if val, ok := m.healthByID.Load(id); ok {
		return val.(*modelHealth)
	}

	health := &modelHealth{state: Closed}
	actual, _ := m.healthByID.LoadOrStore(id, health)
	return actual.(*modelHealth)
}

func (m *ModelHealthStore) allowCall(id string) bool {
	if id == "" {
		return false
	}

	health := m.getPoint(id)
	health.mu.Lock()
	defer health.mu.Unlock()

	switch health.state {
	case Open:
		if health.openUntil.After(time.Now()) {
			return false
		}
		health.state = HalfOpen
		health.halfOpenInFlight = true
		return true
	case HalfOpen:
		if health.halfOpenInFlight {
			return false
		}
		health.halfOpenInFlight = true
		return true
	default:
		return true
	}
}

func (m *ModelHealthStore) AllowCall(id string) bool {
	return m.allowCall(id)
}

func (m *ModelHealthStore) markSuccess(id string) {
	if id == "" {
		return
	}

	health := m.getPoint(id)
	health.mu.Lock()
	defer health.mu.Unlock()

	health.consecutiveFailures = 0
	health.halfOpenInFlight = false
	health.openUntil = time.Time{}
	health.state = Closed
}

func (m *ModelHealthStore) MarkSuccess(id string) {
	m.markSuccess(id)
}

func (m *ModelHealthStore) markFailure(id string) {
	if id == "" {
		return
	}

	health := m.getPoint(id)
	health.mu.Lock()
	defer health.mu.Unlock()

	if health.state == HalfOpen {
		health.state = Open
		health.openUntil = time.Now().Add(openDuration())
		health.consecutiveFailures = 0
		health.halfOpenInFlight = false
		return
	}

	health.consecutiveFailures++
	if health.consecutiveFailures >= failureThreshold() {
		health.state = Open
		health.openUntil = time.Now().Add(openDuration())
		health.consecutiveFailures = 0
		health.halfOpenInFlight = false
	}
}

func (m *ModelHealthStore) MarkFailure(id string) {
	m.markFailure(id)
}

func (m *ModelHealthStore) isUnavailable(id string) bool {
	val, ok := m.healthByID.Load(id)
	if !ok {
		return false
	}

	health := val.(*modelHealth)
	health.mu.Lock()
	defer health.mu.Unlock()

	if health.state == Open && health.openUntil.After(time.Now()) {
		return true
	}
	return health.state == HalfOpen && health.halfOpenInFlight
}

func (m *ModelHealthStore) IsUnavailable(id string) bool {
	return m.isUnavailable(id)
}

func failureThreshold() int {
	cfg := config.Get()
	if cfg == nil || cfg.AI.Selection.FailureThreshold <= 0 {
		return 1
	}
	return cfg.AI.Selection.FailureThreshold
}

func openDuration() time.Duration {
	cfg := config.Get()
	if cfg == nil || cfg.AI.Selection.OpenDurationMs <= 0 {
		return 30 * time.Second
	}
	return time.Duration(cfg.AI.Selection.OpenDurationMs) * time.Millisecond
}
