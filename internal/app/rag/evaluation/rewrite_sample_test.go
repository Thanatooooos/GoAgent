package evaluation

import (
	"encoding/json"
	"testing"
)

func TestParseRewriteSamplesSupportsPhase1Schema(t *testing.T) {
	rawSamples := []json.RawMessage{
		json.RawMessage(`{
			"name":"rewrite-sample-1",
			"tags":["alias_normalization"],
			"input":{
				"query":"它有哪些应用场景",
				"history":[
					{"role":"user","content":"什么是向量数据库"},
					{"role":"assistant","content":"向量数据库用于语义检索"}
				]
			},
			"rewrite_expectation":{
				"need_retrieval":true,
				"must_keep_terms":["向量数据库"],
				"critical_terms":["向量数据库"],
				"alias_groups":[["向量数据库","vector database"]],
				"forbidden_rewrites":["它有哪些应用场景"],
				"sub_question_count":{"min":1,"max":2}
			},
			"retrieval_expectation":{
				"target":"knowledge_base",
				"expected_ids":["doc-1"],
				"critical_expected_ids":["doc-1"],
				"top_k":5,
				"search_mode":"hybrid",
				"must_not_regress":true
			},
			"metadata":{"author":"manual"}
		}`),
	}

	samples, err := ParseRewriteSamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseRewriteSamples() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("ParseRewriteSamples() len = %d, want 1", len(samples))
	}
	sample := samples[0]
	if sample.Query != "它有哪些应用场景" {
		t.Fatalf("query = %q, want normalized input query", sample.Query)
	}
	if len(sample.History) != 2 {
		t.Fatalf("history len = %d, want 2", len(sample.History))
	}
	if len(sample.Expect.CriticalTerms) != 1 || sample.Expect.CriticalTerms[0] != "向量数据库" {
		t.Fatalf("critical terms = %+v", sample.Expect.CriticalTerms)
	}
	if sample.RetrievalExpectation.Target != "knowledge_base" {
		t.Fatalf("retrieval target = %q, want knowledge_base", sample.RetrievalExpectation.Target)
	}
	if !sample.RetrievalExpectation.MustNotRegress {
		t.Fatal("must_not_regress expected true")
	}
}

func TestParseRewriteSamplesBatch1Asset(t *testing.T) {
	raw, err := LoadRawSampleFile("testdata/evals/rewrite/samples_batch1.json")
	if err != nil {
		t.Skipf("batch1 asset unavailable: %v", err)
	}
	rawSamples, err := ExtractSampleArray(raw)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	samples, err := ParseRewriteSamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseRewriteSamples() error = %v", err)
	}
	if len(samples) != 12 {
		t.Fatalf("ParseRewriteSamples() len = %d, want 12", len(samples))
	}
	if samples[0].Name != "coref_go_slice_followup" {
		t.Fatalf("first sample = %q, want coref_go_slice_followup", samples[0].Name)
	}
	if samples[6].Name != "preserve_hmap_term" {
		t.Fatalf("seventh sample = %q, want preserve_hmap_term", samples[6].Name)
	}
}

func TestParseRewriteSamplesBatch2Asset(t *testing.T) {
	raw, err := LoadRawSampleFile("testdata/evals/rewrite/samples_batch2.json")
	if err != nil {
		t.Skipf("batch2 asset unavailable: %v", err)
	}
	rawSamples, err := ExtractSampleArray(raw)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	samples, err := ParseRewriteSamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseRewriteSamples() error = %v", err)
	}
	if len(samples) != 12 {
		t.Fatalf("ParseRewriteSamples() len = %d, want 12", len(samples))
	}
	if samples[0].Name != "alias_redis_colloquial" {
		t.Fatalf("first sample = %q, want alias_redis_colloquial", samples[0].Name)
	}
	if samples[6].Name != "no_split_defer_lifo" {
		t.Fatalf("seventh sample = %q, want no_split_defer_lifo", samples[6].Name)
	}
	if samples[11].Expect.SubQuestionCountMax != 1 {
		t.Fatalf("last sample sub_question_count.max = %d, want 1", samples[11].Expect.SubQuestionCountMax)
	}
}

func TestParseRewriteSamplesBatch3Asset(t *testing.T) {
	raw, err := LoadRawSampleFile("testdata/evals/rewrite/samples_batch3.json")
	if err != nil {
		t.Skipf("batch3 asset unavailable: %v", err)
	}
	rawSamples, err := ExtractSampleArray(raw)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	samples, err := ParseRewriteSamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseRewriteSamples() error = %v", err)
	}
	if len(samples) != 12 {
		t.Fatalf("ParseRewriteSamples() len = %d, want 12", len(samples))
	}
	if samples[0].Name != "split_gmp_and_netpoller" {
		t.Fatalf("first sample = %q, want split_gmp_and_netpoller", samples[0].Name)
	}
	if samples[5].Expect.SubQuestionCountMin != 2 {
		t.Fatalf("split sample min = %d, want 2", samples[5].Expect.SubQuestionCountMin)
	}
}

func TestParseRewriteSamplesBatch4Asset(t *testing.T) {
	raw, err := LoadRawSampleFile("testdata/evals/rewrite/samples_batch4.json")
	if err != nil {
		t.Skipf("batch4 asset unavailable: %v", err)
	}
	rawSamples, err := ExtractSampleArray(raw)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	samples, err := ParseRewriteSamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseRewriteSamples() error = %v", err)
	}
	if len(samples) != 12 {
		t.Fatalf("ParseRewriteSamples() len = %d, want 12", len(samples))
	}
	if samples[0].Name != "skip_hello" {
		t.Fatalf("first sample = %q, want skip_hello", samples[0].Name)
	}
	if samples[0].Expect.NeedRetrieval == nil || *samples[0].Expect.NeedRetrieval {
		t.Fatal("skip_hello should expect need_retrieval=false")
	}
	if samples[6].Name != "regress_guard_slice_118" {
		t.Fatalf("seventh sample = %q, want regress_guard_slice_118", samples[6].Name)
	}
	if !samples[6].RetrievalExpectation.MustNotRegress {
		t.Fatal("regress_guard_slice_118 expected must_not_regress=true")
	}
}

func TestParseRewriteSamplesMergedAsset(t *testing.T) {
	raw, err := LoadRawSampleFile("testdata/evals/rewrite/samples.json")
	if err != nil {
		t.Skipf("merged asset unavailable: %v", err)
	}
	rawSamples, err := ExtractSampleArray(raw)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	samples, err := ParseRewriteSamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseRewriteSamples() error = %v", err)
	}
	if len(samples) != 48 {
		t.Fatalf("ParseRewriteSamples() len = %d, want 48", len(samples))
	}
}
