package service

import (
	"strings"

	"local/rag-project/internal/app/dailybrief/domain"
)

// LimitCandidatesPerTopic selects ranked candidates with round-robin fairness across
// subscribed topics. Each topic receives at most perTopicLimit items; globalMax caps
// the total. When perTopicLimit <= 0, only globalMax is applied (slice head).
func LimitCandidatesPerTopic(
	ranked []domain.Candidate,
	selectedTopics []string,
	perTopicLimit int,
	globalMax int,
) []domain.Candidate {
	if len(ranked) == 0 {
		return nil
	}
	if globalMax <= 0 && perTopicLimit <= 0 {
		return ranked
	}
	if perTopicLimit <= 0 {
		if globalMax > 0 && len(ranked) > globalMax {
			return ranked[:globalMax]
		}
		return ranked
	}

	buckets := make(map[string][]domain.Candidate)
	for _, candidate := range ranked {
		topic := strings.TrimSpace(candidate.Topic)
		if topic == "" {
			continue
		}
		buckets[topic] = append(buckets[topic], candidate)
	}

	topicOrder := normalizeTopicOrder(selectedTopics, buckets)
	if len(topicOrder) == 0 {
		if globalMax > 0 && len(ranked) > globalMax {
			return ranked[:globalMax]
		}
		return ranked
	}

	picked := make([]domain.Candidate, 0, minInt(len(ranked), globalMax))
	topicIndex := make(map[string]int, len(topicOrder))
	topicCount := make(map[string]int, len(topicOrder))

	for {
		if globalMax > 0 && len(picked) >= globalMax {
			break
		}
		added := false
		for _, topic := range topicOrder {
			if globalMax > 0 && len(picked) >= globalMax {
				break
			}
			if topicCount[topic] >= perTopicLimit {
				continue
			}
			bucket := buckets[topic]
			index := topicIndex[topic]
			if index >= len(bucket) {
				continue
			}
			picked = append(picked, bucket[index])
			topicIndex[topic] = index + 1
			topicCount[topic]++
			added = true
		}
		if !added {
			break
		}
	}
	return picked
}

func normalizeTopicOrder(selectedTopics []string, buckets map[string][]domain.Candidate) []string {
	seen := make(map[string]struct{}, len(selectedTopics))
	order := make([]string, 0, len(selectedTopics))
	for _, topic := range selectedTopics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		if _, ok := seen[topic]; ok {
			continue
		}
		if len(buckets[topic]) == 0 {
			continue
		}
		seen[topic] = struct{}{}
		order = append(order, topic)
	}
	for topic := range buckets {
		if _, ok := seen[topic]; ok {
			continue
		}
		order = append(order, topic)
	}
	return order
}

func EffectiveMaxItems(topicCount int, globalMax int, perTopicLimit int) int {
	if perTopicLimit > 0 && topicCount > 0 {
		perTopicTotal := topicCount * perTopicLimit
		if globalMax <= 0 {
			return perTopicTotal
		}
		if perTopicTotal < globalMax {
			return perTopicTotal
		}
	}
	if globalMax <= 0 {
		return 5
	}
	return globalMax
}
