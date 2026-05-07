# Development Notes 2026-04-30

更新时间：2026-04-30

这份文档记录今天在代码和联调中已经确认并解决的问题。

说明：

- 这里只记录今天已解决的代码层、联调层问题
- 不记录配置、中间件、容器编排类问题

## 今日解决的问题

### 1. Knowledge 缺少后台鉴权保护

问题表现：

- `knowledge` 相关接口最初未统一挂后台登录与管理员权限
- 存在未收口的访问边界

已完成处理：

- `knowledge` 路由统一要求登录
- 同时要求 `admin` 角色

结果：

- 知识库、文档、分块相关接口已回到后台受控访问范围内

### 2. 知识库 / 文档分页存在性能问题

问题表现：

- 知识库分页存在文档统计 N+1
- 文档分页和 chunk log 分页存在先全量 list 再截页的问题

已完成处理：

- 为文档、chunk log、schedule exec 增加 count 能力
- 分页逻辑改为 count + page query
- 知识库文档数统计改为批量方式

结果：

- 分页逻辑更符合正常数据量增长场景
- 降低了联调期和后续放量时的性能风险

### 3. 缺少文档级 schedule exec 排障入口

问题表现：

- schedule 执行记录已有表结构，但后端缺少排障查询接口

已完成处理：

- 新增文档级 `schedule exec` 分页接口

结果：

- URL 文档刷新和定时处理链路具备了基础排障入口

### 4. 创建知识库时 embedding 模型列表为空

问题表现：

- 前端创建知识库弹窗显示“暂无可用模型”
- 原因是前端依赖 `GET /api/ragent/rag/settings`
- 后端当时未提供该接口

已完成处理：

- 补齐 `GET /api/ragent/rag/settings`
- 返回前端所需的 embedding / chat / rerank / provider 配置结构

结果：

- 创建知识库弹窗已能正常展示 embedding 模型候选

### 5. RocketMQ 客户端日志噪音过大

问题表现：

- RocketMQ client 的 `info` 级别日志大量刷屏
- 正常业务日志难以观察

已完成处理：

- 在 MQ 适配层统一设置 `rocketmq-client-go` 日志级别为 `warn`

结果：

- MQ 仍保留必要告警
- 正常业务日志可读性明显提升

### 6. 文档处理失败后状态未及时回写

问题表现：

- 文档分块失败后，文档状态可能长期停留在 `running`
- MQ 重试场景下状态流转不稳定

根因：

- consumer 真正执行处理前，没有重新显式认领 `running`
- 后续成功 / 失败状态更新又依赖当前状态必须是 `running`

已完成处理：

- 在 `DocumentProcessService.ExecuteChunk` 执行前增加显式 `ensureDocumentRunning`
- 允许从 `pending / failed / success` 重新进入 `running`

结果：

- MQ 重试时状态流转更稳定
- 失败和成功回写不会轻易卡住

### 7. Markdown 语义分块策略名与后端实现不一致

问题表现：

- 前端“语义感知（Markdown友好）”传值为 `structure_aware`
- 后端 chunk selector 仅识别 `markdown`
- 触发分块时会报 `failed to chunk knowledge document`

已完成处理：

- 在 chunk selector 的 strategy normalize 层增加兼容映射
- 将 `structure_aware` 归一化到 `markdown`

结果：

- 前端现有策略值无需修改
- Markdown 语义分块链路已恢复正常

## 今日联调结论

截至今天，`knowledge` 主链路已经从“结构存在”推进到“可实际联调”状态。

当前已验证通过：

- 登录
- 知识库列表
- 创建知识库
- 文档上传
- 手动触发分块
- chunk log 查看
- Markdown 语义分块可执行

## 留待后续处理的重点

今天没有继续展开、但已经明确的重要方向：

- chunk 质量仍需继续做“更接近语义级切分”的增强
- pipeline 模式仍未真正闭环
- retrieval 效果还没有建立评测基线
