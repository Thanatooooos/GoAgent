package evaluation

import (
	"sort"
	"strings"
)

type ChannelSampleResult struct {
	ChannelName          string          `json:"channelName"`
	HitAtK               map[int]bool    `json:"hitAtK"`
	FirstRelevantRank    int             `json:"firstRelevantRank,omitempty"`
	UniqueHitCount       int             `json:"uniqueHitCount"`
	OverlapHitCount      int             `json:"overlapHitCount"`
	RetrievedCount       int             `json:"retrievedCount"`
}

type ChannelAggregateMetrics struct {
	ChannelName              string             `json:"channelName"`
	SampleCount              int                `json:"sampleCount"`
	HitRateAtK               map[int]float64    `json:"hitRateAtK"`
	AverageFirstRelevantRank float64            `json:"averageFirstRelevantRank,omitempty"`
	UniqueHitCount           int                `json:"uniqueHitCount"`
	OverlapHitCount          int                `json:"overlapHitCount"`
}

func evaluateChannelSample(sample Sample, ks []int) ([]ChannelSampleResult, error) {
	if len(sample.ChannelRetrieved) == 0 {
		return nil, nil
	}

	target := normalizeTarget(sample.Target)
	if target == "" {
		return nil, nil
	}

	expectedSet := map[string]struct{}{}
	for _, id := range sample.ExpectedIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			expectedSet[id] = struct{}{}
		}
	}
	if len(expectedSet) == 0 {
		return nil, nil
	}

	maxK := ks[len(ks)-1]
	channelHits := make(map[string]map[string]struct{}, len(sample.ChannelRetrieved))
	channelResults := make([]ChannelSampleResult, 0, len(sample.ChannelRetrieved))

	channelNames := make([]string, 0, len(sample.ChannelRetrieved))
	for name := range sample.ChannelRetrieved {
		channelNames = append(channelNames, name)
	}
	sort.Strings(channelNames)

	for _, channelName := range channelNames {
		items := sample.ChannelRetrieved[channelName]
		rankedIDs := rankedTargetIDs(items, target, maxK)
		hits := map[string]struct{}{}
		firstRelevantRank := 0
		for index, id := range rankedIDs {
			if _, ok := expectedSet[id]; !ok {
				continue
			}
			hits[id] = struct{}{}
			if firstRelevantRank == 0 {
				firstRelevantRank = index + 1
			}
		}
		channelHits[channelName] = hits

		hitAtK := make(map[int]bool, len(ks))
		for _, k := range ks {
			topIDs := rankedTargetIDs(items, target, k)
			hitAtK[k] = intersectsExpected(topIDs, expectedSet)
		}

		channelResults = append(channelResults, ChannelSampleResult{
			ChannelName:       channelName,
			HitAtK:            hitAtK,
			FirstRelevantRank: firstRelevantRank,
			RetrievedCount:    len(items),
		})
	}

	expectedChannels := make(map[string][]string, len(expectedSet))
	for expectedID := range expectedSet {
		channels := make([]string, 0, len(channelHits))
		for channelName, hits := range channelHits {
			if _, ok := hits[expectedID]; ok {
				channels = append(channels, channelName)
			}
		}
		sort.Strings(channels)
		expectedChannels[expectedID] = channels
	}

	for index := range channelResults {
		channelName := channelResults[index].ChannelName
		for _, channels := range expectedChannels {
			if !containsString(channels, channelName) {
				continue
			}
			switch len(channels) {
			case 1:
				channelResults[index].UniqueHitCount++
			default:
				channelResults[index].OverlapHitCount++
			}
		}
	}

	return channelResults, nil
}

func aggregateChannelMetrics(results []SampleResult, ks []int) []ChannelAggregateMetrics {
	channelSamples := map[string][]ChannelSampleResult{}
	for _, sample := range results {
		for _, channel := range sample.Channels {
			channelSamples[channel.ChannelName] = append(channelSamples[channel.ChannelName], channel)
		}
	}
	if len(channelSamples) == 0 {
		return nil
	}

	names := make([]string, 0, len(channelSamples))
	for name := range channelSamples {
		names = append(names, name)
	}
	sort.Strings(names)

	aggregates := make([]ChannelAggregateMetrics, 0, len(names))
	for _, name := range names {
		samples := channelSamples[name]
		metrics := ChannelAggregateMetrics{
			ChannelName: name,
			SampleCount: len(samples),
			HitRateAtK:  make(map[int]float64, len(ks)),
		}
		var firstRankTotal float64
		var firstRankCount int
		for _, sample := range samples {
			for _, k := range ks {
				if sample.HitAtK[k] {
					metrics.HitRateAtK[k]++
				}
			}
			if sample.FirstRelevantRank > 0 {
				firstRankTotal += float64(sample.FirstRelevantRank)
				firstRankCount++
			}
			metrics.UniqueHitCount += sample.UniqueHitCount
			metrics.OverlapHitCount += sample.OverlapHitCount
		}
		total := float64(len(samples))
		for _, k := range ks {
			metrics.HitRateAtK[k] /= total
		}
		if firstRankCount > 0 {
			metrics.AverageFirstRelevantRank = firstRankTotal / float64(firstRankCount)
		}
		aggregates = append(aggregates, metrics)
	}
	return aggregates
}

func rankedTargetIDs(items []RetrievedItem, target Target, limit int) []string {
	if limit <= 0 {
		limit = len(items)
	}
	ranked := make([]string, 0, min(limit, len(items)))
	for _, item := range items {
		id := extractTargetID(item, target)
		if strings.TrimSpace(id) == "" {
			continue
		}
		ranked = append(ranked, strings.TrimSpace(id))
		if len(ranked) >= limit {
			break
		}
	}
	return ranked
}

func intersectsExpected(ids []string, expected map[string]struct{}) bool {
	for _, id := range ids {
		if _, ok := expected[id]; ok {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
