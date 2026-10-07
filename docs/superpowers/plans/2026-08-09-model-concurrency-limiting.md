# A2 模型并发限流（Governor）实施计划

> 2026-10-01 适用范围：历史施工计划保留：并发门控仍有对应实现，但旧 ingestion 路径和命令已失效；不要把本计划步骤直接当作当前待办。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增进程内 `Governor` 并发门控，只对 ingestion 批量 embedding 限流（`EmbedBatch`），交互聊天 `Embed` 永不过闸，防止批量入库打爆模型 API。

**Architecture:** `internal/infra-ai/limiter/Governor`（channel 槽位信号量，fail-open）→ `concurrencyEmbeddingClient` 装饰器包装 `EmbeddingClient`（`Embed` 直通、`EmbedBatch` 过闸，limit 取 `target.Candidate.MaxConcurrency` 否则全局默认）→ `assembly.go` 装配时包装 embedding clients。配置：`ai.concurrency.max-per-model` 全局默认 + `ModelCandidate.max-concurrency` 按模型覆盖。

**Tech Stack:** Go 1.25 标准库（`sync`）。

**参考设计:** `docs/superpowers/specs/2026-08-09-model-concurrency-limiting-design.md`

---

## 文件结构

**新建：**
- `internal/infra-ai/limiter/limiter.go` — `Governor`
- `internal/infra-ai/limiter/limiter_test.go`
- `internal/infra-ai/embedding/concurrency_client.go` — `concurrencyEmbeddingClient` + `WithConcurrencyLimit`
- `internal/infra-ai/embedding/concurrency_client_test.go`

**修改：**
- `internal/framework/config/config.go` — `ModelCandidate.MaxConcurrency` + `AIConfig.Concurrency`/`AIConcurrencyConfig`
- `internal/framework/config/config_test.go` — 新字段断言
- `configs/application.yaml` — `ai.concurrency.max-per-model`
- `internal/infra-ai/assembly.go` — 装配时包装 embedding clients

---

## Task 1: limiter.Governor

**Files:**
- Create: `internal/infra-ai/limiter/limiter.go`
- Test: `internal/infra-ai/limiter/limiter_test.go`

- [ ] **Step 1: 写失败测试**

```go
package limiter

import (
	"sync"
	"testing"
	"time"
)

func TestGovernorPassthroughWhenNoLimit(t *testing.T) {
	g := NewGovernor()
	called := 0
	if err := g.Gate("m", 0, func() error { called++; return nil }); err != nil {
		t.Fatalf("err = %v", err)
	}
	if err := g.Gate("", 2, func() error { called++; return nil }); err != nil {
		t.Fatalf("empty key err = %v", err)
	}
	if called != 2 {
		t.Fatalf("called = %d, want 2", called)
	}
}

func TestGovernorLimitsConcurrency(t *testing.T) {
	g := NewGovernor()
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	var wg sync.WaitGroup
	const n = 8
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = g.Gate("m", 3, func() error {
				mu.Lock()
				inFlight++
				if inFlight > maxInFlight {
					maxInFlight = inFlight
				}
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				inFlight--
				mu.Unlock()
				return nil
			})
		}()
	}
	close(start)
	wg.Wait()
	if maxInFlight > 3 {
		t.Fatalf("max in-flight = %d, want <= 3", maxInFlight)
	}
	if maxInFlight < 1 {
		t.Fatal("expected at least 1 in-flight")
	}
}

func TestGovernorIsolatesKeys(t *testing.T) {
	g := NewGovernor()
	release := make(chan struct{})
	started := make(chan struct{})
	go func() {
		_ = g.Gate("a", 1, func() error { close(started); <-release; return nil })
	}()
	<-started // key "a" slot held
	done := make(chan struct{})
	go func() {
		_ = g.Gate("b", 1, func() error { close(done); return nil })
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("key b should run while key a is saturated")
	}
	close(release)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/infra-ai/limiter/ -count=1`
Expected: FAIL（`undefined: NewGovernor`）

- [ ] **Step 3: 实现 limiter.go**

