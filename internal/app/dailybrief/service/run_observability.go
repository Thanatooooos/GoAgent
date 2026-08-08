package service

import (
	"sort"
	"strings"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/framework/log"
)

func logGenerationFinished(
	request GenerationRequest,
	run domain.GenerationRun,
	collectResult SourceCollectResult,
	candidateCount int,
	selectedCount int,
	itemCount int,
) {
	status := strings.TrimSpace(run.Status)
	if status == "" {
		status = "unknown"
	}
	failedSources := sortedSourceKeys(collectResult.Failures)
	fields := []any{
		"user_id", strings.TrimSpace(request.Subscription.UserID),
		"brief_date", strings.TrimSpace(request.BriefDate),
		"run_id", strings.TrimSpace(run.ID),
		"trigger_type", strings.TrimSpace(request.TriggerType),
		"status", status,
		"failed_sources", failedSources,
		"candidate_count", candidateCount,
		"selected_count", selectedCount,
	}
	if itemCount > 0 {
		fields = append(fields, "final_item_count", itemCount)
	}
	switch status {
	case domain.GenerationRunStatusFailed:
		log.Warnw("daily brief generation finished", fields...)
	default:
		log.Infow("daily brief generation finished", fields...)
	}
}

func sortedSourceKeys(failures map[string]error) []string {
	if len(failures) == 0 {
		return nil
	}
	keys := make([]string, 0, len(failures))
	for key := range failures {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func recordRunObservability(
	metrics *MetricsService,
	request GenerationRequest,
	run domain.GenerationRun,
	collectResult SourceCollectResult,
	candidateCount int,
	selectedCount int,
	itemCount int,
) {
	if metrics != nil {
		metrics.RecordSourceCollect(collectResult)
		metrics.RecordPipelineCounts(candidateCount, selectedCount)
		if itemCount > 0 {
			metrics.RecordFinalItemCount(itemCount)
		}
		metrics.RecordRunStatus(run.Status)
	}
	logGenerationFinished(request, run, collectResult, candidateCount, selectedCount, itemCount)
}
