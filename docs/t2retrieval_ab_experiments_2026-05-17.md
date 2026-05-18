# T2Retrieval A/B Experiments（2026-05-17）

## 目的

针对当前 10K stratified benchmark，验证两个最优先的 retrieve 调优方向：

1. `rerank on/off` 是否真的影响结果
2. 放大 `vector_global` 候选池是否真的改善结果

## 代码准备

本轮实验前做了两项准备：

- `retrieve-eval` / `retrieve-inspect` 支持实验参数覆盖
  - `-rerank-model`
  - `-vector-topk-multiplier`
- `vector_global` channel 真正接入 `rag.search.channels.vector-global.top-k-multiplier`

注意：为了和 2026-05-17 上午那版 baseline 保持可比，对照组显式使用 `vector-topk-multiplier=2`。虽然 `application.yaml` 当前默认是 `3`，但旧 baseline 实际是硬编码 `2`。

## 实验命令

### 对照组

```powershell
$env:GOCACHE=(Resolve-Path .gocache)
go run ./cmd/retrieve-eval -input testdata\t2retrieval_eval_10k_resolved.json -execute -k 1,3,5,10 -json -output testdata\t2retrieval_10k_results_control_m2.json -rerank-model qwen3-rerank -vector-topk-multiplier 2
```

### 关闭 rerank

```powershell
$env:GOCACHE=(Resolve-Path .gocache)
go run ./cmd/retrieve-eval -input testdata\t2retrieval_eval_10k_resolved.json -execute -k 1,3,5,10 -json -output testdata\t2retrieval_10k_results_rerank_noop_m2.json -rerank-model rerank-noop -vector-topk-multiplier 2
```

### 放大 vector 候选池

```powershell
$env:GOCACHE=(Resolve-Path .gocache)
go run ./cmd/retrieve-eval -input testdata\t2retrieval_eval_10k_resolved.json -execute -k 1,3,5,10 -json -output testdata\t2retrieval_10k_results_rerank_on_m4.json -rerank-model qwen3-rerank -vector-topk-multiplier 4
```

## 结果

### overall

| experiment | Hit@1 | MRR | Recall@10 | NDCG@10 |
|---|---:|---:|---:|---:|
| control `rerank=qwen3-rerank, m=2` | 0.9279 | 0.9506 | 0.9379 | 0.9359 |
| `rerank=noop, m=2` | 0.9279 | 0.9507 | 0.9374 | 0.9357 |
| `rerank=qwen3-rerank, m=4` | 0.9279 | 0.9507 | 0.9374 | 0.9356 |

### 重点子集

| experiment | single-positive Hit@1 | multi-positive Recall@10 |
|---|---:|---:|
| control `rerank=qwen3-rerank, m=2` | 0.7679 | 0.9157 |
| `rerank=noop, m=2` | 0.7679 | 0.9145 |
| `rerank=qwen3-rerank, m=4` | 0.7679 | 0.9145 |

### 样本级变化

- 两个实验都只影响了 1 条 query
- 改变的是：`t2_1816 名为完全败北之鞭怎么获得`
  - rank `7 -> 6`
- 其余 221 条 query 的 `firstRelevantRank` 没有变化

## 结论

### 1. 当前 rerank 基本没有提供可观测增益

在这组 benchmark 上，`rerank=qwen3-rerank` 和 `rerank=noop` 几乎等价。说明至少在当前链路和当前候选集下：

- rerank 没有明显改善 top1
- rerank 也没有明显改善 recall@10

这意味着后续如果继续优化 rerank，本轮 benchmark 很可能不是最敏感的验证场。

### 2. 单纯把 vector 候选池从 `2x` 扩到 `4x` 也没有带来收益

虽然代码和日志都确认 `vector_global` 的 SQL `LIMIT` 已经从 `20` 放大到了 `40`，但结果几乎不变。说明当前问题大概率不是“候选池太小到连 gold 都进不来”的简单情形，至少在这 222 条样本上不是。

### 3. 当前 benchmark 对这两类改动不敏感

综合来看，这组 10K stratified benchmark 现在能稳定暴露：

- `single-positive` 的脆弱性
- `multi-positive` 的覆盖问题

但它对以下两类改动几乎不敏感：

- rerank 开关
- vector 候选池从 `2x` 到 `4x` 的扩大

## 下一步建议

1. 不要继续在这组数据上优先投入 `rerank on/off` 和单纯放大 candidate pool。
2. 下一轮更值得做的是：
   - 比较 `semantic / keyword / hybrid`
   - 分析 query rewrite 是否能改善 `single-positive miss`
   - 构建更贴近产品的 markdown 语料，测试 chunk 与 metadata 检索价值
3. 如果还想继续试 retrieve 调优，建议改成更有信息量的实验：
   - `query rewrite on/off`
   - `semantic vs hybrid`
   - `keyword fallback` 是否能救回 single-positive miss
