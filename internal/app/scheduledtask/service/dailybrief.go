package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	briefdomain "local/rag-project/internal/app/dailybrief/domain"
	briefservice "local/rag-project/internal/app/dailybrief/service"
	"local/rag-project/internal/app/scheduledtask/domain"
)

func dailyBriefInstructions(contract briefdomain.TaskContract) string {
	raw, _ := json.Marshal(contract)
	feeds := make([]briefdomain.SourceFeedSpec, 0, len(contract.Sources))
	for _, key := range contract.Sources {
		if spec, ok := briefdomain.SourceFeedSpecByKey(key); ok {
			feeds = append(feeds, spec)
		}
	}
	feedJSON, _ := json.Marshal(feeds)
	quota := fmt.Sprintf(dailyBriefTotalQuotaTemplate, briefservice.EffectiveMaxItems(len(contract.Topics), contract.MaxItems, contract.MaxItemsPerTopic))
	if contract.MaxItemsPerTopic > 0 {
		quota += fmt.Sprintf(dailyBriefTopicQuotaTemplate, contract.MaxItemsPerTopic)
	}
	quota += dailyBriefResearchInstruction
	return dailyBriefContractInstruction + string(raw) + dailyBriefSourceHintsPrefix + string(feedJSON) + quota
}

// NormalizeDailyBrief derives both user-visible views from the same artifact.
// Validation checks the page contract, not factual correctness/source coverage.
func NormalizeDailyBrief(outcome domain.Outcome, contract briefdomain.TaskContract) (domain.Outcome, briefdomain.BriefArtifact, error) {
	var artifact briefdomain.BriefArtifact
	if outcome.Signal != domain.SignalReport {
		return outcome, artifact, nil
	}
	d := json.NewDecoder(strings.NewReader(string(outcome.Artifact)))
	d.DisallowUnknownFields()
	if err := d.Decode(&artifact); err != nil {
		return outcome, artifact, fmt.Errorf("DailyBrief artifact: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return outcome, artifact, fmt.Errorf("DailyBrief artifact contains trailing data")
	}
	if err := briefservice.ValidateBriefArtifact(artifact, 0); err != nil {
		return outcome, artifact, err
	}
	if len([]rune(artifact.Headline)) > 512 {
		return outcome, artifact, fmt.Errorf("DailyBrief headline exceeds the page storage limit")
	}
	seenTopics := map[string]bool{}
	for _, section := range artifact.Sections {
		if !containsTool(contract.Topics, section.Key) || seenTopics[section.Key] || len(section.Items) == 0 {
			return outcome, artifact, fmt.Errorf("invalid DailyBrief section %q", section.Key)
		}
		seenTopics[section.Key] = true
		for _, item := range section.Items {
			if len([]rune(item.Title)) > 512 || len([]rune(item.Source)) > 64 || len([]rune(item.URL)) > 2048 {
				return outcome, artifact, fmt.Errorf("DailyBrief item exceeds the page storage limits")
			}
			u, err := url.Parse(item.URL)
			if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || item.Topic != section.Key {
				return outcome, artifact, fmt.Errorf("DailyBrief item has an invalid URL or topic")
			}
		}
	}
	// Validate the whole output before trimming, including discarded items.
	// Apply the same deterministic quota as the legacy generator, then derive
	// the persisted artifact, message and sources from the retained items.
	max := briefservice.EffectiveMaxItems(len(contract.Topics), contract.MaxItems, contract.MaxItemsPerTopic)
	artifact = briefservice.TruncateBriefArtifactPerTopic(artifact, contract.MaxItemsPerTopic, max)
	raw, err := json.Marshal(artifact)
	if err != nil {
		return outcome, artifact, err
	}
	outcome.Artifact = raw
	var body strings.Builder
	fmt.Fprintf(&body, "# %s\n\n%s\n", artifact.Headline, artifact.TopSummary)
	outcome.Sources = nil
	for _, section := range artifact.Sections {
		fmt.Fprintf(&body, "\n## %s\n", section.Title)
		for _, item := range section.Items {
			fmt.Fprintf(&body, "\n### %s\n\n%s\n\n%s\n\n来源：[%s](%s)\n", item.Title, item.Summary, item.WhyItMatters, item.Source, item.URL)
			outcome.Sources = append(outcome.Sources, item.URL)
		}
	}
	outcome.Body = strings.TrimSpace(body.String())
	return outcome, artifact, nil
}
