package chunk

import (
	"fmt"
	"strings"
)

type Selector struct {
	chunkers map[Strategy]Chunker
}

func NewSelector(chunkers ...Chunker) *Selector {
	normalized := make(map[Strategy]Chunker, len(chunkers))
	for _, each := range chunkers {
		if each == nil {
			continue
		}
		strategy := each.Strategy()
		if strategy == "" {
			continue
		}
		if _, exists := normalized[strategy]; exists {
			continue
		}
		normalized[strategy] = each
	}
	return &Selector{chunkers: normalized}
}

func NewDefaultSelector() *Selector {
	return NewSelector(
		NewFixedSizeChunker(),
		NewMarkdownChunker(),
	)
}

func (s *Selector) Select(strategy Strategy) (Chunker, bool) {
	if s == nil {
		return nil, false
	}
	chunker, ok := s.chunkers[normalizeStrategy(strategy)]
	return chunker, ok
}

func (s *Selector) Chunk(text string, opts Options) ([]Chunk, error) {
	if s == nil {
		return nil, ErrSelectorNil
	}
	opts = opts.Normalize()
	chunker, ok := s.Select(opts.Strategy)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrChunkerNotFound, opts.Strategy)
	}
	return chunker.Chunk(text, opts)
}

func (s *Selector) AvailableStrategies() []Strategy {
	if s == nil {
		return nil
	}
	result := make([]Strategy, 0, len(s.chunkers))
	for strategy := range s.chunkers {
		result = append(result, strategy)
	}
	return result
}

func normalizeStrategy(strategy Strategy) Strategy {
	switch Strategy(strings.TrimSpace(strings.ToLower(string(strategy)))) {
	case "structure_aware":
		return StrategyMarkdown
	default:
		return Strategy(strings.TrimSpace(strings.ToLower(string(strategy))))
	}
}
