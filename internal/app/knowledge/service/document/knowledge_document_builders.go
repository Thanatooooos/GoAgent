package document

import (
	"context"
	"encoding/json"
	"strings"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

func (s *KnowledgeDocumentService) buildKnowledgeDocumentForUpload(
	ctx context.Context,
	knowledgeBase domain.KnowledgeBase,
	documentID string,
	sourceType string,
	input UploadKnowledgeDocumentInput,
	operatorID string,
) (domain.KnowledgeDocument, func(), error) {
	switch sourceType {
	case domain.KnowledgeDocumentSourceURL:
		return s.buildRemoteKnowledgeDocument(ctx, knowledgeBase, documentID, input, operatorID)
	default:
		return s.buildUploadedKnowledgeDocument(ctx, knowledgeBase, documentID, input, operatorID)
	}
}

func (s *KnowledgeDocumentService) buildUploadedKnowledgeDocument(
	ctx context.Context,
	knowledgeBase domain.KnowledgeBase,
	documentID string,
	input UploadKnowledgeDocumentInput,
	operatorID string,
) (domain.KnowledgeDocument, func(), error) {
	fileName := sanitizeDocumentFileName(input.FileName)
	if fileName == "" {
		return domain.KnowledgeDocument{}, nil, exception.NewClientException("file name is required", nil)
	}
	if input.Body == nil {
		return domain.KnowledgeDocument{}, nil, exception.NewClientException("file body is required", nil)
	}
	if input.Size < 0 {
		return domain.KnowledgeDocument{}, nil, exception.NewClientException("file size is invalid", nil)
	}

	storageKey := buildKnowledgeDocumentStorageKey(knowledgeBase.CollectionName, documentID, fileName)
	contentType := strings.TrimSpace(input.ContentType)
	stored, err := s.storage.Upload(ctx, port.FileUpload{
		Key:         storageKey,
		FileName:    fileName,
		ContentType: contentType,
		Size:        input.Size,
		Body:        input.Body,
	})
	if err != nil {
		return domain.KnowledgeDocument{}, nil, exception.NewServiceException("failed to upload knowledge document file", err)
	}
	stored = normalizeStoredKnowledgeDocumentFile(stored, storageKey, fileName, contentType, input.Size)

	document := domain.NewUploadedKnowledgeDocument(
		documentID,
		knowledgeBase.ID,
		stored.FileName,
		stored.Key,
		resolveKnowledgeDocumentFileType(stored.FileName, stored.ContentType),
		operatorID,
		stored.Size,
	)
	document.ProcessMode = normalizeKnowledgeDocumentProcessModeValue(input.ProcessMode)
	document.ChunkStrategy = strings.TrimSpace(input.ChunkStrategy)
	if strings.TrimSpace(input.ChunkConfig) != "" {
		if !json.Valid([]byte(input.ChunkConfig)) {
			_ = s.storage.Delete(ctx, stored.Key)
			return domain.KnowledgeDocument{}, nil, exception.NewClientException("chunk config must be valid json", nil)
		}
		document.ChunkConfig = []byte(strings.TrimSpace(input.ChunkConfig))
	}
	document.PipelineID = strings.TrimSpace(input.PipelineID)
	return document, func() { _ = s.storage.Delete(newCleanupContext(ctx), stored.Key) }, nil
}

func (s *KnowledgeDocumentService) buildRemoteKnowledgeDocument(
	ctx context.Context,
	knowledgeBase domain.KnowledgeBase,
	documentID string,
	input UploadKnowledgeDocumentInput,
	operatorID string,
) (domain.KnowledgeDocument, func(), error) {
	if s.remoteFetcher == nil {
		return domain.KnowledgeDocument{}, nil, exception.NewServiceException("remote file fetcher is required", nil)
	}
	sourceLocation := strings.TrimSpace(input.SourceLocation)
	if sourceLocation == "" {
		return domain.KnowledgeDocument{}, nil, exception.NewClientException("source location is required", nil)
	}
	fallbackName := sanitizeDocumentFileName(input.FileName)
	if fallbackName == "" {
		fallbackName = "remote-file"
	}
	storageKey := buildKnowledgeDocumentStorageKey(knowledgeBase.CollectionName, documentID, fallbackName)
	stored, err := s.remoteFetcher.FetchAndStore(ctx, sourceLocation, storageKey, fallbackName)
	if err != nil {
		return domain.KnowledgeDocument{}, nil, err
	}
	document := domain.NewUploadedKnowledgeDocument(
		documentID,
		knowledgeBase.ID,
		stored.OriginFileName,
		stored.Url,
		resolveKnowledgeDocumentFileType(stored.OriginFileName, stored.DetectedType),
		operatorID,
		stored.Size,
	)
	document.SourceType = domain.KnowledgeDocumentSourceURL
	document.SourceLocation = sourceLocation
	document.ScheduleEnabled = input.ScheduleEnabled
	document.ScheduleCron = strings.TrimSpace(input.ScheduleCron)
	document.ProcessMode = normalizeKnowledgeDocumentProcessModeValue(input.ProcessMode)
	document.ChunkStrategy = strings.TrimSpace(input.ChunkStrategy)
	if strings.TrimSpace(input.ChunkConfig) != "" {
		if !json.Valid([]byte(input.ChunkConfig)) {
			_ = s.storage.Delete(ctx, stored.Url)
			return domain.KnowledgeDocument{}, nil, exception.NewClientException("chunk config must be valid json", nil)
		}
		document.ChunkConfig = []byte(strings.TrimSpace(input.ChunkConfig))
	}
	document.PipelineID = strings.TrimSpace(input.PipelineID)
	return document, func() { _ = s.storage.Delete(newCleanupContext(ctx), stored.Url) }, nil
}
