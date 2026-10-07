package parser_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	parser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/framework/config"
)

func TestDocReaderLiveFormats(t *testing.T) {
	if os.Getenv("DOCREADER_LIVE") != "1" {
		t.Skip("set DOCREADER_LIVE=1 to run against a live DocReader")
	}
	fixtureDir := os.Getenv("DOCREADER_FIXTURE_DIR")
	if fixtureDir == "" {
		fixtureDir = filepath.Join("fixtures", "docreader")
	}
	if err := config.LoadConfig(filepath.Join("..", "..", "..", "..", "..", "configs")); err != nil {
		t.Fatal(err)
	}
	selector := parser.NewDefaultSelector(nil)
	for _, tc := range []struct {
		name string
		mime string
		want string
	}{
		{"report.pdf", "application/pdf", "Quarterly Report"},
		{"report.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "three milestones"},
		{"metrics.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "Documents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(fixtureDir, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			selected := selector.SelectFor(tc.mime, tc.name)
			if selected == nil || selected.ParserType() != parser.ParserTypeDocReader {
				t.Fatalf("expected DocReader, got %v", selected)
			}
			result, err := selected.Parse(context.Background(), content, tc.mime, map[string]any{"file_name": tc.name})
			if err != nil {
				t.Fatal(err)
			}
			if result.Metadata["parser_type"] != parser.ParserTypeDocReader {
				t.Fatalf("unexpected parser metadata: %#v", result.Metadata)
			}
			if !strings.Contains(result.Text, tc.want) {
				t.Fatalf("missing %q in parsed content: %q", tc.want, result.Text)
			}
			t.Logf("parsed %d bytes into %d characters", len(content), len(result.Text))
		})
	}
	t.Run("page.html via Tika", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(fixtureDir, "page.html"))
		if err != nil {
			t.Fatal(err)
		}
		selected := selector.SelectFor("text/html", "page.html")
		if selected == nil || selected.ParserType() != parser.ParserTypeTika {
			t.Fatalf("expected Tika for HTML, got %v", selected)
		}
		result, err := selected.Parse(context.Background(), content, "text/html", map[string]any{"file_name": "page.html"})
		if err != nil || !strings.Contains(result.Text, "Service Guide") {
			t.Fatalf("Tika HTML parse: result=%q err=%v", result.Text, err)
		}
	})
	t.Run("scanned.pdf OCR text", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(fixtureDir, "scanned.pdf"))
		if err != nil {
			t.Fatal(err)
		}
		selected := selector.SelectFor("application/pdf", "scanned.pdf")
		result, err := selected.Parse(context.Background(), content, "application/pdf", map[string]any{"file_name": "scanned.pdf"})
		if err != nil || !strings.Contains(result.Text, "SCANNED INVOICE 2026") {
			t.Fatalf("expected OCR text, got result=%q err=%v", result.Text, err)
		}
	})
	t.Run("blank.pdf no text", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(fixtureDir, "blank.pdf"))
		if err != nil {
			t.Fatal(err)
		}
		selected := selector.SelectFor("application/pdf", "blank.pdf")
		_, err = selected.Parse(context.Background(), content, "application/pdf", map[string]any{"file_name": "blank.pdf"})
		if err == nil || !strings.Contains(err.Error(), "no searchable text") {
			t.Fatalf("expected no searchable text, got %v", err)
		}
	})
	t.Run("mixed.pdf preserves page order", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(fixtureDir, "mixed.pdf"))
		if err != nil {
			t.Fatal(err)
		}
		selected := selector.SelectFor("application/pdf", "mixed.pdf")
		result, err := selected.Parse(context.Background(), content, "application/pdf", map[string]any{"file_name": "mixed.pdf"})
		if err != nil {
			t.Fatal(err)
		}
		first := strings.Index(result.Text, "Quarterly Report")
		second := strings.Index(result.Text, "SCANNED INVOICE 2026")
		if first < 0 || second <= first || result.Metadata["ocr_no_text_count"] != 1 {
			t.Fatalf("unexpected mixed PDF result: text=%q metadata=%#v", result.Text, result.Metadata)
		}
	})
	t.Run("text.png OCR text", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(fixtureDir, "text.png"))
		if err != nil {
			t.Fatal(err)
		}
		selected := selector.SelectFor("image/png", "text.png")
		if selected == nil || selected.ParserType() != parser.ParserTypeDocReader {
			t.Fatalf("expected DocReader for image, got %v", selected)
		}
		result, err := selected.Parse(context.Background(), content, "image/png", map[string]any{"file_name": "text.png"})
		if err != nil || !strings.Contains(result.Text, "IMAGE RECEIPT 42") {
			t.Fatalf("expected image OCR text, got result=%q err=%v", result.Text, err)
		}
	})
	t.Run("chinese.png OCR text", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(fixtureDir, "chinese.png"))
		if err != nil {
			t.Fatal(err)
		}
		selected := selector.SelectFor("image/png", "chinese.png")
		result, err := selected.Parse(context.Background(), content, "image/png", map[string]any{"file_name": "chinese.png"})
		if err != nil || !strings.Contains(result.Text, "发票金额四十二元") {
			t.Fatalf("expected Chinese OCR text, got result=%q err=%v", result.Text, err)
		}
	})
	t.Run("blank.png no text", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(fixtureDir, "blank.png"))
		if err != nil {
			t.Fatal(err)
		}
		selected := selector.SelectFor("image/png", "blank.png")
		_, err = selected.Parse(context.Background(), content, "image/png", map[string]any{"file_name": "blank.png"})
		if err == nil || !strings.Contains(err.Error(), "no searchable text") {
			t.Fatalf("expected no searchable text, got %v", err)
		}
	})
}

func TestDocReaderLiveStructuredImages(t *testing.T) {
	if os.Getenv("DOCREADER_LIVE") != "1" {
		t.Skip("set DOCREADER_LIVE=1")
	}
	if err := config.LoadConfig(filepath.Join("..", "..", "..", "..", "..", "configs")); err != nil {
		t.Fatal(err)
	}
	selector := parser.NewDefaultSelector(nil)
	for _, name := range []string{"mixed.pdf", "scanned.pdf", "blank.png"} {
		t.Run(name, func(t *testing.T) {
			mimeType := "application/pdf"
			if strings.HasSuffix(name, ".png") {
				mimeType = "image/png"
			}
			content, err := os.ReadFile(filepath.Join("fixtures", "docreader", name))
			if err != nil {
				t.Fatal(err)
			}
			selected := selector.SelectFor(mimeType, name)
			structured, ok := selected.(interface {
				ParseStructured(context.Context, []byte, string, map[string]any) (parser.ParseResult, error)
			})
			if !ok {
				t.Fatal("structured parser unavailable")
			}
			result, err := structured.ParseStructured(context.Background(), content, mimeType, map[string]any{"file_name": name})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Images) == 0 || !result.ImageInventoryConfirmed {
				t.Fatalf("missing image inventory: count=%d confirmed=%t", len(result.Images), result.ImageInventoryConfirmed)
			}
			for _, image := range result.Images {
				if image.Error != "" || len(image.Data) == 0 {
					t.Fatalf("unreadable image: ref=%q err=%q", image.OriginalRef, image.Error)
				}
				t.Logf("image ref=%q mime=%q bytes=%d", image.OriginalRef, image.MIMEType, len(image.Data))
			}
			if strings.Contains(result.Text, "SCANNED INVOICE 2026") {
				t.Fatalf("OCR text leaked into document body")
			}
		})
	}
}
