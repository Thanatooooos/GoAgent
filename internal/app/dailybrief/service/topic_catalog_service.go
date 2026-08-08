package service

import (
	"local/rag-project/internal/app/dailybrief/domain"
)

type TopicCatalogNodeReadModel struct {
	Key         string                      `json:"key"`
	DisplayName string                      `json:"displayName"`
	Description string                      `json:"description"`
	Selectable  bool                        `json:"selectable"`
	Enabled     bool                        `json:"enabled"`
	HasSources  bool                        `json:"hasSources,omitempty"`
	Children    []TopicCatalogNodeReadModel `json:"children"`
}

type TopicCatalogService struct{}

func NewTopicCatalogService() *TopicCatalogService {
	return &TopicCatalogService{}
}

func (s *TopicCatalogService) GetTree() []TopicCatalogNodeReadModel {
	var mapNode func(node domain.TopicNode) TopicCatalogNodeReadModel
	mapNode = func(node domain.TopicNode) TopicCatalogNodeReadModel {
		children := make([]TopicCatalogNodeReadModel, 0, len(node.Children))
		for _, child := range node.Children {
			children = append(children, mapNode(child))
		}
		return TopicCatalogNodeReadModel{
			Key:         node.Key,
			DisplayName: node.DisplayName,
			Description: node.Description,
			Selectable:  node.Selectable,
			Enabled:     node.Enabled,
			HasSources:  node.Selectable && domain.TopicHasSources(node.Key),
			Children:    children,
		}
	}

	tree := domain.TopicCatalogTree()
	result := make([]TopicCatalogNodeReadModel, 0, len(tree))
	for _, node := range tree {
		result = append(result, mapNode(node))
	}
	return result
}
