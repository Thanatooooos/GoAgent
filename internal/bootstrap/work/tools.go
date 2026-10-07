package work

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/work/domain"
	"strings"
)

type documentTextSuggestion struct {
	ArtifactID string `json:"artifactId"`
	TargetID   string `json:"targetId"`
	Text       string `json:"text"`
	Summary    string `json:"summary"`
}
type newDocumentSuggestion struct {
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Paragraphs []string `json:"paragraphs"`
}

// validateDraftParagraphs catches a model response that is structurally valid
// JSON but ends a paragraph at a dangling connector. Without this check the
// incomplete text is persisted as a valid document node and the model receives
// no signal to finish the sentence.
func validateDraftParagraphs(paragraphs []string) error {
	for i, paragraph := range paragraphs {
		text := strings.TrimSpace(paragraph)
		if text == "" {
			continue
		}
		dangling := []string{"从", "和", "或", "与", "及", "把", "将", "对", "在", "为", "以", "而", "并", "但", "因", "如果", "当", "让"}
		for _, suffix := range dangling {
			if !strings.HasSuffix(text, suffix) {
				continue
			}
			return fmt.Errorf("paragraph %d appears incomplete; finish the sentence before suggesting the document", i+1)
		}
		if strings.HasSuffix(text, "…") || strings.HasSuffix(text, "...") {
			return fmt.Errorf("paragraph %d appears truncated; finish the sentence before suggesting the document", i+1)
		}
	}
	return nil
}

