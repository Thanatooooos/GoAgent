package parser

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	docreaderpb "local/rag-project/internal/app/core/parser/docreaderpb"
)

const maxDocReaderMessage = 50 << 20
const maxOCRImages = 100

var imageMarkdownPattern = regexp.MustCompile(`!\[[^]]*\]\(([^\n)]*)\)`)

type DocReaderDocumentParser struct {
	client   docreaderpb.DocReaderClient
	timeout  time.Duration
	fallback DocumentParser
	ocr      OCRClient
}

func NewDocReaderDocumentParser(address string, timeout time.Duration, fallback DocumentParser, ocr OCRClient) (*DocReaderDocumentParser, error) {
	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("docreader address is empty")
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(maxDocReaderMessage), grpc.MaxCallRecvMsgSize(maxDocReaderMessage)),
	)
	if err != nil {
		return nil, fmt.Errorf("create docreader client: %w", err)
	}
	return &DocReaderDocumentParser{client: docreaderpb.NewDocReaderClient(conn), timeout: timeout, fallback: fallback, ocr: ocr}, nil
}

func (p *DocReaderDocumentParser) ParserType() string { return ParserTypeDocReader }

func (p *DocReaderDocumentParser) Supports(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "application/pdf", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/epub+zip", "image/png", "image/jpeg", "image/gif", "image/bmp", "image/tiff", "image/webp":
		return true
	}
	return false
}

func (p *DocReaderDocumentParser) Parse(ctx context.Context, content []byte, mimeType string, options map[string]any) (ParseResult, error) {
	fileName := fileNameFromOptions(options)
	fileType := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	if fileType == "tif" {
		fileType = "tiff"
	}
	if fileType == "" {
		fileType = fileTypeForMime(mimeType)
	}
	callCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	stream, err := p.client.ReadStream(callCtx, &docreaderpb.ReadRequest{
		FileContent: content,
		FileName:    fileName,
		FileType:    fileType,
		Config:      &docreaderpb.ReadConfig{ParserEngine: "builtin"},
	})
	if err == nil {
		var frame *docreaderpb.ReadStreamResponse
		frame, err = stream.Recv()
		if err == nil {
			meta := frame.GetMeta()
			if meta == nil {
				err = fmt.Errorf("docreader returned no metadata")
			} else if meta.GetError() != "" {
				err = fmt.Errorf("docreader: %s", meta.GetError())
			} else if strings.TrimSpace(meta.GetMarkdownContent()) == "" {
				err = fmt.Errorf("docreader returned empty content")
			} else {
				metadata := map[string]any{"parser_type": p.ParserType(), "mime_type": mimeType}
				for key, value := range meta.GetMetadata() {
					metadata[key] = value
				}
				images := make(map[string]OCRResult)
				imageCount := 0
				noTextCount := 0
				if meta.GetImageCount() > maxOCRImages {
					return ParseResult{}, fmt.Errorf("docreader returned too many images: %d", meta.GetImageCount())
				}
				for {
					frame, recvErr := stream.Recv()
					if recvErr == io.EOF {
						text, missing := mergeOCRText(meta.GetMarkdownContent(), images)
						if missing > 0 {
							return ParseResult{}, fmt.Errorf("docreader returned %d image references without image data", missing)
						}
						metadata["ocr_image_count"] = imageCount
						metadata["ocr_no_text_count"] = noTextCount
						if strings.TrimSpace(text) == "" {
							return ParseResult{}, fmt.Errorf("document has no searchable text after OCR")
						}
						return Of(text, metadata), nil
					}
					if recvErr != nil {
						err = recvErr
						break
					}
					if image := frame.GetImage(); image != nil {
						imageCount++
						if imageCount > maxOCRImages {
							return ParseResult{}, fmt.Errorf("docreader returned more than %d images", maxOCRImages)
						}
						if p.ocr == nil {
							return ParseResult{}, fmt.Errorf("OCR is required for document images")
						}
						ocrResult, ocrErr := p.ocr.Recognize(callCtx, image.GetImageData())
						if ocrErr != nil {
							return ParseResult{}, fmt.Errorf("OCR image %q: %w", image.GetOriginalRef(), ocrErr)
						}
						images[image.GetOriginalRef()] = ocrResult
						if ocrResult.Status == "no_text" {
							noTextCount++
						}
					}
				}
			}
		}
	}
	if p.fallback != nil && ctx.Err() == nil {
		result, fallbackErr := p.fallback.Parse(ctx, content, mimeType, options)
		if fallbackErr == nil {
			result.Metadata["fallback_from"] = p.ParserType()
			return result, nil
		}
		return ParseResult{}, fmt.Errorf("docreader: %v; fallback: %w", err, fallbackErr)
	}
	return ParseResult{}, fmt.Errorf("docreader parse: %w", err)
}

func mergeOCRText(markdown string, images map[string]OCRResult) (string, int) {
	matches := imageMarkdownPattern.FindAllStringSubmatchIndex(markdown, -1)
	var merged strings.Builder
	last := 0
	missing := 0
	for _, match := range matches {
		merged.WriteString(markdown[last:match[0]])
		ref := markdown[match[2]:match[3]]
		result, ok := images[ref]
		if !ok {
			missing++
		} else if result.Status == "success" {
			merged.WriteString(result.Text)
		}
		last = match[1]
	}
	merged.WriteString(markdown[last:])
	return merged.String(), missing
}

func (p *DocReaderDocumentParser) ExtractText(stream io.Reader, fileName string) (string, error) {
	content, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	result, err := p.Parse(context.Background(), content, "", map[string]any{"file_name": fileName})
	return result.Text, err
}

func fileTypeForMime(mimeType string) string {
	switch mimeType {
	case "application/pdf":
		return "pdf"
	case "application/msword":
		return "doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/vnd.ms-excel":
		return "xls"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case "text/html":
		return "html"
	case "application/epub+zip":
		return "epub"
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/tiff":
		return "tiff"
	case "image/bmp":
		return "bmp"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	}
	return ""
}
