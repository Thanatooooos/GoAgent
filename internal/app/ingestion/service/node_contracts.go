package service

import (
	"sort"
	"strings"

	"local/rag-project/internal/app/ingestion/domain"
)

// NodeInputRequirement describes a single input requirement for a pipeline node.
// AnyOf means any artifact in the set can satisfy the requirement.
type NodeInputRequirement struct {
	AnyOf       []string
	Description string
}

// NodeIOContract describes the required inputs and produced outputs of a node type.
type NodeIOContract struct {
	NodeType      string
	Requires      []NodeInputRequirement
	Produces      []string
	DisplayName   string
	Summary       string
	Category      string
	SupportsEntry bool
}

func defaultNodeIOContracts() map[string]NodeIOContract {
	return map[string]NodeIOContract{
		domain.PipelineNodeTypeFetcher: {
			NodeType:      domain.PipelineNodeTypeFetcher,
			DisplayName:   "Fetcher",
			Summary:       "Fetches raw document content from file, URL, S3 or Feishu.",
			Category:      "source",
			Produces:      []string{"source"},
			SupportsEntry: true,
		},
		domain.PipelineNodeTypeParser: {
			NodeType:    domain.PipelineNodeTypeParser,
			DisplayName: "Parser",
			Summary:     "Parses raw source content into normalized document text.",
			Category:    "transform",
			Requires: []NodeInputRequirement{
				{AnyOf: []string{"source"}, Description: "Requires raw source content."},
			},
			Produces: []string{"parsed"},
		},
		domain.PipelineNodeTypeEnhancer: {
			NodeType:    domain.PipelineNodeTypeEnhancer,
			DisplayName: "Enhancer",
			Summary:     "Enhances parsed content or chunks with extra context and semantic hints.",
			Category:    "enhance",
			Requires: []NodeInputRequirement{
				{AnyOf: []string{"parsed", "chunks"}, Description: "Requires parsed document text or generated chunks."},
			},
			Produces: []string{"parsed", "chunks", "enhancer"},
		},
		domain.PipelineNodeTypeChunker: {
			NodeType:    domain.PipelineNodeTypeChunker,
			DisplayName: "Chunker",
			Summary:     "Splits parsed document text into indexable chunks.",
			Category:    "transform",
			Requires: []NodeInputRequirement{
				{AnyOf: []string{"parsed"}, Description: "Requires parsed document text."},
			},
			Produces: []string{"chunks"},
		},
		domain.PipelineNodeTypeEnricher: {
			NodeType:    domain.PipelineNodeTypeEnricher,
			DisplayName: "Enricher",
			Summary:     "Enriches chunks with summaries, keywords and structured metadata.",
			Category:    "enhance",
			Requires: []NodeInputRequirement{
				{AnyOf: []string{"chunks"}, Description: "Requires generated chunks."},
			},
			Produces: []string{"chunks", "enricher"},
		},
		domain.PipelineNodeTypeIndexer: {
			NodeType:    domain.PipelineNodeTypeIndexer,
			DisplayName: "Indexer",
			Summary:     "Embeds chunks and writes vectors and knowledge chunks downstream.",
			Category:    "sink",
			Requires: []NodeInputRequirement{
				{AnyOf: []string{"chunks"}, Description: "Requires generated chunks."},
			},
			Produces: []string{"index"},
		},
	}
}

// ListNodeIOContracts returns the built-in pipeline node contracts in stable order.
func ListNodeIOContracts() []NodeIOContract {
	return listNodeIOContracts()
}

// GetNodeIOContract returns a single built-in node contract by node type.
func GetNodeIOContract(nodeType string) (NodeIOContract, bool) {
	return getNodeIOContract(nodeType)
}

func getNodeIOContract(nodeType string) (NodeIOContract, bool) {
	nodeType = strings.TrimSpace(nodeType)
	if nodeType == "" {
		return NodeIOContract{}, false
	}
	contract, ok := defaultNodeIOContracts()[nodeType]
	return contract, ok
}

func listNodeIOContracts() []NodeIOContract {
	contracts := defaultNodeIOContracts()
	keys := make([]string, 0, len(contracts))
	for key := range contracts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]NodeIOContract, 0, len(keys))
	for _, key := range keys {
		result = append(result, contracts[key])
	}
	return result
}

func cloneArtifactSet(values map[string]struct{}) map[string]struct{} {
	if len(values) == 0 {
		return map[string]struct{}{}
	}
	cloned := make(map[string]struct{}, len(values))
	for key := range values {
		cloned[key] = struct{}{}
	}
	return cloned
}

func mergeArtifactSets(base map[string]struct{}, additions ...map[string]struct{}) map[string]struct{} {
	merged := cloneArtifactSet(base)
	for _, addition := range additions {
		for key := range addition {
			merged[key] = struct{}{}
		}
	}
	return merged
}

func artifactSetFromNames(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}
	return result
}

func artifactSetContainsAny(values map[string]struct{}, candidates []string) bool {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := values[candidate]; ok {
			return true
		}
	}
	return false
}

func artifactSetNames(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	names := make([]string, 0, len(values))
	for key := range values {
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}
