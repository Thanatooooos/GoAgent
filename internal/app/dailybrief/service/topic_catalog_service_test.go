package service

import (
	"testing"
)

func TestTopicCatalogServiceProjectsHasSourcesOnLeaves(t *testing.T) {
	service := NewTopicCatalogService()
	tree := service.GetTree()
	if len(tree) == 0 {
		t.Fatal("expected non-empty tree")
	}

	var find func(nodes []TopicCatalogNodeReadModel, key string) *TopicCatalogNodeReadModel
	find = func(nodes []TopicCatalogNodeReadModel, key string) *TopicCatalogNodeReadModel {
		for i := range nodes {
			if nodes[i].Key == key {
				return &nodes[i]
			}
			if found := find(nodes[i].Children, key); found != nil {
				return found
			}
		}
		return nil
	}

	models := find(tree, "tech.ai.models")
	if models == nil {
		t.Fatal("expected tech.ai.models in tree")
	}
	if !models.Selectable || !models.HasSources {
		t.Fatalf("expected selectable leaf with sources, got %+v", models)
	}

	tech := find(tree, "tech")
	if tech == nil {
		t.Fatal("expected tech in tree")
	}
	if tech.Selectable {
		t.Fatal("expected tech to be navigation-only")
	}
}
