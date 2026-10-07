package capability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/core/citation"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/app/runtime/persistence"
)

const RetrieveKnowledgeID = "retrieve_knowledge"

var retrieveKnowledgeSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","minLength":1},"top_k":{"type":"integer","minimum":1,"maximum":20}}}`)

type retrieveKnowledgeArgs struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

func RetrieveKnowledge(service ragretrieve.Service) Def {
	return Def{
		ID: RetrieveKnowledgeID, Description: retrieveKnowledgeDescription, JSONSchema: retrieveKnowledgeSchema,
		Validate: validateRetrieveKnowledge,
		Describe: func(value Value, ctx Context) (Operation, error) {
			args, err := parseRetrieveKnowledgeArgs(value)
			if err != nil {
				return Operation{}, err
			}
			if !ctx.AllowKnowledgeRetrieval || len(ctx.KnowledgeBaseIDs) == 0 {
				return Operation{}, Deny("knowledge retrieval is not permitted")
			}
			input, _ := json.Marshal(struct {
				Query            string   `json:"query"`
				KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
				TopK             int      `json:"top_k"`
			}{args.Query, append([]string(nil), ctx.KnowledgeBaseIDs...), args.TopK})
			return Operation{ID: RetrieveKnowledgeID, Summary: "retrieve knowledge evidence", Input: input}, nil
		},
		Execute: func(value Value, ctx Context) (Result, error) {
			if service == nil {
				return Result{}, fmt.Errorf("knowledge retrieve service is required")
			}
			args, err := parseRetrieveKnowledgeArgs(value)
			if err != nil {
				return Result{}, err
			}
			if !ctx.AllowKnowledgeRetrieval || len(ctx.KnowledgeBaseIDs) == 0 {
				return Result{}, fmt.Errorf("knowledge retrieval is not permitted")
			}
			retrieved, err := service.Retrieve(ctx.Context, ragretrieve.Request{DocumentsOnly: ctx.Work != nil, UserID: ctx.UserID, Query: args.Query, KnowledgeBaseIDs: append([]string(nil), ctx.KnowledgeBaseIDs...), TopK: args.TopK, SearchMode: ctx.RetrieveSearchMode})
			if err != nil {
				return Result{}, fmt.Errorf("retrieve knowledge: %w", err)
			}
			type item struct {
				ID              string  `json:"id"`
				Text            string  `json:"text"`
				Score           float32 `json:"score"`
				DocumentID      string  `json:"document_id"`
				KnowledgeBaseID string  `json:"knowledge_base_id"`
			}
			items := make([]item, 0, len(retrieved.Chunks))
			evidence := make([]persistence.EvidenceRef, 0, len(retrieved.Chunks))
			for _, chunk := range retrieved.Chunks {
				items = append(items, item{chunk.ID, chunk.Text, chunk.Score, chunk.DocumentID, chunk.KnowledgeBaseID})
				evidence = append(evidence, persistence.EvidenceRef{ID: chunk.ID, Kind: "knowledge_chunk", SourceID: chunk.KnowledgeBaseID, DocumentID: chunk.DocumentID})
			}
			valueOut, err := json.Marshal(struct {
				Query  string `json:"query"`
				Chunks []item `json:"chunks"`
			}{args.Query, items})
			if err != nil {
				return Result{}, fmt.Errorf("encode retrieve knowledge result: %w", err)
			}
			return Result{Content: citation.RenderKnowledgeContext(ctx.Citations, retrieved.Chunks), Value: valueOut, Evidence: evidence}, nil
		},
	}
}

func validateRetrieveKnowledge(value Value) error {
	_, err := parseRetrieveKnowledgeArgs(value)
	return err
}
func parseRetrieveKnowledgeArgs(value Value) (retrieveKnowledgeArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var args retrieveKnowledgeArgs
	if err := decoder.Decode(&args); err != nil {
		return retrieveKnowledgeArgs{}, fmt.Errorf("invalid arguments: %w", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return retrieveKnowledgeArgs{}, fmt.Errorf("query is required")
	}
	if args.TopK == 0 {
		args.TopK = ragretrieve.DefaultTopK
	}
	if args.TopK < 1 || args.TopK > 20 {
		return retrieveKnowledgeArgs{}, fmt.Errorf("top_k must be between 1 and 20")
	}
	return args, nil
}
