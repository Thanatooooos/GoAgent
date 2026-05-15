# Retrieve Merge 合并逻辑

更新时间：2026-05-14

## 概览

这个模块讲的是检索链路里的一个很关键但很容易被忽视的工程问题：

当系统把一个用户问题拆成多个子问题分别检索后，最终如何把这些结果合并成一个既尽量保留召回能力、又不让上下文爆炸的结果集？

这个问题看起来像一个“小工具函数”，但实际上它决定了：

- prompt 里塞多少上下文
- 多子问题检索的收益能否真正落地
- 系统是否会被重复 chunk 淹没
- 后续诊断时能否看出通道与召回情况

一句话理解：`MergeResults(...)` 负责把“多路召回”收敛成“可消费上下文”。

## 这个模块在整个系统里的位置

它位于主链路中的 retrieve 阶段内部。

整体顺序是：

1. rewrite 先把问题改写成主问题 + `subQuestions`
2. `runRetrieveStage(...)` 对每个 `subQuestion` 分别调用 retrieve
3. 收到多份 `ragretrieve.Result`
4. 通过 `ragretrieve.MergeResults(...)` 合并
5. 把 merged result 交给 prompt 阶段和 trace 阶段

所以 merge 不是独立能力，而是多子问题检索策略的一部分。

## 功能

这个模块主要做 4 件事。

### 1. 对多个 retrieve 结果按 chunk 去重

每个子问题都可能召回一批 chunk。如果不去重，最终 prompt 里会出现：

- 同一 chunk 重复多次
- 上下文长度膨胀
- 模型注意力浪费在重复内容上

所以 merge 的第一职责就是去重。

### 2. 在重复时保留得分更优的那一份

如果两个子问题都命中同一个 `chunk.ID`，系统不会随机保留，而是保留分数更高的结果。

这背后的隐含逻辑是：

- 同一 chunk 在不同子问题下可能打分不同
- 更高分通常意味着更贴近该轮检索目标

### 3. 聚合多次 retrieve 的通道元数据

除了 chunk 本身，系统还会把多个结果中的：

- `SearchChannels`
- `ChannelStats`

做一次聚合。

这让 merged result 不只是“最终选中的 chunk 列表”，还保留了“它们是怎么被找出来的”这层信息。

### 4. 重新生成统一的 `KnowledgeContext`

merged result 最终会重新构建：

- `KnowledgeContext`

这样 prompt 阶段消费的就是一份去重、裁剪、重新拼接后的上下文，而不是把各个子问题的 context 直接拼起来。

## 核心代码

### 1. 主链路触发点

- 文件：`internal/app/rag/service/rag_chat_service.go`
- 函数：`runRetrieveStage(...)`

这段代码的关键逻辑是：

1. 先取 `rewriteResult.SubQuestions`
2. 如果为空，就退回用户原问题
3. 对每个子问题执行一次 `retrieveService.Retrieve(...)`
4. 全部失败则对原问题再查一次
5. 有多份结果则 `MergeResults(results, defaultTopK)`

### 2. merge 实现本体

- 文件：`internal/app/rag/core/retrieve/search_types.go`
- 函数：`func MergeResults(results []Result, topK int) Result`

这是最核心的实现。

### 3. 通道元数据聚合

- `collectSearchChannels(...)`
- `collectChannelStats(...)`
- `mergeResultMetadata(...)`

这些辅助函数负责把不同结果中的通道信息合并起来。

## `runRetrieveStage(...)` 的真实工作方式

理解 merge，最好先理解它的调用方。

在 `runRetrieveStage(...)` 中，系统做的是：

### 1. 从 rewrite 拿 `SubQuestions`

rewrite 阶段可能把一个复杂问题拆成多个子问题，例如：

- 主问题：A
- 子问题：A1、A2、A3

这么做的原因是：复杂问题通常包含多个检索意图，拆开查比整句查更容易命中知识库。

### 2. 对每个子问题单独查询

每个子问题都会调用：

- `retrieveService.Retrieve(...)`

这意味着 retrieve 的基本单位不是“一轮用户问题”，而是“一个检索子问题”。

### 3. 兜底逻辑

如果所有子问题的 retrieve 都失败了，系统不会直接报错，而是再用原始问题查一次。

这是一个很实用的防御性设计，因为 rewrite 结果未必总是理想。

### 4. 对结果做 merge

只要拿到了多份结果，就会进入：

- `merged := ragretrieve.MergeResults(results, defaultTopK)`

这一步就是从多路召回切回统一上下文的关键。

## `MergeResults(...)` 是怎么工作的

代码实现非常直接，但背后的取舍值得细讲。

### 第一步：遍历所有结果，按 `chunk.ID` 建 map

核心结构是：

- `chunkMap := map[string]convention.RetrievedChunk{}`

遍历所有结果、所有 chunk 时：

- 如果 `chunk.ID` 还没出现，就放进去
- 如果已经出现，就比较 score，保留更高分那条

这一步完成了最核心的去重逻辑。

### 第二步：把 map 里的 chunk 拉平成 slice

得到唯一 chunk 集合之后，系统会把它们重新放进切片。

### 第三步：按 score 从高到低排序

排序规则是：

- 分数高的在前

这一步决定了最终上下文的优先级。

