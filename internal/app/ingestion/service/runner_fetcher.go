package service

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"local/rag-project/internal/adapter/feishu"
	"local/rag-project/internal/app/ingestion/domain"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

// FetcherNodeRunner 提供真实来源读取与统一归一化能力。
type FetcherNodeRunner struct {
	storage      knowledgeport.FileStorage
	httpClient   *http.Client
	feishuClient feishu.DocumentFetcher
}

// NewFetcherNodeRunner 创建 fetcher 运行器。
func NewFetcherNodeRunner(storage knowledgeport.FileStorage, httpClient *http.Client) *FetcherNodeRunner {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &FetcherNodeRunner{
		storage:    storage,
		httpClient: httpClient,
	}
}

// SetFeishuClient 注入飞书 API 客户端，可选配置。
func (r *FetcherNodeRunner) SetFeishuClient(client feishu.DocumentFetcher) {
	if r == nil {
		return
	}
	r.feishuClient = client
}

// NodeType 返回当前运行器负责的节点类型。
func (r *FetcherNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeFetcher
}

// Run 将 task source 读取并归一化为统一 SourcePayload。
func (r *FetcherNodeRunner) Run(ctx context.Context, state ExecutionState, node domain.PipelineNode) (ExecutionState, map[string]any, error) {
	sourceType := pickFirstNonEmpty(
		readStringSetting(node.Settings, "sourceType"),
		state.Task.SourceType,
		state.Source.Type,
	)
	if err := validateTaskSourceType(sourceType); err != nil {
		return state, nil, err
	}

	location := pickFirstNonEmpty(
		readStringSetting(node.Settings, "sourceLocation"),
		state.Task.SourceLocation,
		state.Source.Location,
	)
	fileName := pickFirstNonEmpty(
		readStringSetting(node.Settings, "fileName"),
		state.Task.SourceFileName,
		state.Source.FileName,
	)
	contentType := pickFirstNonEmpty(
		readStringSetting(node.Settings, "contentType"),
		readStringSetting(state.Task.Metadata, "contentType"),
		state.Source.ContentType,
	)
	rawText := pickFirstNonEmpty(
		readStringSetting(node.Settings, "rawText"),
		readStringSetting(state.Task.Metadata, "rawText"),
		readStringSetting(state.Task.Metadata, "content"),
	)

	if sourceType == domain.TaskSourceTypeFile && location == "" && fileName == "" && rawText == "" {
		return state, nil, exception.NewClientException("file source requires source location, file name or inline content", nil)
	}
	if sourceType == domain.TaskSourceTypeURL && location == "" && rawText == "" {
		return state, nil, exception.NewClientException("url source requires source location or inline content", nil)
	}

	next := state.Clone()
	next.Source = SourcePayload{
		Type:        sourceType,
		Location:    location,
		FileName:    fileName,
		ContentType: contentType,
		Metadata:    map[string]any{},
	}

	if rawText != "" {
		next.Source.Bytes = []byte(rawText)
		next.Source.Metadata["source"] = "inline"
	} else {
		fetched, err := r.fetchSource(ctx, sourceType, location, fileName, node.Settings, state.Task.Metadata)
		if err != nil {
			return state, nil, err
		}
		next.Source.Bytes = fetched.Bytes
		next.Source.Location = pickFirstNonEmpty(fetched.Location, next.Source.Location)
		next.Source.FileName = pickFirstNonEmpty(fetched.FileName, next.Source.FileName)
		next.Source.ContentType = pickFirstNonEmpty(fetched.ContentType, next.Source.ContentType)
		next.Source.Metadata = fetched.Metadata
		if readBoolSetting(node.Settings, "cleanupSourceLocation") || readBoolSetting(state.Task.Metadata, "cleanupSourceLocation") {
			if err := cleanupLocalSource(next.Source.Location); err != nil {
				return state, nil, exception.NewServiceException("failed to clean temporary source file", err)
			}
			next.Source.Metadata["cleanedUp"] = true
		}
	}

	if next.Source.FileName == "" {
		next.Source.FileName = inferFileName(next.Source.Location)
	}
	if next.Source.ContentType == "" {
		next.Source.ContentType = detectContentType(next.Source.FileName, next.Source.Bytes)
	}

	output := map[string]any{
		"sourceType":    sourceType,
		"location":      next.Source.Location,
		"fileName":      next.Source.FileName,
		"contentType":   next.Source.ContentType,
		"contentLength": len(next.Source.Bytes),
		"hasBytes":      len(next.Source.Bytes) > 0,
	}
	for key, value := range next.Source.Metadata {
		output[key] = value
	}
	return next, output, nil
}

