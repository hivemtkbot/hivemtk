package cache

import (
	"context"
	"testing"
	"time"
)

// TestCacheManager_New 测试创建缓存管理器
func TestCacheManager_New(t *testing.T) {
	config := CacheConfig{
		Type:       "memory",
		DefaultTTL: 30 * time.Minute,
	}

	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	if manager == nil {
		t.Fatal("NewCacheManager() 返回 nil")
	}
	if manager.cache == nil {
		t.Error("NewCacheManager() cache 未初始化")
	}

	manager.Close()
}

// TestCacheManager_EmptyConfig 测试空配置
func TestCacheManager_EmptyConfig(t *testing.T) {
	config := CacheConfig{}

	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	if manager == nil {
		t.Fatal("NewCacheManager() 返回 nil")
	}

	manager.Close()
}

// TestCacheManager_InvalidType 测试无效类型
func TestCacheManager_InvalidType(t *testing.T) {
	config := CacheConfig{
		Type: "invalid_type",
	}

	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	if manager == nil {
		t.Fatal("NewCacheManager() 返回 nil")
	}

	if _, ok := manager.cache.(*MemoryCache); !ok {
		t.Error("无效类型应该回退到内存缓存")
	}

	manager.Close()
}

// TestCacheManager_RedisFallback 测试 Redis 失败回退
func TestCacheManager_RedisFallback(t *testing.T) {
	config := CacheConfig{
		Type: "redis",
		Redis: RedisConfig{
			Host: "nonexistent_host",
			Port: 6379,
		},
	}

	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 应该回退到内存缓存，返回错误：%v", err)
	}
	if manager == nil {
		t.Fatal("NewCacheManager() 应该回退到内存缓存，返回 nil")
	}

	if _, ok := manager.cache.(*MemoryCache); !ok {
		t.Error("Redis 失败应该回退到内存缓存")
	}

	manager.Close()
}

