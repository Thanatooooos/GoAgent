# Repository Update Usage Requirements

更新时间：2026-04-27

## 1. 背景

当前 `goagent` 的 `knowledge` 相关业务层在调用 repository 做更新时，主要仍然依赖“传入完整实体再执行更新”的方式，例如：

- 先查询原实体
- 在内存里修改其中一两个字段
- 再调用 `Update(...)`

这种模式在简单 CRUD 场景下可以工作，但在知识库文档、调度、状态流转等场景里，业务层对“条件更新”的需求明显更强，继续沿用整实体更新会带来较高的使用成本和持续的接口膨胀问题。

本文档用于总结当前已经暴露出的使用问题，并明确后续 repository 改造时应满足的用户需求。

## 2. 当前遇到的主要问题

### 2.1 只更新少量字段时，调用成本过高

典型场景：

- 只想把 `KnowledgeDocument.status` 更新为 `running`
- 只想更新 `updated_by`
- 只想更新调度状态、锁字段、时间字段

但当前 repository 的整实体更新方式要求：

1. 先构造或查询完整实体
2. 保留所有旧字段值
3. 只修改目标字段
4. 再调用 `Update(...)`

这会导致业务层为了一个小更新承担不必要的上下文负担。

### 2.2 条件更新表达能力不足

业务层真实需要的更新，通常不是“按主键直接覆盖”，而是“满足某些前置条件时才更新”，例如：

- `id = ?`
- `deleted = 0`
- `enabled = 1`
- `status <> running`
- `status = running`
- `update_time < threshold`

这类需求本质上更接近 MyBatis-Plus 的 `lambdaUpdate` / `Wrapper` 用法，而不是简单的 `UpdateByID`。

### 2.3 如果按字段或场景继续加 repository 函数，会快速膨胀

如果采用下面这种方式解决问题：

- `UpdateStatus(...)`
- `UpdateSchedule(...)`
- `UpdateFileMetadata(...)`
- `UpdateChunkResult(...)`
- `RecoverStuckRunning(...)`

那么随着业务增长，repository 层会不断新增大量专用更新函数。这样的方式虽然短期可用，但长期会带来两个问题：

- repository 接口越来越碎
- 每新增一个业务判断条件，都要继续加函数

这不符合“一次解决、后续尽量不再改 repository 接口”的目标。

### 2.4 如果继续为条件结构体增加字段，也不是最终解

另一种过渡方案是给条件结构体持续加字段，例如：

- `StatusEQ`
- `StatusNE`
- `UpdatedAtLT`
- `UpdatedAtGE`
- `Enabled`
- `Deleted`

这种方式比整实体更新更进一步，但本质上仍然是在不断扩张条件结构体。随着业务演进，最终仍会出现：

- 一个实体对应很大的 `Conditions` 结构
- 每遇到新比较操作都要继续改 port 和 repository
- 难以真正做到“一劳永逸”

### 2.5 业务层希望表达“更新意图”，而不是拼装“数据库实体”

从当前讨论里可以明确看出，业务层真正想表达的是：

- 我想更新哪些字段
- 我要求这些字段在什么条件下才能更新
- 如果没有命中记录，返回 false / 0
- 如果执行失败，返回 error

业务层并不希望为了这些目的去关心：

- 是否需要先查整实体
- 哪些非目标字段必须保留旧值
- 某个更新动作是否又需要单独补一个 repository 方法

## 3. 已出现的典型业务需求

### 3.1 尝试把文档状态改成 `running`

目标行为：

- 仅知道 `docId`
- 满足条件时更新：
  - `id = docId`
  - `deleted = 0`
  - `enabled = 1`
  - `status != running`
- 更新内容：
  - `status = running`
  - `updated_by = system`
  - `update_time = now`
- 返回：
  - 更新成功返回 `true`
  - 条件不满足返回 `false`
  - 执行失败返回 `error`

这类需求无法优雅地通过整实体更新表达。

### 3.2 回收长时间卡在 `running` 的文档

目标行为：

- 满足条件时更新：
  - `status = running`
  - `update_time < threshold`
- 更新内容：
  - `status = failed`
  - `updated_by = system`
  - `update_time = now`
- 返回更新条数

这里已经出现了明显的比较运算需求，例如 `lt`。

### 3.3 调度场景中的状态与锁更新

目标行为包括但不限于：

- 更新 `last_status`
- 更新 `last_error`
- 更新 `last_run_time`
- 更新 `last_success_time`
- 更新 `next_run_time`
- 更新 `lock_owner`
- 更新 `lock_until`

并且往往需要和条件组合使用：

- `id = scheduleId`
- `lock_owner = currentToken`
- `enabled = 1`

这类更新天然更适合“条件 + set”的统一表达，而不是整实体覆盖。

## 4. 用户需求总结

结合当前问题和讨论，后续 repository 使用体验需要满足以下要求。

### 4.1 业务层不想再依赖整实体更新

要求：

- 允许只描述需要更新的字段
- 不要求先构造完整实体
- 不要求先查询旧实体再覆盖

### 4.2 业务层需要像 MyBatis-Plus `lambdaUpdate` 一样表达条件更新

要求：

- 支持 `eq`
- 支持 `ne`
- 支持 `lt`
- 支持 `le`
- 支持 `gt`
- 支持 `ge`
- 未来应可扩展 `in`、`like`、`is null`、`is not null`

### 4.3 repository 层应尽量一次改到位，避免以后不断新增函数

要求：

- 不希望每个业务场景都新增一个 repository 更新函数
- 不希望每新增一种判断条件就继续改接口
- 需要一个可复用、可扩展、统一的更新表达方式

### 4.4 业务层需要明确区分“未命中条件”和“执行失败”

要求：

- 更新命中 0 行不是错误
- 返回值要能表达“更新成功 / 未命中 / 执行失败”
- 典型形式应为：
  - `rowsAffected + error`
  - 或 `bool + error`

### 4.5 最终目标是接近 MyBatis-Plus 的使用体验

要求：

- 写法上接近：
  - `set(...)`
  - `eq(...)`
  - `ne(...)`
  - `lt(...)`
- 调用侧强调“更新意图”和“条件约束”
- 尽量减少 repository 专用方法数量

### 4.6 需要兼顾可维护性，不能让业务层直接裸写数据库字段字符串

要求：

- 不能简单把 repository 退化成 `map[string]any + string condition`
- 需要保留一定的类型约束或字段常量约束
- 避免业务层直接深度依赖底层表结构细节

## 5. 结论

当前 repository 的主要使用问题，不是“某一个更新函数不够用”，而是整个更新表达方式仍偏向整实体覆盖，无法满足知识库业务中大量存在的“条件更新、状态流转、轻量字段更新”的真实需求。

用户的核心诉求可以概括为：

- 业务层要能方便地表达更新哪些字段
- 业务层要能方便地表达更新条件
- repository 层要一次性提供统一能力
- 后续不要再因为新增条件或新增状态流转场景，反复给持久层补函数
- 整体使用体验尽量向 MyBatis-Plus 的 `lambdaUpdate` 靠拢

这意味着后续改造的重点，不应继续围绕“补更多 Update 函数”展开，而应围绕“建立统一的条件更新表达模型”展开。
