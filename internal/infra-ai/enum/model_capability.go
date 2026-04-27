package enum

import "strings"

// ModelCapability indicates the capability type supported by a model.
type ModelCapability string

const (
	ModelCapabilityChat      ModelCapability = "chat"
	ModelCapabilityEmbedding ModelCapability = "embedding"
	ModelCapabilityRerank    ModelCapability = "rerank"
)

var modelCapabilityDisplayNames = map[ModelCapability]string{
	ModelCapabilityChat:      "Chat",
	ModelCapabilityEmbedding: "Embedding",
	ModelCapabilityRerank:    "Rerank",
}

func (c ModelCapability) ID() string {
	return string(c)
}

func (c ModelCapability) DisplayName() string {
	if name, ok := modelCapabilityDisplayNames[c]; ok {
		return name
	}
	return ""
}

func (c ModelCapability) Matches(capability string) bool {
	return strings.EqualFold(strings.TrimSpace(capability), string(c))
}

func ParseModelCapability(capability string) (ModelCapability, bool) {
	capability = strings.TrimSpace(capability)
	for _, item := range AllModelCapabilities() {
		if item.Matches(capability) {
			return item, true
		}
	}
	return "", false
}

func AllModelCapabilities() []ModelCapability {
	return []ModelCapability{
		ModelCapabilityChat,
		ModelCapabilityEmbedding,
		ModelCapabilityRerank,
	}
}
