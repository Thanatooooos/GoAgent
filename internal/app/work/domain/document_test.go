package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestScopedEditsPreserveHumanFormattingAndRejectUnknownBlocks(t *testing.T) {
	d := Document{SchemaVersion: 1, Root: Node{ID: "root", Type: "doc", Content: []Node{
		{ID: "heading", Type: "heading", Level: 2, Content: []Node{{Type: "text", Text: "用户标题", Marks: []string{"bold"}}}},
		{ID: "p1", Type: "paragraph", Content: []Node{{Type: "text", Text: "原段落"}}},
		{ID: "list", Type: "bulletList", Content: []Node{{ID: "li", Type: "listItem", Content: []Node{{ID: "lp", Type: "paragraph", Content: []Node{{Type: "text", Text: "用户列表"}}}}}}},
		{ID: "table", Type: "table", Content: []Node{{ID: "row", Type: "tableRow", Content: []Node{{ID: "cell", Type: "tableCell", Content: []Node{{ID: "cp", Type: "paragraph", Content: []Node{{Type: "text", Text: "用户表格"}}}}}}}}},
	}}}
	before, _ := json.Marshal(d)
	updated, err := d.ApplyBlocks([]BlockChange{{Kind: "replace", TargetID: "p1", Node: &Node{ID: "p1", Type: "paragraph", Content: []Node{{Type: "text", Text: "AI 补充"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 2, 3} {
		if !reflect.DeepEqual(d.Root.Content[i], updated.Root.Content[i]) {
			t.Fatalf("untouched block %d changed", i)
		}
	}
	after, _ := json.Marshal(d)
	if string(before) != string(after) {
		t.Fatal("input document was mutated")
	}
	for _, c := range []BlockChange{{Kind: "delete", TargetID: "root"}, {Kind: "delete", TargetID: "missing"}, {Kind: "delete", TargetID: ""}, {Kind: "replace", TargetID: "p1", Node: &Node{ID: "heading", Type: "paragraph"}}} {
		if _, err := d.ApplyBlocks([]BlockChange{c}); err == nil {
			t.Fatalf("invalid change accepted: %+v", c)
		}
	}
}
func TestDocumentRejectsUnsupportedOrMalformedContent(t *testing.T) {
	for _, n := range []Node{{ID: "x", Type: "script"}, {ID: "p1", Type: "paragraph"}, {ID: "t", Type: "table", Content: []Node{{ID: "r", Type: "tableRow"}}}, {ID: "h", Type: "heading", Level: 7}, {ID: "p2", Type: "paragraph", Content: []Node{{Type: "text", Marks: []string{"html"}}}}} {
		d := EmptyDocument()
		d.Root.Content = append(d.Root.Content, n)
		if err := d.Validate(); err == nil {
			t.Fatalf("invalid content accepted: %+v", n)
		}
	}
	if err := EmptyDocument().Validate(); err != nil {
		t.Fatal(err)
	}
}
