# Chunk & Retrieve 质量评估方案

日期：2026-05-16

## 可行性评估

### 已有资产

| 资产 | 状态 | 可复用性 |
|------|------|----------|
| `cmd/retrieve-eval` CLI | 完整，支持 `-execute` 真实验证 | 直接复用 |
| `evaluation.Evaluate()` | 完整，Hit@K / Recall@K / MRR / tag 分组 | 直接复用 |
| `evaluation.Sample` 格式 | 完整，支持 chunk / document / metadata 多级 target | 直接复用 |
| `ragbootstrap.NewRuntime()` | 完整，一键构建 RAG 运行时 | 直接复用 |
| `retrieve.Request` | 完整，支持 SearchMode / TopK / KB 过滤 | 直接复用 |
| 两条 chunk 策略 | fixed_size + markdown，生产可用 | 评估对象 |
| 三条检索通道 | vector_global + keyword + metadata_title | 评估对象 |
| 知识库 ingestion 链路 | 完整，可程序化上传文档 | 语料导入 |

### 缺失项

| 缺失 | 影响 | 可解决性 |
|------|------|----------|
| 无 chunk 质量度量 | 无法回答"哪种切片更好" | 新增 ~80 行 |
| 样本仅 3 个（手写） | 无统计显著性 | 引入语料库 |
| 无 NDCG 指标 | MRR 只关注第一个命中位置 | 新增 ~30 行 |
| 无 A/B 对比框架 | 无法对比策略效果差异 | 新增 ~100 行 |
| 无语料导入工具 | 外部语料无法批量入库 | 新增 ~120 行 |

**结论：可行。** 基础设施完备，缺失的都是增量补强，不需要改动核心链路。

---

### 外部语料库选择

| 语料 | 规模 | 语言 | 文档级 | 与 goagent 的契合度 |
|------|------|------|--------|---------------------|
| **自建中文 Markdown 语料** | 可控 | 中文 | ✅ | ★★★★★ 完全匹配 markdown chunker 场景 |
| **DuReader-Retrieval** | ~200K QA | 中文 | ❌ 段落级 | ★★★☆☆ 标准但粒度不匹配 |
| **T2Ranking / mMARCO-zh** | ~100K | 中文 | ❌ 段落级 | ★★★☆☆ 同上 |
| **BEIR 子集** (NFCorpus, SciFact) | ~3K docs | 英文 | ✅ | ★★☆☆☆ 英文 + 不测中文切分 |
| **LLM 生成 QA** | 可控 | 双语 | ✅ | ★★★★☆ 灵活但需验证质量 |

**建议路径**：以"自建中文 Markdown 语料 + LLM 辅助生成 QA"为主，因为 goagent 的核心能力是中文技术文档的 markdown 切分和混合检索，标准 benchmarks 都不直接匹配这个场景。

---

## 方案

### Phase 1：补齐评估指标（~150 行，1 天）

不依赖外部语料，先把评估维度补全。

#### 1.1 Chunk 质量度量

新增 `internal/app/rag/evaluation/chunk_quality.go`：

```
ChunkQualityReport:
  - AverageChunkSize (字符数，均值 + 标准差)
  - OversizedChunkCount (> MaxChunkSize 的 chunk 数)
  - UndersizedChunkCount (< MinChunkSize 的 chunk 数)
  - BoundaryQuality (切分点落在标题/段落开头的比例)
  - MetadataCompleteness (section/heading_path/code_language 非空率)
  - StrategyBreakdown (按 chunk 策略分组)
```

不依赖 gold standard，纯统计量。可以从 `t_knowledge_chunk_vector` 直接取 chunk 的 metadata 和 content 长度计算。

#### 1.2 NDCG 指标

在 `evaluation.go` 中新增：

```
SampleResult 新增字段:
  - NDCGAtK map[int]float64

AggregateMetrics 新增字段:
  - AverageNDCGAtK map[int]float64
```

需要 `ExpectedIDs` 有 relevance grade（可以用 `expectedIds` 中的顺序隐含 grade：第一位最相关，依次递减，或引入显式 `expectedRelevance map[string]int`）。

#### 1.3 现有评估增强

```
Sample 格式新增可选字段:
  - ChunkStrategy string   // "fixed_size" | "markdown" — 按策略分组
  - ExpectedRelevance map[string]int  // ID → relevance grade (0-3)，供 NDCG
```

---

### Phase 2：语料导入工具（~120 行，1 天）

新增 `cmd/corpus-loader/main.go`：

```
go run ./cmd/corpus-loader \
  -dir ./testdata/corpus/chinese-docs \   # markdown 文件目录
  -kb corpus-bench \                       # 目标知识库
  -chunk-strategy markdown \               # 切分策略
  -embedding-model text-embedding-v4       # embedding 模型
```

