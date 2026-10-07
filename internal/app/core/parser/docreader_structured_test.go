package parser

import (
	"fmt"
	"strings"
	"testing"

	docreaderpb "local/rag-project/internal/app/core/parser/docreaderpb"
)

func TestStructuredImagesKeepAllOccurrences(t *testing.T) {
	var markdown strings.Builder
	frames := make(map[string]*docreaderpb.ImageRef)
	for i := 0; i < 101; i++ {
		ref := fmt.Sprintf("image-%d", i)
		fmt.Fprintf(&markdown, "![page](%s)\n", ref)
		frames[ref] = &docreaderpb.ImageRef{OriginalRef: ref, MimeType: "image/png", ImageData: []byte{1}}
	}
	result := buildStructuredResult(markdown.String(), frames, "application/pdf", nil)
	if !result.ImageInventoryConfirmed || len(result.Images) != 101 {
		t.Fatalf("expected 101 confirmed occurrences, got %d confirmed=%v", len(result.Images), result.ImageInventoryConfirmed)
	}
	if result.Images[0].Index != 0 || result.Images[100].Index != 100 {
		t.Fatalf("unexpected occurrence ordering: first=%d last=%d", result.Images[0].Index, result.Images[100].Index)
	}
}

func TestStructuredImagesPreserveDuplicateAndMissingReferences(t *testing.T) {
	frames := map[string]*docreaderpb.ImageRef{
		"shared": {OriginalRef: "shared", MimeType: "image/png", ImageData: []byte{1}},
	}
	result := buildStructuredResult("![a](shared) ![b](shared) ![c](missing)", frames, "application/pdf", nil)
	if len(result.Images) != 3 || result.Images[0].OriginalRef != "shared" || result.Images[1].OriginalRef != "shared" {
		t.Fatalf("duplicate occurrence was lost: %+v", result.Images)
	}
	if result.Images[2].Error == "" {
		t.Fatal("missing frame was not recorded")
	}
}

func TestStructuredImageKeepsAdjacentBodySeparate(t *testing.T) {
	frames := map[string]*docreaderpb.ImageRef{
		"chart": {OriginalRef: "chart", MimeType: "image/png", ImageData: []byte{1}},
	}
	result := buildStructuredResult("Revenue rose. ![chart](chart) Cost fell.", frames, "application/pdf", nil)
	if len(result.Images) != 1 || !strings.Contains(result.Images[0].AdjacentText, "Revenue rose") ||
		!strings.Contains(result.Images[0].AdjacentText, "Cost fell") {
		t.Fatalf("adjacent body was not kept with the image: %+v", result.Images)
	}
	if !strings.Contains(result.Text, "Revenue rose") || !strings.Contains(result.Text, "Cost fell") {
		t.Fatalf("body text was lost: %q", result.Text)
	}
}
