package schedule

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

const (
	defaultRemoteFileMaxBytes = int64(50 << 20) // 默认最大文件大小：50MB
	remoteFileBufferSize      = 8192            // 读取缓冲区大小：8KB
)

// RemoteFileFetcherOptions 远程文件获取器配置
type RemoteFileFetcherOptions struct {
	HTTPClient *http.Client     // HTTP 客户端（默认 30s 超时）
	Storage    port.FileStorage // 文件存储
	MaxBytes   int64            // 最大文件大小限制
	TempDir    string           // 临时文件目录
}

// RemoteFileFetcher 负责从远程 URL 获取文件
// 核心功能：
// 1. 检查文件是否变化（ETag/Last-Modified/ContentHash）
// 2. 下载文件到临时目录
// 3. 计算文件 SHA256 摘要
// 4. 文件大小限制和错误处理
type RemoteFileFetcher struct {
	httpClient *http.Client
	storage    port.FileStorage
	maxBytes   int64
	tempDir    string
}

// RemoteFetchResult 远程文件获取结果
// Changed=false 表示文件未变化，调用者无需处理
// Changed=true 时，TempFile 包含下载的临时文件路径
type RemoteFetchResult struct {
	Changed      bool   // 文件是否发生变化
	TempFile     string // 临时文件路径（Changed=true 时有效）
	Size         int64  // 文件大小（字节）
	ContentType  string // 文件 MIME 类型
	FileName     string // 文件名
	ContentHash  string // SHA256 摘要
	ETag         string // HTTP ETag
	LastModified string // HTTP Last-Modified
	Message      string // 附加信息（如 "remote file unchanged"）
}

// remoteHeadResult HTTP HEAD 请求结果
type remoteHeadResult struct {
	ContentLength *int64 // 内容长度（可能为 nil）
	ContentType   string // 内容类型
	FileName      string // 文件名（从 Content-Disposition 解析）
	ETag          string // ETag
	LastModified  string // Last-Modified
}

// remoteStreamResult HTTP GET 请求结果
type remoteStreamResult struct {
	Body          io.ReadCloser // 响应体
	ContentLength *int64        // 内容长度
	ContentType   string        // 内容类型
	FileName      string        // 文件名
	ETag          string        // ETag
	LastModified  string        // Last-Modified
}

// NewRemoteFileFetcher 创建远程文件获取器实例
func NewRemoteFileFetcher(options RemoteFileFetcherOptions) *RemoteFileFetcher {
	// 默认 HTTP 客户端：30 秒超时
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	// 默认文件大小限制：50MB
	maxBytes := options.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultRemoteFileMaxBytes
	}
	return &RemoteFileFetcher{
		httpClient: httpClient,
		storage:    options.Storage,
		maxBytes:   maxBytes,
		tempDir:    options.TempDir,
	}
}