func decodeArgs[T any](v capability.Value) (T, error) {
	var out T
	d := json.NewDecoder(bytes.NewReader(v))
	d.DisallowUnknownFields()
	err := d.Decode(&out)
	if err != nil {
		return out, err
	}
	if d.Decode(new(any)) != io.EOF {
		return out, fmt.Errorf("one object required")
	}
	return out, nil
}
func workTool[T any](id, description, schema string, run func(T, capability.Context) (any, error)) capability.Def {
	return capability.Def{ID: id, Description: description, JSONSchema: json.RawMessage(schema), Validate: func(v capability.Value) error { _, err := decodeArgs[T](v); return err }, Describe: func(v capability.Value, c capability.Context) (capability.Operation, error) {
		if c.Work == nil {
			return capability.Operation{}, capability.Deny("Work scope required")
		}
		return capability.Operation{ID: id, Input: v}, nil
	}, Execute: func(v capability.Value, c capability.Context) (capability.Result, error) {
		in, err := decodeArgs[T](v)
		if err != nil {
			return capability.Result{}, err
		}
		out, err := run(in, c)
		if err != nil {
			return capability.Result{}, err
		}
		raw, err := json.Marshal(out)
		return capability.Result{Content: string(raw), Value: raw}, err
	}}
}
func (r *Runtime) registerTools(registry *capability.Registry) error {
	defs := []capability.Def{
		workTool[struct{}]("work_list_materials", workListMaterialsDescription, workListMaterialsSchema, func(_ struct{}, c capability.Context) (any, error) {
			return r.Store.ListSources(c.Context, c.UserID, c.Work.TopicID, c.ConversationID, domain.Page{Limit: 100})
		}),
		workTool[struct {
			SourceID string `json:"sourceId"`
		}]("work_read_material", workReadMaterialDescription, workReadMaterialSchema, func(in struct {
			SourceID string `json:"sourceId"`
		}, c capability.Context) (any, error) {
			text, err := r.Store.SourceText(c.Context, c.UserID, c.Work.TopicID, in.SourceID, c.ConversationID)
			return map[string]string{"sourceId": in.SourceID, "text": text}, err
		}),
		workTool[struct {
			AllItems bool `json:"allItems"`
		}]("work_read_progress", workReadProgressDescription, workReadProgressSchema, func(in struct {
			AllItems bool `json:"allItems"`
		}, c capability.Context) (any, error) {
			state, err := r.Store.GetState(c.Context, c.UserID, c.Work.TopicID)
			if err != nil || in.AllItems {
				return state, err
			}
			entries := []domain.StateEntry{}
			for _, entry := range state.Entries {
				if entry.ItemID == "" || entry.ItemID == c.Work.ItemID {
					entries = append(entries, entry)
				}
			}
			state.Entries = entries
			return state, nil
		}),
		workTool[struct {
			ArtifactID string `json:"artifactId"`
			Revision   int    `json:"revision"`
		}]("work_read_document", workReadDocumentDescription, workReadDocumentSchema, func(in struct {
			ArtifactID string `json:"artifactId"`
			Revision   int    `json:"revision"`
		}, c capability.Context) (any, error) {
			if in.Revision == 0 && in.ArtifactID == c.Work.ArtifactID {
				in.Revision = c.Work.ArtifactRevision
			}
			if in.Revision > 0 {
				return r.Store.GetVersion(c.Context, c.UserID, c.Work.TopicID, in.ArtifactID, in.Revision)
			}
			return r.Store.GetArtifact(c.Context, c.UserID, c.Work.TopicID, in.ArtifactID)
		}),
		workTool[struct {
			Query          string `json:"query"`
			ConversationID string `json:"conversationId"`
			AllItems       bool   `json:"allItems"`
		}]("work_read_history", workReadHistoryDescription, workReadHistorySchema, func(in struct {
			Query          string `json:"query"`
			ConversationID string `json:"conversationId"`
			AllItems       bool   `json:"allItems"`
		}, c capability.Context) (any, error) {
			if in.AllItems {
				return r.Store.SearchHistoryAcrossItems(c.Context, c.UserID, c.Work.TopicID, in.ConversationID, in.Query)
			}
			return r.Store.SearchHistory(c.Context, c.UserID, c.Work.TopicID, c.Work.ItemID, in.ConversationID, in.Query)
		}),
		workTool[domain.AIDocumentChange]("work_write_document", workWriteDocumentDescription, documentSchema, func(in domain.AIDocumentChange, c capability.Context) (any, error) {
			return r.Store.WriteDocument(c, in)
		}),
		workTool[documentTextSuggestion]("work_suggest_document", workSuggestDocumentDescription, workSuggestDocumentSchema, func(in documentTextSuggestion, c capability.Context) (any, error) {
			return r.Store.SuggestDocumentText(c, in.ArtifactID, in.TargetID, in.Text, in.Summary)
		}),
		workTool[newDocumentSuggestion]("work_suggest_new_document", workSuggestNewDocumentDescription, workSuggestNewDocumentSchema, func(in newDocumentSuggestion, c capability.Context) (any, error) {
			if len(in.Paragraphs) < 1 || len(in.Paragraphs) > 100 {
				return nil, fmt.Errorf("draft needs 1..100 paragraphs")
			}
			if err := validateDraftParagraphs(in.Paragraphs); err != nil {
				return nil, err
			}
			body := domain.EmptyDocument()
			body.Root.Content = nil
			for i, text := range in.Paragraphs {
				node := domain.Node{ID: fmt.Sprintf("proposed-p-%d", i), Type: "paragraph"}
				if text != "" {
					node.Content = []domain.Node{{Type: "text", Text: text}}
				}
				body.Root.Content = append(body.Root.Content, node)
			}
			return r.Store.SuggestDocument(c, domain.SuggestDocument{Change: domain.AIDocumentChange{Title: in.Title, Summary: in.Summary, Body: &body}})
		}),
		workTool[struct {
			Changes []domain.StateChange `json:"changes"`
		}]("work_suggest_progress", workSuggestProgressDescription, workSuggestProgressSchema, func(in struct {
			Changes []domain.StateChange `json:"changes"`
		}, c capability.Context) (any, error) {
			return r.Store.SuggestProgress(c, in.Changes)
		}),
	}
	for _, d := range defs {
		if err := registry.Register(d); err != nil {
			return err
		}
	}
	return nil
}

const documentSchema = `{"type":"object","properties":{"title":{"type":"string"},"summary":{"type":"string"},"body":{"type":"object","required":["schemaVersion","root"],"properties":{"schemaVersion":{"type":"integer","enum":[1]},"root":{"$ref":"#/$defs/node"}},"additionalProperties":false},"changes":{"type":"array","items":{"type":"object","required":["kind","targetId"],"properties":{"kind":{"type":"string","enum":["replace","delete","insertAfter"]},"targetId":{"type":"string"},"node":{"$ref":"#/$defs/node"}},"additionalProperties":false}}},"required":["summary"],"additionalProperties":false,"$defs":{"node":{"type":"object","required":["type"],"properties":{"id":{"type":"string"},"type":{"type":"string","enum":["doc","paragraph","heading","text","bulletList","orderedList","listItem","table","tableRow","tableCell","tableHeader"]},"text":{"type":"string"},"level":{"type":"integer"},"marks":{"type":"array","items":{"type":"string","enum":["bold","italic","code","strike"]}},"content":{"type":"array","items":{"$ref":"#/$defs/node"}}},"additionalProperties":false}}}`
