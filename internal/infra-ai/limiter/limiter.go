package limiter

import "sync"

// Governor 按 key（模型 ID）做并发门控。交互路径不调用 Gate 即天然不过闸。
// 进程内实现，fail-open by construction（无后端错误可返回）。
// 每个 key 的槽容量由该 key 的首次 Gate 调用固定（first-wins），
// 之后同 key 的 limit 变化不生效；keys 数量受配置的模型 ID 集合约束，故 map 有界。
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
	if g.slots == nil {
		g.slots = map[string]chan struct{}{}
	}
	if slot, ok := g.slots[key]; ok {
		return slot
	}
	slot := make(chan struct{}, limit)
	g.slots[key] = slot
	return slot
}
