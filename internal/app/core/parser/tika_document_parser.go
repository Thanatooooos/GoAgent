package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const defaultTikaTimeout = 30 * time.Second

var externalHTMLImage = regexp.MustCompile(`(?is)<img\b[^>]*\bsrc\s*=\s*["'](https?://[^"']+)["']`)

type TikaDocumentParser struct {
	httpClient *http.Client
	serviceURL string
}

func NewTikaDocumentParser(httpClient *http.Client, serviceURL string) *TikaDocumentParser {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTikaTimeout}
	}
	return &TikaDocumentParser{
		httpClient: httpClient,
		serviceURL: strings.TrimSpace(serviceURL),
	}
}

func (p *TikaDocumentParser) ParserType() string {
	return ParserTypeTika
}

func (p *TikaDocumentParser) Parse(ctx context.Context, content []byte, mimeType string, options map[string]any) (ParseResult, error) {
	text, err := p.doParse(ctx, bytes.NewReader(content), mimeType, fileNameFromOptions(options))
	if err != nil {
		return ParseResult{}, err
	}
	result := Of(text, map[string]any{
		"mime_type":   mimeType,
		"parser_type": p.ParserType(),
	})
	if strings.Contains(strings.ToLower(mimeType), "html") {
		for _, match := range externalHTMLImage.FindAllSubmatch(content, -1) {
			result.Images = append(result.Images, ImageOccurrence{Index: len(result.Images), OriginalRef: string(match[1]), Error: "external image is not downloaded"})
		}
	}
	return result, nil
}

func (p *TikaDocumentParser) ExtractText(stream io.Reader, fileName string) (string, error) {
	return p.doParse(context.Background(), stream, "", fileName)
}

func (p *TikaDocumentParser) Supports(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return mimeType == "" || !strings.Contains(mimeType, "markdown")
}

func (p *TikaDocumentParser) doParse(ctx context.Context, stream io.Reader, mimeType string, fileName string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("tika parser is nil")
	}
	if strings.TrimSpace(p.serviceURL) == "" {
		return "", fmt.Errorf("tika service url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.serviceURL, stream)
	if err != nil {
		return "", fmt.Errorf("build tika request: %w", err)
	}
	req.Header.Set("Accept", "text/plain")
	if strings.TrimSpace(mimeType) != "" {
		req.Header.Set("Content-Type", mimeType)
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if strings.TrimSpace(fileName) != "" {
		req.Header.Set("X-Tika-Filename", fileName)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call tika service: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read tika response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("tika service returned status %d: %s", resp.StatusCode, string(body))
	}
	return normalizeTextContent(body), nil
}

func fileNameFromOptions(options map[string]any) string {
	if options == nil {
		return ""
	}
	if value, ok := options["file_name"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
