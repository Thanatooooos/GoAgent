package runtime

import (
	"testing"
	"time"
)

func TestContextSourcesPreserveOrderAndReplaceByKey(t *testing.T) {
	t.Parallel()
	sources := NewContextSources(
		ContextSource{Key: "core", Render: func(SourceContext) string { return "core" }},
		ContextSource{Key: "environment", Render: func(SourceContext) string { return "environment" }},
	)
	sources.Append(ContextSource{Key: "core", Render: func(SourceContext) string { return "override" }})
	blocks := sources.Blocks(SourceContext{})
	if len(blocks) != 2 || blocks[0] != "override" || blocks[1] != "environment" {
		t.Fatalf("blocks = %#v", blocks)
	}
}

func TestDateRendersCurrentCalendarDate(t *testing.T) {
	t.Parallel()
	blocks := Date().Render(SourceContext{})
	want := "Today's date: " + time.Now().Format("Mon Jan 2 2006")
	if blocks != want {
		t.Fatalf("date block = %q, want %q", blocks, want)
	}
}
