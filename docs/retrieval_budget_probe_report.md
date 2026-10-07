# 分阶段检索预算检验（2026-09-28）

## 实现范围

- `TopK` 表示最终返回的证据条数；`RecallBudget` 是各通道召回的基数；`CandidateLimit` 是融合去重后进入重排的上限。
- 请求未指定预算且配置未设置 `rag.retrieve.recall-budget` / `candidate-limit` 时，两项预算都等于 `TopK`，保持原有默认行为。
- 候选上限大于最终条数时，若有重排器则取重排前候选中的最终 `TopK` 条；重排失败或未启用时，从融合顺序中取最终 `TopK` 条。常规检索的 Trace 记录三项预算、重排前 ID 和最终 ID。
- 评测器使用同一套请求语义。`EVAL_PRERANK_CANDIDATES` 控制评测候选上限；设置为 `0` 可复现原来的 `TopK` 截断。

## 四题对照

使用 `docs/graph_retrieval_probe_samples.json`，关键词通道，最终 `TopK=5`。三组均为真实数据库检索；10/20 组调用配置的重排器。为保留原评测的召回深度，对照中 `RecallBudget` 与 `CandidateLimit` 同为 5/10/20。

| 召回基数 / 候选上限 | 重排 | 精确标注块齐全@5 | 精确标注块平均召回@5 | 四题运行耗时约 |
| --- | --- | ---: | ---: | ---: |
| 5 / 5 | 否 | 3/4 | 0.875 | 0.22 秒 |
| 10 / 10 | 是 | 2/4 | 0.750 | 2.84 秒 |
| 20 / 20 | 是 | 2/4 | 0.750 | 4.70 秒 |

`go_sse_leak_linux_triage` 在 5/5 中两个指定 chunk ID 都出现，在 10/10 和 20/20 中只出现一个。其余题的精确 ID 覆盖状态没有变化。另做了 10/10、关闭重排的运行，精确 ID 平均召回@5 为 0.875。

后续逐块核查发现，指定 ID 并不等于唯一有效证据：语料采用重叠切块，`linux.md` 的 `23848392107356417` 含有 SSE 排查所需的 `pprof/goroutine`、`ss` 等命令，却不是该题标注的 `23848392107421953`。重排器把前者排第 2，后者排第 7；后者的相关度约 0.672。Go 内存题中，指定 Go 块被排第 8，但前 5 中另有 Go 块包含 `NumGoroutine`、goroutine profile 和 channel 阻塞线索。因此上表**不能解释为答案证据从 3/4 降到 2/4**。

## 扩展样本与证据组复核

`docs/retrieval_multi_evidence_samples.json` 汇集原有四道跨文档题和六道同文档多段题，全部统一为最终 `TopK=5`。后六道来自已有的 `testdata/retrieve_eval_corpus_samples.json`，已逐条核对指定 chunk 的内容。部分指定段落互相重叠，因此十题的精确 ID 指标仍只用于定位排序变化。在其中七题上，进一步标注了回答所需的概念组；Redis 事务题将命令排队与“不回滚”分开标注，其余题为两组。每组允许多个包含等价内容的 chunk ID。其余三题的原标注不能可靠地区分必要概念，未纳入概念组指标。

| 通道 | 召回基数 / 候选上限 | 精确 ID 齐全@5（10 题） | 必要概念均覆盖@5（7 题） |
| --- | --- | ---: | ---: |
| 关键词 | 5 / 5，不重排 | 8/10 | 7/7 |
| 关键词 | 10 / 10，重排 | 8/10 | 7/7 |
| 关键词 | 20 / 20，重排 | 8/10 | 7/7 |
| 混合 | 5 / 5，不重排 | 7/10 | 5/7 |
| 混合 | 10 / 10，重排 | 8/10 | 7/7 |
| 混合 | 20 / 20，重排 | 8/10 | 7/7 |

混合通道的 5/5 在 SSE 题中漏掉 Go 的退出机制，在 Redis 题中漏掉事务执行错误不回滚的依据；10/10 与 20/20 的重排补回了这两组证据。关键词 5/5 已覆盖七题全部必要概念，扩大候选和混合通道在这组题上没有超出关键词基线的覆盖收益。样本较小，且六道新增题来自同一五篇文档语料。**暂不把候选上限 20 设为生产默认值**；后续需要更广的真实问题和答案引用评测。

### 嵌入维度核查

初次混合通道运行在外部嵌入调用后长时间无输出。随后预生成查询向量时发现维度不匹配：`configs/application.yaml` 原先设置 1536 维，而对本地 PostgreSQL 的只读查询显示，23 个知识库的 13,651 条非空向量全部为 4096 维。用 1024 维查询缓存时，数据库明确返回 `different vector dimensions 4096 and 1024`，评测实际退化为仅关键词通道。改用同一硅基流动模型生成 4096 维查询缓存后，十题的向量和关键词通道均成功返回结果。

默认配置已改为 4096 维；随后不设置实验性的维度环境变量，用正式配置运行单题混合检索，向量通道返回 30 条、关键词通道返回 20 条，重排和最终 5 条输出正常。该配置修正只对当前 4096 维索引有验证；未来更换嵌入模型或重建索引时，模型、配置和索引维度必须保持一致。

