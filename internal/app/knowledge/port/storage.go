package port

import (
	"context"
	"io"
)

type FileUpload struct {
	Key         string
	FileName    string
	ContentType string
	Size        int64
	Body        io.Reader
}

type StoredFile struct {
	Key         string
	FileName    string
	ContentType string
	Size        int64
}

type FileStorage interface {
	Upload(ctx context.Context, file FileUpload) (StoredFile, error)
	Delete(ctx context.Context, key string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}
