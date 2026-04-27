package chunk

import "errors"

var (
	ErrSelectorNil       = errors.New("chunk selector is nil")
	ErrChunkerNotFound   = errors.New("chunker not found")
	ErrChunkContentEmpty = errors.New("chunk content is empty")
)
