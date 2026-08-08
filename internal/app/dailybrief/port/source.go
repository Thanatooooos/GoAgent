package port

import (
	"context"

	"local/rag-project/internal/app/dailybrief/domain"
)

// HTTPClient is the minimal interface source providers need.
type HTTPClient interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// SourceProvider fetches and parses one curated source.
type SourceProvider interface {
	SourceKey() string
	Fetch(ctx context.Context, client HTTPClient) ([]domain.Candidate, error)
}
