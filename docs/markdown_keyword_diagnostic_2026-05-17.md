# Markdown Keyword Diagnostic (2026-05-17)

## Goal

Check whether the current PostgreSQL `word_similarity`-based keyword retrieval is a major reason for weak Chinese lexical performance, using the local markdown benchmark as a product-facing probe.

## Commands

```powershell
$env:GOCACHE=(Resolve-Path .gocache); go run ./cmd/retrieve-eval -input testdata\docs_markdown_samples_v2.json -execute -k 1,3,5,10 -json -output testdata\docs_markdown_results_keyword_v2.json -search-mode keyword
$env:GOCACHE=(Resolve-Path .gocache); go run ./cmd/retrieve-eval -input testdata\docs_markdown_samples_v2.json -execute -k 1,3,5,10 -json -output testdata\docs_markdown_results_semantic_v2.json -search-mode semantic
$env:GOCACHE=(Resolve-Path .gocache); go run ./cmd/retrieve-eval -input testdata\docs_markdown_samples_v2.json -execute -k 1,3,5,10 -json -output testdata\docs_markdown_results_hybrid_v2.json -search-mode hybrid
```

## Overall

| Mode | Hit@1 | MRR | Recall@10 |
| --- | ---: | ---: | ---: |
| keyword | 0.6244 | 0.7137 | 0.8065 |
| semantic | 0.6293 | 0.6841 | 0.7463 |
| hybrid | 0.6878 | 0.7543 | 0.8358 |

## By Query Type

| Query type | Count | keyword Hit@1 | keyword Hit@10 | keyword MRR | semantic Hit@1 | hybrid Hit@1 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| file name | 41 | 0.8537 | 1.0000 | 0.9187 | 0.5854 | 0.8780 |
| section metadata | 82 | 0.4024 | 0.7927 | 0.5626 | 0.5122 | 0.4634 |
| section semantic | 82 | 0.7317 | 0.7927 | 0.7622 | 0.7683 | 0.8171 |

## Interpretation

1. `word_similarity` is not useless. It works well for strong literal overlap, especially file-name lookup.
2. It weakens sharply on long section-title retrieval. `section metadata` queries only reach `Hit@1=0.4024`, with `17/82` complete misses at top10.
3. `hybrid` beats `keyword` mainly by rescuing lexical misses with vector retrieval rather than by slightly refining already-good lexical results.
4. The generated `section semantic` queries are still optimistic for `keyword`, because they retain explicit heading text such as `X 主要讲什么`. This means the measured `keyword` score is likely higher than what a more free-form Chinese user query would achieve.

## Sample Evidence

For the query:

`查找 Development Notes - 2026-05-02 > Ingestion 进展 > 9. ingestion 模块骨架、持久化与最小执行链路落地 这一节`

- `keyword` missed entirely: top results collapsed to generic top-level `Development Notes - YYYY-MM-DD` headings.
- `hybrid` recovered the exact target chunk at rank 1 after adding `vector_global`.

This suggests the current trigram similarity is dominated by common shared prefixes and weak at isolating the deep section title.

## Conclusion

The current PostgreSQL `word_similarity` implementation is likely a major limitation for Chinese keyword retrieval, but the problem is nuanced:

- It is good enough as a lightweight lexical supplement for file names and exact heading fragments.
- It is not strong enough to serve as the primary Chinese lexical retrieval strategy.
- The current benchmark likely overestimates its capability on natural-language questions.

## Recommended Next Steps

1. Keep `word_similarity` as a cheap literal-match channel rather than the main Chinese keyword engine.
2. Add a harder hand-written Chinese query set that paraphrases section intent without copying heading text.
3. Evaluate alternatives for Chinese lexical retrieval, such as proper Chinese tokenization + full-text/BM25 style indexing.