func (r *FetcherNodeRunner) fetchSource(
	ctx context.Context,
	sourceType string,
	location string,
	fileName string,
	nodeSettings map[string]any,
	taskMetadata map[string]any,
) (SourcePayload, error) {
	switch sourceType {
	case domain.TaskSourceTypeFile:
		return r.fetchFile(ctx, location, fileName)
	case domain.TaskSourceTypeURL:
		return r.fetchURL(ctx, location, fileName, nodeSettings, taskMetadata)
	case domain.TaskSourceTypeS3:
		return r.fetchStorageObject(ctx, location, fileName, "s3")
	case domain.TaskSourceTypeFeishu:
		return r.fetchFeishu(ctx, location, nodeSettings, taskMetadata)
	default:
		return SourcePayload{}, exception.NewClientException("unsupported source type", nil)
	}
}

func (r *FetcherNodeRunner) fetchFile(ctx context.Context, location string, fileName string) (SourcePayload, error) {
	if len(strings.TrimSpace(location)) == 0 {
		return SourcePayload{}, exception.NewClientException("file source location is required", nil)
	}
	localPath := normalizeLocalFilePath(location)
	if localPath != "" {
		content, err := os.ReadFile(localPath)
		if err == nil {
			return SourcePayload{
				Location:    localPath,
				FileName:    pickFirstNonEmpty(fileName, filepath.Base(localPath)),
				ContentType: detectContentType(localPath, content),
				Bytes:       content,
				Metadata: map[string]any{
					"source": "local_file",
				},
			}, nil
		}
	}

	return r.fetchStorageObject(ctx, location, fileName, "storage")
}