// FetchIfChanged 检查远程文件是否变化，如果有变化则下载
// 变化检测策略（按优先级）：
// 1. ETag 匹配：HTTP 标准缓存验证
// 2. Last-Modified 匹配：HTTP 标准缓存验证
// 3. ContentHash 匹配：应用层 SHA256 摘要验证
//
// 设计意图：
// - 避免重复下载未变化的文件
// - 减少网络带宽和存储开销
// - 支持多种变化检测方式，兼容不同服务器
func (f *RemoteFileFetcher) FetchIfChanged(
	ctx context.Context,
	rawURL string,
	lastETag string,
	lastModified string,
	lastContentHash string,
	fallbackFileName string,
) (RemoteFetchResult, error) {
	// 1. 规范化 URL
	normalizedURL, err := normalizeRemoteFileURL(rawURL)
	if err != nil {
		return RemoteFetchResult{}, err
	}

	// 2. 发送 HEAD 请求获取文件元信息
	headResult, _ := f.tryHead(ctx, normalizedURL)
	if headResult != nil {
		// 检查文件大小限制
		if err := checkRemoteFileSizeLimit(f.effectiveMaxBytes(), headResult.ContentLength); err != nil {
			return RemoteFetchResult{}, err
		}
		// 检查 ETag 或 Last-Modified 是否匹配
		etag := trimOrEmpty(headResult.ETag)
		headLastModified := trimOrEmpty(headResult.LastModified)
		etagMatch := etag != "" && etag == trimOrEmpty(lastETag)
		modifiedMatch := headLastModified != "" && headLastModified == trimOrEmpty(lastModified)
		// 如果匹配，说明文件未变化
		if etagMatch || modifiedMatch {
			return RemoteFetchResult{
				Changed:      false,
				Message:      "remote file unchanged",
				ETag:         etag,
				LastModified: headLastModified,
				ContentHash:  trimOrEmpty(lastContentHash),
			}, nil
		}
	}

	// 3. 发送 GET 请求下载文件
	streamResult, err := f.openStream(ctx, normalizedURL)
	if err != nil {
		return RemoteFetchResult{}, err
	}
	defer streamResult.Body.Close()

	// 4. 检查文件大小限制
	if err := checkRemoteFileSizeLimit(f.effectiveMaxBytes(), streamResult.ContentLength); err != nil {
		return RemoteFetchResult{}, err
	}

	// 5. 复制到临时文件并计算 SHA256 摘要
	tempFile, size, hash, err := f.copyToTempWithDigest(streamResult.Body)
	if err != nil {
		return RemoteFetchResult{}, err
	}
	if size == 0 {
		deleteRemoteTempFileQuietly(tempFile)
		return RemoteFetchResult{}, exception.NewClientException("remote file content is empty", nil)
	}

	// 6. 合并 ETag 和 Last-Modified（优先使用 GET 响应，其次使用 HEAD 响应）
	etag := firstText(streamResult.ETag, headText(headResult, func(h *remoteHeadResult) string { return h.ETag }))
	fetchLastModified := firstText(streamResult.LastModified, headText(headResult, func(h *remoteHeadResult) string { return h.LastModified }))

	// 7. 检查 ContentHash 是否匹配（二次验证）
	if hash != "" && hash == trimOrEmpty(lastContentHash) {
		deleteRemoteTempFileQuietly(tempFile)
		return RemoteFetchResult{
			Changed:      false,
			Message:      "content hash unchanged",
			ETag:         trimOrEmpty(etag),
			LastModified: trimOrEmpty(fetchLastModified),
			ContentHash:  hash,
		}, nil
	}

	// 8. 确定文件名和内容类型
	fileName := firstText(streamResult.FileName, headText(headResult, func(h *remoteHeadResult) string { return h.FileName }), fallbackFileName, fileNameFromURL(normalizedURL), "remote-file")
	contentType := firstText(streamResult.ContentType, headText(headResult, func(h *remoteHeadResult) string { return h.ContentType }))

	// 9. 返回变化结果
	return RemoteFetchResult{
		Changed:      true,
		TempFile:     tempFile,
		Size:         size,
		ContentType:  contentType,
		FileName:     fileName,
		ContentHash:  hash,
		ETag:         trimOrEmpty(etag),
		LastModified: trimOrEmpty(fetchLastModified),
	}, nil
}

// FetchAndStore 下载远程文件并直接上传到存储
// 与 FetchIfChanged 的区别：
// - 不检查文件是否变化
// - 直接上传到指定的 storageKey
// - 自动清理临时文件
func (f *RemoteFileFetcher) FetchAndStore(ctx context.Context, rawURL string, storageKey string, fallbackFileName string) (StoredFileDTO, error) {
	if f == nil || f.storage == nil {
		return StoredFileDTO{}, exception.NewServiceException("file storage is required", nil)
	}
	// 1. 规范化 URL
	normalizedURL, err := normalizeRemoteFileURL(rawURL)
	if err != nil {
		return StoredFileDTO{}, err
	}

	// 2. HEAD 请求获取元信息
	headResult, _ := f.tryHead(ctx, normalizedURL)
	if headResult != nil {
		if err := checkRemoteFileSizeLimit(f.effectiveMaxBytes(), headResult.ContentLength); err != nil {
			return StoredFileDTO{}, err
		}
	}

	// 3. GET 请求下载文件
	streamResult, err := f.openStream(ctx, normalizedURL)
	if err != nil {
		return StoredFileDTO{}, err
	}
	defer streamResult.Body.Close()

	// 4. 检查文件大小限制
	if err := checkRemoteFileSizeLimit(f.effectiveMaxBytes(), streamResult.ContentLength); err != nil {
		return StoredFileDTO{}, err
	}

	// 5. 复制到临时文件
	tempFile, size, err := f.copyToTemp(streamResult.Body)
	if err != nil {
		return StoredFileDTO{}, err
	}
	defer deleteRemoteTempFileQuietly(tempFile)
	if size == 0 {
		return StoredFileDTO{}, exception.NewClientException("remote file content is empty", nil)
	}

	// 6. 打开临时文件准备上传
	file, err := os.Open(tempFile)
	if err != nil {
		return StoredFileDTO{}, exception.NewServiceException("open remote temp file failed", err)
	}
	defer file.Close()

	// 7. 确定文件名和内容类型
	fileName := firstText(streamResult.FileName, headText(headResult, func(h *remoteHeadResult) string { return h.FileName }), fallbackFileName, fileNameFromURL(normalizedURL), "remote-file")
	contentType := firstText(streamResult.ContentType, headText(headResult, func(h *remoteHeadResult) string { return h.ContentType }))

	// 8. 上传到存储
	stored, err := f.storage.Upload(ctx, port.FileUpload{
		Key:         storageKey,
		FileName:    fileName,
		ContentType: contentType,
		Size:        size,
		Body:        file,
	})
	if err != nil {
		return StoredFileDTO{}, exception.NewServiceException("upload remote file failed", err)
	}

	return StoredFileDTO{
		Url:            stored.Key,
		DetectedType:   stored.ContentType,
		Size:           stored.Size,
		OriginFileName: stored.FileName,
	}, nil
}

