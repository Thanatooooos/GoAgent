package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Document is the only editable body. Text projections are derived, never saved
// as a second authoritative copy. IDs identify blocks within this document.
type Document struct {
	SchemaVersion int  `json:"schemaVersion"`
	Root          Node `json:"root"`
}
type Node struct {
	ID      string   `json:"id,omitempty"`
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	Level   int      `json:"level,omitempty"`
	Marks   []string `json:"marks,omitempty"`
	Content []Node   `json:"content,omitempty"`
}

func EmptyDocument() Document {
	return Document{SchemaVersion: 1, Root: Node{ID: "root", Type: "doc", Content: []Node{{ID: "p1", Type: "paragraph"}}}}
}

func (d Document) Validate() error {
	if d.SchemaVersion != 1 || d.Root.Type != "doc" {
		return fmt.Errorf("unsupported document schema")
	}
	seen := map[string]bool{}
	count := 0
	textBytes := 0
	var walk func(Node, string, int) error
	walk = func(n Node, parent string, depth int) error {
		count++
		textBytes += len(n.Text)
		if depth > 24 || count > 20000 || textBytes > 1_000_000 {
			return fmt.Errorf("document exceeds size or nesting limit")
		}
		if n.Type == "text" {
			if parent != "paragraph" && parent != "heading" {
				return fmt.Errorf("text requires paragraph or heading")
			}
			if len(n.Content) > 0 || n.Level != 0 || n.ID != "" {
				return fmt.Errorf("invalid text node")
			}
			for _, m := range n.Marks {
				if m != "bold" && m != "italic" && m != "code" && m != "strike" {
					return fmt.Errorf("unsupported text mark")
				}
			}
			return nil
		}
		if n.ID == "" || len(n.ID) > 128 || seen[n.ID] {
			return fmt.Errorf("block ids must be unique and non-empty")
		}
		seen[n.ID] = true
		if n.Text != "" || len(n.Marks) > 0 {
			return fmt.Errorf("block cannot carry text or marks")
		}
		switch n.Type {
		case "doc":
			if parent != "" || len(n.Content) == 0 {
				return fmt.Errorf("invalid document root")
			}
		case "paragraph", "heading":
			if parent != "doc" && parent != "listItem" && parent != "tableCell" && parent != "tableHeader" {
				return fmt.Errorf("invalid paragraph parent")
			}
		case "bulletList", "orderedList":
			if parent != "doc" && parent != "listItem" {
				return fmt.Errorf("invalid list parent")
			}
			if len(n.Content) == 0 {
				return fmt.Errorf("empty list")
			}
		case "listItem":
			if parent != "bulletList" && parent != "orderedList" {
				return fmt.Errorf("invalid list item parent")
			}
			if len(n.Content) == 0 {
				return fmt.Errorf("empty list item")
			}
		case "table":
			if parent != "doc" || len(n.Content) == 0 {
				return fmt.Errorf("invalid table")
			}
		case "tableRow":
			if parent != "table" || len(n.Content) == 0 {
				return fmt.Errorf("invalid table row")
			}
		case "tableCell", "tableHeader":
			if parent != "tableRow" || len(n.Content) == 0 {
				return fmt.Errorf("invalid table cell")
			}
		default:
			return fmt.Errorf("unsupported document node %q", n.Type)
		}
		if n.Type == "heading" {
			if n.Level < 1 || n.Level > 3 {
				return fmt.Errorf("heading level must be 1..3")
			}
		} else if n.Level != 0 {
			return fmt.Errorf("level is only valid for headings")
		}
		for _, c := range n.Content {
			switch n.Type {
			case "doc":
				if !(c.Type == "paragraph" || c.Type == "heading" || c.Type == "bulletList" || c.Type == "orderedList" || c.Type == "table") {
					return fmt.Errorf("invalid root child")
				}
			case "paragraph", "heading":
				if c.Type != "text" {
					return fmt.Errorf("inline content must be text")
				}
			case "bulletList", "orderedList":
				if c.Type != "listItem" {
					return fmt.Errorf("list children must be list items")
				}
			case "listItem":
				if !(c.Type == "paragraph" || c.Type == "heading" || c.Type == "bulletList" || c.Type == "orderedList") {
					return fmt.Errorf("invalid list item content")
				}
			case "table":
				if c.Type != "tableRow" {
					return fmt.Errorf("table children must be rows")
				}
			case "tableRow":
				if c.Type != "tableCell" && c.Type != "tableHeader" {
					return fmt.Errorf("row children must be cells")
				}
			case "tableCell", "tableHeader":
				if c.Type != "paragraph" {
					return fmt.Errorf("simple cells contain paragraphs")
				}
			}
			if err := walk(c, n.Type, depth+1); err != nil {
				return err
			}
		}
		if n.Type == "table" {
			width := len(n.Content[0].Content)
			if width > 20 || len(n.Content) > 200 {
				return fmt.Errorf("table exceeds size limit")
			}
			for _, r := range n.Content {
				if len(r.Content) != width {
					return fmt.Errorf("table must be rectangular")
				}
			}
		}
		return nil
	}
	return walk(d.Root, "", 0)
}

