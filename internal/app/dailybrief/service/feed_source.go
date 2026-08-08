package service

import (
	"context"
	"fmt"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type feedSource struct {
	spec domain.SourceFeedSpec
}

func NewFeedSource(spec domain.SourceFeedSpec) (port.SourceProvider, error) {
	if !domain.IsSourceKeySupported(spec.Key) {
		return nil, fmt.Errorf("unsupported source key %q", spec.Key)
	}
	if !domain.IsTopicKeySupported(spec.Topic) {
		return nil, fmt.Errorf("unsupported topic %q for source %q", spec.Topic, spec.Key)
	}
	switch spec.Format {
	case domain.SourceFeedFormatRSS, domain.SourceFeedFormatAtom, domain.SourceFeedFormatHTML, domain.SourceFeedFormatStub:
	default:
		return nil, fmt.Errorf("unsupported feed format %q for source %q", spec.Format, spec.Key)
	}
	if spec.Format != domain.SourceFeedFormatStub && spec.URL == "" {
		return nil, fmt.Errorf("source %q requires a feed url", spec.Key)
	}
	return &feedSource{spec: spec}, nil
}

func (s *feedSource) SourceKey() string {
	return s.spec.Key
}

func (s *feedSource) Fetch(ctx context.Context, client port.HTTPClient) ([]domain.Candidate, error) {
	switch s.spec.Format {
	case domain.SourceFeedFormatStub:
		return []domain.Candidate{}, nil
	case domain.SourceFeedFormatRSS:
		body, err := client.Get(ctx, s.spec.URL)
		if err != nil {
			return nil, fmt.Errorf("fetch rss feed %q: %w", s.spec.Key, err)
		}
		candidates, err := parseRSSCandidates(body, s.spec.Key, s.spec.Topic)
		if err != nil {
			return nil, fmt.Errorf("parse rss feed %q: %w", s.spec.Key, err)
		}
		return candidates, nil
	case domain.SourceFeedFormatAtom:
		body, err := client.Get(ctx, s.spec.URL)
		if err != nil {
			return nil, fmt.Errorf("fetch atom feed %q: %w", s.spec.Key, err)
		}
		candidates, err := parseAtomCandidates(body, s.spec.Key, s.spec.Topic)
		if err != nil {
			return nil, fmt.Errorf("parse atom feed %q: %w", s.spec.Key, err)
		}
		return candidates, nil
	case domain.SourceFeedFormatHTML:
		body, err := client.Get(ctx, s.spec.URL)
		if err != nil {
			return nil, fmt.Errorf("fetch html feed %q: %w", s.spec.Key, err)
		}
		candidates, err := parseHTMLCandidates(body, s.spec)
		if err != nil {
			return nil, fmt.Errorf("parse html feed %q: %w", s.spec.Key, err)
		}
		return candidates, nil
	default:
		return nil, fmt.Errorf("unsupported feed format %q", s.spec.Format)
	}
}
