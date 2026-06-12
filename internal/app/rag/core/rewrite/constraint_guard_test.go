package rewrite

import "testing"

func TestExtractRewriteConstraintsFindsIDsAndCodes(t *testing.T) {
	constraints := ExtractRewriteConstraints(`帮我排查 doc_fail_01 最近一次 500 错误，只看 "vector store"`)
	types := map[string]bool{}
	for _, constraint := range constraints {
		types[constraint.Type] = true
	}
	if !types[ConstraintID] || !types[ConstraintHTTPCode] || !types[ConstraintQuoted] || !types[ConstraintTimeRange] || !types[ConstraintLimit] {
		t.Fatalf("expected id/http/quoted/time/limit constraints, got %+v", constraints)
	}
}

func TestGuardRewriteResultAcceptsWhenConstraintsPreserved(t *testing.T) {
	original := "帮我排查 doc_fail_01 最近一次 500 错误"
	result := Result{
		RewrittenQuestion: "排查 doc_fail_01 最近一次 500 错误原因",
		SubQuestions:      []string{"doc_fail_01 失败原因"},
		NeedRetrieval:     true,
	}

	guarded, report := GuardRewriteResult(original, result)
	if !report.Accepted {
		t.Fatalf("expected accepted rewrite, got %+v", report)
	}
	if guarded.RewrittenQuestion != result.RewrittenQuestion {
		t.Fatalf("unexpected guarded result: %+v", guarded)
	}
}

func TestGuardRewriteResultRejectsWhenIDDropped(t *testing.T) {
	original := "帮我排查 doc_fail_01 最近一次 500 错误"
	result := Result{
		RewrittenQuestion: "排查导入失败原因",
		SubQuestions:      []string{"导入失败原因"},
		NeedRetrieval:     true,
	}

	guarded, report := GuardRewriteResult(original, result)
	if report.Accepted {
		t.Fatal("expected rejected rewrite")
	}
	if guarded.RewrittenQuestion != original {
		t.Fatalf("expected fallback to original, got %q", guarded.RewrittenQuestion)
	}
	if guarded.Metadata["rewriteValidation"] == nil {
		t.Fatal("expected rewriteValidation metadata")
	}
}

func TestGuardRewriteResultRejectsWhenHTTPCodeDropped(t *testing.T) {
	original := "doc_fail_01 返回 500"
	result := Result{
		RewrittenQuestion: "doc_fail_01 导入失败",
		SubQuestions:      []string{"导入失败"},
		NeedRetrieval:     true,
	}

	_, report := GuardRewriteResult(original, result)
	if report.Accepted {
		t.Fatal("expected rejected rewrite when HTTP code dropped")
	}
}

func TestGuardRewriteResultKeepsSmallTalkWithoutConstraints(t *testing.T) {
	original := "你好"
	result := Result{
		RewrittenQuestion: "你好",
		SubQuestions:      []string{"你好"},
		NeedRetrieval:     false,
	}

	guarded, report := GuardRewriteResult(original, result)
	if !report.Accepted {
		t.Fatalf("expected accepted small talk, got %+v", report)
	}
	if guarded.RewrittenQuestion != "你好" {
		t.Fatalf("unexpected guarded result: %+v", guarded)
	}
}
