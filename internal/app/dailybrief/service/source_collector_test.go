package service

import (
	"context"
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestSourceCollectorCollectsFromConfiguredSources(t *testing.T) {
	hnSpec, _ := domain.SourceFeedSpecByKey(domain.SourceKeyHackerNews)
	openAISpec, _ := domain.SourceFeedSpecByKey(domain.SourceKeyOpenAIBlog)
	hnSource, err := NewFeedSource(hnSpec)
	if err != nil {
		t.Fatalf("NewFeedSource hacker news: %v", err)
	}
	openAISource, err := NewFeedSource(openAISpec)
	if err != nil {
		t.Fatalf("NewFeedSource openai blog: %v", err)
	}

	registry := NewSourceRegistry(hnSource, openAISource)
	client := &staticHTTPClient{
		bodies: map[string][]byte{
			hnSpec.URL:    loadSourceFixture(t, "hacker-news.rss.xml"),
			openAISpec.URL: loadSourceFixture(t, "openai-blog.rss.xml"),
		},
	}

	collector := NewSourceCollector(registry, client)
	result := collector.Collect(context.Background(), []string{domain.SourceKeyOpenAIBlog, domain.SourceKeyHackerNews})

	if len(result.Failures) != 0 {
		t.Fatalf("expected no collection failures, got %#v", result.Failures)
	}
	if len(result.Candidates) != 4 {
		t.Fatalf("expected 4 collected candidates, got %d", len(result.Candidates))
	}
}

func TestSourceCollectorReportsUnknownSource(t *testing.T) {
	collector := NewSourceCollector(NewSourceRegistry(), &staticHTTPClient{})
	result := collector.Collect(context.Background(), []string{"unknown-source"})
	if len(result.Failures) != 1 {
		t.Fatalf("expected one failure for unknown source, got %#v", result.Failures)
	}
	if _, ok := result.Failures["unknown-source"]; !ok {
		t.Fatalf("expected unknown source failure key, got %#v", result.Failures)
	}
}
