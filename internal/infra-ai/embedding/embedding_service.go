package embedding

type EmbeddingService interface {
	Embed(text string) ([]float32, error)

	EmbedWithModel(text string, modelID string) ([]float32, error)

	EmbedBatch(texts []string) ([][]float32, error)

	EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error)

	Dimension() int
}
