package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	frameworkconfig "local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/stream"
)

// buildStreamManager 按 rag.stream.type 构造流存储：memory（默认）或 redis。
func buildStreamManager(cfg *frameworkconfig.Config) stream.StreamManager {
	if cfg == nil || strings.EqualFold(strings.TrimSpace(cfg.Rag.Stream.Type), "memory") {
		return stream.NewMemoryStreamManager()
	}
	host := strings.TrimSpace(cfg.Spring.Data.Redis.Host)
	port := cfg.Spring.Data.Redis.Port
	if host == "" || port <= 0 {
		return stream.NewMemoryStreamManager()
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: cfg.Spring.Data.Redis.Password,
		DB:       cfg.Spring.Data.Redis.DB,
	})
	ttl := time.Duration(cfg.Rag.Stream.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	return stream.NewRedisStreamManager(client, cfg.Rag.Stream.RedisPrefix, ttl)
}

// startStreamSweep 为内存流存储启动 TTL 清扫协程（redis 版由 Redis Expire 兜底）。
func startStreamSweep(r *Runtime, cfg *frameworkconfig.Config) {
	if cfg == nil || r == nil {
		return
	}
	mm, ok := r.StreamManager.(*stream.MemoryStreamManager)
	if !ok {
		return
	}
	ttl := time.Duration(cfg.Rag.Stream.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.streamSweepCancel = cancel
	r.streamSweepWG.Add(1)
	go func() {
		defer r.streamSweepWG.Done()
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				mm.PruneOlderThan(ctx, time.Now().Add(-ttl))
			case <-ctx.Done():
				return
			}
		}
	}()
}

func stopStreamSweep(r *Runtime) {
	if r == nil || r.streamSweepCancel == nil {
		return
	}
	r.streamSweepCancel()
	r.streamSweepCancel = nil
	r.streamSweepWG.Wait()
}