### 第四步：按 topK 截断

如果传入了 `topK`，并且合并后 chunk 数量过多，就截断。

这一步很重要，因为多子问题检索天然容易带来更多召回，如果不裁剪，prompt 很快就会失控。

### 第五步：聚合通道元数据

`mergeResultMetadata(results)` 会把多份结果中的：

- `SearchChannels`
- `ChannelStats`

进行统一聚合。

### 第六步：重新构建 `KnowledgeContext`

最终返回：

- `Chunks`
- `KnowledgeContext`
- `SearchChannels`
- `ChannelStats`

也就是说 merged result 已经是一份完整、可直接被 prompt 消费的结果对象。

## 为什么按 `chunk.ID` 去重

这是一个非常典型的工程折中。

### 这样做的优点

#### 1. 简单直接

不需要额外做内容相似度比较，也不需要引入复杂的语义聚类逻辑。

#### 2. 稳定

只在“明确是同一个 chunk”时去重，误伤概率低。

#### 3. 成本低

map 去重的复杂度和实现成本都很低，非常适合作为第一版稳定方案。

### 这样做的局限

#### 1. 只能处理“同 ID 重复”

如果两个 chunk 文本内容几乎一样，但来自不同 chunk id，当前不会合并。

#### 2. 会丢失命中次数信息

如果同一个 chunk 被多个子问题命中，系统最终只保留一份 chunk，不会保留“它被命中了几次”的统计特征。

#### 3. 跨子问题 score 的可比性并不绝对

从工程上看，保留高分那条是合理的；但从理论上说，不同子问题下的 score 不一定完全可比。

所以最准确的评价不是“这个方案完美”，而是：

- 这是一个低复杂度、低误伤、足够稳定的第一版实现

## 通道元数据为什么要一起 merge

这部分很值得讲，因为很多系统做 merge 时只留下 chunks，把“怎么召回到的”信息丢掉了。

当前实现没有这么做，而是保留了：

- `SearchChannels`
- `ChannelStats`

### `SearchChannels` 的意义

它告诉你本次结果来自哪些检索通道，比如：

- `vector_global`
- `keyword`
- `metadata_title`

### `ChannelStats` 的意义

它进一步告诉你：

- 每个通道返回了多少 chunk
- 每个通道耗时多少
- 有没有错误
- 有没有附加 metadata

这对于排障非常有用，比如：

- 为什么最后没命中？
- 是 keyword 没命中，还是 vector 没命中？
- 是通道本身报错了，还是只是召回为空？

所以 merge 不只是“去重器”，还是“检索证据整合器”。

## 当前版本的设计判断

如果从工程角度评价，这套设计是合理的，原因如下：

### 1. 它优先解决了最现实的问题

最现实的问题不是“语义去重不够完美”，而是“多子问题检索后上下文会不会爆炸”。

按 `chunk.ID` 去重已经能立刻缓解这个问题。

### 2. 它保留了未来演进空间

当前实现虽然没有做：

- 内容级去重
- hitCount 聚合
- 通道贡献加权

但返回结果结构并没有把这些扩展彻底堵死。

### 3. 它和 trace / diagnosis 体系是协同的

因为保留了 `SearchChannels / ChannelStats`，所以后续 trace、tool diagnose、后台排障都能继续利用这些信息。

## 如果后续要继续优化，可以怎么做

有 3 个比较自然的方向。

### 1. 保留 `hitCount`

对于被多个子问题同时命中的 chunk，可以增加聚合特征：

- `hitCount`
- `matchedSubQuestions`

这样在排序或 prompt 压缩时，可以把“多次命中”当作强相关信号。

### 2. 增加内容级近似去重

对于内容高度重叠但 `ID` 不同的 chunk，可以再做一层近似聚合。

不过这一层必须很谨慎，否则误伤会比较大。

### 3. 对通道贡献做更细粒度统计

例如：

- 最终 topK 中每个通道贡献了多少 chunk
- 哪个通道更常在高分区命中

这会对检索策略优化很有帮助。

## 值得注意的设计细节

### 1. merge 是 RAG 主链路质量的重要组成部分

它不是一个“工具函数细节”，而是决定最终上下文质量的关键节点。

### 2. `KnowledgeContext` 是 merge 后重建的

这意味着 prompt 消费的是统一视图，而不是各子问题结果的简单拼接。

### 3. merge 兼顾了“结果消费”和“排障消费”

既输出最终 chunks，也输出 channel 级元数据，这一点很成熟。

### 4. 当前 `resolveRetrieveSearchMode()` 已经稳定走 `hybrid`

这意味着现阶段系统重点不再是复杂模式判定，而是把多通道召回和合并做稳。

## 预测面试题

1. 你们为什么要把一个问题拆成多个 `subQuestion` 去检索？
2. 多次 retrieve 的结果最后是怎么合并的？
3. `MergeResults(...)` 为什么按 `chunk.ID` 去重？
4. 如果多个子问题命中同一个 chunk，会如何处理？
5. 为什么只保留高分 chunk，而不是把所有命中都保留？
6. 按 `chunk.ID` 去重的优点和局限分别是什么？
7. `SearchChannels / ChannelStats` 为什么要跟着 merge 一起保留？
8. 如果继续优化 merge，你会先补哪一层能力？

