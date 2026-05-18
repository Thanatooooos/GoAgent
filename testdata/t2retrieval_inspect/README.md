# T2Retrieval Inspect Reports

This directory contains per-sample retrieve inspection reports generated from the stratified 10K benchmark rerun.

## Categories

- `single_positive_miss`
  - hard misses where the only gold chunk did not appear in top10
- `single_positive_low_rank`
  - the gold chunk appeared in top10 but missed rank1
- `few_positive_low_rank`
  - 2-3 positive queries where the first relevant chunk missed rank1
- `multi_positive_low_recall`
  - 4+ positive queries with weak recall@10 coverage

## Inputs

- results: `testdata/t2retrieval_10k_results.json`
- resolved samples: `testdata/t2retrieval_eval_10k_resolved.json`
- diagnostics: `testdata/t2retrieval_10k_diagnostics.json`

## Generation

```powershell
python scripts\analyze_t2retrieval_results.py testdata\t2retrieval_10k_results.json --worst 15 --category-limit 8 --export-json testdata\t2retrieval_10k_diagnostics.json --export-md testdata\t2retrieval_10k_diagnostics.md
$env:GOCACHE=(Resolve-Path .gocache)
go run ./cmd/retrieve-inspect -samples testdata\t2retrieval_eval_10k_resolved.json -results testdata\t2retrieval_10k_results.json -names-file testdata\t2retrieval_inspect\single_positive_miss.txt -output-dir testdata\t2retrieval_inspect\single_positive_miss
go run ./cmd/retrieve-inspect -samples testdata\t2retrieval_eval_10k_resolved.json -results testdata\t2retrieval_10k_results.json -names-file testdata\t2retrieval_inspect\single_positive_low_rank.txt -output-dir testdata\t2retrieval_inspect\single_positive_low_rank
go run ./cmd/retrieve-inspect -samples testdata\t2retrieval_eval_10k_resolved.json -results testdata\t2retrieval_10k_results.json -names-file testdata\t2retrieval_inspect\few_positive_low_rank.txt -output-dir testdata\t2retrieval_inspect\few_positive_low_rank
go run ./cmd/retrieve-inspect -samples testdata\t2retrieval_eval_10k_resolved.json -results testdata\t2retrieval_10k_results.json -names-file testdata\t2retrieval_inspect\multi_positive_low_recall.txt -output-dir testdata\t2retrieval_inspect\multi_positive_low_recall
```
