# A2：模型并发限流（Governor）设计

日期：2026-08-09
状态：已批准
来源借鉴：WeKnora `internal/models/limiter/`（Governor + ModelConcurrencyLimiter）

## 背景

ingestion 批量入库会对模型 API 发起大量 embedding 请求（每个文档的 chunk 分组 + 问题向量化），并发写满模型配额会拖垮交互式聊天。WeKnora 的方案是"**只限流后台批处理，交互聊天永不过闸**"，用分布式信号量 + fail-open。

goagent 现状：`internal/infra-ai/embedding` 的 `EmbeddingService.EmbedBatch*` 只被 ingestion 后台使用（`runner_indexer.go`），聊天查询走单条 `Embed`。因此**只对批量 embedding 方法加门控**即可命中核心风险，无需后台任务上下文标记。

goagent 为单实例部署（AGENT.md），用进程内信号量即可，Redis 分布式版留作扩展位。

## 一、新包 `internal/infra-ai/limiter/`（纯 stdlib）

```go
package limiter

// Governor 按 key（模型 ID）做并发门控。交互路径不调用 Gate 即天然不过闸。
// 进程内实现，fail-open by construction（无后端错误可返回）。
type Governor struct {
	mu    sync.Mutex
	slots map[string]chan struct{}
}

func NewGovernor() *Governor

// Gate 对 key 做并发门控：limit<=0 直接执行 fn；否则占一个槽，fn 返回后释放。
// 槽持有者的调用受 HTTP 超时（默认 60s）约束，故获取必然有界、不会死锁。
func (g *Governor) Gate(key string, limit int, fn func() error) error
```

实现要点：
- 每个 key 一个容量为 limit 的 channel 作为槽位；`slot <- struct{}{}` 占槽（阻塞），`defer func(){ <-slot }()` 释放。
- `slotFor(key, limit)` 用互斥锁惰性创建；limit 对同一 key 恒定（每模型限额固定）。
- 空串 key 或 limit<=0 直接 `fn()`。

## 二、配置

`internal/framework/config/config.go`：
- `ModelCandidate` 加字段：`MaxConcurrency int mapstructure:"max-concurrency"`（按模型覆盖）。
- `AIConfig` 加字段：`Concurrency AIConcurrencyConfig mapstructure:"concurrency"`，其中 `AIConcurrencyConfig{ MaxPerModel int mapstructure:"max-per-model" }`（全局默认）。

`configs/application.yaml`：
```yaml
ai:
  concurrency:
    max-per-model: 4
  embedding:
    candidates:
      - id: qwen-emb-8b
        ...
        max-concurrency: 4   # 可选按模型覆盖
```

## 三、集成

### `internal/infra-ai/embedding/concurrency_client.go`

新增装饰器包装 `EmbeddingClient`（接口方法仅 `Provider`/`Embed`/`EmbedBatch`；`EmbeddingService.EmbedBatchWithModel` 最终也落到 `client.EmbedBatch`，同样被门控）：

```go
// concurrencyEmbeddingClient 只对批量 embedding（ingestion 后台）限流；
// 单条 Embed（交互查询）直通。
type concurrencyEmbeddingClient struct {
	inner        EmbeddingClient
	gov          *limiter.Governor
	defaultLimit int
}

func (c *concurrencyEmbeddingClient) Provider() string { return c.inner.Provider() }

func (c *concurrencyEmbeddingClient) Embed(text string, target model.ModelTarget) ([]float32, error) {
	return c.inner.Embed(text, target)
}

func (c *concurrencyEmbeddingClient) EmbedBatch(texts []string, target model.ModelTarget) ([][]float32, error) {
	return c.gateEmbedBatch(func() ([][]float32, error) {
		return c.inner.EmbedBatch(texts, target)
	}, target)
}
```

`gateEmbedBatch` 内：
- `limit := c.defaultLimit; if target.Candidate.MaxConcurrency > 0 { limit = target.Candidate.MaxConcurrency }`
- `key := target.Candidate.Id; if key == "" { key = "embedding" }`
- 调 `c.gov.Gate(key, limit, fn)`。

### `internal/infra-ai/assembly.go`

`NewRuntimeWithOptions` 中，对 `NewDefaultOpenAIStyleEmbeddingClients(httpClient)` 产出的每个 client 用 `concurrencyEmbeddingClient` 包装（defaultLimit 来自 `cfg.AI.Concurrency.MaxPerModel`，用 `config.Get()`）。

## 四、测试策略

### limiter 包单测
- limit<=0 直通（不阻塞）。
- 并发下 max-in-flight ≤ limit：用带计数/等待的 fn 验证同时进入的 goroutine 数不超过 limit。
- 释放正确：顺序执行 limit+1 次调用全部完成（前 limit 次并发 + 第 limit+1 次等待槽释放）。
- 空 key / limit=1 / 多 key 隔离。

### embedding 装饰器测试
- `Embed` 直通（不经过 Gate）。
- `EmbedBatch` 走 Gate：用一个可观测 Governor（记录 key 与调用次数）验证门控生效。
- limit 优先级：target.Candidate.MaxConcurrency > defaultLimit。

### 回归
- `go build ./internal/infra-ai/...`
- `go test ./internal/infra-ai/... ./internal/framework/config/... -count=1`
- 全量 `go test ./cmd/... ./internal/... -count=1`（除仓库既有失败包）。

## 五、明确不做（YAGNI）

- 不做 Redis 分布式信号量（单实例不需要；Governor 接口已为扩展留位）。
- 不做聊天/重排门控（交互聊天永不过闸；rerank 调用量小）。
- 不做后台任务上下文标记（`EmbedBatch` 已天然限定 ingestion）。
- 不做动态限流（基于失败率的熔断/弹性——已有 `ModelHealthStore` 熔断覆盖）。

## 回归约束

- 只新增 limiter 包 + embedding 装饰器 + config 字段；不改既有 embedding 客户端行为（装饰器默认直通）。
- `configs/application.yaml` 加 `ai.concurrency.max-per-model` 时保持既有字段不动。
- 不引入新依赖。
