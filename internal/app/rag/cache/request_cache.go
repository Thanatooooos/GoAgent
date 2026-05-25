package cache

import (
	"container/list"
	"context"
	"sync"
)

type requestCacheContextKey struct{}

type requestCacheEntry struct {
	key   string
	value any
}

// RequestCache stores lightweight per-request values shared across chat stages.
type RequestCache struct {
	mu         sync.RWMutex
	maxEntries int
	items      map[string]*list.Element
	order      *list.List
}

func NewRequestCache(maxEntries int) *RequestCache {
	if maxEntries <= 0 {
		maxEntries = 128
	}
	return &RequestCache{
		maxEntries: maxEntries,
		items:      make(map[string]*list.Element, maxEntries),
		order:      list.New(),
	}
}

func WithRequestCache(ctx context.Context, cache *RequestCache) context.Context {
	if cache == nil {
		return ctx
	}
	return context.WithValue(ctx, requestCacheContextKey{}, cache)
}

func RequestCacheFromContext(ctx context.Context) *RequestCache {
	if ctx == nil {
		return nil
	}
	cache, _ := ctx.Value(requestCacheContextKey{}).(*RequestCache)
	return cache
}

func (c *RequestCache) Get(key string) (any, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	element := c.items[key]
	if element == nil {
		return nil, false
	}
	entry, _ := element.Value.(requestCacheEntry)
	return entry.value, true
}

func (c *RequestCache) Set(key string, value any) bool {
	if c == nil || key == "" {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if current := c.items[key]; current != nil {
		current.Value = requestCacheEntry{key: key, value: value}
		c.order.MoveToBack(current)
		return false
	}

	evicted := false
	if c.maxEntries > 0 && len(c.items) >= c.maxEntries {
		front := c.order.Front()
		if front != nil {
			entry, _ := front.Value.(requestCacheEntry)
			delete(c.items, entry.key)
			c.order.Remove(front)
			evicted = true
		}
	}

	element := c.order.PushBack(requestCacheEntry{key: key, value: value})
	c.items[key] = element
	return evicted
}
