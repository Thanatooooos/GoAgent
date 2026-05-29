package service

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

const knowledgeDocumentCleanupTimeout = 30 * time.Second

func newCleanupContext(ctx context.Context) context.Context {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	cleanupCtx, _ := context.WithTimeout(base, knowledgeDocumentCleanupTimeout)
	return cleanupCtx
}

func normalizeKnowledgeDocumentSourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", domain.KnowledgeDocumentSourceFile:
		return domain.KnowledgeDocumentSourceFile
	case domain.KnowledgeDocumentSourceURL:
		return domain.KnowledgeDocumentSourceURL
	default:
		return ""
	}
}

func normalizeKnowledgeDocumentProcessMode(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", domain.KnowledgeDocumentProcessModeChunk:
		return domain.KnowledgeDocumentProcessModeChunk, nil
	case domain.KnowledgeDocumentProcessModePipeline:
		return domain.KnowledgeDocumentProcessModePipeline, nil
	default:
		return "", exception.NewClientException("process mode must be chunk or pipeline", nil)
	}
}

func normalizeKnowledgeDocumentProcessModeValue(value string) string {
	mode, err := normalizeKnowledgeDocumentProcessMode(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return mode
}

func effectiveKnowledgeDocumentProcessMode(current string, input string) (string, error) {
	if strings.TrimSpace(input) != "" {
		return normalizeKnowledgeDocumentProcessMode(input)
	}
	return normalizeKnowledgeDocumentProcessMode(current)
}

func normalizeKnowledgeDocumentChunkStrategy(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "":
		return "", nil
	case "fixed_size":
		return "fixed_size", nil
	case "markdown", "structure_aware":
		return "structure_aware", nil
	default:
		return "", exception.NewClientException("chunk strategy is invalid", nil)
	}
}

func effectiveKnowledgeDocumentChunkStrategy(current string, input string) (string, error) {
	if strings.TrimSpace(input) != "" {
		return normalizeKnowledgeDocumentChunkStrategy(input)
	}
	return normalizeKnowledgeDocumentChunkStrategy(current)
}

func validateKnowledgeDocumentProcessingConfig(processMode string, chunkStrategy string, pipelineID string, validateModeSpecificFields bool) error {
	switch processMode {
	case domain.KnowledgeDocumentProcessModeChunk:
		if chunkStrategy == "" {
			chunkStrategy = "fixed_size"
		}
		if validateModeSpecificFields && strings.TrimSpace(pipelineID) != "" {
			return exception.NewClientException("pipeline id is only allowed when process mode is pipeline", nil)
		}
		return nil
	case domain.KnowledgeDocumentProcessModePipeline:
		if strings.TrimSpace(pipelineID) == "" {
			return exception.NewClientException("pipeline id is required when process mode is pipeline", nil)
		}
		if validateModeSpecificFields && strings.TrimSpace(chunkStrategy) != "" {
			return exception.NewClientException("chunk strategy is only allowed when process mode is chunk", nil)
		}
		return nil
	default:
		return exception.NewClientException("process mode must be chunk or pipeline", nil)
	}
}

func buildKnowledgeDocumentStorageKey(collectionName, documentID, fileName string) string {
	collectionName = strings.Trim(strings.TrimSpace(collectionName), "/")
	if collectionName == "" {
		collectionName = "knowledge"
	}
	return fmt.Sprintf("knowledge/%s/documents/%s/%s", collectionName, documentID, fileName)
}

func normalizeStoredKnowledgeDocumentFile(stored port.StoredFile, key, fileName, contentType string, size int64) port.StoredFile {
	if strings.TrimSpace(stored.Key) == "" {
		stored.Key = key
	}
	if strings.TrimSpace(stored.FileName) == "" {
		stored.FileName = fileName
	}
	if strings.TrimSpace(stored.ContentType) == "" {
		stored.ContentType = contentType
	}
	if stored.Size == 0 && size > 0 {
		stored.Size = size
	}
	return stored
}

func sanitizeDocumentFileName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return ""
	}
	fileName = filepath.Base(fileName)
	if fileName == "." || fileName == string(filepath.Separator) {
		return ""
	}
	return fileName
}

func resolveKnowledgeDocumentFileType(fileName, contentType string) string {
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), "."); ext != "" {
		return truncateKnowledgeDocumentFileType(ext)
	}

	if mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType)); err == nil && mediaType != "" {
		if slash := strings.LastIndex(mediaType, "/"); slash >= 0 && slash < len(mediaType)-1 {
			return truncateKnowledgeDocumentFileType(mediaType[slash+1:])
		}
		return truncateKnowledgeDocumentFileType(mediaType)
	}

	return "unknown"
}

func truncateKnowledgeDocumentFileType(fileType string) string {
	fileType = strings.TrimSpace(strings.ToLower(fileType))
	if fileType == "" {
		return "unknown"
	}
	if len(fileType) > 16 {
		return fileType[:16]
	}
	return fileType
}
