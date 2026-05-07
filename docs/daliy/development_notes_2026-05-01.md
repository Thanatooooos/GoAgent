# Development Notes 2026-05-01

更新时间：2026-05-01

这份文档记录今天已经确认并解决的问题、踩到的坑，以及对后续开发有指导意义的工程结论。  
说明：

- 这里只记录今天已经确认下来的代码层、联调层和架构层问题
- 不记录容器编排、机器权限或外部环境偶发现象

## 今日完成的工作

### 1. 收口了 RAG 安全与校验问题

已完成：

- `/rag/settings` 不再向前端返回 provider `apiKey`
- `knowledge document` 的 `processMode / chunkStrategy / pipelineId` 已在提交阶段严格校验

意义：

- 避免服务端密钥泄露到前端
- 避免非法处理配置被持久化到数据库

### 2. 打通了最小 chat 主链路

已完成：

- `RagChatService`
- `rag` HTTP handler
- `bootstrap/rag/runtime.go`
- `cmd/server/main.go` 路由注册
- SSE 聊天、stop、消息落库、trace 收口

意义：

- RAG 从“基础积木已存在”推进到“真正可多轮问答”

### 3. 补齐了 chat 全链路文档

已新增：

- `docs/chat_full_chain.md`

内容覆盖：

- 前端请求发起
- 后端每个关键函数
- 流式调用过程
- 取消句柄作用方式
- 会话历史持久化与上下文搭建

### 4. 补齐了 trace 最小查询闭环

已完成：

- trace run 分页接口
- trace detail 接口
- trace nodes 接口
- runtime 注入与路由注册
- `docs/rag_trace_query.md`

意义：

- trace 从“只能写不能查”变成“可用于后台排障”

### 5. 明确了 ingestion 的定位和方向

已新增：

- `docs/ingestion_module_goal.md`

今天已经明确：

- ingestion 应作为独立模块建设
- 核心交互是 `pipeline + task`
- 第一阶段应先做最小执行闭环
- EINO 适合做执行编排层，不适合直接替代整个模块

## 今日解决的问题

### 1. chat 多轮会话被错误拆成多个单轮会话

问题表现：

- 左侧历史会话里不断出现只有一轮的新会话
- 本应属于同一轮对话的问题被拆散

最终根因：

- 后端 SSE `meta` 事件的字段名和前端预期不一致
- 后端返回的是 `ConversationID / TaskID`
- 前端读取的是 `conversationId / taskId`

结果：

- 第一轮新建出来的 `conversationId` 没有被前端正确接住
- 后续消息就无法稳定续接同一个会话

处理结果：

- 给 `RagChatMeta` 明确补上 JSON 标签
- 前端续聊逻辑恢复正常

结论：

- SSE 事件结构不能只靠 Go 默认 JSON 字段名
- 只要前后端约定里使用了 camelCase，就必须显式写 JSON tag

### 2. trace 节点详情为空

问题表现：

- trace 列表有 run
- 详情页却显示“暂无节点记录”

最终根因：

- `t_rag_trace_node.id` 表结构是 `varchar(20)`
- 代码之前用 `traceID:nodeID` 作为 node 主键
- 实际长度超出字段限制，导致 node 插入失败

更隐蔽的问题：

- `recordTraceNode(...)` 的调用方没有把这类写入失败暴露到主链路表象
- 所以表面上 chat 成功、run 成功，但 node 一直是空

处理结果：

- trace node 改为使用独立短 ID
- 新 trace 节点可正常落库

结论：

- 观测类数据即使不阻断主链路，也不能默默失败太久
- 关键 tracing/日志写入至少要有可见告警或排障入口

### 3. trace 列表排序不符合“最近优先”

问题表现：

- trace 列表里旧 run 可能排在新 run 前面

根因：

- 旧数据里 `start_time` 可能为空
- 单纯按 `start_time desc` 排序时，会导致结果不稳定

处理结果：

- 改成 `coalesce(start_time, create_time) desc`
- 再补 `create_time desc`

结论：

- 列表排序不能假设历史数据完全完整
- 排序字段要考虑缺失值回退策略

### 4. trace 列表里大量字段为空

问题表现：

- `Trace Name`
- `执行时间`
- `用户名`

出现大量 `-`

原因分类：

#### 一类是历史数据本身不完整
- 比如旧 run 没有写 `traceName / startTime`

#### 一类是查询返回没有做展示层回退
- 比如 `startTime` 可以回退到 `createTime`
- `username` 可以通过 `userId` 再解析

处理结果：

- 查询层补了 `traceName / entryMethod / startTime` 的回退值
- trace 查询支持通过 `userId` 解析真实 `username`

结论：

- 后台观测页的查询接口不应只机械回传数据库原值
- 对历史数据和兼容期数据，要允许“查询层做适度修复性回退”

## 今日踩到的坑

### 1. 不要把“前端状态问题”和“协议字段问题”混为一谈

今天 chat 会话拆分问题一开始很像前端 store 丢状态，但最终根因是协议字段名不一致。  
教训是：

- 当联调表现像“状态不稳”时，必须先检查请求/响应契约
- 先确认实际发出的 `conversationId`
- 再确认 SSE 返回的 `conversationId`

不要过早把问题归因到前端状态管理。

### 2. 历史数据会持续影响新功能观感

今天 trace 功能里，列表和详情的很多“空值”问题，不全是当前代码逻辑错误，而是旧 run 的历史遗留。  
这类问题的教训是：

- 新功能上线时要明确区分“新数据正确”与“历史数据兼容”
- 如果不做回填，就要在查询层做合理降级

### 3. 观测数据表的字段长度不能随手估

trace node 主键长度问题暴露出一个典型陷阱：

- 业务主键长度可能短
- 但“拼接型主键”很容易超过预期

后续类似表设计时：

- 观测/日志类表优先用独立 ID
- 不要默认拼接串一定安全

## 今天确认下来的工程结论

### 1. chat 和 trace 已经足够支撑后续继续推进 ingestion

当前项目主矛盾已经不是“RAG chat 能不能跑”，而是：

- ingestion / pipeline 后端仍未建立

所以后续重点应转向 ingestion，而不是继续在 chat 上做次优先级优化。

### 2. ingestion 应独立成模块

今天已经确认：

- ingestion 不适合继续塞在 `knowledge` service 里
- 应当单独建设 `domain / repo / service / http / runtime`

### 3. EINO 可以考虑用于执行编排层

今天对 ingestion 的结论是：

- `EINO` 适合做 executor 内部的 workflow/graph 编排
- 不适合直接承担 pipeline CRUD、task 管理、上传和权限等业务壳

## 对明天开发的建议

建议下一步直接进入 ingestion 第一阶段：

1. 建立独立 `ingestion` 模块骨架
2. 落最小表结构：
   - `pipeline`
   - `task`
   - `task_node`
3. 先打通最小执行链路：
   - `fetcher`
   - `parser`
   - `chunker`
   - `indexer`

这样可以让项目从“chat 已闭环、pipeline 还空白”推进到“两个核心方向都开始有真实后端结构”。