```go
package limiter

import "sync"

// Governor 按 key（模型 ID）做并发门控。交互路径不调用 Gate 即天然不过闸。
// 进程内实现，fail-open by construction（无后端错误可返回）。
type Governor struct {
	mu    sync.Mutex
	slots map[string]chan struct{}
}

func NewGovernor() *Governor {
	return &Governor{slots: map[string]chan struct{}{}}
}

// Gate 对 key 做并发门控：limit<=0 或 key 为空直接执行 fn；否则占一个槽，
// fn 返回后释放。槽持有者的调用受 HTTP 超时约束，故获取必然有界、不会死锁。
func (g *Governor) Gate(key string, limit int, fn func() error) error {
	if g == nil || limit <= 0 || key == "" {
		return fn()
	}
	slot := g.slotFor(key, limit)
	slot <- struct{}{}
	defer func() { <-slot }()
	return fn()
}

func (g *Governor) slotFor(key string, limit int) chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	if slot, ok := g.slots[key]; ok {
		return slot
	}
	slot := make(chan struct{}, limit)
	g.slots[key] = slot
	return slot
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/infra-ai/limiter/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/infra-ai/limiter/limiter.go internal/infra-ai/limiter/limiter_test.go
git commit -m "feat: add in-process model concurrency governor"
```

---

## Task 2: 配置字段

**Files:**
- Modify: `internal/framework/config/config.go`
- Modify: `internal/framework/config/config_test.go`
- Modify: `configs/application.yaml`

- [ ] **Step 1: config.go 加字段**

`ModelCandidate`（约 line 415）加：

```go
type ModelCandidate struct {
	Id               string `mapstructure:"id"`
	Provider         string `mapstructure:"provider"`
	Model            string `mapstructure:"model"`
	Url              string `mapstructure:"url"`
	Dimension        any    `mapstructure:"dimension"`
	Priority         int    `mapstructure:"priority"`
	Enabled          *bool  `mapstructure:"enabled"`
	SupportsThinking *bool  `mapstructure:"supports-thinking"`
	MaxConcurrency   int    `mapstructure:"max-concurrency"`
}
```

`AIConfig`（约 line 389）加 `Concurrency` 字段，并新增类型：

```go
type AIConfig struct {
	Providers map[string]ProviderConfig `mapstructure:"providers"`
	Chat      ModelGroup                `mapstructure:"chat"`
	Embedding ModelGroup                `mapstructure:"embedding"`
	Rerank    ModelGroup                `mapstructure:"rerank"`
	Selection Selection                 `mapstructure:"selection"`
	Stream    Stream                    `mapstructure:"stream"`
	HTTP      AIHTTPConfig              `mapstructure:"http"`
	Concurrency AIConcurrencyConfig     `mapstructure:"concurrency"`
}

// AIConcurrencyConfig 配置模型调用并发门控的全局默认值。
type AIConcurrencyConfig struct {
	MaxPerModel int `mapstructure:"max-per-model"`
}
```

- [ ] **Step 2: application.yaml 加配置**

在 `ai:` 段落（顶层）加（保留既有字段）：

```yaml
ai:
  concurrency:
    max-per-model: 4
```

- [ ] **Step 3: config_test.go 加断言**

在 `config_test.go` 找到加载 application.yaml 的用例，加断言：

```go
	if cfg.AI.Concurrency.MaxPerModel != 4 {
		t.Fatalf("unexpected ai.concurrency.max-per-model: %d", cfg.AI.Concurrency.MaxPerModel)
	}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/framework/config/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/framework/config/config.go internal/framework/config/config_test.go configs/application.yaml
git commit -m "feat: add model concurrency config"
```

---

## Task 3: embedding 并发装饰器

**Files:**
- Create: `internal/infra-ai/embedding/concurrency_client.go`
- Test: `internal/infra-ai/embedding/concurrency_client_test.go`

- [ ] **Step 1: 写失败测试**

