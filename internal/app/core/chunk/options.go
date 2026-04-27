package chunk

const (
	defaultChunkSize   = 800
	defaultOverlapSize = 120
)

type Options struct {
	Strategy     Strategy
	ChunkSize    int
	OverlapSize  int
	MinChunkSize int
}

func (o Options) Normalize() Options {
	if o.Strategy == "" {
		o.Strategy = StrategyFixedSize
	}
	if o.ChunkSize <= 0 {
		o.ChunkSize = defaultChunkSize
	}
	if o.OverlapSize < 0 {
		o.OverlapSize = 0
	}
	if o.ChunkSize == 1 {
		o.OverlapSize = 0
	}
	if o.OverlapSize >= o.ChunkSize {
		o.OverlapSize = o.ChunkSize - 1
		if o.OverlapSize < 0 {
			o.OverlapSize = 0
		}
	}
	if o.MinChunkSize < 0 {
		o.MinChunkSize = 0
	}
	return o
}
