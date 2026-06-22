package chat

import (
	"context"

	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

func (s *RagChatService) runRewriteStage(ctx context.Context, question string, history []convention.ChatMessage, traceID string) (ragChatRewriteStageResult, error) {
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatRewriteStageResult]{
		node: ragChatTraceNode{
			NodeID:   "rewrite",
			NodeType: "rewrite",
			NodeName: "query_rewrite",
		},
		run: func(context.Context) (ragChatRewriteStageResult, error) {
			if s.rewriteService == nil {
				result := ragrewrite.Result{
					RewrittenQuestion: question,
					SubQuestions:      []string{question},
					NeedRetrieval:     ragrewrite.InferNeedRetrieval(question),
				}
				return ragChatRewriteStageResult{result: result}, nil
			}
			result := s.rewriteService.RewriteWithHistory(question, history)
			return ragChatRewriteStageResult{result: result}, nil
		},
		buildExtra: func(result ragChatRewriteStageResult) map[string]any {
			return map[string]any{
				"subQuestionCount": len(result.result.SubQuestions),
			}
		},
	})
}
