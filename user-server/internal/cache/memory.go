package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// MemoryCache 内存缓存实现
type MemoryCache struct {
	data   map[string]cacheItem
	mu     sync.RWMutex
	stop   chan struct{} // 停止信号，用于关闭清理 goroutine
	closed chan struct{} // 标记已关闭，防止重复关闭
}

type cacheItem struct {
	value      any
	expiration time.Time
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache() *MemoryCache {
	cache := &MemoryCache{
		data:   make(map[string]cacheItem),
		stop:   make(chan struct{}),
		closed: make(chan struct{}),
	}
	// 启动清理过期缓存的 goroutine
	go cache.cleanup()
	return cache
}

// Close 关闭缓存清理 goroutine
func (m *MemoryCache) Close() {
	select {
	case <-m.closed:
		// 已经关闭，直接返回
		return
	default:
		close(m.closed)
		close(m.stop)
	}
}

// cleanup 定期清理过期缓存
func (m *MemoryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.deleteExpired()
		case <-m.stop:
			return
		}
	}
}

// deleteExpired 删除所有过期缓存
func (m *MemoryCache) deleteExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for key, item := range m.data {
		if !item.expiration.IsZero() && item.expiration.Before(now) {
			delete(m.data, key)
		}
	}
}

// Get 获取缓存
func (m *MemoryCache) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return "", nil
	}

	if !item.expiration.IsZero() && item.expiration.Before(time.Now()) {
		return "", nil
	}

	switch v := item.value.(type) {
	case string:
		return v, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

// Set 设置缓存
func (m *MemoryCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var exp time.Time
	if expiration > 0 {
		exp = time.Now().Add(expiration)
	}

	m.data[key] = cacheItem{
		value:      value,
		expiration: exp,
	}
	return nil
}

// Delete 删除缓存
func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// Exists 检查缓存是否存在
func (m *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return false, nil
	}

	if !item.expiration.IsZero() && item.expiration.Before(time.Now()) {
		return false, nil
	}

	return true, nil
}

// GetJSON 获取 JSON 缓存并反序列化
func (m *MemoryCache) GetJSON(ctx context.Context, key string, dest any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return nil
	}

	if !item.expiration.IsZero() && item.expiration.Before(time.Now()) {
		return nil
	}

	// 如果已经是目标类型，直接赋值
	switch v := item.value.(type) {
	case string:
		return json.Unmarshal([]byte(v), dest)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, dest)
	}
}

// SetJSON 设置 JSON 缓存
func (m *MemoryCache) SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error {
	return m.Set(ctx, key, value, expiration)
}

// Clear 清空缓存
func (m *MemoryCache) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]cacheItem)
	return nil
}
