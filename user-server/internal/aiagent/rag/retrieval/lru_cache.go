package ragretrieval

import (
	"container/list"
	"sync"
	"time"
)

// LRUCache 线程安全的 LRU 缓存
// 用于 RAG 三级架构的 L1（高频 query 答案缓存）
type LRUCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
}

type lruEntry struct {
	key      string
	value    any
	expireAt time.Time
}

// NewLRUCache 构造 LRU 缓存
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	if capacity <= 0 {
		capacity = 100
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &LRUCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

// Get 获取缓存值（命中且未过期）
func (c *LRUCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	entry := el.Value.(*lruEntry)
	if time.Now().After(entry.expireAt) {
		c.order.Remove(el)
		delete(c.items, key)
		return nil, false
	}
	c.order.MoveToFront(el)
	return entry.value, true
}

// Set 设置缓存
func (c *LRUCache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ttl <= 0 {
		ttl = c.ttl
	}
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
	el := c.order.PushFront(&lruEntry{
		key:      key,
		value:    value,
		expireAt: time.Now().Add(ttl),
	})
	c.items[key] = el
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*lruEntry).key)
	}
}

// Delete 删除
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
}

// Clear 清空
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element, c.capacity)
	c.order = list.New()
}

// Len 数量
func (c *LRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Capacity 容量
func (c *LRUCache) Capacity() int { return c.capacity }

// CleanupExpired 清理过期项
func (c *LRUCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	removed := 0
	for el := c.order.Back(); el != nil; {
		entry := el.Value.(*lruEntry)
		prev := el.Prev()
		if now.After(entry.expireAt) {
			c.order.Remove(el)
			delete(c.items, entry.key)
			removed++
		}
		el = prev
	}
	return removed
}
