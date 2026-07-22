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
	// listMode 标识当前 key 是否为 list 模式
	listMode bool
	// listItems 仅在 listMode=true 时使用
	listItems []string
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

// SetNX 仅在 key 不存在时设置；返回 true 表示本次设置成功
func (m *MemoryCache) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if item, exists := m.data[key]; exists {
		if item.expiration.IsZero() || item.expiration.After(time.Now()) {
			return false, nil
		}
	}
	var exp time.Time
	if expiration > 0 {
		exp = time.Now().Add(expiration)
	}
	m.data[key] = cacheItem{value: value, expiration: exp}
	return true, nil
}

// LPush 向列表头部推入
func (m *MemoryCache) LPush(ctx context.Context, key string, value any, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := stringifyValue(value)
	item, ok := m.data[key]
	if !ok || !item.listMode {
		m.data[key] = cacheItem{
			listMode:  true,
			listItems: []string{s},
		}
	} else {
		// 头部插入
		item.listItems = append([]string{s}, item.listItems...)
		m.data[key] = item
	}
	if expiration > 0 {
		exp := time.Now().Add(expiration)
		ci := m.data[key]
		ci.expiration = exp
		m.data[key] = ci
	}
	return nil
}

// RPush 向列表尾部推入
func (m *MemoryCache) RPush(ctx context.Context, key string, value any, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := stringifyValue(value)
	item, ok := m.data[key]
	if !ok || !item.listMode {
		m.data[key] = cacheItem{
			listMode:  true,
			listItems: []string{s},
		}
	} else {
		item.listItems = append(item.listItems, s)
		m.data[key] = item
	}
	if expiration > 0 {
		exp := time.Now().Add(expiration)
		ci := m.data[key]
		ci.expiration = exp
		m.data[key] = ci
	}
	return nil
}

// LPop 从列表头部弹出
func (m *MemoryCache) LPop(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.data[key]
	if !ok || !item.listMode || len(item.listItems) == 0 {
		return "", nil
	}
	v := item.listItems[0]
	item.listItems = item.listItems[1:]
	if len(item.listItems) == 0 {
		delete(m.data, key)
	} else {
		m.data[key] = item
	}
	return v, nil
}

// LRange 获取列表区间
func (m *MemoryCache) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.data[key]
	if !ok || !item.listMode || len(item.listItems) == 0 {
		return []string{}, nil
	}
	items := item.listItems
	n := int64(len(items))
	if start < 0 {
		start += n
	}
	if stop < 0 {
		stop += n
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop || start >= n {
		return []string{}, nil
	}
	return append([]string{}, items[start:stop+1]...), nil
}

// LLen 列表长度
func (m *MemoryCache) LLen(ctx context.Context, key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.data[key]
	if !ok || !item.listMode {
		return 0, nil
	}
	return int64(len(item.listItems)), nil
}

func stringifyValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
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
