package parser_test

import (
	"testing"
	"time"

	parser "local/rag-project/internal/app/core/parser"
)

func TestSelectorSelectForPrefersFileName(t *testing.T) {
	selector := parser.NewSelector(
		parser.NewMarkdownDocumentParser(),
		parser.NewTikaDocumentParser(nil, "http://localhost:9998/tika"),
	)

	selected := selector.SelectFor("text/plain", "README.md")
	if selected == nil {
		t.Fatal("expected parser")
	}
	if selected.ParserType() != parser.ParserTypeMarkdown {
		t.Fatalf("expected markdown parser, got %s", selected.ParserType())
	}
}

func TestSelectorSelectForUsesDocReaderForComplexFiles(t *testing.T) {
	docReader, err := parser.NewDocReaderDocumentParser("127.0.0.1:50051", time.Second, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	selector := parser.NewSelector(parser.NewMarkdownDocumentParser(), docReader)
	selected := selector.SelectFor("application/octet-stream", "report.pdf")
	if selected == nil || selected.ParserType() != parser.ParserTypeDocReader {
		t.Fatalf("expected DocReader for PDF, got %v", selected)
	}
	if selected = selector.SelectFor("application/octet-stream", "unknown.bin"); selected != nil {
		t.Fatalf("expected unsupported format to have no parser, got %s", selected.ParserType())
	}
}

func TestSelectorSelectForFallsBackToTika(t *testing.T) {
	selector := parser.NewSelector(
		parser.NewMarkdownDocumentParser(),
		parser.NewTikaDocumentParser(nil, "http://localhost:9998/tika"),
	)

	selected := selector.SelectFor("application/octet-stream", "notes.unknown")
	if selected == nil {
		t.Fatal("expected parser")
	}
	if selected.ParserType() != parser.ParserTypeTika {
		t.Fatalf("expected tika fallback parser, got %s", selected.ParserType())
	}
}

func TestSelectorAvailableTypes(t *testing.T) {
	selector := parser.NewSelector(
		parser.NewMarkdownDocumentParser(),
		parser.NewTikaDocumentParser(nil, "http://localhost:9998/tika"),
	)

	types := selector.AvailableTypes()
	if len(types) != 2 {
		t.Fatalf("expected 2 parser types, got %d", len(types))
	}
	if types[0] != parser.ParserTypeMarkdown || types[1] != parser.ParserTypeTika {
		t.Fatalf("unexpected parser types: %#v", types)
	}
}
