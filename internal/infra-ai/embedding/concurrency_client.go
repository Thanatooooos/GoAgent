package embedding

import (
	"local/rag-project/internal/infra-ai/limiter"
	"local/rag-project/internal/infra-ai/model"
)

// concurrencyEmbeddingClient 只对批量 embedding（ingestion 后台）限流；
// 单条 Embed（交互查询）直通，永不过闸。
type concurrencyEmbeddingClient struct {
	inner        EmbeddingClient
	gov          *limiter.Governor
	defaultLimit int
}

func (c *concurrencyEmbeddingClient) Provider() string { return c.inner.Provider() }

func (c *concurrencyEmbeddingClient) Embed(text string, target model.ModelTarget) ([]float32, error) {
	return c.inner.Embed(text, target)
}

func (c *concurrencyEmbeddingClient) EmbedBatch(texts []string, target model.ModelTarget) ([][]float32, error) {
	return c.gateEmbedBatch(func() ([][]float32, error) {
		return c.inner.EmbedBatch(texts, target)
	}, target)
}

func (c *concurrencyEmbeddingClient) gateEmbedBatch(fn func() ([][]float32, error), target model.ModelTarget) ([][]float32, error) {
	limit := c.defaultLimit
	if target.Candidate.MaxConcurrency > 0 {
		limit = target.Candidate.MaxConcurrency
	}
	key := target.Id
	if key == "" {
		key = "embedding"
	}
	var out [][]float32
	err := c.gov.Gate(key, limit, func() error {
		var callErr error
		out, callErr = fn()
		return callErr
	})
	return out, err
}

// WithConcurrencyLimit 包装 clients，使批量 embedding 遵循 per-model 并发上限；
// defaultLimit<=0 时原样返回（不限流）。返回的包装器共享同一个 Governor。
func WithConcurrencyLimit(clients []EmbeddingClient, defaultLimit int) []EmbeddingClient {
	if defaultLimit <= 0 || len(clients) == 0 {
		return clients
	}
	gov := limiter.NewGovernor()
	wrapped := make([]EmbeddingClient, 0, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		wrapped = append(wrapped, &concurrencyEmbeddingClient{inner: client, gov: gov, defaultLimit: defaultLimit})
	}
	return wrapped
}
