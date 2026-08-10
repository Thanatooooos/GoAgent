package rag

import (
	"context"
	"time"

	frameworkconfig "local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/stream"
)

const (
	internalStopEventName = "stop"

	defaultStreamPollInterval  = 100 * time.Millisecond
	defaultStopWatcherInterval = 200 * time.Millisecond
	defaultMaxStreamDuration   = 5 * time.Minute
)

// streamEventSender 是 pollLoop 写 SSE 的最小依赖（SseEmitterSender 满足）。
type streamEventSender interface {
	SendEvent(eventName string, data interface{}) error
	Complete()
}

// pollLoop 从 startOffset 增量读流并写 SSE，读到 Done=true 事件或 ctx 结束。
// stop 是内部控制事件，不转发给客户端。
func pollLoop(ctx context.Context, sender streamEventSender, manager stream.StreamManager, streamID string, startOffset int, interval time.Duration, maxDuration time.Duration) {
	if interval <= 0 {
		interval = defaultStreamPollInterval
	}
	if maxDuration <= 0 {
		maxDuration = resolveDefaultMaxStreamDuration()
	}
	offset := startOffset
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	maxTimer := time.NewTimer(maxDuration)
	defer maxTimer.Stop()
	for {
		events, next, err := manager.GetEvents(ctx, streamID, offset)
		if err == nil {
			for _, e := range events {
				if e.Name == internalStopEventName {
					continue
				}
				if sendErr := sender.SendEvent(e.Name, e.Data); sendErr != nil {
					return
				}
				if e.Done {
					sender.Complete()
					return
				}
			}
			offset = next
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-maxTimer.C:
			sender.Complete()
			return
		}
	}
}

// stopWatcher 在执行节点监听 stop 控制事件，触发 cancelTask。
// 反复调用 cancelTask 直到成功或任务结束，规避任务尚未注册的竞态。
func stopWatcher(ctx context.Context, manager stream.StreamManager, streamID string, cancelTask func() bool, interval time.Duration, maxDuration time.Duration) {
	if interval <= 0 {
		interval = defaultStopWatcherInterval
	}
	if maxDuration <= 0 {
		maxDuration = resolveDefaultMaxStreamDuration()
	}
	offset := 0
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	maxTimer := time.NewTimer(maxDuration)
	defer maxTimer.Stop()
	stopSeen := false
	for {
		events, next, err := manager.GetEvents(ctx, streamID, offset)
		if err == nil {
			for _, e := range events {
				if e.Name == internalStopEventName {
					stopSeen = true
				} else if e.Done {
					return
				}
			}
			offset = next
		}
		if stopSeen && cancelTask() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-maxTimer.C:
			return
		}
	}
}

// resolveDefaultMaxStreamDuration 复用 rag.default.sse-timeout-ms，兜底 5 分钟。
func resolveDefaultMaxStreamDuration() time.Duration {
	d := defaultMaxStreamDuration
	if cfg := frameworkconfig.Get(); cfg != nil && cfg.Rag.Default.SseTimeoutMs > 0 {
		d = time.Duration(cfg.Rag.Default.SseTimeoutMs) * time.Millisecond
	}
	return d
}
