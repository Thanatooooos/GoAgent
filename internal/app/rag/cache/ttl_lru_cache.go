package cache

import (
	"container/list"
	"sync"
	"time"
)

type ttlCacheEntry struct {
	key       string
	value     any
	expiresAt time.Time
}

// TTLLRUCache stores best-effort local cache entries with TTL and LRU eviction.
type TTLLRUCache struct {
	mu         sync.RWMutex
	maxEntries int
	items      map[string]*list.Element
	order      *list.List
	onEvict    func(key string)
}

func NewTTLLRUCache(maxEntries int) *TTLLRUCache {
	if maxEntries <= 0 {
		maxEntries = 1024
	}
	return &TTLLRUCache{
		maxEntries: maxEntries,
		items:      make(map[string]*list.Element, maxEntries),
		order:      list.New(),
	}
}

func (c *TTLLRUCache) SetOnEvict(fn func(key string)) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onEvict = fn
}

func (c *TTLLRUCache) Get(key string) (any, bool) {
	if c == nil || key == "" {
		return nil, false
	}

	c.mu.Lock()
	element := c.items[key]
	if element == nil {
		c.mu.Unlock()
		return nil, false
	}
	entry, _ := element.Value.(ttlCacheEntry)
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		delete(c.items, key)
		c.order.Remove(element)
		onEvict := c.onEvict
		c.mu.Unlock()
		if onEvict != nil {
			onEvict(key)
		}
		return nil, false
	}
	c.order.MoveToBack(element)
	c.mu.Unlock()
	return entry.value, true
}

func (c *TTLLRUCache) Set(key string, value any, ttl time.Duration) bool {
	if c == nil || key == "" {
		return false
	}

	c.mu.Lock()
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	if current := c.items[key]; current != nil {
		current.Value = ttlCacheEntry{key: key, value: value, expiresAt: expiresAt}
		c.order.MoveToBack(current)
		c.mu.Unlock()
		return false
	}

	evicted := false
	evictedKey := ""
	if c.maxEntries > 0 && len(c.items) >= c.maxEntries {
		front := c.order.Front()
		if front != nil {
			entry, _ := front.Value.(ttlCacheEntry)
			delete(c.items, entry.key)
			c.order.Remove(front)
			evicted = true
			evictedKey = entry.key
		}
	}

	element := c.order.PushBack(ttlCacheEntry{key: key, value: value, expiresAt: expiresAt})
	c.items[key] = element
	onEvict := c.onEvict
	c.mu.Unlock()
	if evicted && onEvict != nil && evictedKey != "" {
		onEvict(evictedKey)
	}
	return evicted
}

func (c *TTLLRUCache) Delete(key string) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	element := c.items[key]
	if element == nil {
		c.mu.Unlock()
		return
	}
	delete(c.items, key)
	c.order.Remove(element)
	onEvict := c.onEvict
	c.mu.Unlock()
	if onEvict != nil {
		onEvict(key)
	}
}
