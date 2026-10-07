package parser

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	docreaderpb "local/rag-project/internal/app/core/parser/docreaderpb"
)

// ParseStructured returns document text and image occurrences without calling
// OCR. The ingestion worker owns OCR, captioning, and their retries.
func (p *DocReaderDocumentParser) ParseStructured(ctx context.Context, content []byte, mimeType string, options map[string]any) (ParseResult, error) {
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
		var first *docreaderpb.ReadStreamResponse
		first, err = stream.Recv()
		if err == nil {
			meta := first.GetMeta()
			switch {
			case meta == nil:
				err = fmt.Errorf("docreader returned no metadata")
			case meta.GetError() != "":
				err = fmt.Errorf("docreader: %s", meta.GetError())
			default:
				frames := make(map[string]*docreaderpb.ImageRef)
				for {
					frame, recvErr := stream.Recv()
					if recvErr == io.EOF {
						result := buildStructuredResult(meta.GetMarkdownContent(), frames, mimeType, meta.GetMetadata())
						if strings.TrimSpace(result.Text) == "" && len(result.Images) == 0 {
							return ParseResult{}, fmt.Errorf("docreader returned no text or images")
						}
						return result, nil
					}
					if recvErr != nil {
						err = recvErr
						break
					}
					if image := frame.GetImage(); image != nil {
						ref := image.GetOriginalRef()
						if ref == "" {
							err = fmt.Errorf("docreader returned an image without original reference")
							break
						}
						if _, exists := frames[ref]; exists {
							err = fmt.Errorf("docreader returned duplicate image frame %q", ref)
							break
						}
						frames[ref] = image
					}
				}
			}
		}
	}
	if p.fallback != nil && ctx.Err() == nil {
		result, fallbackErr := p.fallback.Parse(ctx, content, mimeType, options)
		if fallbackErr == nil {
			result.Metadata["fallback_from"] = p.ParserType()
			result.ImageInventoryConfirmed = false
			return result, nil
		}
		return ParseResult{}, fmt.Errorf("docreader: %v; fallback: %w", err, fallbackErr)
	}
	return ParseResult{}, fmt.Errorf("docreader structured parse: %w", err)
}

func buildStructuredResult(markdown string, frames map[string]*docreaderpb.ImageRef, mimeType string, sourceMetadata map[string]string) ParseResult {
	metadata := map[string]any{"parser_type": ParserTypeDocReader, "mime_type": mimeType}
	for key, value := range sourceMetadata {
		metadata[key] = value
	}
	result := Of("", metadata)
	matches := imageMarkdownPattern.FindAllStringSubmatchIndex(markdown, -1)
	var body strings.Builder
	used := make(map[string]bool, len(frames))
	last := 0
	for _, match := range matches {
		body.WriteString(markdown[last:match[0]])
		ref := markdown[match[2]:match[3]]
		occurrence := ImageOccurrence{Index: len(result.Images), OriginalRef: ref}
		occurrence.AdjacentText = adjacentMarkdownText(markdown, match[0], match[1])
		if image := frames[ref]; image != nil {
			used[ref] = true
			occurrence.Filename = image.GetFilename()
			occurrence.MIMEType = image.GetMimeType()
			occurrence.StorageKey = image.GetStorageKey()
			occurrence.Data = image.GetImageData()
			if len(occurrence.Data) == 0 && occurrence.StorageKey == "" {
				occurrence.Error = "image frame has no data"
			}
		} else if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
			occurrence.Error = "external image is not downloaded"
		} else {
			occurrence.Error = "image reference has no frame"
		}
		result.Images = append(result.Images, occurrence)
		last = match[1]
	}
	body.WriteString(markdown[last:])
	result.Text = body.String()
	var unreferenced []string
	for ref := range frames {
		if !used[ref] {
			unreferenced = append(unreferenced, ref)
		}
	}
	sort.Strings(unreferenced)
	for _, ref := range unreferenced {
		image := frames[ref]
		result.Images = append(result.Images, ImageOccurrence{
			Index: len(result.Images), OriginalRef: ref,
			Filename: image.GetFilename(), MIMEType: image.GetMimeType(),
			StorageKey: image.GetStorageKey(), Data: image.GetImageData(),
			Error: "image frame is not referenced by markdown",
		})
	}
	return result
}

func adjacentMarkdownText(markdown string, start, end int) string {
	before := markdown[max(0, start-240):start]
	after := markdown[end:min(len(markdown), end+240)]
	context := imageMarkdownPattern.ReplaceAllString(before+" "+after, " ")
	context = strings.Join(strings.Fields(strings.ToValidUTF8(context, "")), " ")
	runes := []rune(context)
	if len(runes) > 240 {
		return string(runes[:240])
	}
	return context
}
