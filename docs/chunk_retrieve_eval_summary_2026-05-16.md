# Chunk & Retrieve 评估总结

日期：2026-05-16

## 过程

1. **补齐评估指标** — 在 `internal/app/rag/evaluation/` 新增 NDCG 指标、分级相关性支持、ChunkStrategy 字段
2. **新建 chunk 质量度量** — `chunk_quality.go`，含大小分布、边界质量、元数据覆盖率
3. **新建语料导入工具** — `cmd/corpus-loader`，批量导入 passages → 知识库 + 向量库
4. **修复关键词检索性能** — 新增 GIN trigram 索引 + 查询改写（`word_similarity > 0` → `content % query`）
5. **拉取 T2Retrieval 语料** — 通过 HuggingFace 获取 C-MTEB 中文段落检索数据集，生成 eval 样本
6. **端到端基准测试** — 全链路：导入 → embedding → 检索 → 评估

## 语料库

**T2Retrieval**（C-MTEB / FlagEmbedding / BAAI）

| 属性 | 值 |
|------|-----|
| 来源 | HuggingFace `C-MTEB/T2Retrieval` |
| 内容 | 中文段落检索基准，覆盖医疗/学术/金融/政府等领域 |
| 查询 | 22,812 条真实中文搜索提问 |
| 段落 | 118,605 条，平均 587 字符 |
| 标注 | qrels：每条查询对应 5-10 个相关段落（人工标注） |

另尝试了 CommonCrawl 原始 WET 文件，因中文占比仅 6% 且含大量垃圾内容，已弃用。

## 结果

### 检索基准（T2Retrieval hybrid search, rerank on）

| 规模 | 查询数 | MRR | Hit@1 | Rec@5 | Rec@10 |
|------|--------|-----|-------|-------|--------|
| 5K | 200 | 1.0000 | 1.0000 | 0.5051 | 0.8913 |
| 10K | 222 | 0.9977 | 0.9955 | 0.4377 | 0.8443 |

### 关键词检索性能

| 阶段 | 单次查询耗时 |
|------|-------------|
| 优化前（无索引, word_similarity > 0） | ~400ms |
| 优化后（GIN trigram 索引, content % query） | ~20-50ms |

## 结论

1. **5K 规模基准饱和** — Hit@1=100%, MRR=1.0，无法指导优化
2. **10K 开始有区分度** — Hit@1 降 0.5pp，Recall@10 降 5pp，随规模增大区分度会继续提升
3. **关键词检索性能可接受** — GIN trigram 索引将 400ms 降至 20-50ms，10K 规模下可用
4. **语料规模是区分度的关键** — 封闭集检索在数据量不够时指标会触顶，需要足够多的干扰项才能测出排序质量差异
5. **基准可作为回归哨兵** — 后续改检索链路、chunk 策略、rerank 配置时，对比此基线即可判断是否回退
