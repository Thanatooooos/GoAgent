# Tool 模块改造约束

日期：2026-05-18

## 一、目的

本文档用于约束 `internal/app/rag/tool/` 后续改造方式，尤其是当前 P0 收口阶段的实现边界。

后续所有与 tool 模块相关的代码改动，默认以本文档为准；若与旧实现、旧兼容层或临时写法冲突，应优先遵循本文档，再决定是否保留兼容适配。

## 二、适用范围

本文档适用于以下目录：

- `internal/app/rag/tool/`
- `internal/app/rag/tool/core/`
- `internal/app/rag/tool/runtime/`
- `internal/app/rag/tool/modules/`
- `internal/app/rag/tool/invokers/`
- `internal/app/rag/tool/assembly/`

## 三、总体原则

### 3.1 module-first

tool 模块的主路径必须坚持 `module-first`：

- `Invoker` 负责执行
- `Behavior` 负责语义
- `Registry` 负责注册与查找
- `Runtime` 负责编排

不得为了接入新能力，回退到中心化的 `switch toolName` 或临时 if/else 分支表。

### 3.2 显式依赖优先

运行时依赖必须尽量通过构造参数、结构体字段或函数参数显式传递。

除非是明确的只读常量，否则不应新增包级可变全局状态，尤其禁止新增：

- 全局 registry
- 全局 behavior hook
- 全局 spec 推断器
- 依赖初始化顺序才能工作的包级 setter

### 3.3 单一真源

同一份业务语义只能有一个真源。

典型包括：

- tool 行为推断
- workflow control 推断
- next action 推断
- legacy tool 到 module 的适配规则

如果顶层 `tool` 包与 `core/runtime` 同时存在近似实现，应以 `core` 或 `runtime` 为真源，另一侧退化为纯转发或兼容壳。

### 3.4 兼容层只做兼容

`internal/app/rag/tool/` 根目录下的 facade / alias / wrapper 文件，职责仅限：

- 类型别名
- 纯转发
- 向后兼容导出
- 测试辅助兼容

禁止在兼容层继续新增以下内容：

- 新的业务决策逻辑
- 新的推断规则
- 新的工具编排语义
- 与 `core/runtime/modules` 平行的第二套实现

### 3.5 可降级优先于 panic

assembly 阶段对可预期失败应优先采用以下方式处理：

- 返回错误
- 跳过单个 family 注册
- 显式降级

除非是明确的不可恢复编程错误，否则不应使用 `panic` 作为主路径控制流。

### 3.6 增量改造优先

当前阶段目标是收口和减债，不是推倒重写。

允许保留必要兼容层，但要求：

- 兼容层数量只减不增
- 重复逻辑只减不增
- 新逻辑不得继续落在旧壳上

## 四、分层职责约束

### 4.1 `core/`

`core/` 是协议层真源，负责：

- 基础类型
- 接口定义
- module/spec/behavior 抽象
- registry 抽象
- workflow 输入输出协议

`core/` 不负责具体业务 family 的知识，不应依赖具体 tool 名称做中心化分支。

### 4.2 `runtime/`

`runtime/` 是执行编排层真源，负责：

- `Executor`
- `AgentLoop`
- `Observer`
- workflow control 归纳
- result/context/guidance 的运行时拼装

`runtime/` 可以依赖 registry 和 behavior，但不应维护与 `modules/*` 平行的 family 业务语义副本。

### 4.3 `modules/*`

`modules/*` 是 family 语义真源，负责：

- `Decode`
- `Next`
- `Observe`
- `RenderContext`
- `BuildGuidance`

某个 family 的下一步推断、终止条件、结果解释，应优先落在对应 `modules/*` 中，而不是散落在 `runtime` 或顶层 `tool` 包。

### 4.4 `invokers/*`

`invokers/*` 只负责真正执行调用：

- 查系统记录
- 调外部搜索
- 拼 graph
- 返回 `Result`

`invokers/*` 不应承担 tool 链路编排职责。

### 4.5 `assembly/`

`assembly/` 负责：

- 构造依赖
- 注册 module
- 选择 runtime 组件
- 组装 workflow

`assembly/` 不应承载大量业务规则；新增规则应尽量下沉到 `modules/*` 或配置化结构。

### 4.6 顶层 `tool/`

顶层 `tool/` 包是兼容入口，不是新的业务实现层。

后续若新增业务能力，默认应落在：

- `core/`
- `runtime/`
- `modules/*`
- `invokers/*`
- `assembly/`

而不是继续堆在顶层 `tool/*.go`。

## 五、P0 阶段硬约束

### 5.1 不再新增全局可变 registry 路径

P0 改造期间，所有新提交禁止新增类似以下模式：

- `var xxxRegistry *Registry`
- `SetXxxRegistry(...)`
- `SetInferXxx(...)`

目标是逐步把已有全局状态改为显式注入，而不是继续扩散。

### 5.2 不再新增重复实现

P0 期间，如果发现某个逻辑在两处都有：

- 优先抽到真源
- 另一处改为转发
- 不允许复制后再改第三份

### 5.3 不在 compat 文件中继续补业务逻辑

以下类型文件默认视为 compat 文件：

- `*_forward.go`
- `modular_wrappers.go`
- 根目录下仅做 alias/wrapper 的文件

P0 期间禁止继续向这些文件写入新的核心语义。

### 5.4 assembly 主路径不新增 panic

P0 期间如果新增或调整 family 注册逻辑：

- 优先返回错误
- 或记录降级原因并跳过

不得新增新的 `panic(...)` 作为常规失败处理。

## 六、允许的兼容策略

为降低改造风险，以下兼容策略是允许的：

- 保留旧导出符号，但内部改为转发到真源
- 保留 legacy adapter，但其行为来源必须是单一真源
- 保留 legacy string hint 兼容，但结构化 `HintCall` 是主语义
- 保留旧测试入口，但新增测试应优先面向真源层

## 七、测试约束

凡是触及以下内容的改动，必须补充或更新测试：

- `AgentLoop` 规划与观察分支
- registry 注入路径
- legacy adapter 行为推断
- workflow control / trace meta 推断
- family behavior 的 `Next` / `Observe`
- assembly 降级或错误返回路径

测试优先级要求：

1. 先补真源层测试
2. 再保留必要的顶层集成回归

不鼓励只在顶层 `tool` 包加大而全的回归测试，而缺少底层语义测试。

## 八、变更检查清单

每次修改 tool 模块前，默认检查以下问题：

1. 这段逻辑应该放在 `core`、`runtime`、`modules`、`invokers` 还是 `assembly`？
2. 这是不是又在兼容层里补了一套新语义？
3. 这是不是引入了新的全局可变状态？
4. 这是不是复制了已有实现，而不是复用真源？
5. 这条失败路径能否返回错误或降级，而不是直接 `panic`？
6. 是否补上了对应真源层测试？

只要上述任一问题答案不合理，就不应提交代码。

## 九、当前推荐真源

在本轮收口中，默认采用以下真源划分：

- 类型协议：`core/`
- 执行编排：`runtime/`
- family 语义：`modules/*`
- 真实执行：`invokers/*`
- 组装入口：`assembly/`
- 顶层 `tool/`：兼容 facade

## 十、与后续改造的关系

本文档优先服务当前 P0：

- 收口全局 registry / hook
- 收口重复实现
- 冻结兼容层扩散
- 去掉 assembly 中不必要的 panic

后续 P1/P2 改造也应继续遵循本文档，除非明确更新本文档本身。
