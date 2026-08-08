package evaluation

func buildRewriteChannelAggregateMetrics(results []SampleResult, ks []int) []ChannelAggregateMetrics {
	if len(results) == 0 {
		return nil
	}
	return aggregateChannelMetrics(results, ks)
}

// BuildRewriteRetrievalChannelReport aggregates per-channel retrieval metrics for
// rewrite eval baseline and candidate sample results.
func BuildRewriteRetrievalChannelReport(baseline, candidate []SampleResult, ks []int) map[string]any {
	baselineChannels := buildRewriteChannelAggregateMetrics(baseline, ks)
	candidateChannels := buildRewriteChannelAggregateMetrics(candidate, ks)
	if len(baselineChannels) == 0 && len(candidateChannels) == 0 {
		return nil
	}
	return map[string]any{
		"baseline_channels":       baselineChannels,
		"candidate_channels":      candidateChannels,
		"channel_mrr_uplift":      channelMRRUplift(baselineChannels, candidateChannels),
		"channel_hit_at_k_uplift": channelHitAtKUplift(baselineChannels, candidateChannels, ks),
	}
}

func channelMetricsByName(channels []ChannelAggregateMetrics) map[string]ChannelAggregateMetrics {
	if len(channels) == 0 {
		return nil
	}
	byName := make(map[string]ChannelAggregateMetrics, len(channels))
	for _, channel := range channels {
		byName[channel.ChannelName] = channel
	}
	return byName
}

func channelMRRUplift(baseline, candidate []ChannelAggregateMetrics) map[string]float64 {
	baselineByName := channelMetricsByName(baseline)
	candidateByName := channelMetricsByName(candidate)
	if len(baselineByName) == 0 && len(candidateByName) == 0 {
		return nil
	}

	names := make(map[string]struct{}, len(baselineByName)+len(candidateByName))
	for name := range baselineByName {
		names[name] = struct{}{}
	}
	for name := range candidateByName {
		names[name] = struct{}{}
	}

	uplift := make(map[string]float64, len(names))
	for name := range names {
		uplift[name] = roundSummaryScore(candidateByName[name].MRR - baselineByName[name].MRR)
	}
	return uplift
}

func channelHitAtKUplift(baseline, candidate []ChannelAggregateMetrics, ks []int) map[string]map[int]float64 {
	baselineByName := channelMetricsByName(baseline)
	candidateByName := channelMetricsByName(candidate)
	if len(baselineByName) == 0 && len(candidateByName) == 0 {
		return nil
	}

	names := make(map[string]struct{}, len(baselineByName)+len(candidateByName))
	for name := range baselineByName {
		names[name] = struct{}{}
	}
	for name := range candidateByName {
		names[name] = struct{}{}
	}

	uplift := make(map[string]map[int]float64, len(names))
	for name := range names {
		perK := make(map[int]float64, len(ks))
		for _, k := range ks {
			perK[k] = roundSummaryScore(candidateByName[name].HitRateAtK[k] - baselineByName[name].HitRateAtK[k])
		}
		uplift[name] = perK
	}
	return uplift
}
