package work

import (
	"context"

	conversationruntime "local/rag-project/internal/app/runtime"
)

type History struct{ Store *Store }

func (h History) Messages(ctx context.Context, request conversationruntime.RunRequest) ([]conversationruntime.ModelMessage, error) {
	snapshot, err := h.Snapshot(ctx, request)
	if err != nil {
		return nil, err
	}
	out := []conversationruntime.ModelMessage{}
	if snapshot.Summary.Content != "" {
		out = append(out, conversationruntime.ModelMessage{Role: conversationruntime.ModelRoleSystem, Content: workHistorySummaryInstruction + snapshot.Summary.Content})
	}
	size := 0
	recent := []conversationruntime.ModelMessage{}
	for i := len(snapshot.Messages) - 1; i >= 0 && len(recent) < 40; i-- {
		m := snapshot.Messages[i]
		if size+len(m.Content) > 60000 {
			continue
		}
		size += len(m.Content)
		recent = append(recent, conversationruntime.ModelMessage{Role: m.Role, Content: m.Content})
	}
	for i := len(recent) - 1; i >= 0; i-- {
		out = append(out, recent[i])
	}
	return out, nil
}
