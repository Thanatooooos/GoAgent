package cachemetrics

import (
	"sort"
	"strings"
	"sync"
)

type EventSnapshot struct {
	CacheKind string `json:"cacheKind"`
	Layer     string `json:"layer"`
	Outcome   string `json:"outcome"`
	Count     int64  `json:"count"`
}

type MetricsSnapshot struct {
	Events                   []EventSnapshot `json:"events"`
	LocalEvictions           int64           `json:"localEvictions"`
	VersionInvalidations     int64           `json:"versionInvalidations"`
	FingerprintInvalidations int64           `json:"fingerprintInvalidations"`
	RedisDecodeFailures      int64           `json:"redisDecodeFailures"`
}

type Service struct {
	mu                      sync.RWMutex
	events                  map[string]int64
	localEvictions          int64
	versionInvalidations    int64
	fingerprintInvalidation int64
	redisDecodeFailures     int64
}

func NewService() *Service {
	return &Service{
		events: make(map[string]int64),
	}
}

func (s *Service) Record(cacheKind string, layer string, outcome string) {
	if s == nil {
		return
	}
	cacheKind = strings.TrimSpace(cacheKind)
	layer = strings.TrimSpace(layer)
	outcome = strings.TrimSpace(outcome)
	if cacheKind == "" || layer == "" || outcome == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[eventKey(cacheKind, layer, outcome)]++
}

func (s *Service) RecordLocalEviction() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localEvictions++
}

func (s *Service) RecordVersionInvalidation() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versionInvalidations++
}

func (s *Service) RecordFingerprintInvalidation() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fingerprintInvalidation++
}

func (s *Service) RecordRedisDecodeFailure() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.redisDecodeFailures++
}

func (s *Service) Snapshot() MetricsSnapshot {
	if s == nil {
		return MetricsSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := MetricsSnapshot{
		LocalEvictions:           s.localEvictions,
		VersionInvalidations:     s.versionInvalidations,
		FingerprintInvalidations: s.fingerprintInvalidation,
		RedisDecodeFailures:      s.redisDecodeFailures,
	}
	for key, count := range s.events {
		cacheKind, layer, outcome := splitEventKey(key)
		snapshot.Events = append(snapshot.Events, EventSnapshot{
			CacheKind: cacheKind,
			Layer:     layer,
			Outcome:   outcome,
			Count:     count,
		})
	}
	sort.Slice(snapshot.Events, func(i, j int) bool {
		if snapshot.Events[i].CacheKind != snapshot.Events[j].CacheKind {
			return snapshot.Events[i].CacheKind < snapshot.Events[j].CacheKind
		}
		if snapshot.Events[i].Layer != snapshot.Events[j].Layer {
			return snapshot.Events[i].Layer < snapshot.Events[j].Layer
		}
		return snapshot.Events[i].Outcome < snapshot.Events[j].Outcome
	})
	return snapshot
}

func eventKey(cacheKind string, layer string, outcome string) string {
	return cacheKind + "|" + layer + "|" + outcome
}

func splitEventKey(key string) (string, string, string) {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 {
		return key, "", ""
	}
	return parts[0], parts[1], parts[2]
}