```go
package embedding

import (
	"sync"
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/infra-ai/limiter"
	"local/rag-project/internal/infra-ai/model"
)

type recordingEmbeddingClient struct {
	embedCalls int
	batchCalls int
	batchFn    func(texts []string, target model.ModelTarget) ([][]float32, error)
}

func (c *recordingEmbeddingClient) Provider() string { return "fake" }

func (c *recordingEmbeddingClient) Embed(text string, target model.ModelTarget) ([]float32, error) {
	c.embedCalls++
	return []float32{1}, nil
}

func (c *recordingEmbeddingClient) EmbedBatch(texts []string, target model.ModelTarget) ([][]float32, error) {
	c.batchCalls++
	if c.batchFn != nil {
		return c.batchFn(texts, target)
	}
	return [][]float32{{1}}, nil
}

func TestConcurrencyEmbeddingClientEmbedPassthrough(t *testing.T) {
	inner := &recordingEmbeddingClient{}
	client := &concurrencyEmbeddingClient{inner: inner, gov: limiter.NewGovernor(), defaultLimit: 1}
	if _, err := client.Embed("q", model.ModelTarget{Id: "m"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if inner.embedCalls != 1 {
		t.Fatalf("embedCalls = %d", inner.embedCalls)
	}
}

func TestConcurrencyEmbeddingClientEmbedBatchGates(t *testing.T) {
	inner := &recordingEmbeddingClient{}
	client := &concurrencyEmbeddingClient{inner: inner, gov: limiter.NewGovernor(), defaultLimit: 2}
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	inner.batchFn = func([]string, model.ModelTarget) ([][]float32, error) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		return [][]float32{{1}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.EmbedBatch([]string{"x"}, model.ModelTarget{Id: "m"}); err != nil {
				t.Errorf("EmbedBatch err = %v", err)
			}
		}()
	}
	wg.Wait()
	if maxInFlight > 2 {
		t.Fatalf("max in-flight = %d, want <= 2", maxInFlight)
	}
	if inner.batchCalls != 6 {
		t.Fatalf("batchCalls = %d, want 6", inner.batchCalls)
	}
}

func TestConcurrencyEmbeddingClientCandidateLimitOverridesDefault(t *testing.T) {
	inner := &recordingEmbeddingClient{}
	client := &concurrencyEmbeddingClient{inner: inner, gov: limiter.NewGovernor(), defaultLimit: 5}
	target := model.ModelTarget{Id: "m", Candidate: config.ModelCandidate{MaxConcurrency: 1}}
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	inner.batchFn = func([]string, model.ModelTarget) ([][]float32, error) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		return [][]float32{{1}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.EmbedBatch([]string{"x"}, target)
		}()
	}
	wg.Wait()
	if maxInFlight > 1 {
		t.Fatalf("candidate limit 1 should cap in-flight, got %d", maxInFlight)
	}
}

func TestWithConcurrencyLimitPassthroughWhenDisabled(t *testing.T) {
	inner := &recordingEmbeddingClient{}
	clients := WithConcurrencyLimit([]EmbeddingClient{inner}, 0)
	if len(clients) != 1 || clients[0] != inner {
		t.Fatalf("zero default limit should pass through unchanged: %#v", clients)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/infra-ai/embedding/ -run 'TestConcurrency|TestWithConcurrency' -count=1`
Expected: FAIL（`undefined: concurrencyEmbeddingClient`）

- [ ] **Step 3: 实现 concurrency_client.go**