func (d Document) Text() string {
	var b strings.Builder
	var walk func(Node)
	walk = func(n Node) {
		if n.Type == "text" {
			b.WriteString(n.Text)
			return
		}
		if n.Type == "heading" {
			b.WriteString(strings.Repeat("#", n.Level) + " ")
		}
		for _, c := range n.Content {
			walk(c)
		}
		switch n.Type {
		case "paragraph", "heading", "listItem", "tableRow":
			b.WriteByte('\n')
		case "tableCell", "tableHeader":
			b.WriteByte('\t')
		}
	}
	walk(d.Root)
	return strings.TrimSpace(b.String())
}

type BlockChange struct {
	Kind     string `json:"kind"`
	TargetID string `json:"targetId"`
	Node     *Node  `json:"node,omitempty"`
}

// ApplyBlocks permits only named block edits. Unchanged nodes retain identity,
// table structure and marks. A whole rewrite must be explicitly authorized.
func (d Document) ApplyBlocks(changes []BlockChange) (Document, error) {
	if err := d.Validate(); err != nil {
		return Document{}, err
	}
	if len(changes) == 0 || len(changes) > 100 {
		return Document{}, fmt.Errorf("block changes must contain 1..100 edits")
	}
	data, _ := json.Marshal(d)
	var result Document
	if err := json.Unmarshal(data, &result); err != nil {
		return Document{}, err
	}
	for _, change := range changes {
		if change.TargetID == "" {
			return Document{}, fmt.Errorf("block target id is required")
		}
		if change.TargetID == result.Root.ID {
			return Document{}, fmt.Errorf("root replacement requires an explicit whole rewrite")
		}
		if change.Kind != "delete" && change.Node == nil {
			return Document{}, fmt.Errorf("block edit requires a node")
		}
		found := false
		var edit func(*Node) error
		edit = func(parent *Node) error {
			for i := 0; i < len(parent.Content); i++ {
				if parent.Content[i].ID == change.TargetID {
					found = true
					switch change.Kind {
					case "replace":
						parent.Content[i] = *change.Node
					case "delete":
						parent.Content = append(parent.Content[:i], parent.Content[i+1:]...)
					case "insertAfter":
						parent.Content = append(parent.Content[:i+1], append([]Node{*change.Node}, parent.Content[i+1:]...)...)
					default:
						return fmt.Errorf("unsupported block edit")
					}
					return nil
				}
				if err := edit(&parent.Content[i]); err != nil {
					return err
				}
				if found {
					return nil
				}
			}
			return nil
		}
		if err := edit(&result.Root); err != nil {
			return Document{}, err
		}
		if !found {
			return Document{}, fmt.Errorf("block %q not found", change.TargetID)
		}
	}
	if err := result.Validate(); err != nil {
		return Document{}, err
	}
	return result, nil
}
