package longtermmemory

import (
	"context"

	"local/rag-project/internal/app/rag/port"
)

type MemoryMutationTransaction func(
	ctx context.Context,
	fn func(ctx context.Context, repo port.MemoryItemRepository) error,
) error
