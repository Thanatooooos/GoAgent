package port

import "context"

type MemoryMutationTransaction func(
	ctx context.Context,
	fn func(ctx context.Context, repo MemoryItemRepository) error,
) error
