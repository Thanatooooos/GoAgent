package chunk

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