func (r *FetcherNodeRunner) fetchStorageObject(ctx context.Context, location string, fileName string, source string) (SourcePayload, error) {
	if r == nil || r.storage == nil {
		return SourcePayload{}, exception.NewServiceException("file storage is required for storage-backed fetcher", nil)
	}
	reader, err := r.storage.Open(ctx, location)
	if err != nil {
		return SourcePayload{}, exception.NewServiceException("failed to open source object", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return SourcePayload{}, exception.NewServiceException("failed to read source object", err)
	}
	return SourcePayload{
		Location:    location,
		FileName:    pickFirstNonEmpty(fileName, inferFileName(location)),
		ContentType: detectContentType(fileName, content),
		Bytes:       content,
		Metadata: map[string]any{
			"source": source,
		},
	}, nil
}

func (r *FetcherNodeRunner) fetchURL(
	ctx context.Context,
	location string,
	fileName string,
	nodeSettings map[string]any,
	taskMetadata map[string]any,
) (SourcePayload, error) {
	if r == nil || r.httpClient == nil {
		return SourcePayload{}, exception.NewServiceException("http client is required for url fetcher", nil)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return SourcePayload{}, exception.NewClientException("invalid source url", err)
	}
	for key, value := range mergeHeaderSettings(nodeSettings, taskMetadata) {
		req.Header.Set(key, value)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return SourcePayload{}, exception.NewServiceException("failed to fetch source url", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SourcePayload{}, exception.NewServiceException(
			fmt.Sprintf("source url returned unexpected status: %d", resp.StatusCode),
			nil,
		)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return SourcePayload{}, exception.NewServiceException("failed to read source url response", err)
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if mediaType, _, parseErr := mime.ParseMediaType(contentType); parseErr == nil {
		contentType = mediaType
	}
	return SourcePayload{
		Location:    location,
		FileName:    pickFirstNonEmpty(fileName, inferFileName(location)),
		ContentType: pickFirstNonEmpty(contentType, detectContentType(location, content)),
		Bytes:       content,
		Metadata: map[string]any{
			"source":     "url",
			"statusCode": resp.StatusCode,
		},
	}, nil
}

// fetchFeishu 通过飞书 API 客户端拉取文档内容。
func (r *FetcherNodeRunner) fetchFeishu(
	ctx context.Context,
	location string,
	nodeSettings map[string]any,
	taskMetadata map[string]any,
) (SourcePayload, error) {
	documentID := feishu.ExtractDocumentID(location)
	if documentID == location {
		// location 不是 URL，检查是否有显式 documentId 设置
		documentID = pickFirstNonEmpty(
			readStringSetting(nodeSettings, "documentId"),
			readStringSetting(taskMetadata, "documentId"),
			location,
		)
	}
	if documentID == "" {
		return SourcePayload{}, exception.NewClientException("feishu source requires document id or url", nil)
	}

	// 从 settings/metadata 读取飞书应用凭证，优先使用 node 级别配置
	appID := pickFirstNonEmpty(
		readStringSetting(nodeSettings, "appId"),
		readStringSetting(taskMetadata, "appId"),
	)
	appSecret := pickFirstNonEmpty(
		readStringSetting(nodeSettings, "appSecret"),
		readStringSetting(taskMetadata, "appSecret"),
	)

	// 优先使用注入的客户端，未注入时尝试从 settings 创建。
	feishuClient := r.feishuClient
	if feishuClient == nil && appID != "" && appSecret != "" {
		feishuClient = feishu.NewClient(appID, appSecret)
	}
	if feishuClient == nil {
		return SourcePayload{}, exception.NewServiceException("feishu client is required, configure appId/appSecret or inject a client", nil)
	}

	content, err := feishuClient.FetchDocumentContent(ctx, documentID)
	if err != nil {
		return SourcePayload{}, err
	}

	fileName := pickFirstNonEmpty(
		readStringSetting(nodeSettings, "fileName"),
		readStringSetting(taskMetadata, "fileName"),
		documentID+".md",
	)

	return SourcePayload{
		Location:    location,
		FileName:    fileName,
		ContentType: "text/markdown",
		Bytes:       content,
		Metadata: map[string]any{
			"source":     "feishu",
			"documentId": documentID,
		},
	}, nil
}

func mergeHeaderSettings(nodeSettings map[string]any, taskMetadata map[string]any) map[string]string {
	result := map[string]string{}
	appendHeaders := func(raw any) {
		switch typed := raw.(type) {
		case map[string]string:
			for key, value := range typed {
				key = strings.TrimSpace(key)
				value = strings.TrimSpace(value)
				if key != "" && value != "" {
					result[key] = value
				}
			}
		case map[string]any:
			for key, value := range typed {
				key = strings.TrimSpace(key)
				stringValue := strings.TrimSpace(fmt.Sprint(value))
				if key != "" && stringValue != "" {
					result[key] = stringValue
				}
			}
		}
	}

	appendHeaders(nodeSettings["headers"])
	appendHeaders(nodeSettings["credentials"])
	appendHeaders(taskMetadata["headers"])
	appendHeaders(taskMetadata["sourceHeaders"])
	appendHeaders(taskMetadata["sourceCredentials"])
	return result
}

func normalizeLocalFilePath(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	if parsed, err := url.Parse(location); err == nil && strings.EqualFold(parsed.Scheme, "file") {
		return filepath.Clean(parsed.Path)
	}
	return filepath.Clean(location)
}

func cleanupLocalSource(location string) error {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil
	}
	return os.Remove(location)
}

func inferFileName(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	if parsed, err := url.Parse(location); err == nil && parsed.Path != "" {
		return filepath.Base(parsed.Path)
	}
	return filepath.Base(location)
}

func detectContentType(fileName string, content []byte) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	if ext != "" {
		if mimeType := mime.TypeByExtension(ext); mimeType != "" {
			if mediaType, _, err := mime.ParseMediaType(mimeType); err == nil {
				return mediaType
			}
			return mimeType
		}
	}
	if len(content) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(content)
}
