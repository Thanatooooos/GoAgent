package chunk

const (
	defaultParentChunkSize   = 800
	defaultParentOverlapSize = 120
	defaultChildChunkSize    = 300
	defaultChildOverlapSize  = 60
)

// ParentChildOptions 由显式参数构造父子分块选项；参数为 0 时使用默认父子尺寸
// （parent 800/120，child 300/60）。strategy 同时应用到父块与子块。
// 注意：默认 overlap 只在对应 chunkSize 同样未指定（=0）时应用，显式给出尺寸但
// 不给 overlap 时 overlap 保持 0。
func ParentChildOptions(strategy Strategy, parentChunkSize, parentOverlapSize, childChunkSize, childOverlapSize int) (Options, Options) {
	parent := Options{Strategy: strategy}
	if parent.ChunkSize = parentChunkSize; parent.ChunkSize <= 0 {
		parent.ChunkSize = defaultParentChunkSize
		if parentOverlapSize == 0 {
			parentOverlapSize = defaultParentOverlapSize
		}
	}
	if parent.OverlapSize = parentOverlapSize; parent.OverlapSize < 0 {
		parent.OverlapSize = 0
	}

	child := Options{Strategy: strategy}
	if child.ChunkSize = childChunkSize; child.ChunkSize <= 0 {
		child.ChunkSize = defaultChildChunkSize
		if childOverlapSize == 0 {
			childOverlapSize = defaultChildOverlapSize
		}
	}
	if child.OverlapSize = childOverlapSize; child.OverlapSize < 0 {
		child.OverlapSize = 0
	}

	return parent.Normalize(), child.Normalize()
}

// ParentChildResult contains large parent chunks for answer context and small
// child chunks for retrieval. Child indexes are global within the document.
type ParentChildResult struct {
	Parents  []Chunk
	Children []ChildChunk
}

// ChildChunk associates a retrievable chunk with a parent result index.
type ChildChunk struct {
	Chunk
	ParentIndex int
}

// SplitParentChild first splits text into parent chunks, then splits each
// parent into child chunks using the same registered chunking strategies.
func SplitParentChild(text string, parentOptions Options, childOptions Options) (ParentChildResult, error) {
	selector := NewDefaultSelector()
	parents, err := selector.Chunk(text, parentOptions)
	if err != nil {
		return ParentChildResult{}, err
	}

	result := ParentChildResult{Parents: parents}
	childIndex := 0
	for parentIndex, parent := range parents {
		children, err := selector.Chunk(parent.Text, childOptions)
		if err != nil {
			return ParentChildResult{}, err
		}
		for _, child := range children {
			child.Index = childIndex
			result.Children = append(result.Children, ChildChunk{
				Chunk:       child,
				ParentIndex: parentIndex,
			})
			childIndex++
		}
	}
	return result, nil
}
