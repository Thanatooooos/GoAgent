package domain

import "fmt"

// PrepareNewDocument supplies server-owned block identities for a new/full
// body. Existing documents are edited through named block operations instead.
// Inline list/cell text is wrapped in a paragraph without changing its content.
func PrepareNewDocument(body Document) Document {
	used := map[string]bool{}
	var collect func(Node, int)
	count := 0
	invalid := false
	collect = func(n Node, depth int) {
		count++
		if depth > 24 || count > 20000 {
			invalid = true
			return
		}
		if n.ID != "" {
			used[n.ID] = true
		}
		for _, child := range n.Content {
			collect(child, depth+1)
		}
	}
	collect(body.Root, 0)
	if invalid {
		return body
	}
	counter := 0
	next := func() string {
		for {
			counter++
			id := fmt.Sprintf("ai-block-%d", counter)
			if !used[id] {
				used[id] = true
				return id
			}
		}
	}
	var normalize func(Node) Node
	normalize = func(n Node) Node {
		n.Marks = append([]string(nil), n.Marks...)
		content := append([]Node(nil), n.Content...)
		n.Content = nil
		if n.Type != "text" && n.ID == "" {
			n.ID = next()
		}
		wrapInline := n.Type == "listItem" || n.Type == "tableCell" || n.Type == "tableHeader"
		var inline []Node
		flush := func() {
			if len(inline) > 0 {
				n.Content = append(n.Content, Node{ID: next(), Type: "paragraph", Content: inline})
				inline = nil
			}
		}
		for _, child := range content {
			child = normalize(child)
			if wrapInline && child.Type == "text" {
				inline = append(inline, child)
			} else {
				flush()
				n.Content = append(n.Content, child)
			}
		}
		flush()
		return n
	}
	body.Root = normalize(body.Root)
	return body
}
