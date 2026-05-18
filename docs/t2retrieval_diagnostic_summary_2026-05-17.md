# T2Retrieval 10K 分层评估诊断总结（2026-05-17）

## 背景

基于 10K stratified baseline：

- 结果文件：`testdata/t2retrieval_10k_results.json`
- 分类诊断：`testdata/t2retrieval_10k_diagnostics.json`
- inspect 报告目录：`testdata/t2retrieval_inspect/`

核心指标：

- Hit@1 = `0.9279`
- MRR = `0.9506`
- Recall@10 = `0.9379`
- NDCG@10 = `0.9359`

## 关键发现

### 1. 真正的短板是单正例 query

- `single_positive_miss`: 3 条
- `single_positive_low_rank`: 8 条
- 单正例 bucket 的 Hit@1 只有 `0.7679`

这说明当前系统在“只有一个 gold chunk”的 query 上更脆弱。很多 query 不是完全没有相关语义，而是没有把唯一正确答案稳定推到 top1，甚至 top10。

### 2. inspected failure 几乎完全由 vector 通道驱动

对 20 条 inspect 样本的复查结果显示：

- `keyword chunks=[1-9]`：0 条
- `metadata_title chunks=[1-9]`：0 条
- `vector_global chunks=20`：20 条

这意味着当前这批困难样本里，`keyword` 和 `metadata_title` 几乎没有提供任何补充召回，检索结果几乎完全取决于 `vector_global`。

### 3. single-positive low-rank 更像 rerank / exactness 问题

典型样本：

- `t2_280`: `m1h是什么材质管道`
- `t2_15142`: `之前有过胸膜炎可以接种新冠疫苗吗`

这些 query 的 gold chunk 已经进入 top10，但排在 rank 2-4。说明：

- 向量召回通常已经接近正确语义簇
- 但 top1 精排不够稳定
- 当前链路对术语、问法细节、唯一答案的区分还不够强

### 4. single-positive miss 更像“语义相近但 gold 没进 top10”

典型样本：

- `t2_2700`: `如何在网上兼职赚钱`
- `t2_1371`: `开户许可证变成一张纸`
- `t2_1774`: `抚养费为什么两个账号`

这些 query 的 top10 通常已经落在相近主题附近，但 gold chunk 不在 top10。说明当前问题更像：

- vector recall 召回了相近簇
- 但候选池里没有把 gold 带上来，或太早被截断

### 5. multi-positive low-recall 是覆盖问题，不是 top1 问题

典型样本：

- `t2_150`: `什么是红色革命`
- `t2_2486`: `怎么样可以挣钱`
- `t2_53`: `杜仲泡水喝功效与作用`

这些 query 经常能命中 rank1，但 `recall@10` 很低，说明：

- 首条最相关结果通常没问题
- 后续候选很快漂到相近但不属于 gold 的语义邻居
- 主要瓶颈在“持续覆盖同主题多个相关 chunk”

## 调优优先级建议

### 优先级 1：先优化 vector-only 路径

因为这批 hardest samples 基本没有吃到 `keyword` 和 `metadata_title` 的收益，所以下一轮最值得优先验证的是：

- 增大 vector 初始候选池
- 检查 fusion / dedup 是否过早截断
- 检查 rerank 是否把相关簇压散

如果这些点不先处理，继续调关键词通道对这批问题的帮助可能有限。

### 优先级 2：专门打 single-positive query

建议单独拉一组 single-positive regression set，重点观察：

- miss 是否变少
- rank2-4 是否推到 rank1

这组样本会比总 Hit@1 更能反映“唯一答案检索”的真实改进。

### 优先级 3：把 recall@10 当作多正例主指标

对于多正例 query，不要只看 Hit@1。下一轮调优应优先比较：

- Recall@10
- NDCG@10
- 以及 top10 中是否持续留在正确主题簇内

### 优先级 4：不要高估 keyword / metadata_title 在这条 benchmark 上的代表性

当前 inspect 样本表明，这两个通道在 T2Retrieval 这批困难样本上几乎没有贡献。因此：

- T2Retrieval 目前更适合作为 `vector / rerank / fusion` 调优场
- `metadata_title` 与 markdown 结构收益，仍然需要产品贴近型 markdown 语料来验证

## 建议的下一步实验

1. 固定当前 10K stratified 样本集，做一轮 `rerank on/off` 对比。
2. 固定当前样本集，做一轮更大的 vector candidate pool 对比。
3. 对 `single_positive_miss` 和 `single_positive_low_rank` 单独汇报结果，不再只看 overall。
4. 对 `multi_positive_low_recall` 单独汇报 Recall@10 和 top10 漂移情况。
