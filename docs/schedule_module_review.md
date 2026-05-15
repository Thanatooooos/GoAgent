# Schedule 模块深度设计文档

> 日期: 2026-05-12

---

## 一、模块定位

Schedule 模块是知识库系统中负责**定时自动刷新远程 URL 文档**的子系统。用户可以为任意 URL 类型的知识文档绑定一个 cron 表达式，系统会按 cron 表达式的节奏，自动下载远程文件的最新版本，比对是否有变化，有变化则更新文档内容和元数据。

这是一个典型的**分布式定时任务调度 + 幂等数据同步**问题。

---

## 二、整体架构

### 2.1 分层视图

```
┌──────────────────────────────────────────────────────┐
│ bootstrap/knowledge/runtime.go                       │
│ • 依赖注入 & 组装                                      │
│ • 启动后台主循环 (goroutine + ticker)                   │
│ • 优雅关闭：cancel → wg.Wait → Close                  │
└───────────────────┬──────────────────────────────────┘
                    │
    ┌───────────────┼───────────────┐
    ▼               ▼               ▼
┌───────────┐ ┌───────────┐ ┌──────────────┐
│ Schedule  │ │ Document  │ │ TaskQueue    │
│ Service   │ │ Service   │ │ (RocketMQ)   │
└─────┬─────┘ └─────┬─────┘ └──────┬───────┘
      │             │              │
      │  创建/更新   │  文档CRUD    │  异步chunk
      │  schedule   │  联动同步    │  处理任务
      │             │              │
      ▼             ▼              │
┌──────────────────────────────────┼───────┐
│  schedule/ (核心包)               │       │
│                                  │       │
│  ┌─────────────────────┐         │       │
│  │ ScheduleJob (调度层) │◄────────┘       │
│  │ • Scan()            │                 │
│  │ • RecoverStuck()    │                 │
│  │ • dispatchLease()   │                 │
│  └─────────┬───────────┘                 │
│            │                              │
│  ┌─────────▼───────────┐                 │
│  │ RefreshProcessor    │                 │
│  │ (执行层, 9阶段管线)   │                 │
│  └─────────┬───────────┘                 │
│            │                              │
│  ┌─────────┼──────────────────────┐       │
│  ▼         ▼         ▼            ▼       │
│ LockMgr  StateMgr  DocHelper  Fetcher    │
│ (锁管理)  (状态写)  (文档CAS)  (HTTP)     │
└──────────────────────────────────────────┘
```

### 2.2 三层职责

| 层 | 组件 | 职责 |
|----|------|------|
| **调度层** | `ScheduleJob` | 轮询到期记录、抢分布式锁、分发给执行层 |
| **执行层** | `RefreshProcessor` | 9 阶段刷新管线，每阶段间检查锁有效性 |
| **工具层** | `LockManager`, `StateManager`, `DocHelper`, `Fetcher` | 各自封装单一职责 |

### 2.3 核心设计决策

**为什么不用现成的调度框架（如 Quartz/xxl-job）？**
- 调度逻辑和数据在同一数据库，省去额外的基础设施
- schedule 和 document 状态在一个事务内联动，一致性有保证
- 业务耦合度高——调度过程中需要读 document 状态、写 document 元数据，通用框架做不到这种程度的集成

**为什么用 DB 行锁而不是 Redis 分布式锁？**
- 减少运维依赖，不引入 Redis
- 锁信息和 schedule 记录存在同一行，读写在一个 UPDATE 里完成，原子性天然保证
- 对于 10s 级别的调度间隔，DB 的性能足够

---

## 三、端口层（Port）设计

### 3.1 仓库接口

