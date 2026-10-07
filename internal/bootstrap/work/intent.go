package work

import (
	"context"
	"encoding/json"
	"fmt"
	conversationruntime "local/rag-project/internal/app/runtime"
)

type Intent struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func (r *Runtime) ResolveIntent(ctx context.Context, user, topic, question, artifact string) (Intent, error) {
	if _, err := r.Store.GetTopic(ctx, user, topic); err != nil {
		return Intent{}, err
	}
	if question == "" || len(question) > 60000 {
		return Intent{}, fmt.Errorf("invalid question")
	}
	if artifact != "" {
		if _, err := r.Store.GetArtifact(ctx, user, topic, artifact); err != nil {
			return Intent{}, err
		}
	}
	if r.Kernel == nil || r.Kernel.Model == nil {
		return Intent{}, fmt.Errorf("Work model unavailable")
	}
	raw, _ := json.Marshal(struct{ Question, FocusedDocument string }{question, artifact})
	turn, err := r.Kernel.Model.Stream(ctx, conversationruntime.ModelRequest{JSONMode: true, System: []string{workIntentSystemPrompt}, Messages: []conversationruntime.ModelMessage{{Role: conversationruntime.ModelRoleUser, Content: string(raw)}}}, func(conversationruntime.ModelEvent) error { return nil })
	if err != nil {
		return Intent{}, err
	}
	var out Intent
	if err := json.Unmarshal([]byte(turn.Content), &out); err != nil {
		return Intent{}, fmt.Errorf("无法识别本轮意图，请选择工作方式")
	}
	switch out.Action {
	case "discuss", "create_document":
	case "edit_document", "rewrite_document":
		if artifact == "" {
			out.Action = "discuss"
		}
	default:
		out.Action = "discuss"
	}
	return out, nil
}
