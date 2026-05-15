# Interview 深度复习目录

更新时间：2026-05-14

这个目录不是提纲，而是给“第一次接触这个项目的人”准备的模块化说明文档。

目标不是只告诉你“这个模块有哪些点”，而是尽量回答下面这几类问题：

- 这个模块在整个系统里到底处于什么位置？
- 它解决什么业务问题？
- 数据是怎么流过来的，又是怎么流出去的？
- 代码里最重要的对象、函数、阶段分别是什么？
- 为什么这么设计，而不是用另一种更直观但更脆弱的写法？
- 如果面试官沿着某个实现细节追问，应该怎么展开？

## 阅读顺序

建议按下面顺序阅读：

1. [01_rag_agent_main_flow.md](D:/goagent/docs/interview/01_rag_agent_main_flow.md)
   - 先建立系统主链路视角，知道一个聊天请求会经过哪些阶段。
2. [02_trace_sse_observability.md](D:/goagent/docs/interview/02_trace_sse_observability.md)
   - 再理解系统是如何把主链路过程记录下来、暴露出来的。
3. [05_streaming_chat_and_cancellation.md](D:/goagent/docs/interview/05_streaming_chat_and_cancellation.md)
   - 然后看真正的流式输出与取消链路，补全“请求是怎么结束的”。
4. [03_retrieve_merge.md](D:/goagent/docs/interview/03_retrieve_merge.md)
   - 再看检索层里一个典型的工程取舍点。
5. [04_infra_ai_routing_and_circuit_breaker.md](D:/goagent/docs/interview/04_infra_ai_routing_and_circuit_breaker.md)
   - 最后看底层模型路由与熔断，这部分适合面试深挖。
6. [06_ingestion_pipeline.md](D:/goagent/docs/interview/06_ingestion_pipeline.md)
   - 把知识导入主链、执行器、observer、ExecutionState 和 indexer 生产化细节连起来看。

## 当前已整理模块

- `01` RAG / Agent 总入口与主链路
- `02` Trace / SSE 可观测性
- `03` Retrieve Merge 合并逻辑
- `04` Infra-AI 模型选择、路由执行与熔断
- `05` 流式聊天、首包探测与取消
- `06` Ingestion 流水线与执行架构

## 这些文档之间的关系

可以把整个系统想成 5 层：

1. `RagChatService` 主编排层
   - 决定一个请求按什么阶段推进。
2. `rewrite / retrieve / tool workflow / prompt` 业务处理层
   - 负责把“用户问题”变成“模型可回答的问题”。
3. `trace / SSE` 观测层
   - 负责把内部过程记录和输出出来。
4. `routing llm / selector / circuit breaker` AI 基础设施层
   - 负责模型调用的稳定性。
5. `stream callback / cancel / sender` 流式输出层
   - 负责把结果稳定推给前端，并支持中途停止。

如果后面继续扩展目录，最自然的新增模块是：

- `ingestion` 流水线
- `knowledge -> ingestion -> retrieve` 闭环
- `memory` 会话记忆
- `tool workflow / agent loop` 专题
