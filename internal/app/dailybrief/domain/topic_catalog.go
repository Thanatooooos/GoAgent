package domain

import (
	"sort"
	"strings"
)

type TopicNode struct {
	Key         string
	ParentKey   string
	DisplayName string
	Description string
	SortOrder   int
	Selectable  bool
	Enabled     bool
	Children    []TopicNode
}

const (
	TopicKeyTech           = "tech"
	TopicKeyTechAI         = "tech.ai"
	TopicKeyTechAIModels   = "tech.ai.models"
	TopicKeyTechAIResearch = "tech.ai.research"
	TopicKeyTechAITools    = "tech.ai.tools"
	TopicKeyTechDev        = "tech.dev"
	TopicKeyTechStartups   = "tech.startups"

	TopicKeyArt              = "art"
	TopicKeyArtContemporary  = "art.contemporary"
	TopicKeyArtDesign        = "art.design"
	TopicKeyArtFilm          = "art.film"
	TopicKeyArtPhotography   = "art.photography"
	TopicKeyArtArchitecture  = "art.architecture"
	TopicKeyArtDigital       = "art.digital"

	TopicKeyMusic            = "music"
	TopicKeyMusicIndustry    = "music.industry"
	TopicKeyMusicElectronic  = "music.electronic"
	TopicKeyMusicRockPop     = "music.rock-pop"
	TopicKeyMusicClassical   = "music.classical"
	TopicKeyMusicTech        = "music.tech"
	TopicKeyMusicLive        = "music.live"

	TopicKeyPolitics              = "politics"
	TopicKeyPoliticsChina         = "politics.china"
	TopicKeyPoliticsGlobal        = "politics.global"
	TopicKeyPoliticsEconomyPolicy = "politics.economy-policy"
	TopicKeyPoliticsTechPolicy    = "politics.tech-policy"
	TopicKeyPoliticsEnergy        = "politics.energy"
	TopicKeyPoliticsElections     = "politics.elections"
)

var topicCatalogFlat = []TopicNode{
	{Key: TopicKeyTech, DisplayName: "科技", Description: "AI、开发者、创业与产业", SortOrder: 10, Enabled: true},
	{Key: TopicKeyTechAI, ParentKey: TopicKeyTech, DisplayName: "AI", Description: "模型、研究与工具", SortOrder: 20, Enabled: true},
	{Key: TopicKeyTechAIModels, ParentKey: TopicKeyTechAI, DisplayName: "模型发布", SortOrder: 30, Selectable: true, Enabled: true},
	{Key: TopicKeyTechAIResearch, ParentKey: TopicKeyTechAI, DisplayName: "研究进展", SortOrder: 40, Selectable: true, Enabled: true},
	{Key: TopicKeyTechAITools, ParentKey: TopicKeyTechAI, DisplayName: "工具与论文", SortOrder: 50, Selectable: true, Enabled: true},
	{Key: TopicKeyTechDev, ParentKey: TopicKeyTech, DisplayName: "开发者与开源", SortOrder: 60, Selectable: true, Enabled: true},
	{Key: TopicKeyTechStartups, ParentKey: TopicKeyTech, DisplayName: "创业与产业", SortOrder: 70, Selectable: true, Enabled: true},

	{Key: TopicKeyArt, DisplayName: "艺术", Description: "视觉、设计、电影与当代艺术", SortOrder: 80, Enabled: true},
	{Key: TopicKeyArtContemporary, ParentKey: TopicKeyArt, DisplayName: "当代艺术", Description: "展览、艺术家、艺术市场", SortOrder: 81, Selectable: true, Enabled: true},
	{Key: TopicKeyArtDesign, ParentKey: TopicKeyArt, DisplayName: "设计与创意", Description: "平面、工业、UX", SortOrder: 82, Selectable: true, Enabled: true},
	{Key: TopicKeyArtFilm, ParentKey: TopicKeyArt, DisplayName: "电影与影像", Description: "院线、流媒体、制作", SortOrder: 83, Selectable: true, Enabled: true},
	{Key: TopicKeyArtPhotography, ParentKey: TopicKeyArt, DisplayName: "摄影", Description: "纪实、商业、器材", SortOrder: 84, Selectable: true, Enabled: true},
	{Key: TopicKeyArtArchitecture, ParentKey: TopicKeyArt, DisplayName: "建筑与空间", Description: "城市、地标、室内设计", SortOrder: 85, Selectable: true, Enabled: true},
	{Key: TopicKeyArtDigital, ParentKey: TopicKeyArt, DisplayName: "数字艺术", Description: "生成艺术、新媒体创作", SortOrder: 86, Selectable: true, Enabled: true},

	{Key: TopicKeyMusic, DisplayName: "音乐", Description: "创作、产业、技术与现场文化", SortOrder: 90, Enabled: true},
	{Key: TopicKeyMusicIndustry, ParentKey: TopicKeyMusic, DisplayName: "音乐产业", Description: "版权、流媒体、商业", SortOrder: 91, Selectable: true, Enabled: true},
	{Key: TopicKeyMusicElectronic, ParentKey: TopicKeyMusic, DisplayName: "电子音乐", Description: "制作、巡演、厂牌", SortOrder: 92, Selectable: true, Enabled: true},
	{Key: TopicKeyMusicRockPop, ParentKey: TopicKeyMusic, DisplayName: "流行与摇滚", Description: "专辑、艺人、现场", SortOrder: 93, Selectable: true, Enabled: true},
	{Key: TopicKeyMusicClassical, ParentKey: TopicKeyMusic, DisplayName: "古典与实验", Description: "古典、先锋、剧场", SortOrder: 94, Selectable: true, Enabled: true},
	{Key: TopicKeyMusicTech, ParentKey: TopicKeyMusic, DisplayName: "音乐科技", Description: "制作工具、AI 音乐、硬件", SortOrder: 95, Selectable: true, Enabled: true},
	{Key: TopicKeyMusicLive, ParentKey: TopicKeyMusic, DisplayName: "现场与音乐节", Description: "演出、节庆、票务", SortOrder: 96, Selectable: true, Enabled: true},

	{Key: TopicKeyPolitics, DisplayName: "时政", Description: "国内外政策、地缘与公共议题", SortOrder: 100, Enabled: true},
	{Key: TopicKeyPoliticsChina, ParentKey: TopicKeyPolitics, DisplayName: "国内时政", Description: "政策、治理、社会议题", SortOrder: 101, Selectable: true, Enabled: true},
	{Key: TopicKeyPoliticsGlobal, ParentKey: TopicKeyPolitics, DisplayName: "国际局势", Description: "地缘、外交、冲突", SortOrder: 102, Selectable: true, Enabled: true},
	{Key: TopicKeyPoliticsEconomyPolicy, ParentKey: TopicKeyPolitics, DisplayName: "经济政策", Description: "财政、货币、监管", SortOrder: 103, Selectable: true, Enabled: true},
	{Key: TopicKeyPoliticsTechPolicy, ParentKey: TopicKeyPolitics, DisplayName: "科技政策", Description: "AI 监管、数据、反垄断", SortOrder: 104, Selectable: true, Enabled: true},
	{Key: TopicKeyPoliticsEnergy, ParentKey: TopicKeyPolitics, DisplayName: "能源与气候政治", Description: "碳排、能源安全", SortOrder: 105, Selectable: true, Enabled: true},
	{Key: TopicKeyPoliticsElections, ParentKey: TopicKeyPolitics, DisplayName: "选举与政党", Description: "重要选举、民调", SortOrder: 106, Selectable: true, Enabled: true},
}