// TestCacheManager_Get 测试获取缓存
func TestCacheManager_Get(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	err = manager.Set(ctx, "test_key", "test_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	result, err := manager.Get(ctx, "test_key")
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != "test_value" {
		t.Errorf("Get() = %v, 期望 test_value", result)
	}
}

// TestCacheManager_Set 测试设置缓存
func TestCacheManager_Set(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	err = manager.Set(ctx, "key1", "value1", 5*time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	err = manager.Set(ctx, "key2", "value2", 0)
	if err != nil {
		t.Fatalf("Set() with zero expiration 返回错误：%v", err)
	}

	result1, _ := manager.Get(ctx, "key1")
	if result1 != "value1" {
		t.Errorf("key1 = %v, 期望 value1", result1)
	}

	result2, _ := manager.Get(ctx, "key2")
	if result2 != "value2" {
		t.Errorf("key2 = %v, 期望 value2", result2)
	}
}

// TestCacheManager_Delete 测试删除缓存
func TestCacheManager_Delete(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	err = manager.Set(ctx, "to_delete", "delete_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	err = manager.Delete(ctx, "to_delete")
	if err != nil {
		t.Fatalf("Delete() 返回错误：%v", err)
	}

	result, _ := manager.Get(ctx, "to_delete")
	if result != "" {
		t.Errorf("Delete() 后 Get() 应该返回空字符串，得到：%v", result)
	}
}

// TestCacheManager_Exists 测试检查缓存是否存在
func TestCacheManager_Exists(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	err = manager.Set(ctx, "exists_key", "exists_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	exists, err := manager.Exists(ctx, "exists_key")
	if err != nil {
		t.Fatalf("Exists() 返回错误：%v", err)
	}
	if !exists {
		t.Error("Exists() 应该返回 true")
	}

	exists, err = manager.Exists(ctx, "non_existent")
	if err != nil {
		t.Fatalf("Exists() 返回错误：%v", err)
	}
	if exists {
		t.Error("Exists() 非existent key 应该返回 false")
	}
}

// TestCacheManager_GetJSON 测试获取 JSON 缓存
func TestCacheManager_GetJSON(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()
	value := map[string]any{
		"name": "test",
		"age":  25,
	}

	err = manager.SetJSON(ctx, "json_key", value, time.Minute)
	if err != nil {
		t.Fatalf("SetJSON() 返回错误：%v", err)
	}

	// 获取 JSON 缓存
	var result map[string]any
	err = manager.GetJSON(ctx, "json_key", &result)
	if err != nil {
		t.Fatalf("GetJSON() 返回错误：%v", err)
	}

	if result["name"] != "test" {
		t.Errorf("GetJSON() name = %v, 期望 test", result["name"])
	}
	if result["age"].(float64) != 25 {
		t.Errorf("GetJSON() age = %v, 期望 25", result["age"])
	}
}

// TestCacheManager_SetJSON 测试设置 JSON 缓存
func TestCacheManager_SetJSON(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	value := TestStruct{
		Name:  "test_struct",
		Value: 42,
	}

	err = manager.SetJSON(ctx, "struct_key", value, time.Minute)
	if err != nil {
		t.Fatalf("SetJSON() 返回错误：%v", err)
	}

	// 获取并反序列化
	var result TestStruct
	err = manager.GetJSON(ctx, "struct_key", &result)
	if err != nil {
		t.Fatalf("GetJSON() 返回错误：%v", err)
	}

	if result.Name != "test_struct" {
		t.Errorf("GetJSON() Name = %v, 期望 test_struct", result.Name)
	}
	if result.Value != 42 {
		t.Errorf("GetJSON() Value = %v, 期望 42", result.Value)
	}
}

// TestCacheManager_Clear 测试清空缓存
func TestCacheManager_Clear(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	err = manager.Set(ctx, "key1", "value1", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}
	err = manager.Set(ctx, "key2", "value2", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	err = manager.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() 返回错误：%v", err)
	}

	result1, _ := manager.Get(ctx, "key1")
	if result1 != "" {
		t.Errorf("Clear() 后 key1 应该为空")
	}
	result2, _ := manager.Get(ctx, "key2")
	if result2 != "" {
		t.Errorf("Clear() 后 key2 应该为空")
	}
}

// TestCacheManager_GetDefaultTTL 测试获取默认 TTL
func TestCacheManager_GetDefaultTTL(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	err = manager.Set(ctx, "default_ttl_key", "default_ttl_value", 0)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	result, err := manager.Get(ctx, "default_ttl_key")
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != "default_ttl_value" {
		t.Errorf("Get() = %v, 期望 default_ttl_value", result)
	}
}

// TestCacheManager_Close 测试关闭缓存管理器
func TestCacheManager_Close(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}

	manager.Close()
}

// TestCacheManager_CloseWithoutCache 测试关闭没有缓存的管理器
func TestCacheManager_CloseWithoutCache(t *testing.T) {
	manager := &CacheManager{}
	manager.Close()
}

// TestMemoryCache_deleteExpired 测试删除过期缓存
func TestMemoryCache_deleteExpired(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()

	cache.Set(ctx, "expired_key", "expired_value", 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	result, _ := cache.Get(ctx, "expired_key")
	if result != "" {
		t.Logf("Key should be expired but got value: %v", result)
	}

	cache.deleteExpired()

	cache.mu.RLock()
	_, exists := cache.data["expired_key"]
	cache.mu.RUnlock()

	if exists {
		t.Error("deleteExpired() 后过期 key 应该被删除")
	}
}

// TestMemoryCache_deleteExpired_MultipleKeys 测试删除多个过期缓存
func TestMemoryCache_deleteExpired_MultipleKeys(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()

	cache.Set(ctx, "expired1", "value1", 1*time.Millisecond)
	cache.Set(ctx, "expired2", "value2", 1*time.Millisecond)
	cache.Set(ctx, "valid", "valid_value", 1*time.Hour) 

	time.Sleep(10 * time.Millisecond)

	cache.deleteExpired()

	cache.mu.RLock()
	_, expired1Exists := cache.data["expired1"]
	_, expired2Exists := cache.data["expired2"]
	_, validExists := cache.data["valid"]
	cache.mu.RUnlock()

	if expired1Exists {
		t.Error("deleteExpired() 后 expired1 应该被删除")
	}
	if expired2Exists {
		t.Error("deleteExpired() 后 expired2 应该被删除")
	}
	if !validExists {
		t.Error("deleteExpired() 后 valid 应该仍然存在")
	}
}


// TestCacheManager_Stats_HitRate 测试缓存命中率统计
func TestCacheManager_Stats_HitRate(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	// 初始状态：命中率应为 0
	stats := manager.GetStats()
	if stats.HitRate() != 0 {
		t.Errorf("初始命中率应为 0，得到：%f", stats.HitRate())
	}

	// 设置缓存
	manager.Set(ctx, "key1", "value1", time.Minute)

	// 命中
	manager.Get(ctx, "key1")
	stats = manager.GetStats()
	if stats.Hits != 1 {
		t.Errorf("命中次数应为 1，得到：%d", stats.Hits)
	}

	// 未命中
	manager.Get(ctx, "nonexistent")
	stats = manager.GetStats()
	if stats.Misses != 1 {
		t.Errorf("未命中次数应为 1，得到：%d", stats.Misses)
	}

	// 验证命中率
	if stats.HitRate() != 50.0 {
		t.Errorf("命中率应为 50%%，得到：%f", stats.HitRate())
	}

	// 验证总次数
	if stats.Total() != 2 {
		t.Errorf("总查询次数应为 2，得到：%d", stats.Total())
	}
}


// TestCacheManager_Stats_ResetStats 测试重置统计
func TestCacheManager_Stats_ResetStats(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	// 产生一些统计
	manager.Set(ctx, "key1", "value1", time.Minute)
	manager.Get(ctx, "key1")
	manager.Get(ctx, "nonexistent")

	// 重置前验证
	stats := manager.GetStats()
	if stats.Total() != 2 {
		t.Errorf("重置前总次数应为 2，得到：%d", stats.Total())
	}

	// 重置
	manager.ResetStats()

	// 重置后验证
	stats = manager.GetStats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("重置后统计应为 0，得到：Hits=%d, Misses=%d", stats.Hits, stats.Misses)
	}
}


// TestCacheManager_Stats_GetJSON 测试 GetJSON 统计
func TestCacheManager_Stats_GetJSON(t *testing.T) {
	config := CacheConfig{Type: "memory"}
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 返回错误：%v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	type TestStruct struct {
		Name string `json:"name"`
	}

	// 设置 JSON 缓存
	manager.SetJSON(ctx, "json_key", TestStruct{Name: "test"}, time.Minute)

	// 命中
	var result TestStruct
	manager.GetJSON(ctx, "json_key", &result)
	stats := manager.GetStats()
	if stats.Hits != 1 {
		t.Errorf("GetJSON 命中次数应为 1，得到：%d", stats.Hits)
	}

	// 未命中
	var result2 TestStruct
	manager.GetJSON(ctx, "nonexistent", &result2)
	stats = manager.GetStats()
	if stats.Misses != 1 {
		t.Errorf("GetJSON 未命中次数应为 1，得到：%d", stats.Misses)
	}
}