有两个核心接口（[port/repository.go:86-107](internal/app/knowledge/port/repository.go#L86-L107)）：

```go
type KnowledgeDocumentScheduleRepository interface {
    Create / Update / UpdateWhere / Delete / DeleteByDocumentID
    GetByID / GetByDocumentID
    ListDue(ctx, before time.Time, limit int) ([]KnowledgeDocumentSchedule, error)
    TryAcquireLock(ctx, lease, lockUntil, now) (bool, error)
    RenewLock(ctx, lease, lockUntil) (bool, error)
    ReleaseLock(ctx, lease) (bool, error)
}

type KnowledgeDocumentScheduleExecRepository interface {
    Create / Update / UpdateWhere / GetByID / DeleteByDocumentID
    List(ctx, filter) ([]KnowledgeDocumentScheduleExec, error)
}
```

**`ListDue`** 是调度层的入口查询。其 SQL 逻辑：
```sql
SELECT * FROM t_knowledge_document_schedule
WHERE enabled = 1
  AND (next_run_time IS NULL OR next_run_time <= ?)
  AND (lock_until IS NULL OR lock_until < ?)
ORDER BY next_run_time ASC
LIMIT ?
```

三个条件：① 启用了 ② 到了执行时间 ③ 锁已过期或从未被锁。`next_run_time IS NULL` 的处理意味着新创建的 schedule（还没计算过下次执行时间）会立即被调度。

### 3.2 条件/补丁类型（UpdateSpec）

[port/update_spec.go](internal/app/knowledge/port/update_spec.go) 定义了一套类型安全的更新 DSL：

```go
// UpdateValue 用 Set 字段区分 "不设置" 和 "设置为零值"
type UpdateValue[T any] struct {
    Set   bool   // true = 显式设置, false = 不更新
    Value T
}

// schedule 更新条件
type KnowledgeDocumentScheduleConditions struct {
    ID           string
    DocumentID   string
    Enabled      *bool
    LastStatusEQ string
    LockOwnerEQ  string  // ← IfOwned 模式的关键字段
}

// schedule 更新补丁
type KnowledgeDocumentSchedulePatch struct {
    CronExpr    UpdateValue[string]
    Enabled     UpdateValue[bool]
    NextRunTime UpdateValue[*time.Time]
    LockOwner   UpdateValue[*string]
    LockUntil   UpdateValue[*time.Time]
    // ... 其他字段
}
```

**为什么需要 `UpdateValue.Set`？** 因为 Go 的零值无法区分"不设置"和"设置为空"。比如 `ReleaseLock` 需要把 `lock_owner` 设为 NULL（即 `(*string)(nil)`）——如果没有 `Set` 字段，`LockOwner = ""` 无法区分是"清空"还是"不更新"。用显式的 `Set: true` 配合 `Value: (*string)(nil)` 来表达"设置为 NULL"，`Set: false` 表示"跳过这个字段"。

对应 repo 层的实现：
```go
// knowledge_document_schedule_repo.go:204-249
func buildKnowledgeDocumentScheduleUpdates(patch port.KnowledgeDocumentSchedulePatch) map[string]any {
    updates := map[string]any{}
    if patch.LockOwner.Set { updates["lock_owner"] = patch.LockOwner.Value }  // nil → SQL NULL
    if patch.LockUntil.Set { updates["lock_until"] = patch.LockUntil.Value }
    // ...
}
```

### 3.3 文档更新的 Domain-Field 模式

另一套更新 API 通过 `UpdateFields` + `Where/Set` 构建器：

```go
// 类型安全的字段引用
var KnowledgeDocument = KnowledgeDocumentFieldSet{
    Status: Field[string]{Key: "knowledge_document.status"},
    // ...
}

// 使用
documentRepo.UpdateFields(ctx,
    port.Where(
        port.KnowledgeDocument.ID.Eq(docID),
        port.KnowledgeDocument.Status.In("pending", "success", "failed"),
    ),
    port.Set(
        port.KnowledgeDocument.Status.To("running"),
    ),
)
```

这套模式在 `DocumentStatusHelper` 的 CAS 操作中大量使用（TryMarkRunning、MarkFailedIfRunning 等）。它的特点是编译期类型安全——`Field[T]` 的泛型约束确保 `Status.To(42)` 会编译报错。

---

## 四、数据模型

### 4.1 主表 `t_knowledge_document_schedule`

```
字段              类型         说明
─────────────────────────────────────────────
id               varchar(20)  PK, 雪花ID
doc_id           varchar(20)  UNIQUE, 关联文档
kb_id            varchar(20)  知识库ID
cron_expr        varchar(64)  cron 表达式
enabled          smallint     启用标志 (0/1)
next_run_time    timestamptz  下一次执行时间
last_run_time    timestamptz  上次实际执行时间
last_success_time timestamptz 上次成功时间
last_status      varchar(16)  上次执行结果 running/success/failed/skipped
last_error       varchar(512) 上次失败原因 (512字节截断)
last_etag        varchar(256) 远端文件的 ETag
last_modified    varchar(256) 远端文件的 Last-Modified
last_content_hash varchar(128) 上次下载内容的 SHA256
lock_owner       varchar(128) INDEX, 当前持锁者
lock_until       timestamptz  INDEX, 锁过期时间
create_time      timestamptz
update_time      timestamptz
```

设计要点：
- **`doc_id` 唯一索引**：一个文档最多一条 schedule 记录，天然 1:1
- **`lock_owner` / `lock_until` 建索引**：`ListDue` 查询的 WHERE 条件用到 `lock_until`，`TryAcquireLock` 的 WHERE 用到 `lock_until`；锁释放的 WHERE 用到 `lock_owner`
- **`last_error` 512 字节截断**：防止异常堆栈撑爆字段
- **`enabled` 使用 smallint**：GORM 的 bool 映射惯例

### 4.2 执行记录表 `t_knowledge_document_schedule_exec`

```
字段              类型         说明
─────────────────────────────────────────────
id               varchar(20)  PK
schedule_id      varchar(20)  INDEX, 关联schedule
doc_id           varchar(20)  INDEX, 关联文档
kb_id            varchar(20)  知识库ID
status           varchar(16)  running/success/failed/skipped
message          varchar(512) 详情
start_time       timestamptz  INDEX (联合idx_schedule_time)
end_time         timestamptz
file_name        varchar(512) 下载的文件名
file_size        bigint      文件大小
content_hash     varchar(128) SHA256
etag             varchar(256)
last_modified    varchar(256)
create_time      timestamptz
update_time      timestamptz
```

设计要点：
- **联合索引 `idx_schedule_time(schedule_id, start_time)`**：用于按 schedule 查询执行历史，按时间排序
- **exec 记录是追加的、不可变的主要字段**：写入后 status/end_time 等会被更新，但不会删除（除了 `DeleteByDocumentID` 级联清理）
- **每次刷新产生一条 exec 记录**，即使结果是 skipped（内容未变化）

### 4.3 领域实体（Domain）

```go
// knowledge_document_schedule.go
type KnowledgeDocumentSchedule struct {
    ID, DocumentID, KnowledgeBaseID string
    CronExpr                        string
    Enabled                         bool
    NextRunTime, LastRunTime, LastSuccessTime *time.Time
    LastStatus, LastError           string
    LastETag, LastModified, LastContentHash string
    LockOwner                       string
    LockUntil                       *time.Time
    CreatedAt, UpdatedAt            time.Time
}

// knowledge_document_schedule_exec.go
type KnowledgeDocumentScheduleExec struct {
    ID, ScheduleID, DocumentID, KnowledgeBaseID string
    Status, Message                             string
    StartTime, EndTime                          *time.Time
    FileName                                    string
    FileSize                                    *int64
    ContentHash, ETag, LastModified             string
    CreatedAt, UpdatedAt                        time.Time
}
func (e KnowledgeDocumentScheduleExec) IsFinished() bool {
    return e.Status IN (success, failed, skipped)
}

// knowledge_document_schedule_runtime.go (值对象, 不持久化)
type KnowledgeDocumentScheduleLockLease struct {
    ScheduleID string
    LockToken  string  // 格式: "kb-schedule-<hex8>:<hex16>"
}
type KnowledgeDocumentScheduleStateContext struct {
    ScheduleID, ExecID, CronExpr string
    StartTime                    time.Time
    NextRunTime                  *time.Time
}
```

---

## 五、分布式锁设计（核心）

### 5.1 锁的生命周期

```
实例启动
  │
  ▼
后台 ticker 每 10s 触发 Scan()
  │
  ├─ ListDue → 找到到期记录
  │
  ├─ TryAcquire(scheduleID)
  │   └─ SQL: UPDATE ... SET lock_owner=?, lock_until=now+TTL
  │       WHERE id=? AND (lock_until IS NULL OR lock_until < now)
  │   └─ RowsAffected > 0 → 获取成功
  │
  ├─ dispatchLease → goroutine
  │   ├─ StartHeartbeat → 后台 goroutine 定期续期
  │   │   └─ 每 60s: Renew() → UPDATE lock_until=now+TTL WHERE id=? AND lock_owner=?
  │   │
  │   └─ Process (9阶段管线)
  │       └─ 每个阶段间: shouldAbortForLeaseLoss()
  │           ├─ heartbeat.IsLost()? → 中断
  │           └─ Renew() 是否成功? → 不成功则中断
  │
  └─ Release
      └─ SQL: UPDATE ... SET lock_owner=NULL, lock_until=NULL
          WHERE id=? AND lock_owner=?
```

### 5.2 关键参数和计算

```
LockSeconds (TTL)        = 900 (15分钟, 可配置, 最小60s)
心跳间隔                   = clamp(TTL/3, 5s, 60s) = 60s
TTL 内心跳次数             = 900/60 = 15 次
单次续期失败行为           = 不立即标记 lost
标记 lost 条件             = now - lastConfirmedAt >= TTL (15分钟内无确认)
```

**为什么心跳间隔是 TTL/3？**
- 太小（比如 TTL/10）：DB 压力大
- 太大（比如 TTL/1.5）：只有一次续期机会，失败就丢锁
- TTL/3 是经典的平衡点：有多次续期机会，单次失败不会丢锁

**为什么 clamp 到 [5s, 60s]？**
- 下限 5s：即使用户配了很短的 TTL（如 60s），心跳也不会短于 5s
- 上限 60s：即使用户配了很长的 TTL（如 1 小时），心跳也不会长于 60s，防止续期失败后过太久才发现

### 5.3 锁令牌格式

```
kb-schedule-<random8>:<random16>
```

- `kb-schedule-`：固定前缀，用于在 DB 中识别这是 schedule 的锁
- `<random8>`：实例级别随机数，在 `ScheduleLockManager` 初始化时生成，当前实例的所有锁共享
- `<random16>`：每次 `TryAcquire` 时生成，每个锁唯一

**为什么有两段随机数？** 实例前缀方便运维排查（哪个实例持有了锁），后缀保证每次获取的 token 唯一。

### 5.4 心跳保活机制详解

```go
func (m *ScheduleLockManager) doHeartbeat(heartbeat *ScheduleLockHeartbeat) {
    ok, err := m.Renew(m.ctx, heartbeat.Lease())
    if err == nil && ok {
        heartbeat.markRenewed(m.now())  // 更新 lastConfirmedAt
        return
    }
    // 续期失败：不立即标记 lost
    // 只有超过一个完整 TTL 没有确认才标记 lost
    if err == nil || heartbeat.isExpiredWithoutConfirmation(m.now()) {
        heartbeat.markLost()  // 标记 lost 并自动 Close
    }
}
```

这里的逻辑值得展开：

1. **`err == nil && ok`（续期成功）**：更新 `lastConfirmedAt`，一切正常
2. **`err != nil`（DB 错误）**：不标记 lost——可能是瞬时网络问题，等下次 tick 重试
3. **`err == nil && !ok`（续期返回 false，即 RowsAffected=0，说明锁不在了）**：检查是否超过 TTL 无确认。如果上次确认到现在 < TTL，可能是瞬时问题，再等等；如果 ≥ TTL，确认丢锁

### 5.5 为什么不是 SELECT FOR UPDATE？

`SELECT FOR UPDATE`（悲观锁）会持有行锁直到事务提交。但在我们的场景中：
- 刷新管线可能执行几分钟（下载大文件、chunk 处理），事务不能跨这么长时间
- 乐观锁（UPDATE WHERE）不持有行锁，只在写瞬间检查条件，不会阻塞其他读操作
- 心跳续期是另一个独立的 UPDATE，与主业务逻辑的事务分离

### 5.6 锁的安全性分析

**场景：实例 A 持有锁，网络分区导致它和 DB 断开**
- 心跳续期失败（err != nil），A 不标记 lost
- 15 分钟（TTL）后，`isExpiredWithoutConfirmation` 为 true，A 标记 lost
- 此时实例 B 的 TryAcquire 会因为 `lock_until < now` 而成功
- **风险窗口**：从 A 断开到 A 标记 lost 之间，A 以为自己还持有锁。但 A 的写操作会带 `LockOwnerEQ`，而 B 拿到锁后 `lock_owner` 变了，所以 A 写不进去（IfOwned 防护）

**场景：GC 停顿导致心跳漏了**
- 一次漏了：err == nil && ok == false（因为锁还没过期，renew 本身成功但可能被抢占）或者 renew 成功
- 连续漏多次直到 TTL 过期：B 抢到锁，A 的下次写操作会因 IfOwned 失败

---

## 六、刷新管线（RefreshProcessor）

### 6.1 完整状态机

```
                    ┌──────────┐
                    │  Init    │ lease 校验, 加载 schedule+document
                    └────┬─────┘
                         │ (lease OK + data loaded)
                    ┌────▼─────┐
                    │ Validate │ 文档存在? 启用? cron有效? URL源?
                    └────┬─────┘
                         │ (验证通过)
                    ┌────▼──────┐
                    │CreateExec │ 生成 exec 记录 (status=running)
                    └────┬──────┘
                         │
                    ┌────▼─────┐
                    │  Fetch   │ HEAD→ETag比对 → GET+SHA256 → 比对Hash
                    └────┬─────┘
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
          unchanged   changed    error
          → skip     → 继续      → failed
                         │
                    ┌────▼─────┐
                    │ClaimDoc  │ CAS: document.status → running
                    └────┬─────┘
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
           occupied   not occupied  error
           → 继续     → skip       → failed
                         │
                    ┌────▼─────┐
                    │  Store   │ 上传临时文件到S3
                    │phase=    │
                    │FileStored│
                    └────┬─────┘
                         │ (上传成功)
                    ┌────▼─────┐
                    │ Process  │ 可选: DocumentProcessor
                    │(optional)│ (chunking/embedding等)
                    └────┬─────┘
                         │
                    ┌────▼──────┐
                    │SwitchMeta │ CAS: doc→success + 更新元数据
                    │phase=     │
                    │FileSwitched│
                    └────┬──────┘
                         │
                    ┌────▼─────┐
                    │WriteState│ IfOwned: 写schedule+exec状态
                    └──────────┘
```

### 6.2 每个阶段的锁检查

在 ClaimDoc、Store、Process、SwitchMeta 之前都调用 `shouldAbortForLeaseLoss`：

```go
func (p *ScheduleRefreshProcessor) shouldAbortForLeaseLoss(...) bool {
    if heartbeat != nil && heartbeat.IsLost() { return true }
    renewed, err := p.lockManager.Renew(ctx, lease)
    if err != nil { return true }      // DB 错误，保守处理
    return !renewed                     // false = 锁已被抢占
}
```

注意：`Init` 阶段只在 start 处检查了一次，**在 `GetByID` 加载 schedule 之后没有再次检查**。如果 DB 查询延迟导致锁在此期间过期，后续的锁检查会在第一个 `shouldAbortForLeaseLoss` 处发现丢锁。但此时已经创建了 exec 记录（在 CreateExec 阶段）。这没问题——exec 记录会留在 running 状态，`RecoverStuckRunningDocuments` 会清理它。

### 6.3 清理回滚（cleanupAfterProcess）

```go
func cleanupAfterProcess(ctx, lease, state, heartbeat) {
    // 1. 文档状态回滚：只有在 ClaimDoc 成功但后续失败时才需要
    if heartbeat.IsLost() && state.phase == DocOccupied {
        MarkFailedIfRunning(ctx, documentID)
    }
    // 2. 文件清理：上传了但还没正式切换到文档上
    if state.stored != nil && state.phase < FileSwitched {
        storage.Delete(ctx, stored.Url)
    }
}
```

**局限性（已知问题）**：
- 只在 `heartbeat.IsLost()` 时回滚文档状态，正常 ctx cancel 不会回滚
- `storage.Delete` 失败被静默忽略，孤儿文件会残留
- 如果 `MarkFailedIfRunning` 执行失败（因为此时 document 可能已被另一个操作改了状态），错误也被忽略——这是合理的，因为 document 状态可能已经被 `RecoverStuckRunning` 或手动操作修改

### 6.4 优雅降级：exec-only 写入

管线的最后一步 `WriteState` 有一个关键的降级设计：

```go
// schedule_refresh_processor.go:257-258
if !p.markSuccessIfOwnedOrMarkLeaseLost(ctx, lease, state, fetchResult, "write success state") {
    // schedule 写不进去（锁丢了），降级为只写 exec
    _ = p.stateManager.MarkSuccessExecOnly(ctx, state.ctx, state.stored,
        fetchResult.ContentHash, fetchResult.ETag, fetchResult.LastModified,
        "refresh success; schedule state write failed")
}
```

`MarkSuccessIfOwned` 内部先尝试写 schedule（WHERE id=? AND lock_owner=?），如果 RowsAffected=0（锁丢了），则写 exec 记录并追加 `" (schedule lock lost; schedule state was not written back)"` 标记。

设计理由：exec 记录了这次执行的确发生了并且成功了——运维需要知道"虽然 fetch/store/switch 都成功了，但 schedule 状态没更新"这个事实。这对排查"为什么文档内容更新了但 schedule 的 last_status 还是 running"非常关键。

---

## 七、FetchIfChanged 三重去重

### 7.1 算法流程

```
输入: rawURL, lastETag, lastModified, lastContentHash

1. HEAD 请求
   ├─ 成功 → 比对 ETag, Last-Modified
   │   ├─ 完全匹配 → Changed=false (无需下载)
   │   └─ 不匹配 → 继续第2步
   └─ 失败 → 忽略, 继续第2步

2. GET 请求 (流式写入临时文件)
   ├─ 边下载边 SHA256 哈希
   ├─ 内容为空 → error
   ├─ ContentHash == lastContentHash → 删除临时文件, Changed=false
   └─ ContentHash 不同 → Changed=true, 返回临时文件路径

3. (Changed=true) 调用方负责: Store → Switch → Cleanup temp file
```

### 7.2 为什么是三层？

| 层 | 成本 | 准确度 | 失败处理 |
|----|------|--------|----------|
| HEAD + ETag | 极低 (无body) | 取决于服务器实现 | 失败则跳过 |
| HEAD + Last-Modified | 极低 | 秒级精度 | 失败则跳过 |
| GET + SHA256 | 高 (全量下载) | 100% (内容寻址) | 失败则报错 |

第一层（ETag/Last-Modified）能过滤掉 99% 的未变更请求。但有几种情况会穿透到第二层：
- 服务器不支持 HEAD
- ETag 使用 weak validator（`W/"xxx"`）且微变
- CDN 边缘节点 ETag 不稳定
- 服务器不返回 ETag 和 Last-Modified

SHA256 比对即使下载了文件，也能发现内容没变，从而跳过后续的存储上传和文档处理——节省了最昂贵的 chunk + embedding 开销。

### 7.3 临时文件管理

- 创建在 `os.CreateTemp(tempDir, "knowledge-schedule-*.tmp")`
- 下载时用 `io.MultiWriter(tempFile, sha256)` 同时写文件和算 hash
- `RemoteFetchResult.Close()` 删除临时文件——调用方在 defer 中调用
- 如果上传到 S3 成功，临时文件在 defer Close 时被清理

---

## 八、文档状态 CAS 操作

### 8.1 DocumentStatusHelper 的四个 CAS 方法

```go
// 乐观占用：只有 pending/success/failed 状态的文档可以标记为 running
TryMarkRunning(docID) → bool
  WHERE id=? AND enabled=true AND deleted=false AND status IN (pending, success, failed)
  SET status=running

// 只有 running 状态的文档可以标记为 failed
MarkFailedIfRunning(docID) → error
  WHERE id=? AND status=running
  SET status=failed

// 只有 running 状态的文档可以标记为 success
MarkSuccessIfRunning(docID) → error
  WHERE id=? AND status=running
  SET status=success

// 批量恢复：卡在 running 超过 N 分钟的文档 → failed
RecoverStuckRunning(ctx, timeoutMinutes) → (affected int64, error)
  WHERE status=running AND update_time < threshold
  SET status=failed
```

### 8.2 为什么用 CAS 而不是事务？

- 状态转换是单行操作，乐观锁的 CAS（Conditional Update）足够
- 不需要跨行事务——document 的状态转换和 schedule 的 `WriteState` 是通过 `LockOwnerEQ` 条件来协调的，而不是数据库事务
- 性能更好——CAS 是一个 UPDATE 语句，事务需要 BEGIN/COMMIT 两个额外指令

### 8.3 已知 Bug：常量误用

```go
// document_status_helper.go:86
// 错误: 使用的是 ChunkLog 的状态常量
port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentChunkLogStatusRunning)
// 应为:
port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning)
```

当前 `ChunkLogStatusRunning` 和 `DocumentStatusRunning` 都是 `"running"`，所以运行时不暴露。但语义错误，将来分叉则静默失效。

---

## 九、调度层详细设计

### 9.1 Scan 的完整流程

```go
func (j *KnowledgeDocumentScheduleJob) Scan(ctx context.Context) error {
    now := j.now()
    schedules, err := j.scheduleRepo.ListDue(ctx, now, j.batchSize) // 最多 batchSize 条
    if err != nil { return err }

    for _, item := range schedules {
        if ctx.Err() != nil { return ctx.Err() }

        lease, acquired, err := j.lockManager.TryAcquire(ctx, item.ID, now)
        if err != nil { return err }       // ← fail-fast: 中断批次
        if !acquired { continue }           // 被别人抢了，继续下一个

        if err := j.dispatchLease(ctx, lease); err != nil {
            // dispatch 失败，释放锁
            j.lockManager.Release(ctx, lease)
            return err                      // ← fail-fast: 中断批次
        }
    }
    return nil
}
```

**`ListDue` 返回的记录有两个条件**：① 到期了 (`next_run_time <= now`) ② 没被锁 (`lock_until < now`)。所以已经被其他实例持有的 schedule 不会出现在列表中——`TryAcquire` 再过滤一层是为了防止当前实例内部多个 tick 之间的竞争（虽然理论上不太可能，因为 tick 是串行的）。

### 9.2 dispatchLease 的 goroutine 分发

```go
func (j *KnowledgeDocumentScheduleJob) dispatchLease(ctx, lease) error {
    return j.dispatcher.Submit(func() {
        defer func() {
            // 使用独立的 ctx (WithoutCancel) + 5s超时来释放锁
            // 即使原始 ctx 已被 cancel，也要尝试释放
            releaseCtx, _ := newBackgroundTaskContext(ctx, 5*time.Second)
            j.lockManager.Release(releaseCtx, lease)
        }()

        processCtx := ctx
        // managedDispatcher 有自己的 ctx，用于统一 cancel
        if managed, ok := j.dispatcher.(*managedScheduleTaskDispatcher); ok {
            processCtx = managed.ctx
        }
        j.processor.Process(processCtx, lease)
    })
}
```

关键设计：
- 使用 `context.WithoutCancel(ctx)` 来释放锁——主 ctx 可能已被取消（服务正在关闭），但锁释放必须尽力完成
- `managedScheduleTaskDispatcher` 有独立的 ctx，在 `ScheduleJob.Close()` 时 cancel，用于等待所有正在执行的任务优雅结束
- goroutine 内部有 panic recovery，防止一个任务的 panic 影响整个进程

### 9.3 RecoverStuckRunningDocuments

每次 tick 开始时执行，恢复卡在 `running` 状态超过 30 分钟的文档：

```go
func RecoverStuckRunning(ctx, timeoutMinutes) (int64, error) {
    threshold := now - timeoutMinutes  // 默认30分钟
    UPDATE knowledge_document
    SET status='failed'
    WHERE status='running' AND update_time < threshold
}
```

为什么是 30 分钟？一个刷新周期：获取锁 + HEAD + GET(大文件) + upload + chunk + embedding，正常情况应该在分钟级别完成。30 分钟是一个保守的超时——如果超过了，可以安全地认为执行实例已经崩溃。

---

## 十、服务层联动

### 10.1 SyncSchedule 触发时机

`KnowledgeDocumentService` 在四个场景调用 `SyncSchedule`：

1. **文档创建**（上传 URL 文档）→ 创建 schedule 记录
2. **文档更新**（修改 cron 或 source location）→ 更新 schedule 的 cron/next_run_time
3. **启用/禁用 toggle** → 更新 schedule 的 enabled 字段
4. **来源位置变更** → 重新同步

### 10.2 SyncSchedule 的实现细节

```go
func (s *KnowledgeDocumentScheduleService) SyncSchedule(ctx, document, allowCreate) error {
    // 1. 只处理 URL 类型文档
    if document.SourceType != "url" { return error }

    // 2. 计算是否启用
    enabled := document.ScheduleEnabled
    if cron == "" || !document.Enabled { enabled = false }

    // 3. 校验 cron 间隔
    if enabled {
        ok, _ := schedule.IsIntervalLessThan(cron, time.Now(), s.scheduleSeconds)
        if ok { return error("cron interval too short") }
        nextRunTime, _ = schedule.NextRunTime(cron, time.Now())
    }

    // 4. 创建或更新
    existing, _ := s.scheduleRepo.GetByDocumentID(ctx, document.ID)
    if existing.ID == "" && allowCreate {
        // INSERT 新记录
    } else {
        // UPDATE: 只修改 CronExpr / Enabled / NextRunTime
        // 不动 LockOwner / LockUntil / LastStatus 等运行时字段
    }
}
```

**为什么 UPDATE 只动三个字段？** 用户的意图是"改变调度规则"（换 cron、开关），不是"改变运行时状态"。如果 UPDATE 覆盖了 `lock_owner`，可能干扰正在执行的刷新任务。

### 10.3 DeleteByDocID 的事务性

```go
func DeleteByDocID(ctx, docID) error {
    return transaction(ctx, func(txCtx, scheduleRepo, execRepo) error {
        scheduleExecRepo.DeleteByDocumentID(txCtx, docID)  // 先删子表
        scheduleRepo.DeleteByDocumentID(txCtx, docID)       // 再删主表
        return nil
    })
}
```

在一个事务中完成级联删除，保证不会出现"exec 删了但 schedule 没删"的中间状态。在 `KnowledgeDocumentDeleteTransaction` 中，文档删除、schedule 删除、exec 删除在同一事务中。

---

## 十一、启动和关闭流程

### 11.1 启动顺序（runtime.go）

```
1. 创建 DB 连接池 (GORM + PGX)
2. 创建所有 Repository
3. 创建 ScheduleService (用于文档 CRUD 联动)
4. 创建 RemoteFileFetcher (HTTP client + storage)
5. 创建 DocumentService (注入 ScheduleService)
6. 创建 ScheduleJob
   ├─ ScheduleRefreshProcessor (注入所有依赖)
   ├─ LockManager (注入 scheduleRepo)
   └─ managedDispatcher (goroutine pool)
7. 启动后台循环 startScheduleLoop()
8. 如果 RocketMQ 启用，启动 ChunkConsumer
```

### 11.2 关闭顺序（runtime.go Close）

```go
1. scheduleLoopCancel() → scheduleLoopWG.Wait()
   └─ 等待当前 tick 完成（不中断正在执行的 tick）
2. ScheduleJob.Close()
   ├─ dispatcherCancel() → 拒绝新任务
   └─ dispatcherWG.Wait() → 等待已提交任务完成
3. ChunkConsumer.Shutdown()
4. TaskQueue.Shutdown()
5. PGXPool.Close()
6. GORM DB.Close()
```

关闭顺序保证了：
- 先停止调度新任务（cancel loop + cancel dispatcher）
- 再等待正在执行的任务完成（WaitGroup）
- 最后关闭 DB 连接

---

## 十二、测试覆盖

### 12.1 已有的测试

| 测试文件 | 覆盖内容 |
|----------|----------|
| `cron_schedule_helper_test.go` | cron 解析：基本匹配、5字段格式、空白/零值输入、非法表达式、间隔校验 |
| `schedule_state_manager_test.go` | MarkSuccess: IfOwned 条件校验; MarkSkipped: lease lost 降级标记; Disable: 字段校验 |
| `schedule_refresh_processor_test.go` | 远程未变更→skip、变更→上传→success、非法cron→disable |

### 12.2 未覆盖的关键场景

- **锁竞争**：两个 processor 同时抢同一个 schedule
- **锁过期后处理**：管线执行到一半锁过期，IfOwned 降级
- **RecoverStuckRunning**：卡住的文档被恢复
- **心跳丢失**：heartbeat 标记 lost 后的管线中断
- **cleanupAfterProcess** 的各种 phase 组合
- **FetchIfChanged** 的三层比对组合（HEAD 失败→GET、ETag 匹配、Hash 匹配）
- **SyncSchedule** 的 create vs update 路径

---

## 十三、已知问题和改进方向

### Bug

| # | 位置 | 描述 | 严重度 |
|---|------|------|--------|
| 1 | [document_status_helper.go:86-89](internal/app/knowledge/schedule/document_status_helper.go#L86-L89) | `RecoverStuckRunning` 使用 ChunkLog 常量而非 Document 常量 | 中 |

### 设计问题

| # | 位置 | 描述 |
|---|------|------|
| 2 | [schedule_job.go:121-135](internal/app/knowledge/schedule/knowledge_document_schedule_job.go#L121-L135) | Scan 的 fail-fast：TryAcquire/dispatchLease 错误导致整批中断 |
| 3 | [refresh_processor.go:365-375](internal/app/knowledge/schedule/schedule_refresh_processor.go#L365-L375) | cleanup 只在 heartbeat.IsLost() 时回滚文档状态，ctx cancel 不覆盖 |
| 4 | [refresh_processor.go:130-136](internal/app/knowledge/schedule/schedule_refresh_processor.go#L130-L136) | GetByID 后缺少锁有效性二次检查 |
| 5 | [runtime.go:262-279](internal/bootstrap/knowledge/runtime.go#L262-L279) | 主循环无单次执行时间上限 |

### 可观测性缺失

- 无 metrics（刷新耗时、成功率、锁等待时间、心跳续期失败次数）
- 无 tracing（跨 fetch→store→process→switch 的全链路追踪）
- 孤儿文件无监控告警

---

## 十四、面试速查 Q&A

**Q: 为什么用 DB 锁而不是 Redis？**
A: 省依赖。对 10s 级调度间隔 DB 性能足够，且锁信息和 schedule 数据在同一行，读写原子。

**Q: 如果持锁的实例崩溃了怎么办？**
A: 锁 TTL 15 分钟后自动过期，其他实例的 `ListDue` 会发现该记录（`lock_until < now`），TryAcquire 成功。同时 `RecoverStuckRunningDocuments` 每 10s 检查一次卡在 running 的文档（30 分钟超时）。

**Q: 怎么保证不会重复执行？**
A: 三层防护——(1) TryAcquire 乐观锁；(2) ClaimDoc 的 CAS 只有非 running 文档能占用；(3) IfOwned 模式保证只有持锁者能写状态。

**Q: 怎么避免重复下载同一个文件？**
A: 三重去重——HEAD 比对 ETag/Last-Modified → GET 比对 ContentHash(SHA256)。HEAD 过滤 99% 场景，SHA256 兜底。

**Q: 如果执行过程中锁被抢走了怎么办？**
A: 每个阶段之间检查锁（shouldAbortForLeaseLoss），一旦丢锁就中断。清理逻辑回滚已占用的 document 状态和已上传的文件。exec 记录仍然写入（带 lease lost 标记），保证可追溯。

**Q: cron 表达式的 search horizon 为什么是 5 年？**
A: 覆盖极端场景——如 `0 0 29 2 *`（每闰年 2 月 29 日），如果从 2 月 28 日之后开始搜索，需要跨越约 4 年到下一个 2 月 29 日。5 年给了 1 年余量。

**Q: batchSize 的作用是什么？**
A: 限制单次 `ListDue` 返回的记录数（默认 20）。防止大量 schedule 同时到期时撑爆 goroutine——每个到期 schedule 都会启动一个 goroutine 执行刷新管线，需要有上限。

**Q: Dispatch 的并发度控制在哪里？**
A: 当前没有显式的并发度控制。每个到期 schedule 直接 `go task()`。潜在的并发上限受 `batchSize`（默认 20）和 schedule 到期数量共同限制。如果 1000 个 schedule 同时到期，每 10s tick 最多处理 20 个（batchSize），其余的要等后续 tick。