var topicNodeByKey = func() map[string]TopicNode {
	result := make(map[string]TopicNode, len(topicCatalogFlat))
	for _, node := range topicCatalogFlat {
		result[node.Key] = node
	}
	return result
}()

func TopicCatalogFlat() []TopicNode {
	return append([]TopicNode(nil), topicCatalogFlat...)
}

func TopicCatalogTree() []TopicNode {
	childrenByParent := make(map[string][]TopicNode)
	for _, node := range topicCatalogFlat {
		childrenByParent[node.ParentKey] = append(childrenByParent[node.ParentKey], node)
	}
	var build func(parentKey string) []TopicNode
	build = func(parentKey string) []TopicNode {
		children := append([]TopicNode(nil), childrenByParent[parentKey]...)
		sort.Slice(children, func(i, j int) bool {
			return children[i].SortOrder < children[j].SortOrder
		})
		result := make([]TopicNode, 0, len(children))
		for _, child := range children {
			child.Children = build(child.Key)
			result = append(result, child)
		}
		return result
	}
	return build("")
}

func IsTopicKeyKnown(key string) bool {
	_, ok := topicNodeByKey[key]
	return ok
}

func IsLeafTopicKey(key string) bool {
	node, ok := topicNodeByKey[key]
	return ok && node.Selectable
}

func IsTopicKeySelectable(key string) bool {
	node, ok := topicNodeByKey[key]
	return ok && node.Selectable && node.Enabled
}

// IsTopicKeySupported reports whether key is an enabled selectable leaf topic.
func IsTopicKeySupported(key string) bool {
	return IsTopicKeySelectable(key)
}

func TopicDisplayName(key string) string {
	node, ok := topicNodeByKey[key]
	if !ok {
		return key
	}
	return node.DisplayName
}

// TopicDisplayTitle returns the Simplified Chinese section title for generation and UI.
func TopicDisplayTitle(key string) string {
	return TopicDisplayName(key)
}

func TopicBreadcrumb(key string) string {
	node, ok := topicNodeByKey[key]
	if !ok {
		return key
	}
	parts := []string{node.DisplayName}
	parentKey := node.ParentKey
	for parentKey != "" {
		parent := topicNodeByKey[parentKey]
		parts = append([]string{parent.DisplayName}, parts...)
		parentKey = parent.ParentKey
	}
	return strings.Join(parts, " · ")
}

func TopicKeys() []string {
	return LeafTopicKeys()
}

func LeafTopicKeys() []string {
	keys := make([]string, 0)
	for _, node := range topicCatalogFlat {
		if node.Selectable && node.Enabled {
			keys = append(keys, node.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

func SourceKeysForTopics(topics []string) []string {
	allowed := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if IsTopicKeySelectable(topic) {
			allowed[topic] = struct{}{}
		}
	}
	keys := make([]string, 0)
	seen := make(map[string]struct{})
	for _, spec := range DefaultSourceFeedSpecs() {
		if _, ok := allowed[spec.Topic]; !ok {
			continue
		}
		if _, dup := seen[spec.Key]; dup {
			continue
		}
		seen[spec.Key] = struct{}{}
		keys = append(keys, spec.Key)
	}
	sort.Strings(keys)
	return keys
}

func TopicHasSources(key string) bool {
	for _, spec := range DefaultSourceFeedSpecs() {
		if spec.Topic == key {
			return true
		}
	}
	return false
}

func TopicForSourceKey(key string) string {
	spec, ok := SourceFeedSpecByKey(key)
	if !ok {
		return ""
	}
	return spec.Topic
}