流程：
1. 遍历目录下所有 `.md` 文件
2. 通过 `KnowledgeDocumentService.Upload()` 上传到指定 KB
3. 等待 ingestion pipeline 完成
4. 输出 `corpus_manifest.json`（文档 ID → 文件路径映射，供后续生成 eval sample）

需要复用 `ragbootstrap.NewRuntime()` 来获取 `KnowledgeDocumentService`。

---

### Phase 3：样本生成（~200 行，1-2 天）

新增 `cmd/eval-sample-gen/main.go`：

```
go run ./cmd/eval-sample-gen \
  -manifest corpus_manifest.json \         # Phase 2 产出
  -output testdata/eval_samples_v2.json \  # 产出样本文件
  -count 50                                # 目标样本数
```

两种生成策略：

**策略 A：从 chunk 自动推导 query**（全自动）
- 遍历每个 chunk，用其 metadata（section、document_name、code_language）生成检索目标
- 已知 ground truth：chunk 自己的 ID、所在 document ID、section 名等
- 自动生成三种样本类型：
  - `semantic`: query = chunk 内容的前 N 个关键词 → expected = chunkID
  - `metadata`: query = "查找 {section} 章节" → expected = documentID
  - `keyword`: query = "文件 {source_file_name}" → expected = chunkID

**策略 B：LLM 辅助生成 question**（半自动，质量更高）
- 把每个 chunk 内容发给 LLM，要求生成一个能用该 chunk 回答的问题
- LLM 返回 question → question 成为 query，chunk ID 成为 expected
- 可控制难度：简单（直接引用）/ 中等（同义改写）/ 困难（需要推理）

推荐先用策略 A 快速生成 50-100 个 baseline 样本，再用策略 B 扩充 良态样本。

---

### Phase 4：A/B 对比框架（~100 行，1 天）

新增 `cmd/retrieve-compare/main.go`：

```
go run ./cmd/retrieve-compare \
  -input testdata/eval_samples_v2.json \
  -compare chunk_strategy \               # 对比维度
  -k 1,3,5 \
  -json > comparison_report.json
```

对比维度（通过 `Sample` 的 tag 或新增字段区分）：

| 维度 | 对比内容 |
|------|----------|
| `chunk_strategy` | fixed_size vs markdown |
| `search_mode` | semantic vs keyword vs hybrid vs auto |
| `rerank` | rerank on vs off |
| `top_k` | K=3 vs K=5 vs K=10 |

输出格式：

```
                   MRR    Hit@1   Hit@3   Hit@5   NDCG@3  NDCG@5
fixed_size         0.45   0.32    0.58    0.71    0.38    0.52
markdown           0.52   0.40    0.63    0.75    0.43    0.57
markdown vs fixed  +15.6% +25.0%  +8.6%   +5.6%   +13.2%  +9.6%
```

---

### Phase 5：外部标准语料接入（按需，1-2 天）

如果 Phase 1-4 完成后仍觉得样本不够权威，可以接入：

**推荐：中文 Markdown 开源文档语料**

```
testdata/corpus/
  chinese-docs/
    go/          ← https://github.com/polaris1119/golang-handbook 等
      basics.md
      generics.md
    vue/         ← Vue 中文文档
    python/      ← Python 中文教程
```

- 天然 markdown 格式，直接测 goagent 的 markdown chunker 和 metadata_title 通道
- 有真实的 section heading 结构，适合测 metadata 检索
- 可以手动标注 20-30 个高质量 QA 对作为 gold standard

---

## 优先级与里程碑

| 阶段 | 产出 | 改动量 | 可单独交付 |
|------|------|--------|------------|
| Phase 1 | NDCG + chunk 质量报告 | ~150 行 | ✅ |
| Phase 2 | 语料导入工具 | ~120 行 | ✅ |
| Phase 3 | 50+ 自动生成样本 | ~200 行 | ✅ |
| Phase 4 | A/B 对比 CLI | ~100 行 | ✅ |
| Phase 5 | 外部标准语料 | 按需 | ✅ |

总改动量 ~570 行，不修改任何核心链路代码，全部是新增工具和评估逻辑。

---

## 风险与注意事项

1. **语料版权**：外部语料只用于测试，不打包进产品。建议用 MIT/CC-BY 许可的仓库。
2. **LLM 生成 QA 的偏差**：LLM 生成的 question 可能偏向表层匹配，真实用户的问法更口语化。建议策略 A（自动推导）作为主力，策略 B（LLM 生成）作为补充。
3. **embedding 成本**：Phase 3 需要把语料写入向量库，会消耗 embedding API 额度。建议先用本地 mock 验证样本生成逻辑，再跑真实验证。
4. **NDCG 需要 relevance grade**：如果只用 expectedIds 二值标注，NDCG 退化为 DCG。建议引入 3 级标注（3=精确匹配，2=相关，1=弱相关）才有区分力。
