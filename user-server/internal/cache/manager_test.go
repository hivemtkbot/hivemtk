package cache

import (
	"context"
	"testing"
	"time"
)

// TestCacheManager_New 测试创建缓存管理器
func TestCacheManager_New(t *testing.T) {
	// 测试内存缓存配置
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

	// 无效类型应该回退到内存缓存
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

	// Redis 连接失败应该回退到内存缓存，不返回错误
	manager, err := NewCacheManager(config)
	if err != nil {
		t.Fatalf("NewCacheManager() 应该回退到内存缓存，返回错误：%v", err)
	}
	if manager == nil {
		t.Fatal("NewCacheManager() 应该回退到内存缓存，返回 nil")
	}

	// 验证回退到内存缓存
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

	// 先设置缓存
	err = manager.Set(ctx, "test_key", "test_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	// 获取缓存
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

	// 设置带过期时间的缓存
	err = manager.Set(ctx, "key1", "value1", 5*time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	// 设置不带过期时间（使用默认 TTL）
	err = manager.Set(ctx, "key2", "value2", 0)
	if err != nil {
		t.Fatalf("Set() with zero expiration 返回错误：%v", err)
	}

	// 验证两个缓存都存在
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

	// 设置缓存
	err = manager.Set(ctx, "to_delete", "delete_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	// 删除缓存
	err = manager.Delete(ctx, "to_delete")
	if err != nil {
		t.Fatalf("Delete() 返回错误：%v", err)
	}

	// 验证已删除
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

	// 设置缓存
	err = manager.Set(ctx, "exists_key", "exists_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	// 检查存在
	exists, err := manager.Exists(ctx, "exists_key")
	if err != nil {
		t.Fatalf("Exists() 返回错误：%v", err)
	}
	if !exists {
		t.Error("Exists() 应该返回 true")
	}

	// 检查不存在
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

	// 设置 JSON 缓存
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

	// 设置 JSON 缓存
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

	// 设置多个缓存
	err = manager.Set(ctx, "key1", "value1", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}
	err = manager.Set(ctx, "key2", "value2", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	// 清空
	err = manager.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() 返回错误：%v", err)
	}

	// 验证已清空
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

	// 设置不带过期时间的缓存，应该使用默认 TTL 30 分钟
	err = manager.Set(ctx, "default_ttl_key", "default_ttl_value", 0)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	// 立即获取应该成功
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

	// 关闭不应该 panic
	manager.Close()
}

// TestCacheManager_CloseWithoutCache 测试关闭没有缓存的管理器
func TestCacheManager_CloseWithoutCache(t *testing.T) {
	manager := &CacheManager{}
	// 关闭不应该 panic
	manager.Close()
}

// TestMemoryCache_deleteExpired 测试删除过期缓存
func TestMemoryCache_deleteExpired(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()

	// 设置一个立即过期的缓存
	cache.Set(ctx, "expired_key", "expired_value", 1*time.Millisecond)

	// 等待过期
	time.Sleep(10 * time.Millisecond)

	// 在 deleteExpired 之前，Get 应该返回空（因为已过期）
	result, _ := cache.Get(ctx, "expired_key")
	if result != "" {
		t.Logf("Key should be expired but got value: %v", result)
	}

	// 手动调用 deleteExpired 清理
	cache.deleteExpired()

	// 验证已清理
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

	// 设置多个立即过期的缓存
	cache.Set(ctx, "expired1", "value1", 1*time.Millisecond)
	cache.Set(ctx, "expired2", "value2", 1*time.Millisecond)
	cache.Set(ctx, "valid", "valid_value", 1*time.Hour) // 这个不过期

	// 等待过期
	time.Sleep(10 * time.Millisecond)

	// 手动调用 deleteExpired 清理
	cache.deleteExpired()

	// 验证过期 key 已删除
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
