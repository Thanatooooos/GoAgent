# LightRAG 多文档检索实验

## 当前状态（2026-09-27）

- LightRAG 1.5.7 与 Ollama 已在独立 Compose 项目 `goagent-graph-probe` 中运行，端口只绑定 `127.0.0.1:9621` 和 `127.0.0.1:11435`。LightRAG 使用独立 Docker 卷中的文件图、KV 和向量存储；未改动 goagent 的 PostgreSQL 或业务知识库。
- 本地模型为 `qwen2.5:3b-instruct` 和 `qwen3-embedding:0.6b`，实测嵌入维度 1024。Docker 分配的总内存约 7.4 GiB，因此本地模型只用于验证服务流程；LightRAG [官方文档](https://github.com/HKUDS/LightRAG/blob/main/docs/LightRAG-API-Server.md)建议本地实体抽取使用更强的模型。
- HotpotQA distractor 验证集的固定子集包含 10 道 bridge、10 道 comparison 问题，共 199 篇去重段落。原始数据及其衍生物采用 [CC BY-SA 4.0](https://hotpotqa.github.io/)；本仓库只提交生成脚本，下载的文本位于被 Git 忽略的 `testdata/`。
- 本地实例 `127.0.0.1:9621` 已验证健康、图接口和 `file_path` 引用链路；本地 3B 模型有 3 篇非支持文档因重复输出失败，不能用作完整语料对照。
- 硅基流动实例 `127.0.0.1:9622` 使用 `Qwen/Qwen3-32B` 抽取关系、`Qwen/Qwen3-Embedding-8B` 生成 1024 维向量，索引位于独立 Docker 卷。199 篇均已 processed，失败 0 篇，20 题的五种模式评测已完成。配置文件 `.env.siliconflow` 被 Git 忽略，密钥从本机已有的 `AI_PROVIDERS_SILICONFLOW_API_KEY` 注入；仓库仅提交无密钥的 `.env.siliconflow.example`。
- 完整索引曾因一次嵌入 API 超时而取消同批 136 篇文档。将 `EMBEDDING_TIMEOUT` 设为 120 秒、嵌入并发限制为 2 后重试成功；这 136 条是批次取消记录，不能解释为 136 次独立模型错误。

## 复现

从仓库根目录执行：

```powershell
python -X utf8 scripts/build_hotpot_graph_probe.py
Copy-Item experiments/graph_retrieval/.env.example experiments/graph_retrieval/.env
docker compose -f experiments/graph_retrieval/compose.yaml up -d
docker compose -f experiments/graph_retrieval/compose.yaml exec -T ollama ollama pull qwen2.5:3b-instruct
docker compose -f experiments/graph_retrieval/compose.yaml exec -T ollama ollama pull qwen3-embedding:0.6b
python -X utf8 scripts/load_hotpot_lightrag.py --wait-seconds 600
python -X utf8 scripts/eval_hotpot_graph_probe.py
python -X utf8 scripts/eval_hotpot_dense.py
python -X utf8 scripts/eval_lightrag_hotpot.py --limit 1 --allow-partial
```

硅基流动双外部模型实例在复制 `.env.siliconflow.example` 为 `.env.siliconflow` 并填入 API Key 后，可运行：

```powershell
docker compose -f experiments/graph_retrieval/compose.yaml --profile siliconflow up -d lightrag-siliconflow
python -X utf8 scripts/load_hotpot_lightrag.py --url http://127.0.0.1:9622 --all --wait-seconds 0
python -X utf8 scripts/eval_hotpot_dense.py --provider siliconflow
python -X utf8 scripts/wait_hotpot_lightrag.py --url http://127.0.0.1:9622 --output testdata/hotpot_siliconflow_results.json --eval-log testdata/hotpot_siliconflow_eval.log
python -X utf8 scripts/summarize_hotpot_lightrag.py --input testdata/hotpot_siliconflow_results.json
```

服务地址：`http://127.0.0.1:9621`。停止服务：`docker compose -f experiments/graph_retrieval/compose.yaml down`。数据位于独立 Docker 卷中，普通 `down` 不删除它。

完整 20 题对照需要先索引全部 199 篇段落。可运行 `python -X utf8 scripts/load_hotpot_lightrag.py --all --wait-seconds 0` 提交文档；重复提交已有段落会跳过。随后运行 `python -X utf8 scripts/wait_hotpot_lightrag.py`，它每 30 秒检查一次状态，发现失败即停止，全部处理完成后自动执行 `eval_lightrag_hotpot.py`。服务中途停止后，文档状态会保留，但处理任务不会自行恢复；等待脚本发现索引未完成且流水线空闲时，会调用 `/documents/reprocess_failed` 恢复待处理及中断文档。结果位于 Git 忽略的 `testdata/hotpot_lightrag_results.json`，逐题日志位于 `testdata/hotpot_lightrag_eval.log`。评测脚本会拒绝把部分索引的结果误报成完整评测。更换嵌入模型或维度前须新建存储卷并重新索引；本次 3B 抽取模型下的结果不应推广为 LightRAG 的性能结论。

## 已测结果

在 199 篇段落的同一全局语料上，找齐两篇支持段落的题数如下：

| 检索方法 | Top 3 | Top 5 | Top 10 |
| --- | ---: | ---: | ---: |
| TF-IDF | 9/20 | 15/20 | 19/20 |
| 同模型纯向量（`qwen3-embedding:0.6b`） | 14/20 | 18/20 | 19/20 |
| 硅基流动纯向量（`Qwen/Qwen3-Embedding-8B`） | 17/20 | 18/20 | 20/20 |

纯向量 Top 5 在 bridge 题上为 `8/10`，在 comparison 题上为 `10/10`。这是后续图通道至少需要对照的基线。

仅有 3 篇已索引文档的第 1 题冒烟测试中，LightRAG `naive` 和 `mix` 返回两篇支持段落，`local`、`global`、`hybrid` 各只返回一篇。这个单题实验确认了模式差异和引用链路，不能与 199 篇段落上的 TF-IDF 指标直接比较。LightRAG API 的 `top_k` 在 `local` 模式表示实体数、在 `global` 模式表示关系数，也不能直接等同于 TF-IDF 的段落 Top K；正式对照应同时记录最终返回的来源段落数。

硅基流动实例的 199 篇完整语料上，按 `file_path` 对齐到支持段落后的结果如下。`前 5 来源` 是每题返回引用中去重后的前五篇；`全部来源` 是不限制最终引用数量时的结果。

| 方法 | 平均返回来源数 | 前 5 来源找齐双证据 | 全部来源找齐双证据 |
| --- | ---: | ---: | ---: |
| 同嵌入模型纯向量 | 5 | 18/20 | — |
| LightRAG naive | 19.7 | 18/20 | 20/20 |
| LightRAG local | 6.0 | 10/20 | 10/20 |
| LightRAG global | 7.7 | 4/20 | 4/20 |
| LightRAG hybrid | 12.3 | 16/20 | 16/20 |
| LightRAG mix | 19.8 | 19/20 | 20/20 |

`mix` 在前五篇来源中相对纯向量多找齐 1 道 bridge 题，没有逐题退步；对应问题是 Guns N' Roses 为《End of Days》宣传曲演出的年份。该题的第二篇支持文档在 `naive` 引用中排第 6，在 `local`、`hybrid`、`mix` 中排第 5。20 题中只有这一题出现双证据覆盖差异，且 `mix` 平均返回约 20 篇来源，因此只能算小样本的增益信号，尚不足以证明接入 goagent 默认检索有稳定收益。`top_k=5` 在 LightRAG 图模式控制实体或关系候选，不直接限制最终来源数；以上“前 5 来源”是对返回引用做的后处理，也未验证答案正确率或端到端延迟。

数据来源：[HotpotQA 官方网站](https://hotpotqa.github.io/)、[HotpotQA 数据集镜像](https://huggingface.co/datasets/hotpotqa/hotpot_qa)。部署依据：[LightRAG 官方服务文档](https://github.com/HKUDS/LightRAG/blob/main/docs/LightRAG-API-Server.md)。
