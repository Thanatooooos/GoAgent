package workflow

import (
	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// ValidateTaskSourceType 校验 source type 是否在当前支持范围内。
func ValidateTaskSourceType(sourceType string) error {
	switch sourceType {
	case domain.TaskSourceTypeFile, domain.TaskSourceTypeURL, domain.TaskSourceTypeFeishu, domain.TaskSourceTypeS3:
		return nil
	default:
		return exception.NewClientException("source type must be file, url, feishu or s3", nil)
	}
}
