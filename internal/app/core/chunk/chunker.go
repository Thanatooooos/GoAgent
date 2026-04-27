package chunk

type Chunker interface {
	Strategy() Strategy
	Chunk(text string, opts Options) ([]Chunk, error)
}