// Close 清理临时文件
// 设计意图：实现 io.Closer 接口，支持 defer 调用
func (r *RemoteFetchResult) Close() error {
	if r == nil || strings.TrimSpace(r.TempFile) == "" {
		return nil
	}
	tempFile := r.TempFile
	r.TempFile = "" // 防止重复删除
	return os.Remove(tempFile)
}

// tryHead 发送 HTTP HEAD 请求获取文件元信息
// 设计意图：
// - 不下载文件内容，仅获取头部信息
// - 用于变化检测和文件大小预检查
func (f *RemoteFileFetcher) tryHead(ctx context.Context, rawURL string) (*remoteHeadResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return nil, exception.NewClientException("build remote file HEAD request failed", err)
	}
	resp, err := f.effectiveHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote file HEAD returned status %d", resp.StatusCode)
	}
	return remoteHeadFromResponse(resp), nil
}

// openStream 发送 HTTP GET 请求下载文件
// 设计意图：
// - 返回流式响应体，支持大文件下载
// - 调用者负责关闭 Body
func (f *RemoteFileFetcher) openStream(ctx context.Context, rawURL string) (remoteStreamResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return remoteStreamResult{}, exception.NewClientException("build remote file GET request failed", err)
	}
	resp, err := f.effectiveHTTPClient().Do(req)
	if err != nil {
		return remoteStreamResult{}, exception.NewServiceException("remote file fetch failed", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return remoteStreamResult{}, remoteFileStatusError(resp.StatusCode)
	}
	head := remoteHeadFromResponse(resp)
	return remoteStreamResult{
		Body:          resp.Body,
		ContentLength: head.ContentLength,
		ContentType:   head.ContentType,
		FileName:      head.FileName,
		ETag:          head.ETag,
		LastModified:  head.LastModified,
	}, nil
}

// copyToTempWithDigest 将数据流复制到临时文件，同时计算 SHA256 摘要
// 返回：临时文件路径、文件大小、SHA256 摘要
func (f *RemoteFileFetcher) copyToTempWithDigest(input io.Reader) (string, int64, string, error) {
	temp, err := os.CreateTemp(f.tempDir, "knowledge-schedule-*.tmp")
	if err != nil {
		return "", 0, "", exception.NewServiceException("create remote temp file failed", err)
	}
	defer temp.Close()

	// 使用 MultiWriter 同时写入文件和摘要计算器
	digest := sha256.New()
	writer := io.MultiWriter(temp, digest)
	size, err := copyRemoteWithLimit(writer, input, f.effectiveMaxBytes())
	if err != nil {
		deleteRemoteTempFileQuietly(temp.Name())
		return "", 0, "", err
	}
	return temp.Name(), size, hex.EncodeToString(digest.Sum(nil)), nil
}

// copyToTemp 将数据流复制到临时文件（不计算摘要）
func (f *RemoteFileFetcher) copyToTemp(input io.Reader) (string, int64, error) {
	temp, err := os.CreateTemp(f.tempDir, "knowledge-upload-*.tmp")
	if err != nil {
		return "", 0, exception.NewServiceException("create remote upload temp file failed", err)
	}
	defer temp.Close()

	size, err := copyRemoteWithLimit(temp, input, f.effectiveMaxBytes())
	if err != nil {
		deleteRemoteTempFileQuietly(temp.Name())
		return "", 0, err
	}
	return temp.Name(), size, nil
}

