package scheduledtask

import "testing"

func TestReportSourcesPreservesReferencesAndDeduplicates(t *testing.T) {
	sources := reportSources([]string{" https://example.org/official ", "https://example.org/official", "KB chunk 42", "javascript:alert(1)", ""})
	if len(sources) != 3 || sources[0].Type != "web" || sources[0].URL != "https://example.org/official" || sources[1].Title != "KB chunk 42" || sources[2].URL != "" {
		t.Fatalf("sources = %+v", sources)
	}
	if reportSources(nil) == nil {
		t.Fatal("empty sources should serialize as []")
	}
}

func TestConversationTitleUsesBoundedUnicodePrompt(t *testing.T) {
	title := conversationTitle("  比赛延期\n官方公告  ")
	if title != "定时任务 · 比赛延期 官方公告" {
		t.Fatalf("title = %q", title)
	}
}