```go
package embedding

import (
	"local/rag-project/internal/infra-ai/limiter"
	"local/rag-project/internal/infra-ai/model"
)

// concurrencyEmbeddingClient 只对批量 embedding（ingestion 后台）限流；
// 单条 Embed（交互查询）直通，永不过闸。
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

func (c *concurrencyEmbeddingClient) gateEmbedBatch(fn func() ([][]float32, error), target model.ModelTarget) ([][]float32, error) {
	limit := c.defaultLimit
	if target.Candidate.MaxConcurrency > 0 {
		limit = target.Candidate.MaxConcurrency
	}
	key := target.Candidate.Id
	if key == "" {
		key = "embedding"
	}
	var out [][]float32
	err := c.gov.Gate(key, limit, func() error {
		var callErr error
		out, callErr = fn()
		return callErr
	})
	return out, err
}

// WithConcurrencyLimit 包装 clients，使批量 embedding 遵循 per-model 并发上限；
// defaultLimit<=0 时原样返回（不限流）。返回的包装器共享同一个 Governor。
func WithConcurrencyLimit(clients []EmbeddingClient, defaultLimit int) []EmbeddingClient {
	if defaultLimit <= 0 || len(clients) == 0 {
		return clients
	}
	gov := limiter.NewGovernor()
	wrapped := make([]EmbeddingClient, 0, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		wrapped = append(wrapped, &concurrencyEmbeddingClient{inner: client, gov: gov, defaultLimit: defaultLimit})
	}
	return wrapped
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/infra-ai/embedding/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/infra-ai/embedding/concurrency_client.go internal/infra-ai/embedding/concurrency_client_test.go
git commit -m "feat: gate batch embedding with concurrency limit"
```

---

## Task 4: assembly 装配

**Files:**
- Modify: `internal/infra-ai/assembly.go`

- [ ] **Step 1: 装配时包装 embedding clients**

`NewRuntimeWithOptions` 中，`embeddingClients := embedding.NewDefaultOpenAIStyleEmbeddingClients(httpClient)` 之后加一行包装：

```go
	embeddingClients := embedding.NewDefaultOpenAIStyleEmbeddingClients(httpClient)
	embeddingClients = embedding.WithConcurrencyLimit(embeddingClients, defaultModelConcurrencyLimit())
```

在文件内新增 helper（与 `httpTimeout`/`streamTimeout` 并列）：

```go
func defaultModelConcurrencyLimit() int {
	cfg := config.Get()
	if cfg == nil {
		return 0
	}
	return cfg.AI.Concurrency.MaxPerModel
}
```

（`config` 已在 import 中；`embedding` 包已在 import 中。）

- [ ] **Step 2: 编译确认**

Run: `go build ./internal/infra-ai/...`
Expected: 成功

- [ ] **Step 3: 提交**

```bash
git add internal/infra-ai/assembly.go
git commit -m "feat: wire concurrency governor into embedding clients"
```

---

## Task 5: 全量验证

- [ ] **Step 1: 编译 + 定向测试**

Run:
- `go build ./internal/infra-ai/... ./internal/framework/config/...`
- `go test ./internal/infra-ai/... ./internal/framework/config/... -count=1`

Expected: 全部通过。

- [ ] **Step 2: 全仓核心测试**

Run: `go test ./cmd/... ./internal/... -count=1`
Expected: 除仓库既有失败包（cmd/eval-runner、cmd/rewrite-eval、config 默认值测试如存在）外全绿；infra-ai 相关包必须通过。

- [ ] **Step 3: 自检设计覆盖**

对照 `docs/superpowers/specs/2026-08-09-model-concurrency-limiting-design.md`：
- Governor.Gate 语义（limit<=0/空 key 直通、槽位、释放）→ Task 1 ✓
- config 字段 + yaml + 断言 → Task 2 ✓
- 装饰器 Embed 直通 / EmbedBatch 过闸 / limit 优先级 / WithConcurrencyLimit → Task 3 ✓
- assembly 装配 → Task 4 ✓
- 明确不做（Redis、聊天/重排门控、上下文标记、动态限流）均未触碰 ✓

---

## 自审记录

- **Spec 覆盖**：设计文档全部小节有对应任务。
- **已知细化**：`concurrencyEmbeddingClient` 只实现 `EmbeddingClient` 接口（`Provider`/`Embed`/`EmbedBatch`）——`EmbeddingService.EmbedBatchWithModel` 最终落到 `client.EmbedBatch`，同样被门控，无需额外方法。
- **类型一致性**：`Governor.Gate(key, limit, fn)` 在 Task 1 定义、Task 3 使用；`WithConcurrencyLimit(clients, defaultLimit)` Task 3 定义、Task 4 使用；`config.ModelCandidate.MaxConcurrency` Task 2 定义、Task 3 使用。
- **无新依赖**；不改既有 embedding 客户端行为（装饰器默认直通）。
