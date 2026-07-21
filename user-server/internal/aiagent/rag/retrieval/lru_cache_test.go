package ragretrieval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestLRUCache_NewLRUCache 构造默认值
func TestLRUCache_NewLRUCache(t *testing.T) {
	// 正常构造
	c := NewLRUCache(10, time.Minute)
	assert.Equal(t, 10, c.Capacity())
	assert.Equal(t, 0, c.Len())

	// capacity<=0 回退默认 100
	c = NewLRUCache(0, time.Minute)
	assert.Equal(t, 100, c.Capacity())

	// ttl<=0 回退默认 30min
	c = NewLRUCache(10, 0)
	assert.Equal(t, 30*time.Minute, c.ttl)
}

// TestLRUCache_SetGet 基本存取
func TestLRUCache_SetGet(t *testing.T) {
	c := NewLRUCache(5, time.Minute)
	c.Set("k1", "v1", 0)
	v, ok := c.Get("k1")
	assert.True(t, ok)
	assert.Equal(t, "v1", v)

	// 未命中
	_, ok = c.Get("missing")
	assert.False(t, ok)
}

// TestLRUCache_TTLExpiry TTL 过期
func TestLRUCache_TTLExpiry(t *testing.T) {
	c := NewLRUCache(5, time.Minute)
	c.Set("k1", "v1", 1*time.Nanosecond)
	time.Sleep(2 * time.Millisecond)
	_, ok := c.Get("k1")
	assert.False(t, ok, "key with expired ttl should not be returned")
}

// TestLRUCache_UpdateExisting 更新已存在键
func TestLRUCache_UpdateExisting(t *testing.T) {
	c := NewLRUCache(5, time.Minute)
	c.Set("k1", "v1", 0)
	c.Set("k1", "v2", 0)
	v, ok := c.Get("k1")
	assert.True(t, ok)
	assert.Equal(t, "v2", v)
	assert.Equal(t, 1, c.Len(), "updating existing key should not grow size")
}

// TestLRUCache_Eviction 超出容量淘汰最久未使用项
func TestLRUCache_Eviction(t *testing.T) {
	c := NewLRUCache(2, time.Minute)
	c.Set("a", 1, 0)
	c.Set("b", 2, 0)
	// 容量 2，插入第 3 项应淘汰最久未用的 "a"
	c.Set("c", 3, 0)
	assert.Equal(t, 2, c.Len())
	_, ok := c.Get("a")
	assert.False(t, ok, "a should be evicted")
	_, ok = c.Get("b")
	assert.True(t, ok)
	_, ok = c.Get("c")
	assert.True(t, ok)
}

// TestLRUCache_AccessReorders Get 更新 LRU 顺序
func TestLRUCache_AccessReorders(t *testing.T) {
	c := NewLRUCache(2, time.Minute)
	c.Set("a", 1, 0)
	c.Set("b", 2, 0)
	// 访问 a，使其变为最近使用；b 变成最久未用
	c.Get("a")
	// 插入 c，应淘汰 b 而非 a
	c.Set("c", 3, 0)
	_, ok := c.Get("a")
	assert.True(t, ok, "a should remain after being accessed")
	_, ok = c.Get("b")
	assert.False(t, ok, "b should be evicted as LRU")
	_, ok = c.Get("c")
	assert.True(t, ok)
}

// TestLRUCache_Delete 删除
func TestLRUCache_Delete(t *testing.T) {
	c := NewLRUCache(5, time.Minute)
	c.Set("k1", "v1", 0)
	c.Delete("k1")
	_, ok := c.Get("k1")
	assert.False(t, ok)
	// 删除不存在的键不 panic
	c.Delete("missing")
}

// TestLRUCache_Clear 清空
func TestLRUCache_Clear(t *testing.T) {
	c := NewLRUCache(5, time.Minute)
	c.Set("k1", "v1", 0)
	c.Set("k2", "v2", 0)
	c.Clear()
	assert.Equal(t, 0, c.Len())
	_, ok := c.Get("k1")
	assert.False(t, ok)
}

// TestLRUCache_CleanupExpired 清理过期项
func TestLRUCache_CleanupExpired(t *testing.T) {
	c := NewLRUCache(10, time.Minute)
	// 1 个永不过期/长期项
	c.Set("keep", 1, time.Hour)
	// 2 个立即过期项
	c.Set("e1", 1, 1*time.Nanosecond)
	c.Set("e2", 1, 1*time.Nanosecond)
	time.Sleep(2 * time.Millisecond)

	removed := c.CleanupExpired()
	assert.Equal(t, 2, removed)
	assert.Equal(t, 1, c.Len())
	_, ok := c.Get("keep")
	assert.True(t, ok)
}

// TestLRUCache_Len 长度统计
func TestLRUCache_Len(t *testing.T) {
	c := NewLRUCache(10, time.Minute)
	assert.Equal(t, 0, c.Len())
	c.Set("a", 1, 0)
	c.Set("b", 2, 0)
	assert.Equal(t, 2, c.Len())
}

// TestLRUCache_Capacity 容量配置
func TestLRUCache_Capacity(t *testing.T) {
	c := NewLRUCache(100, time.Minute)
	assert.Equal(t, 100, c.Capacity())
}

// TestLRUCache_DefaultParams 非法参数回退默认值
func TestLRUCache_DefaultParams(t *testing.T) {
	c := NewLRUCache(0, 0)
	assert.Equal(t, 100, c.Capacity())
}
