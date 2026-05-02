package s3

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/config"
)

const defaultBucket = "knowledge"

type FileStorage struct {
	client *minio.Client
	bucket string
}

var _ port.FileStorage = (*FileStorage)(nil)

func NewFileStorage(cfg config.RustFSConfig) (*FileStorage, error) {
	endpoint, secure, err := parseEndpoint(cfg.Url)
	if err != nil {
		return nil, err
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(strings.TrimSpace(cfg.AccessKeyId), strings.TrimSpace(cfg.SecretAccessKey), ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}

	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		bucket = defaultBucket
	}
	return &FileStorage{client: client, bucket: bucket}, nil
}

func NewFileStorageWithClient(client *minio.Client, bucket string) *FileStorage {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		bucket = defaultBucket
	}
	return &FileStorage{client: client, bucket: bucket}
}

func (s *FileStorage) Upload(ctx context.Context, file port.FileUpload) (port.StoredFile, error) {
	if s == nil || s.client == nil {
		return port.StoredFile{}, fmt.Errorf("s3 client is required")
	}
	key := normalizeObjectKey(file.Key, file.FileName)
	if key == "" {
		return port.StoredFile{}, fmt.Errorf("file key is required")
	}
	if file.Body == nil {
		return port.StoredFile{}, fmt.Errorf("file body is required")
	}

	contentType := strings.TrimSpace(file.ContentType)
	opts := minio.PutObjectOptions{ContentType: contentType}
	info, err := s.client.PutObject(ctx, s.bucket, key, file.Body, file.Size, opts)
	if err != nil {
		return port.StoredFile{}, fmt.Errorf("upload s3 object: %w", err)
	}

	size := info.Size
	if size == 0 {
		size = file.Size
	}
	return port.StoredFile{
		Key:         key,
		FileName:    strings.TrimSpace(file.FileName),
		ContentType: contentType,
		Size:        size,
	}, nil
}

func (s *FileStorage) Delete(ctx context.Context, key string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("s3 client is required")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete s3 object: %w", err)
	}
	return nil
}

func (s *FileStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("s3 client is required")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("file key is required")
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("open s3 object: %w", err)
	}
	return object, nil
}

func parseEndpoint(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, fmt.Errorf("rustfs url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("parse rustfs url: %w", err)
	}
	if parsed.Host == "" {
		return raw, false, nil
	}
	secure := parsed.Scheme == "https"
	return parsed.Host, secure, nil
}

func normalizeObjectKey(key, fileName string) string {
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key != "" {
		return path.Clean(strings.ReplaceAll(key, "\\", "/"))
	}
	fileName = strings.Trim(strings.TrimSpace(fileName), "/")
	if fileName == "" {
		return ""
	}
	return path.Clean(strings.ReplaceAll(fileName, "\\", "/"))
}
