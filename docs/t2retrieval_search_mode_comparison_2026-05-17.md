# T2Retrieval Search Mode Comparison (2026-05-17)

## Goal

Compare `semantic` / `keyword` / `hybrid` on the stratified 10K T2Retrieval benchmark after making `search_mode` actually control channel enablement.

## Commands

```powershell
$env:GOCACHE=(Resolve-Path .gocache); go run ./cmd/retrieve-eval -input testdata\t2retrieval_eval_10k_resolved.json -execute -k 1,3,5,10 -json -output testdata\t2retrieval_10k_results_semantic_m3.json -search-mode semantic -rerank-model qwen3-rerank -vector-topk-multiplier 3
$env:GOCACHE=(Resolve-Path .gocache); go run ./cmd/retrieve-eval -input testdata\t2retrieval_eval_10k_resolved.json -execute -k 1,3,5,10 -json -output testdata\t2retrieval_10k_results_keyword_m2.json -search-mode keyword -rerank-model qwen3-rerank -vector-topk-multiplier 2
$env:GOCACHE=(Resolve-Path .gocache); go run ./cmd/retrieve-eval -input testdata\t2retrieval_eval_10k_resolved.json -execute -k 1,3,5,10 -json -output testdata\t2retrieval_10k_results_hybrid_m3.json -search-mode hybrid -rerank-model qwen3-rerank -vector-topk-multiplier 3
```

## Overall Metrics

| Mode | Hit@1 | MRR | Recall@10 | NDCG@10 |
| --- | ---: | ---: | ---: | ---: |
| semantic | 0.9279 | 0.9507 | 0.9374 | 0.9356 |
| keyword | 0.0000 | 0.0000 | 0.0000 | 0.0000 |
| hybrid | 0.9279 | 0.9506 | 0.9379 | 0.9359 |

## Bucket Metrics

| Bucket | semantic Hit@1 | semantic Recall@10 | keyword Hit@1 | keyword Recall@10 | hybrid Hit@1 | hybrid Recall@10 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 positive | 0.7679 | 0.9464 | 0.0000 | 0.0000 | 0.7679 | 0.9464 |
| 2-3 positives | 0.9821 | 0.9732 | 0.0000 | 0.0000 | 0.9821 | 0.9732 |
| 4-7 positives | 0.9818 | 0.9611 | 0.0000 | 0.0000 | 0.9818 | 0.9611 |
| 8+ positives | 0.9818 | 0.8679 | 0.0000 | 0.0000 | 0.9818 | 0.8702 |

## Sample-Level Comparison

- `hybrid` vs `semantic`: `1` better / `1` worse / `220` ties
- `hybrid` vs `keyword`: `219` better / `0` worse / `3` ties
- `semantic` vs `keyword`: `219` better / `0` worse / `3` ties

Notable per-sample changes:

- `hybrid` improved `t2_1555` on `Recall@10` from `0.875` to `1.0`
- `semantic` slightly improved `t2_1816` from `rank 7` to `rank 6`

## Smoke Check

`retrieve-inspect` with `-search-mode keyword` confirms only `keyword` and `metadata_title` channels run:

- `keyword chunks=0`
- `metadata_title chunks=0`

This means the all-zero `keyword` result is not a reporting bug. On this benchmark, the lexical/title channels currently produce no useful retrieval.

## Interpretation

1. Current T2Retrieval Chinese benchmark performance is almost entirely carried by `semantic` retrieval.
2. `hybrid` is effectively equal to `semantic` right now, with only negligible sample-level differences.
3. `keyword` is not merely weaker; it is currently non-functional for this benchmark in practical terms.
4. The next optimization step should not prioritize `keyword` tuning inside this benchmark unless we first solve why lexical retrieval yields zero chunks.

## Recommended Next Steps

1. Verify whether Chinese lexical retrieval is limited by the current index/matching strategy rather than by benchmark difficulty.
2. Run a focused experiment on a handful of Chinese keyword-heavy queries against raw chunk text to see whether `pg_trgm`-style matching is fundamentally unsuitable here.
3. Continue product-facing evaluation with markdown corpora, but treat T2Retrieval as a `semantic-first retriever benchmark` until keyword retrieval becomes observable.
