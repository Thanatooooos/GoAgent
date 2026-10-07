package domain

import "testing"

func TestPrepareNewDocumentSuppliesIDsAndWrapsInlineListText(t *testing.T) {
	body := Document{SchemaVersion: 1, Root: Node{Type: "doc", Content: []Node{{Type: "bulletList", Content: []Node{{Type: "listItem", Content: []Node{{Type: "text", Text: "目标", Marks: []string{"bold"}}}}}}}}}
	normalized := PrepareNewDocument(body)
	if err := normalized.Validate(); err != nil {
		t.Fatal(err)
	}
	paragraph := normalized.Root.Content[0].Content[0].Content[0]
	if paragraph.Type != "paragraph" || paragraph.Content[0].Text != "目标" || paragraph.Content[0].Marks[0] != "bold" {
		t.Fatal("content changed")
	}
	if body.Root.ID != "" || body.Root.Content[0].Content[0].Content[0].Type != "text" {
		t.Fatal("input mutated")
	}
	if PrepareNewDocument(body).Root.ID != normalized.Root.ID {
		t.Fatal("new body identities are not deterministic")
	}
	duplicate := EmptyDocument()
	duplicate.Root.Content = append(duplicate.Root.Content, duplicate.Root.Content[0])
	if PrepareNewDocument(duplicate).Validate() == nil {
		t.Fatal("duplicate supplied identities were silently replaced")
	}
}
