# 图检索价值检验：跨文档证据基线

日期：2026-09-27。

后续已启动独立 LightRAG 服务，并选取 HotpotQA 的 20 道双证据问题扩展对照；运行方法和当前结果见 [LightRAG 多文档检索实验](../experiments/graph_retrieval/README.md)。下文“未运行 LightRAG”等描述只指本报告的第一阶段 4 题检验。

## 检验范围

从 `testdata/corpus/manifest.json` 的 5 份文档、41 个 chunk 中，人工编写 4 个需要两个不同文档共同作答的问题。每条样本的两个 `expectedIds` 均在 manifest 中核对过，题目和标注见 `docs/graph_retrieval_probe_samples.json`。这是一组探索性小样本，不能代表线上问题分布。

本次运行的是 goagent 现有检索，不是图检索：本机未运行 LightRAG 或 Neo4j，也未发现可用于这组语料的已建图索引。历史 54 题评测集的 `multi_chunk` 标签大多表示同一文档内的多个 chunk，因此不能替代此次跨文档检验。

## 实测结果

所有模式使用同一批问题、同一知识库，输出 Top 5。`完整证据@5` 表示两个标注 chunk 都被检出；`任一证据@5` 是现有评测工具的 Hit@5；`平均证据召回@5` 是两个标注 chunk 的平均召回率。

| goagent 检索模式 | 完整证据@5 | 任一证据@5 | 平均证据召回@5 |
| --- | ---: | ---: | ---: |
| 关键词，关闭重排 | 3/4 | 4/4 | 0.875 |
| 混合，关闭重排 | 2/4 | 4/4 | 0.750 |
| 混合，配置的重排器 | 2/4 | 4/4 | 0.750 |

逐题 `完整证据@5`：

| 问题 | 关键词 | 混合无重排 | 混合含重排 |
| --- | --- | --- | --- |
| Go 内存泄漏 + Linux 排查 | 缺 1 条 | 齐 | 缺 1 条 |
| Go SSE 泄漏 + Linux 排查 | 齐 | 缺 1 条 | 缺 1 条 |
| HTTPS 概念 + Linux 验证命令 | 齐 | 齐 | 齐 |
| Redis 事务 + 执行模型 | 齐 | 缺 1 条 | 齐 |

现有 Hit@5 会把只命中一份证据的题算作成功，因此不能单独衡量跨文档问答。混合检索与重排在此小样本上没有稳定改善完整证据覆盖；目前不能据此推断图检索会改善它们。

## 漏证据定位

评测器将每题的候选池扩到 20，再要求重排器输出 5 条。新增的 `-executed-output` 明细保存了每个通道的候选 ID、融合后重排前的 ID 顺序、最终 ID 顺序。稳定重跑后，两道缺证据的题都能确认：**两条标注证据均进入重排前候选池，其中一条被重排器排出最终 5 条**。

| 问题 | 被排出的证据 | 关键词排名 | 混合融合后排名 | 重排后 |
| --- | --- | ---: | ---: | --- |
| Go 内存泄漏 + Linux 排查 | Go.md 的泄漏原因 | 3 | 2 | 未进前 5 |
| Go SSE 泄漏 + Linux 排查 | linux.md 的线上排查流程 | 5 | 6 | 未进前 5 |

这说明当前两处精确 ID 失分不是图通道才能解决的“完全召不回”：至少在这组题里，重排器会改变被标注 chunk 的位置。另两题中，HTTPS 题的两条标注 chunk 始终齐全；Redis 题经重排由缺 1 条变为两条齐全。融合也会改变位置：SSE 的 Linux 标注块从关键词第 5 名移到混合第 6 名，Redis 的事务标注块从关键词第 1 名移到混合第 6 名。若再加图通道，需要观察融合后是否挤掉原本可用的来源。

**2026-09-28 复核：**这些资料使用重叠切块，另一块可能包含相同或更具体的证据。SSE 题被排出的 Linux 标注块在关键词重排中排第 7，但前 5 中的另一 Linux 块含有 `pprof/goroutine`、`ss` 等排查命令。按人工审阅的证据概念组计算，扩展后的七题在关键词通道候选 5、10、20 三种配置下均为 7/7。故上面的精确 ID 失分不能直接解释为答案证据缺失；详见 `docs/retrieval_budget_probe_report.md`。

