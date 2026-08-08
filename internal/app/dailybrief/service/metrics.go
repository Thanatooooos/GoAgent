package service

import (
	"sort"
	"sync"

	"local/rag-project/internal/app/dailybrief/domain"
)

type SourceMetricsSnapshot struct {
	Source    string `json:"source"`
	Successes int64  `json:"successes"`
	Failures  int64  `json:"failures"`
}

type MetricsSnapshot struct {
	SubscribedUsersScanned int64                   `json:"subscribedUsersScanned"`
	SuccessfulRuns         int64                   `json:"successfulRuns"`
	DegradedRuns           int64                   `json:"degradedRuns"`
	FailedRuns             int64                   `json:"failedRuns"`
	RetryAttempts          int64                   `json:"retryAttempts"`
	Sources                []SourceMetricsSnapshot `json:"sources"`
	CandidateCountBefore   int64                   `json:"candidateCountBefore"`
	CandidateCountAfter    int64                   `json:"candidateCountAfter"`
	FinalItemCount         int64                   `json:"finalItemCount"`
}

type MetricsService struct {
	mu      sync.RWMutex
	totals  MetricsSnapshot
	sources map[string]*sourceMetricCounts
}

type sourceMetricCounts struct {
	successes int64
	failures  int64
}

func NewMetricsService() *MetricsService {
	return &MetricsService{
		sources: make(map[string]*sourceMetricCounts),
	}
}

func (s *MetricsService) Snapshot() MetricsSnapshot {
	if s == nil {
		return MetricsSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := s.totals
	if len(s.sources) == 0 {
		return snapshot
	}
	snapshot.Sources = make([]SourceMetricsSnapshot, 0, len(s.sources))
	for source, counts := range s.sources {
		snapshot.Sources = append(snapshot.Sources, SourceMetricsSnapshot{
			Source:    source,
			Successes: counts.successes,
			Failures:  counts.failures,
		})
	}
	sort.Slice(snapshot.Sources, func(i, j int) bool {
		return snapshot.Sources[i].Source < snapshot.Sources[j].Source
	})
	return snapshot
}

func (s *MetricsService) RecordSubscribedUsersScanned(count int) {
	if s == nil || count <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totals.SubscribedUsersScanned += int64(count)
}

func (s *MetricsService) RecordRetryAttempt() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totals.RetryAttempts++
}

func (s *MetricsService) RecordPipelineCounts(candidateBefore int, candidateAfter int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if candidateBefore > 0 {
		s.totals.CandidateCountBefore += int64(candidateBefore)
	}
	if candidateAfter > 0 {
		s.totals.CandidateCountAfter += int64(candidateAfter)
	}
}

func (s *MetricsService) RecordFinalItemCount(count int) {
	if s == nil || count <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totals.FinalItemCount += int64(count)
}

func (s *MetricsService) RecordSourceCollect(result SourceCollectResult) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for sourceKey := range result.Failures {
		s.bumpSourceLocked(sourceKey).failures++
	}
	perSource := make(map[string]int64)
	for _, candidate := range result.Candidates {
		if _, failed := result.Failures[candidate.Source]; failed {
			continue
		}
		perSource[candidate.Source]++
	}
	for sourceKey, count := range perSource {
		s.bumpSourceLocked(sourceKey).successes += count
	}
}

func (s *MetricsService) RecordRunStatus(status string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch status {
	case domain.GenerationRunStatusSucceeded:
		s.totals.SuccessfulRuns++
	case domain.GenerationRunStatusDegraded:
		s.totals.DegradedRuns++
	case domain.GenerationRunStatusFailed:
		s.totals.FailedRuns++
	}
}

func (s *MetricsService) bumpSourceLocked(sourceKey string) *sourceMetricCounts {
	counts := s.sources[sourceKey]
	if counts == nil {
		counts = &sourceMetricCounts{}
		s.sources[sourceKey] = counts
	}
	return counts
}
