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
	defaultRemoteFileMaxBytes = int64(50 << 20)
	remoteFileBufferSize      = 8192
)

type RemoteFileFetcherOptions struct {
	HTTPClient *http.Client
	Storage    port.FileStorage
	MaxBytes   int64
	TempDir    string
}

type RemoteFileFetcher struct {
	httpClient *http.Client
	storage    port.FileStorage
	maxBytes   int64
	tempDir    string
}

type RemoteFetchResult struct {
	Changed      bool
	TempFile     string
	Size         int64
	ContentType  string
	FileName     string
	ContentHash  string
	ETag         string
	LastModified string
	Message      string
}

type remoteHeadResult struct {
	ContentLength *int64
	ContentType   string
	FileName      string
	ETag          string
	LastModified  string
}

type remoteStreamResult struct {
	Body          io.ReadCloser
	ContentLength *int64
	ContentType   string
	FileName      string
	ETag          string
	LastModified  string
}

func NewRemoteFileFetcher(options RemoteFileFetcherOptions) *RemoteFileFetcher {
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
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

func (f *RemoteFileFetcher) FetchIfChanged(
	ctx context.Context,
	rawURL string,
	lastETag string,
	lastModified string,
	lastContentHash string,
	fallbackFileName string,
) (RemoteFetchResult, error) {
	normalizedURL, err := normalizeRemoteFileURL(rawURL)
	if err != nil {
		return RemoteFetchResult{}, err
	}

	headResult, _ := f.tryHead(ctx, normalizedURL)
	if headResult != nil {
		if err := checkRemoteFileSizeLimit(f.effectiveMaxBytes(), headResult.ContentLength); err != nil {
			return RemoteFetchResult{}, err
		}
		etag := trimOrEmpty(headResult.ETag)
		headLastModified := trimOrEmpty(headResult.LastModified)
		etagMatch := etag != "" && etag == trimOrEmpty(lastETag)
		modifiedMatch := headLastModified != "" && headLastModified == trimOrEmpty(lastModified)
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

	streamResult, err := f.openStream(ctx, normalizedURL)
	if err != nil {
		return RemoteFetchResult{}, err
	}
	defer streamResult.Body.Close()

	if err := checkRemoteFileSizeLimit(f.effectiveMaxBytes(), streamResult.ContentLength); err != nil {
		return RemoteFetchResult{}, err
	}

	tempFile, size, hash, err := f.copyToTempWithDigest(streamResult.Body)
	if err != nil {
		return RemoteFetchResult{}, err
	}
	if size == 0 {
		deleteRemoteTempFileQuietly(tempFile)
		return RemoteFetchResult{}, exception.NewClientException("remote file content is empty", nil)
	}

	etag := firstText(streamResult.ETag, headText(headResult, func(h *remoteHeadResult) string { return h.ETag }))
	fetchLastModified := firstText(streamResult.LastModified, headText(headResult, func(h *remoteHeadResult) string { return h.LastModified }))
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

	fileName := firstText(streamResult.FileName, headText(headResult, func(h *remoteHeadResult) string { return h.FileName }), fallbackFileName, fileNameFromURL(normalizedURL), "remote-file")
	contentType := firstText(streamResult.ContentType, headText(headResult, func(h *remoteHeadResult) string { return h.ContentType }))
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

func (f *RemoteFileFetcher) FetchAndStore(ctx context.Context, rawURL string, storageKey string, fallbackFileName string) (StoredFileDTO, error) {
	if f == nil || f.storage == nil {
		return StoredFileDTO{}, exception.NewServiceException("file storage is required", nil)
	}
	normalizedURL, err := normalizeRemoteFileURL(rawURL)
	if err != nil {
		return StoredFileDTO{}, err
	}

	headResult, _ := f.tryHead(ctx, normalizedURL)
	if headResult != nil {
		if err := checkRemoteFileSizeLimit(f.effectiveMaxBytes(), headResult.ContentLength); err != nil {
			return StoredFileDTO{}, err
		}
	}

	streamResult, err := f.openStream(ctx, normalizedURL)
	if err != nil {
		return StoredFileDTO{}, err
	}
	defer streamResult.Body.Close()

	if err := checkRemoteFileSizeLimit(f.effectiveMaxBytes(), streamResult.ContentLength); err != nil {
		return StoredFileDTO{}, err
	}

	tempFile, size, err := f.copyToTemp(streamResult.Body)
	if err != nil {
		return StoredFileDTO{}, err
	}
	defer deleteRemoteTempFileQuietly(tempFile)
	if size == 0 {
		return StoredFileDTO{}, exception.NewClientException("remote file content is empty", nil)
	}

	file, err := os.Open(tempFile)
	if err != nil {
		return StoredFileDTO{}, exception.NewServiceException("open remote temp file failed", err)
	}
	defer file.Close()

	fileName := firstText(streamResult.FileName, headText(headResult, func(h *remoteHeadResult) string { return h.FileName }), fallbackFileName, fileNameFromURL(normalizedURL), "remote-file")
	contentType := firstText(streamResult.ContentType, headText(headResult, func(h *remoteHeadResult) string { return h.ContentType }))
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

func (r *RemoteFetchResult) Close() error {
	if r == nil || strings.TrimSpace(r.TempFile) == "" {
		return nil
	}
	tempFile := r.TempFile
	r.TempFile = ""
	return os.Remove(tempFile)
}

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

func (f *RemoteFileFetcher) copyToTempWithDigest(input io.Reader) (string, int64, string, error) {
	temp, err := os.CreateTemp(f.tempDir, "knowledge-schedule-*.tmp")
	if err != nil {
		return "", 0, "", exception.NewServiceException("create remote temp file failed", err)
	}
	defer temp.Close()

	digest := sha256.New()
	writer := io.MultiWriter(temp, digest)
	size, err := copyRemoteWithLimit(writer, input, f.effectiveMaxBytes())
	if err != nil {
		deleteRemoteTempFileQuietly(temp.Name())
		return "", 0, "", err
	}
	return temp.Name(), size, hex.EncodeToString(digest.Sum(nil)), nil
}

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

func (f *RemoteFileFetcher) effectiveHTTPClient() *http.Client {
	if f != nil && f.httpClient != nil {
		return f.httpClient
	}
	return http.DefaultClient
}

func (f *RemoteFileFetcher) effectiveMaxBytes() int64 {
	if f == nil || f.maxBytes == 0 {
		return defaultRemoteFileMaxBytes
	}
	return f.maxBytes
}

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

func copyRemoteWithLimit(writer io.Writer, reader io.Reader, maxBytes int64) (int64, error) {
	buffer := make([]byte, remoteFileBufferSize)
	var total int64
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			total += int64(n)
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

func checkRemoteFileSizeLimit(maxBytes int64, contentLength *int64) error {
	if maxBytes > 0 && contentLength != nil && *contentLength > maxBytes {
		return exception.NewClientException(fmt.Sprintf("remote file size exceeds limit: %d bytes", maxBytes), nil)
	}
	return nil
}

func remoteFileStatusError(statusCode int) error {
	message := fmt.Sprintf("remote file fetch returned status %d", statusCode)
	if statusCode >= 400 && statusCode < 500 {
		return exception.NewClientException(message, nil)
	}
	return exception.NewServiceException(message, nil)
}

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

func deleteRemoteTempFileQuietly(tempFile string) {
	if strings.TrimSpace(tempFile) != "" {
		_ = os.Remove(tempFile)
	}
}

func firstText(values ...string) string {
	for _, value := range values {
		if trimmed := trimOrEmpty(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func trimOrEmpty(value string) string {
	return strings.TrimSpace(value)
}

func headText(head *remoteHeadResult, pick func(*remoteHeadResult) string) string {
	if head == nil {
		return ""
	}
	return pick(head)
}

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
