package retrieve

import (
	"context"
	"testing"

	"local/rag-project/internal/framework/convention"
)

type stubWikiRetriever struct {
	chunks []convention.RetrievedChunk
	err    error
}

func (s *stubWikiRetriever) SearchWiki(ctx context.Context, request WikiSearchRequest) ([]convention.RetrievedChunk, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.chunks, nil
}

func TestWikiPageChannelEnabledModes(t *testing.T) {
	channel := NewWikiPageChannel(&stubWikiRetriever{})
	cases := []struct {
		mode string
		want bool
	}{{SearchModeAuto, true}, {SearchModeKeyword, true}, {SearchModeHybrid, true}, {SearchModeSemantic, false}}
	for _, tc := range cases {
		if got := channel.Enabled(SearchContext{SearchMode: tc.mode}); got != tc.want {
			t.Fatalf("mode %s enabled = %v, want %v", tc.mode, got, tc.want)
		}
	}
	if NewWikiPageChannel(nil).Enabled(SearchContext{SearchMode: SearchModeHybrid}) {
		t.Fatal("nil retriever should disable channel")
	}
}

func TestWikiPageChannelSearch(t *testing.T) {
	channel := NewWikiPageChannel(&stubWikiRetriever{chunks: []convention.RetrievedChunk{{ID: "p1", Text: "x"}}})
	result, err := channel.Search(context.Background(), SearchContext{Query: "q", KnowledgeBaseIDs: []string{"kb1"}, TopK: 5, SearchMode: SearchModeHybrid})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "p1" {
		t.Fatalf("chunks = %#v", result.Chunks)
	}
	if result.Metadata["expandedTopK"] != 10 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}
