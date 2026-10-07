package vision

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/framework/config"
)

func TestSiliconFlowVisionLive(t *testing.T) {
	if os.Getenv("VISION_LIVE") != "1" {
		t.Skip("set VISION_LIVE=1")
	}
	if err := config.LoadConfig(filepath.Join("..", "..", "..", "..", "configs")); err != nil {
		t.Fatal(err)
	}
	key := config.Get().Parser.Vision.APIKey
	if key == "" {
		key = config.Get().AI.Providers["siliconflow"].ApiKey
	}
	if key == "" {
		t.Fatal("SiliconFlow API key is missing")
	}
	for _, name := range []string{"text.png", "blank.png"} {
		t.Run(name, func(t *testing.T) {
			image, err := os.ReadFile(filepath.Join("..", "parser", "test", "fixtures", "docreader", name))
			if err != nil {
				t.Fatal(err)
			}
			client := NewClient(config.Get().Parser.Vision.URL, key, 90*time.Second, 512)
			result, err := client.Describe(context.Background(), image, "image/png")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "described" && result.Status != "no_content" {
				t.Fatalf("unexpected status %q", result.Status)
			}
			t.Logf("status=%s chars=%d input_tokens=%d output_tokens=%d", result.Status, len([]rune(result.Text)), result.InputTokens, result.OutputTokens)
		})
	}
}

func TestSiliconFlowExtractedPDFImageLive(t *testing.T) {
	if os.Getenv("VISION_LIVE") != "1" || os.Getenv("DOCREADER_LIVE") != "1" {
		t.Skip("set VISION_LIVE=1 and DOCREADER_LIVE=1")
	}
	if err := config.LoadConfig(filepath.Join("..", "..", "..", "..", "configs")); err != nil {
		t.Fatal(err)
	}
	key := config.Get().Parser.Vision.APIKey
	if key == "" {
		key = config.Get().AI.Providers["siliconflow"].ApiKey
	}
	if key == "" {
		t.Fatal("SiliconFlow API key is missing")
	}
	content, err := os.ReadFile(filepath.Join("..", "parser", "test", "fixtures", "docreader", "mixed.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	selected := parser.NewDefaultSelector(nil).SelectFor("application/pdf", "mixed.pdf")
	structured, ok := selected.(interface {
		ParseStructured(context.Context, []byte, string, map[string]any) (parser.ParseResult, error)
	})
	if !ok {
		t.Fatal("structured DocReader is unavailable")
	}
	parsed, err := structured.ParseStructured(context.Background(), content, "application/pdf", map[string]any{"file_name": "mixed.pdf"})
	if err != nil || len(parsed.Images) == 0 {
		t.Fatalf("mixed PDF image extraction failed: count=%d err=%v", len(parsed.Images), err)
	}
	image := parsed.Images[0]
	result, err := NewClient(config.Get().Parser.Vision.URL, key, 90*time.Second, 512).
		Describe(context.Background(), image.Data, image.MIMEType)
	if err != nil || result.Status != "described" || result.InputTokens == 0 {
		t.Fatalf("extracted PDF image description failed: status=%s tokens=%d err=%v",
			result.Status, result.InputTokens, err)
	}
	t.Logf("extracted PDF image: mime=%s chars=%d input_tokens=%d output_tokens=%d",
		image.MIMEType, len([]rune(result.Text)), result.InputTokens, result.OutputTokens)
}
