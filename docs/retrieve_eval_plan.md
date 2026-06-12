# Retrieve 评估计划

## 样本文件

- 主评测集：`testdata/retrieve_eval_samples.json`（22 条离线样本，含 `expectedIds` 与预填 `retrieved`）
- 记忆事实评测集：`testdata/memory_fact_phase3_samples.json`（6 条，专项 memory_fact 场景）

## 样本标签定义

| 标签 | 含义 |
|------|------|
| `alias` | 别名/缩写查询（pg、es、向量库） |
| `abbreviation` | 英文缩写 |
| `colloquial` | 口语化表达 |
| `coreference` | 指代消解类问题 |
| `multi_condition` | 多条件组合查询 |
| `diagnosis` | 诊断/排障类问题 |
| `metadata` | 元数据标题/文件名检索 |
| `keyword` | 关键词/BM25 友好查询 |
| `semantic` | 语义/概念类查询 |
| `memory_fact` | 长期记忆事实检索 |
| `rewrite` | 依赖 query rewrite 的查询 |

## Baseline 命令（离线）

不连接数据库，直接使用样本中的 `retrieved` 字段：

```powershell
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -k 1,3,5 -json -output testdata/retrieve_eval_summary.json
```

记忆事实集：

```powershell
go run ./cmd/retrieve-eval -input testdata/memory_fact_phase3_samples.json -k 1,3,5 -json
```

## Candidate 命令（在线执行）

需要本地 DB、向量库和 `configs/` 配置可用：

```powershell
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -execute -config-dir configs -k 1,3,5 -json -output testdata/retrieve_eval_execute_summary.json
```

对比不同 search mode：

```powershell
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -execute -search-mode semantic -config-dir configs -k 1,3,5 -json -output testdata/retrieve_eval_semantic.json
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -execute -search-mode keyword -config-dir configs -k 1,3,5 -json -output testdata/retrieve_eval_keyword.json
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -execute -search-mode hybrid -config-dir configs -k 1,3,5 -json -output testdata/retrieve_eval_hybrid.json
```

## 指标说明

| 指标 | 含义 |
|------|------|
| Hit@K | Top-K 中是否出现任一 expected id |
| Recall@K | Top-K 中命中的 expected id 占比 |
| NDCG@K | 排序质量（支持分级 relevance） |
| MRR | 第一个相关结果的倒数排名均值 |
| ByTag | 按标签聚合的上述指标 |

## 如何判断退化

1. 对比 baseline/candidate 的 `overall.hitRateAtK`、`overall.mrr`。
2. 重点看 `byTag` 中 `alias`、`diagnosis`、`metadata` 是否下降。
3. 检查 `samples` 数组中 `firstRelevantRank` 变大的条目。
4. 若 Hit@1 下降但 Hit@5 上升，可能是排序问题而非召回问题。

## 如何追加样本

1. 在 `testdata/retrieve_eval_samples.json` 的 `samples` 数组追加条目。
2. 必须填写：`name`、`query`、`tags`、`target`、`expectedIds`。
3. 离线评估必须填写 `retrieved`；在线评估可省略 `retrieved`，使用 `-execute`。
4. 没有真实 chunk id 的样本不要放入可执行 eval 文件。
5. 运行 `go test ./internal/app/rag/evaluation ./cmd/retrieve-eval -count=1` 确认格式正确。

## 与 Rewrite 改进的对比流程

P0-1/P0-2 合入后，建议：

1. 先跑离线 baseline 并保存 `retrieve_eval_summary.json`。
2. 改 rewrite 后重新跑在线 candidate（若环境可用）。
3. 用 `docs/retrieve_eval_report_template.md` 填写对比报告。
