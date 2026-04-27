package parser_test

import (
	"strings"
	"testing"

	parser "local/rag-project/internal/app/core/parser"
)

func TestMarkdownDocumentParserParse(t *testing.T) {
	docParser := parser.NewMarkdownDocumentParser()

	result, err := docParser.Parse([]byte("# title\r\n\n- item"), "text/markdown", nil)
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	if result.Text != "# title\n\n- item" {
		t.Fatalf("unexpected text: %q", result.Text)
	}
	if result.Metadata["format"] != "markdown" {
		t.Fatalf("unexpected metadata: %#v", result.Metadata)
	}
}

func TestMarkdownDocumentParserExtractText(t *testing.T) {
	docParser := parser.NewMarkdownDocumentParser()

	text, err := docParser.ExtractText(strings.NewReader("## subtitle\r\nbody"), "README.md")
	if err != nil {
		t.Fatalf("extract text returned error: %v", err)
	}
	if text != "## subtitle\nbody" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestMarkdownDocumentParserSupportsMimeTypes(t *testing.T) {
	docParser := parser.NewMarkdownDocumentParser()

	if !docParser.Supports("text/markdown") {
		t.Fatal("expected text/markdown to be supported")
	}
	if !docParser.Supports("text/x-markdown") {
		t.Fatal("expected text/x-markdown to be supported")
	}
	if docParser.Supports("text/plain") {
		t.Fatal("did not expect text/plain to be supported")
	}
}
