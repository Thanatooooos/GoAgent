# FlagEmbedding 语料库集成方案

日期：2026-05-16

## 语料库概况

[FlagEmbedding](https://github.com/FlagOpen/FlagEmbedding) 是 BAAI（北京智源人工智能研究院）的开源项目，核心产出是 BGE 系列中文向量模型 + C-MTEB 中文评测基准。

### C-MTEB 检索类数据集（8 个）

| 数据集 | 查询数 | 文档数 | 平均文档长度 | 领域 |
|--------|--------|--------|-------------|------|
| **T2Retrieval** | 22,812 | 118,605 | 874 字符 | 医疗/学术/金融/政府 |
| MMarcoRetrieval | 7,437 | ~1M | 短段落 | 多语言 MS MARCO |
| DuRetrieval | 4,000 | ~100K | 短段落 | 网页搜索 |
| CovidRetrieval | 949 | ~100K | 短段落 | 新冠疫情新闻 |
| CmedqaRetrieval | 3,999 | ~100K | 短段落 | 在线医疗咨询 |
| EcomRetrieval | 1,000 | ~100K | 短段落 | 阿里电商 |
| MedicalRetrieval | 1,000 | ~100K | 短段落 | 阿里医疗 |
| VideoRetrieval | 1,000 | ~100K | 短段落 | 阿里视频 |

### 数据格式（以 T2Retrieval 为例）

三个 HuggingFace 数据集：

```
C-MTEB/T2Retrieval        → corpus (passages) + queries
C-MTEB/T2Retrieval-qrels  → query-passage 相关性标注
```

| 文件 | 内容 | 示例 |
|------|------|------|
| `corpus-*.parquet` | `{id, text}` | `"pid_001", "胆碱是人体必需..."` |
| `queries-*.parquet` | `{id, text}` | `"qid_001", "胆碱重要吗"` |
| `dev-*.parquet` | `{qid, pid, score}` | `"qid_001", "pid_001", 1` |

查询示例（非常自然的真实提问）：
```
'大学怎么网上选宿舍'
'怎么判断鱼卵是否活着'
'中国进口鳕鱼主要那些国家'
'生产过后怎么还有一层肚子'
```

---

## 可行性判断

### 能测什么

| goagent 能力 | T2Retrieval 能否测 | 说明 |
|-------------|-------------------|------|
| 向量检索质量 (vector_global) | ✅ 直接测试 | passages ≈ chunks，直接测 embedding → 检索链路 |
| 关键词检索 (keyword) | ✅ 直接测试 | 中文关键词匹配，pg_trgm 效果 |
| 混合检索 + 融合排序 | ✅ 直接测试 | 多通道 → fusion → dedup → rerank 全链路 |
| 检索指标 (MRR/NDCG/Recall) | ✅ 直接测试 | 有金标准 qrels（22K 查询 × 5.2 相关段落） |
| Markdown 切分质量 | ❌ 测不了 | passages 不是 markdown，无 section/heading/code |
| Metadata 检索 | ❌ 测不了 | passages 无 metadata（无 section、source_file_name） |
| 真实文档级 RAG | ❌ 测不了 | 段落级，不是多段落文档 |

### 核心约束

1. **粒度不匹配**：C-MTEB 是段落级（~800 字 ≈ 1 个 chunk），goagent 是文档级。passages 不需要切分，直接一条 passage = 一个 chunk。
2. **无 metadata**：passages 没有 section、heading_path、code_language，metadata_title 通道无法发挥作用。
3. **数据量**：118K passages 全部上传到 pgvector + embedding 需要 ~$50-200 embedding API 费用（取决于模型定价）。建议抽取子集。

---

## 集成方案

### 方案：T2Retrieval 抽取子集 → 知识库 → retrieve-eval

#### Step 1：获取数据（Python 脚本）

```python
# scripts/fetch_t2retrieval.py
from datasets import load_dataset
import json

# 加载 corpus, queries, qrels
corpus = load_dataset("C-MTEB/T2Retrieval", split="dev")
qrels = load_dataset("C-MTEB/T2Retrieval-qrels", split="dev")

# 抽取子集：取前 5000 个 passages + 1000 个 queries
# （控制 embedding 成本，同时保证统计显著性）
passages = {}
for i, row in enumerate(corpus):
    if i >= 5000:
        break
    passages[row['id']] = row['text']

# 找到有相关 passage 的 queries
queries_seen = set()
eval_samples = []
for row in qrels:
    qid = row['qid']
    pid = row['pid']
    if pid in passages and qid not in queries_seen and len(eval_samples) < 200:
        queries_seen.add(qid)
        # 收集该 query 的所有相关 passages
        related = [r['pid'] for r in qrels if r['qid'] == qid and r['pid'] in passages]
        eval_samples.append({
            "qid": qid,
            "query": row['queries_text'],  # 需要 join
            "related_pids": related,
        })

# 输出
with open('testdata/t2retrieval_passages.json', 'w') as f:
    json.dump(passages, f, ensure_ascii=False)
with open('testdata/t2retrieval_samples.json', 'w') as f:
    json.dump(eval_samples, f, ensure_ascii=False)
```

#### Step 2：导入 goagent 知识库（Go 工具）

```bash
go run ./cmd/corpus-loader \
  -input testdata/t2retrieval_passages.json \
  -kb t2retrieval-bench \
  -chunk-strategy fixed_size   # passages 很短，fixed_size 直接当作单 chunk
```

每条 passage 作为一个"文档"上传 → markdown parser 做空操作（纯文本无 markdown）→ fixed_size chunker 产 1 个 chunk → embed → 写入向量库。

#### Step 3：生成 eval samples

将 `t2retrieval_samples.json` 转换为 goagent 的 `retrieve_eval_samples.json` 格式：

```json
{
  "samples": [
    {
      "name": "t2_q001_choline",
      "query": "胆碱重要吗",
      "tags": ["t2retrieval", "semantic", "medical"],
      "target": "chunk",
      "expectedIds": ["chunk-pid-001", "chunk-pid-005"],
      "searchMode": "hybrid",
      "topK": 10
    }
  ]
}
```

#### Step 4：执行评估

```bash
go run ./cmd/retrieve-eval \
  -input testdata/t2retrieval_eval_samples.json \
  -execute -k 1,3,5,10 \
  -json > testdata/results/t2retrieval_results.json
```

---

## 适用场景

### 可以回答的问题

| 问题 | 方法 |
|------|------|
| goagent 的向量检索在中文上的 MRR/NDCG 是多少？ | 跑 T2Retrieval 200 queries，得出绝对指标 |
| 开 hybrid（向量+关键词）比纯 semantic 好多少？ | 同一个样本用不同 searchMode 跑两次，对比 A/B |
| rerank 提升多少 NDCG？ | 同一份结果开关 rerank 对比 |
| 和 BGE 原生模型比差距多少？ | C-MTEB 上有 BGE 的公开基准分，直接对比 |

### 不能回答的问题

| 问题 | 原因 |
|------|------|
| markdown chunker 比 fixed_size chunker 检索效果好多少？ | passages 不是 markdown |
| metadata_title 通道对 section 检索有多大提升？ | passages 没有 section metadata |
| 真实文档场景下的端到端 RAG 质量？ | C-MTEB 是 passage 检索，不是文档级 RAG |

---

## 性价比评估

| 维度 | 评分 | 说明 |
|------|------|------|
| 数据质量 | ★★★★★ | 真实中文查询，人工标注相关性 |
| 接入成本 | ★★★☆☆ | 需要 Python 脚本取数据 + Go 工具导入 |
| Embedding 成本 | ★★★☆☆ | 5000 passages × embedding 单价，约 $10-50 |
| 对 goagent 的覆盖度 | ★★★☆☆ | 只测向量检索链路，不测 chunk/markdown/metadata |
| 长期可复用性 | ★★★★★ | 一次导入后可反复跑不同 searchMode/rerank 对比 |

**结论**：T2Retrieval 最适合用来做**检索链路的绝对性能基准**和**search mode / rerank 的 A/B 对比**。但它不能替代"真实 markdown 文档 + metadata 检索"的专项测试——那个需要自建中文 Markdown 语料（方案文档 Phase 5）。

建议优先用 T2Retrieval 跑通检索 benchmark，得到一个可引用的绝对指标（"goagent hybrid search MRR=0.XX on T2Retrieval subset"），然后再用自建 markdown 语料做 chunk 专项评测。
