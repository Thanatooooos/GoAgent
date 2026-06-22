package evaluation

import (
	"context"
	"fmt"

	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
)

func ExecuteRewriteSample(ctx context.Context, sample *RewriteSample, rewrite ragrewrite.Service) error {
	if sample == nil {
		return fmt.Errorf("rewrite sample is required")
	}
	if rewrite == nil {
		return fmt.Errorf("rewrite service is required")
	}

	history := ToChatHistory(sample.History)
	var result ragrewrite.Result
	if len(history) > 0 {
		result = rewrite.RewriteWithHistory(sample.Query, history)
	} else {
		result = rewrite.RewriteWithSplit(sample.Query)
	}
	ApplyRewriteResult(sample, result)
	return nil
}