一次带 20 秒逐题超时的运行出现向量查询超时，因此诊断采用随后保存了两个检索通道结果的完整重跑。虽然 4 题的总指标一致，单次服务延迟和排序波动仍限制结论外推。含重排结果只输出 5 条，所以其 `@10` 指标并不代表从 10 个最终结果中检索。

## 对 goagent 的判断

goagent 已有 Wiki 页的关键词命中加一跳邻居召回，但当前文档处理链路不自动生成 Wiki 页与链接。这是一个可利用的轻量关系检索接口，不能视为本次已验证的图检索效果。Ragent 的 LightRAG 是一个独立检索通道，经 RRF 与其他通道融合；其单实例全图查询后按来源路径过滤知识库范围。移植时必须验证知识库隔离、来源 chunk 可追溯、文档更新/删除后的同步，以及构图耗时和费用。

### 本地图数据与真实提问核查

只读核查本地 PostgreSQL：共有 53 个 Wiki 页面、180 条链接，但分布于另外两个知识库；此次 `kb_eval` 知识库中页面和链接均为 0。现有 53 个页面的 `source_chunk_ids` 均为空，不能直接充当可追溯到原文 chunk 的图证据。当前 Wiki 通道能做页面邻居扩展，但不能用它对这组 4 题完成有效的图检索 A/B。

库中有 328 条用户提问。筛出的 30 条包含 Go、Redis 或传感器关键词的问题主要是单主题提问或 Wiki 生成测试；其中未找到能直接用于此次五篇文档、且有可核验跨文档证据标注的真实问题。这是数据覆盖限制，不应通过把已有链接当作答案标签来制造图通道收益。

Ragent 的图检索栈要求 Neo4j、LightRAG、PostgreSQL 与模型密钥。本机 9621/7687 端口未开放，Ragent 图栈目录没有运行所需的 `.env`，当前账户查询 Docker Desktop 引擎也得到权限拒绝。因此本轮没有实际构图或 LightRAG 查询，不能报告图检索增益。

建议先扩充到至少 20 条由真实使用问题改写、确实需要跨文档关系的样本，保留单跳题作回归。当前 5 篇面试资料不足以凭空构造代表真实使用分布的 20 题。先用完整证据覆盖检查重排与融合，再让一个只读图通道与现有检索并行，对照完整证据@5、答案引用正确率、P95 延迟和构图成本。只有图通道能稳定补齐原本缺失的来源证据，且不引入明显回归时，才值得接入默认链路。本次结果不足以支持直接部署 LightRAG。

## 复现

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go run ./cmd/retrieve-eval -input docs/graph_retrieval_probe_samples.json -execute -search-mode keyword -disable-rerank -k 1,3,5,10 -json -output testdata/graph_retrieval_probe_keyword_result.json -executed-output testdata/graph_retrieval_probe_keyword_executed.json
go run ./cmd/retrieve-eval -input docs/graph_retrieval_probe_samples.json -execute -search-mode hybrid -disable-rerank -k 1,3,5,10 -json -output testdata/graph_retrieval_probe_hybrid_result.json -executed-output testdata/graph_retrieval_probe_hybrid_executed.json
go run ./cmd/retrieve-eval -input docs/graph_retrieval_probe_samples.json -execute -search-mode hybrid -k 1,3,5,10 -json -output testdata/graph_retrieval_probe_hybrid_rerank_result.json -executed-output testdata/graph_retrieval_probe_hybrid_rerank_executed.json
```

三个 `*_result.json` 保留评测汇总和逐题指标；三个 `*_executed.json` 保留候选 ID 与处理轨迹。`testdata` 在本仓库被忽略，这些输出是本机诊断材料，提交后的可复跑输入和结论在 `docs/`。复跑依赖本地数据库、向量与模型服务的实际状态；结果可能随索引和模型配置变化。
