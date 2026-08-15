package tooluse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)



type cacheEntry struct {
	result    ToolResult
	expiredAt time.Time
	prev, next *cacheEntry
	key        string 
}


// ResultCache 工具执行结果缓存器
//
// 线程安全；使用 sync.LRU 简化实现（标准库无 LRU，这里实现简易双向链表 + map）
type ResultCache struct {
	mu         sync.Mutex
	entries    map[string]*cacheEntry
	head, tail *cacheEntry 
	maxEntries int
	defaultTTL time.Duration
	disabledTools map[string]bool
}

// defaultCacheDisabledTools 默认禁用缓存的工具（写入类、有副作用类）
//
// 这些工具调用必然改变状态，缓存会导致后续调用拿到旧结果
var defaultCacheDisabledTools = map[string]bool{
	"order.create":         true,
	"order.update":         true,
	"order.cancel":         true,
	"follow_task.create":   true,
	"follow_task.update":   true,
	"follow_task.complete": true,
	"knowledge.feedback":   true, 
	"rag.feedback":         true,
	"pm.open_session":      true,
	"pm.send_message":      true,
	"customer.create":      true,
	"customer.update":      true,
	"reach.send":           true, 
}

// NewResultCache 创建结果缓存器
//
// 参数：
//   - maxEntries: 最大缓存条目数（默认 1000）
//   - defaultTTL: 默认 TTL（默认 60s）
func NewResultCache(maxEntries int, defaultTTL time.Duration) *ResultCache {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	if defaultTTL <= 0 {
		defaultTTL = 60 * time.Second
	}
	c := &ResultCache{
		entries:       make(map[string]*cacheEntry),
		maxEntries:    maxEntries,
		defaultTTL:    defaultTTL,
		disabledTools: make(map[string]bool),
	}
	for k, v := range defaultCacheDisabledTools {
		c.disabledTools[k] = v
	}
	return c
}

// DisableTool 禁用指定工具的缓存
func (c *ResultCache) DisableTool(toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabledTools[toolName] = true
}

// EnableTool 启用指定工具的缓存（覆盖默认禁用）
func (c *ResultCache) EnableTool(toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.disabledTools, toolName)
}

// IsDisabled 判断工具是否禁用缓存
func (c *ResultCache) IsDisabled(toolName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disabledTools[toolName]
}

// cacheKey 生成缓存 key
//
// key = tool_name + ":" + sha256(args JSON)
// sha256 避免长参数导致 key 过长；同时确保相同参数 → 相同 key
func cacheKey(toolName string, args map[string]any) string {
	if len(args) == 0 {
		return toolName + ":empty"
	}
	b, _ := json.Marshal(args)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%s:%s", toolName, hex.EncodeToString(h[:16])) 
}

// Get 从缓存获取结果
//
// 返回：
//   - (result, true): 缓存命中
//   - (ToolResult{}, false): 缓存未命中或已过期
func (c *ResultCache) Get(toolName string, args map[string]any) (ToolResult, bool) {
	if c.IsDisabled(toolName) {
		return ToolResult{}, false
	}

	key := cacheKey(toolName, args)
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return ToolResult{}, false
	}
	if time.Now().After(entry.expiredAt) {
		c.removeEntry(entry)
		return ToolResult{}, false
	}
	c.moveToFront(entry)
	result := entry.result
	result.AuditTrace = "cache_hit"
	return result, true
}

// Set 写入缓存
//
// 仅缓存 Success=true 的结果；失败结果不缓存
func (c *ResultCache) Set(toolName string, args map[string]any, result ToolResult, ttl time.Duration) {
	if !result.Success {
		return 
	}
	if c.IsDisabled(toolName) {
		return 
	}
	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	key := cacheKey(toolName, args)
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.entries[key]; ok {
		existing.result = result
		existing.expiredAt = time.Now().Add(ttl)
		c.moveToFront(existing)
		return
	}

	entry := &cacheEntry{
		result:    result,
		expiredAt: time.Now().Add(ttl),
		key:       key,
	}
	c.entries[key] = entry
	c.addToFront(entry)

	for len(c.entries) > c.maxEntries {
		oldest := c.tail
		if oldest == nil {
			break
		}
		c.removeEntry(oldest)
	}
}

// Invalidate 失效指定工具的所有缓存
func (c *ResultCache) Invalidate(toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if entry.result.ToolName == toolName {
			c.removeEntry(entry)
			delete(c.entries, key)
		}
	}
}

// Clear 清空所有缓存
func (c *ResultCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
	c.head = nil
	c.tail = nil
}

// Stats 返回缓存统计（用于 /metrics endpoint 暴露）
func (c *ResultCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{
		EntryCount:    len(c.entries),
		MaxEntries:    c.maxEntries,
		DisabledTools: len(c.disabledTools),
	}
}

// CacheStats 缓存统计
type CacheStats struct {
	EntryCount    int `json:"entry_count"`
	MaxEntries    int `json:"max_entries"`
	DisabledTools int `json:"disabled_tools"`
}


func (c *ResultCache) addToFront(e *cacheEntry) {
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *ResultCache) removeEntry(e *cacheEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	delete(c.entries, e.key)
}

func (c *ResultCache) moveToFront(e *cacheEntry) {
	if c.head == e {
		return
	}
	c.removeEntry(e)
	c.addToFront(e)
	c.entries[e.key] = e 
}


// ResultCacheDecorator 结果缓存装饰器
//
// 装饰器链位置：权限 → 限流 → 熔断 → 参数校验 → 缓存 → 重试 → 超时 → 审计
//   - 缓存命中：直接返回缓存结果（不进入后续链路）
//   - 缓存未命中：执行工具，结果若 Success=true 则写入缓存
func ResultCacheDecorator(cache *ResultCache) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			if cache == nil {
				return next(ctx, args)
			}
			toolName := GetToolName(ctx)

			if cached, hit := cache.Get(toolName, args); hit {
				return cached, nil
			}

			result, err := next(ctx, args)

			if err == nil && result.Success {
				cache.Set(toolName, args, result, 0) 
			}

			return result, err
		}
	}
}


// NoOpResultCache 空操作缓存（不进行任何缓存）
// 用于不需要缓存的场景（如写入类工具的强制不缓存）
type NoOpResultCache struct{}

func (NoOpResultCache) Get(toolName string, args map[string]any) (ToolResult, bool) {
	return ToolResult{}, false
}
func (NoOpResultCache) Set(toolName string, args map[string]any, result ToolResult, ttl time.Duration) {
}
func (NoOpResultCache) Invalidate(toolName string)      {}
func (NoOpResultCache) Clear()                          {}
func (NoOpResultCache) IsDisabled(toolName string) bool { return true }
func (NoOpResultCache) Stats() CacheStats {
	return CacheStats{}
}

