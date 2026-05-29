package builtin

import (
	"fmt"
	"strings"
)

func humanizeLongTermMemory(values map[string]string) string {
	selectedCount := values["longTermMemory.selectedCount"]
	candidateCount := values["longTermMemory.candidateCount"]
	ruleCount := values["longTermMemory.ruleCount"]
	factSelectedCount := values["longTermMemory.factSelectedCount"]
	factCandidateCount := values["longTermMemory.factCandidateCount"]
	ruleCacheLayer := values["longTermMemory.ruleCacheLayer"]
	factCacheLayer := values["longTermMemory.factCacheLayer"]
	embeddingCacheLayer := values["longTermMemory.embeddingCacheLayer"]
	recomputeReason := values["longTermMemory.recomputeReason"]
	truncated := values["longTermMemory.truncated"]
	if selectedCount == "" && candidateCount == "" && ruleCount == "" && factSelectedCount == "" && factCandidateCount == "" &&
		ruleCacheLayer == "" && factCacheLayer == "" && embeddingCacheLayer == "" && recomputeReason == "" && truncated == "" {
		return ""
	}

	parts := make([]string, 0, 5)
	if selectedCount != "" && candidateCount != "" {
		parts = append(parts, fmt.Sprintf("长期记忆选中了 %s/%s 条候选", selectedCount, candidateCount))
	} else if selectedCount != "" {
		parts = append(parts, fmt.Sprintf("长期记忆选中了 %s 条内容", selectedCount))
	}
	if ruleCount != "" || factSelectedCount != "" || factCandidateCount != "" {
		parts = append(parts, fmt.Sprintf("其中规则记忆 %s 条、事实记忆 %s/%s 条", defaultTraceCountText(ruleCount), defaultTraceCountText(factSelectedCount), defaultTraceCountText(factCandidateCount)))
	}
	if ruleCacheLayer != "" || factCacheLayer != "" || embeddingCacheLayer != "" {
		cacheParts := make([]string, 0, 3)
		if ruleCacheLayer != "" {
			cacheParts = append(cacheParts, "rule="+ruleCacheLayer)
		}
		if factCacheLayer != "" {
			cacheParts = append(cacheParts, "fact="+factCacheLayer)
		}
		if embeddingCacheLayer != "" {
			cacheParts = append(cacheParts, "embedding="+embeddingCacheLayer)
		}
		parts = append(parts, "缓存层命中情况为 "+strings.Join(cacheParts, "，"))
	}
	if recomputeReason != "" {
		parts = append(parts, "重算原因是 "+recomputeReason)
	}
	if truncated == "true" {
		parts = append(parts, "长期记忆上下文发生了截断")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "，") + "。"
}

func humanizeSessionRecall(values map[string]string) string {
	excerptCount := values["sessionRecall.excerptCount"]
	candidateCount := values["sessionRecall.candidateCount"]
	topScore := values["sessionRecall.topScore"]
	cacheLayer := values["sessionRecall.cacheLayer"]
	embeddingCacheLayer := values["sessionRecall.embeddingCacheLayer"]
	recomputeReason := values["sessionRecall.recomputeReason"]
	truncatedBy := values["sessionRecall.truncatedBy"]
	skippedPerMessageLimit := values["sessionRecall.skippedPerMessageLimit"]
	if excerptCount == "" && candidateCount == "" && topScore == "" && cacheLayer == "" && embeddingCacheLayer == "" &&
		recomputeReason == "" && truncatedBy == "" && skippedPerMessageLimit == "" {
		return ""
	}

	parts := make([]string, 0, 5)
	if excerptCount != "" && candidateCount != "" {
		parts = append(parts, fmt.Sprintf("会话 recall 最终保留了 %s/%s 段候选", excerptCount, candidateCount))
	} else if excerptCount != "" {
		parts = append(parts, fmt.Sprintf("会话 recall 最终保留了 %s 段内容", excerptCount))
	}
	if topScore != "" {
		parts = append(parts, "最高分片分数为 "+topScore)
	}
	if cacheLayer != "" || embeddingCacheLayer != "" {
		cacheParts := make([]string, 0, 2)
		if cacheLayer != "" {
			cacheParts = append(cacheParts, "session="+cacheLayer)
		}
		if embeddingCacheLayer != "" {
			cacheParts = append(cacheParts, "embedding="+embeddingCacheLayer)
		}
		parts = append(parts, "缓存层命中情况为 "+strings.Join(cacheParts, "，"))
	}
	if truncatedBy != "" {
		parts = append(parts, "未能保留更多 excerpt 的直接原因是 "+truncatedBy)
	}
	if skippedPerMessageLimit != "" && skippedPerMessageLimit != "0" {
		parts = append(parts, fmt.Sprintf("有 %s 段候选因为单消息上限被跳过", skippedPerMessageLimit))
	}
	if recomputeReason != "" {
		parts = append(parts, "重算原因是 "+recomputeReason)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "，") + "。"
}

func defaultTraceCountText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}
	return value
}
