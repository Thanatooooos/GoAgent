package chunk

import (
	"fmt"

	"local/rag-project/internal/framework/distributedid"
)

func nextChunkID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}