复现命令（分别把 `EVAL_PRERANK_CANDIDATES` 设为 `0`、`10`、`20`）：

```powershell
$env:GOCACHE='D:\goagent\.gocache'
$env:EVAL_PRERANK_CANDIDATES='10'
go run ./cmd/retrieve-eval -input docs/graph_retrieval_probe_samples.json -execute -search-mode keyword -k 5 -json -per-sample-timeout 10s -output .cache/retrieval_budget_keyword10.json

go run ./cmd/retrieve-eval -input docs/retrieval_multi_evidence_samples.json -execute -search-mode keyword -k 5 -json -output .cache/retrieval_multi_keyword10_v2.json
python scripts/eval_retrieval_evidence_groups.py --samples docs/retrieval_multi_evidence_samples.json --results .cache/retrieval_multi_keyword10_v2.json --top-k 5

# 本工作区的 4096 维查询向量缓存保存在 .cache/ 中；该文件不提交到 Git。
go run ./cmd/retrieve-eval -input docs/retrieval_multi_evidence_samples.json -execute -search-mode hybrid -query-vector-cache .cache/retrieval_multi_query_vectors_4096.json -k 5 -json -output .cache/retrieval_multi_hybrid10_v2.json
python scripts/eval_retrieval_evidence_groups.py --samples docs/retrieval_multi_evidence_samples.json --results .cache/retrieval_multi_hybrid10_v2.json --top-k 5
```

## 答案与引用探针（同日完成）

`scripts/eval_retrieval_answers.py` 从已保存的检索结果读取每题前 5 条 chunk，再从 PostgreSQL 只读提取正文。固定提示词与 `temperature=0`，分别生成关键词 5/5、混合 5/5、混合 10/10 三组带 chunk ID 引用的答案。该探针固定检索证据后单独调用模型，**不是产品对话链路的端到端评测**。原定硅基流动 `Qwen/Qwen3-32B` 在 9/21 条后持续返回 HTTP 429（`50609`，`System is too busy now`）；这些部分结果保存在 `.cache/retrieval_answer_probe.json`，不与后续模型结果合并。项目配置中的硅基流动 GLM-4.7 返回 HTTP 403（`Model disabled`），未用于评测。

为完成同模型、同提示词对照，改用平台模型列表中可用的 `Qwen/Qwen3-8B`，设置 `enable_thinking=false`，从头生成 7 题 × 3 组共 **21/21 条**。结果保存在忽略 Git 的 `.cache/retrieval_answer_probe_qwen8b_nothink.json`。脚本按题保存，支持断点续跑。

| 检查项 | 关键词 5/5 | 混合 5/5 | 混合 10/10 |
| --- | ---: | ---: | ---: |
| 检索证据覆盖全部必要概念 | 7/7 | 5/7 | 7/7 |
| 答案至少引用一个未提供的 chunk ID | 0/7 | 0/7 | 0/7 |

第二项只检查引用 ID 是否属于输入证据，**不代表句子得到该 chunk 支持**。逐条核查 21 条答案后，发现模型常能写出问题要点，却把重要结论引到不支持它的段落：

- 混合 5/5 的 SSE 题没有检索到连接断开后的 Go 退出机制，但答案仍建议使用 `errgroup` 等方式，并引用未包含该机制的段落。先前 32B 的同题答案还生成了在 HTTP handler 返回后由子 goroutine 写 `ResponseWriter` 的不安全示例。
- 混合 5/5 的 Redis 事务题缺少“不回滚”的段落，却回答“不回滚”，把 Lua 脚本失败不撤销的描述当成 Redis `MULTI/EXEC` 的依据。关键词 5/5 和混合 10/10 虽检索到真正依据，8B 答案仍把“不回滚”引用到其他段落。
- slice/map 题三组答案都包含扩容规则，但至少一处关键引用与规则不符：关键词组把 map 扩容阈值引到内存泄漏段落；两组混合答案把 slice 规则引到只包含 map 后半段的 chunk。证据在前 5 条中，问题出在引用选择。
- HTTPS 题中，部分答案给出比被引用 chunk 更具体的摘要算法或握手步骤。检索到正确段落也不能保证逐句引用准确。

人工复核还修正了 `docs/retrieval_multi_evidence_samples.json`：把等价重叠 chunk 纳入概念组，并将 Redis 事务的“排队执行”和“不回滚”拆成两个必要概念。按修正后的标注重算，混合 5/5 从原报的 6/7 降为 **5/7**；其他五组不变。混合 10/10 相比混合 5/5 补回两题证据，但没有超出关键词 5/5 的 7/7，且 8B 的答案引用没有随之稳定改善。当前证据仍不支持提高生产默认预算；下一轮应扩大来自不同知识库、版本明确的真实问题，并把“结论正确”和“引用确实支撑结论”分开人工标注。

复现 8B 生成：

```powershell
python scripts/eval_retrieval_answers.py --model Qwen/Qwen3-8B --no-thinking --output .cache/retrieval_answer_probe_qwen8b_nothink.json
```