// effectiveHTTPClient 获取有效的 HTTP 客户端
func (f *RemoteFileFetcher) effectiveHTTPClient() *http.Client {
	if f != nil && f.httpClient != nil {
		return f.httpClient
	}
	return http.DefaultClient
}

// effectiveMaxBytes 获取有效的文件大小限制
func (f *RemoteFileFetcher) effectiveMaxBytes() int64 {
	if f == nil || f.maxBytes == 0 {
		return defaultRemoteFileMaxBytes
	}
	return f.maxBytes
}

// remoteHeadFromResponse 从 HTTP 响应中提取头部信息
func remoteHeadFromResponse(resp *http.Response) *remoteHeadResult {
	contentLength := resp.ContentLength
	var contentLengthPtr *int64
	if contentLength >= 0 {
		contentLengthPtr = &contentLength
	}
	return &remoteHeadResult{
		ContentLength: contentLengthPtr,
		ContentType:   trimOrEmpty(resp.Header.Get("Content-Type")),
		FileName:      fileNameFromContentDisposition(resp.Header.Get("Content-Disposition")),
		ETag:          trimOrEmpty(resp.Header.Get("ETag")),
		LastModified:  trimOrEmpty(resp.Header.Get("Last-Modified")),
	}
}

// copyRemoteWithLimit 从 reader 复制到 writer，带有大小限制
// 设计意图：防止下载超大文件导致内存/磁盘耗尽
func copyRemoteWithLimit(writer io.Writer, reader io.Reader, maxBytes int64) (int64, error) {
	buffer := make([]byte, remoteFileBufferSize)
	var total int64
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			total += int64(n)
			// 检查是否超过限制
			if maxBytes > 0 && total > maxBytes {
				return total, exception.NewClientException(fmt.Sprintf("remote file size exceeds limit: %d bytes", maxBytes), nil)
			}
			if _, err := writer.Write(buffer[:n]); err != nil {
				return total, exception.NewServiceException("write remote temp file failed", err)
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, exception.NewServiceException("read remote file stream failed", readErr)
		}
	}
}

// checkRemoteFileSizeLimit 检查文件大小是否超过限制
func checkRemoteFileSizeLimit(maxBytes int64, contentLength *int64) error {
	if maxBytes > 0 && contentLength != nil && *contentLength > maxBytes {
		return exception.NewClientException(fmt.Sprintf("remote file size exceeds limit: %d bytes", maxBytes), nil)
	}
	return nil
}

// remoteFileStatusError 根据 HTTP 状态码生成错误
// 4xx → 客户端错误，5xx → 服务端错误
func remoteFileStatusError(statusCode int) error {
	message := fmt.Sprintf("remote file fetch returned status %d", statusCode)
	if statusCode >= 400 && statusCode < 500 {
		return exception.NewClientException(message, nil)
	}
	return exception.NewServiceException(message, nil)
}

// normalizeRemoteFileURL 规范化远程文件 URL
// 校验：
// - 非空
// - 有效的 URL 格式
// - 必须是 http 或 https 协议
func normalizeRemoteFileURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return "", exception.NewClientException("remote file url is required", nil)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", exception.NewClientException("remote file url is invalid", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", exception.NewClientException("remote file url must use http or https", nil)
	}
	return value, nil
}

// deleteRemoteTempFileQuietly 静默删除临时文件
// 设计意图：忽略删除错误，避免干扰主流程
func deleteRemoteTempFileQuietly(tempFile string) {
	if strings.TrimSpace(tempFile) != "" {
		_ = os.Remove(tempFile)
	}
}

// firstText 返回第一个非空字符串
// 设计意图：多级 fallback 机制
func firstText(values ...string) string {
	for _, value := range values {
		if trimmed := trimOrEmpty(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// trimOrEmpty 去除字符串前后空格
func trimOrEmpty(value string) string {
	return strings.TrimSpace(value)
}

// headText 从 HEAD 结果中提取字段
func headText(head *remoteHeadResult, pick func(*remoteHeadResult) string) string {
	if head == nil {
		return ""
	}
	return pick(head)
}

// fileNameFromContentDisposition 从 Content-Disposition 头部解析文件名
// 支持：filename 和 filename*（RFC 5987）
func fileNameFromContentDisposition(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	return firstText(params["filename*"], params["filename"])
}

// fileNameFromURL 从 URL 路径中提取文件名
// 示例：https://example.com/docs/report.pdf → report.pdf
func fileNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	base := path.Base(parsed.Path)
	if base == "." || base == "/" {
		return ""
	}
	return base
}
