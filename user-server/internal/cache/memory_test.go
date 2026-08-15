package cache

import (
	"context"
	"testing"
	"time"
)

// TestMemoryCache_New 测试创建内存缓存
func TestMemoryCache_New(t *testing.T) {
	cache := NewMemoryCache()
	if cache == nil {
		t.Fatal("NewMemoryCache() 返回 nil")
	}
	if cache.data == nil {
		t.Error("NewMemoryCache() data 未初始化")
	}
	if cache.stop == nil {
		t.Error("NewMemoryCache() stop 通道未初始化")
	}
	cache.Close()
}

// TestMemoryCache_Close 测试关闭缓存
func TestMemoryCache_Close(t *testing.T) {
	cache := NewMemoryCache()
	cache.Close()
	cache.Close()
}

// TestMemoryCache_SetAndGet 测试设置和获取缓存
func TestMemoryCache_SetAndGet(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "test_key"
	value := "test_value"

	err := cache.Set(ctx, key, value, time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	result, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != value {
		t.Errorf("Get() = %v, 期望 %v", result, value)
	}
}

// TestMemoryCache_GetNonExistent 测试获取不存在的缓存
func TestMemoryCache_GetNonExistent(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	result, err := cache.Get(ctx, "non_existent_key")
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != "" {
		t.Errorf("Get() 非existent key 应该返回空字符串，得到：%v", result)
	}
}

// TestMemoryCache_SetWithZeroExpiration 测试设置永不过期的缓存
func TestMemoryCache_SetWithZeroExpiration(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "permanent_key"
	value := "permanent_value"

	err := cache.Set(ctx, key, value, 0) 
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	result, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != value {
		t.Errorf("Get() = %v, 期望 %v", result, value)
	}
}

// TestMemoryCache_Expiration 测试缓存过期
func TestMemoryCache_Expiration(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "expiring_key"
	value := "expiring_value"

	err := cache.Set(ctx, key, value, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	result, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != value {
		t.Errorf("Get() = %v, 期望 %v", result, value)
	}

	time.Sleep(200 * time.Millisecond)

	result, err = cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != "" {
		t.Errorf("Get() 过期后应该返回空字符串，得到：%v", result)
	}
}

// TestMemoryCache_Delete 测试删除缓存
func TestMemoryCache_Delete(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "to_delete"
	value := "delete_value"

	err := cache.Set(ctx, key, value, time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	err = cache.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete() 返回错误：%v", err)
	}

	result, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != "" {
		t.Errorf("Delete() 后 Get() 应该返回空字符串，得到：%v", result)
	}
}

// TestMemoryCache_DeleteNonExistent 测试删除不存在的缓存
func TestMemoryCache_DeleteNonExistent(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	err := cache.Delete(ctx, "non_existent_key")
	if err != nil {
		t.Fatalf("Delete() 非existent key 应该不返回错误：%v", err)
	}
}

// TestMemoryCache_Exists 测试检查缓存是否存在
func TestMemoryCache_Exists(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "exists_key"
	value := "exists_value"

	err := cache.Set(ctx, key, value, time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	exists, err := cache.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() 返回错误：%v", err)
	}
	if !exists {
		t.Error("Exists() 应该返回 true")
	}

	exists, err = cache.Exists(ctx, "non_existent")
	if err != nil {
		t.Fatalf("Exists() 返回错误：%v", err)
	}
	if exists {
		t.Error("Exists() 非existent key 应该返回 false")
	}
}

// TestMemoryCache_ExistsExpired 测试检查过期的缓存
func TestMemoryCache_ExistsExpired(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "expires_soon"

	err := cache.Set(ctx, key, "value", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	exists, err := cache.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() 返回错误：%v", err)
	}
	if !exists {
		t.Error("Exists() 刚设置的缓存应该存在")
	}

	time.Sleep(200 * time.Millisecond)

	exists, err = cache.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() 返回错误：%v", err)
	}
	if exists {
		t.Error("Exists() 过期的缓存应该返回 false")
	}
}

// TestMemoryCache_Clear 测试清空缓存
func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()

	err := cache.Set(ctx, "key1", "value1", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}
	err = cache.Set(ctx, "key2", "value2", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	err = cache.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() 返回错误：%v", err)
	}

	result, _ := cache.Get(ctx, "key1")
	if result != "" {
		t.Errorf("Clear() 后 key1 应该为空")
	}
	result, _ = cache.Get(ctx, "key2")
	if result != "" {
		t.Errorf("Clear() 后 key2 应该为空")
	}
}

// TestMemoryCache_SetAndGetJSON 测试 JSON 缓存
func TestMemoryCache_SetAndGetJSON(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "json_key"
	value := map[string]any{
		"name": "test",
		"age":  25,
	}

	err := cache.SetJSON(ctx, key, value, time.Minute)
	if err != nil {
		t.Fatalf("SetJSON() 返回错误：%v", err)
	}

	// 获取 JSON 缓存
	var result map[string]any
	err = cache.GetJSON(ctx, key, &result)
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

// TestMemoryCache_SetJSONAndGetStruct 测试 JSON 缓存获取到 struct
func TestMemoryCache_SetJSONAndGetStruct(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "struct_key"
	value := struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}{
		Name: "struct_test",
		Age:  30,
	}

	err := cache.SetJSON(ctx, key, value, time.Minute)
	if err != nil {
		t.Fatalf("SetJSON() 返回错误：%v", err)
	}

	// 获取到 struct
	var result struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	err = cache.GetJSON(ctx, key, &result)
	if err != nil {
		t.Fatalf("GetJSON() 返回错误：%v", err)
	}

	if result.Name != "struct_test" {
		t.Errorf("GetJSON() Name = %v, 期望 struct_test", result.Name)
	}
	if result.Age != 30 {
		t.Errorf("GetJSON() Age = %v, 期望 30", result.Age)
	}
}

// TestMemoryCache_GetJSONNonExistent 测试获取不存在的 JSON 缓存
func TestMemoryCache_GetJSONNonExistent(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	var result map[string]any
	err := cache.GetJSON(ctx, "non_existent", &result)
	if err != nil {
		t.Fatalf("GetJSON() 非existent key 不应该返回错误：%v", err)
	}
}

// TestMemoryCache_ConcurrentAccess 测试并发访问
func TestMemoryCache_ConcurrentAccess(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	key := "concurrent_key"

	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(n int) {
			value := string(rune('A' + n%26))
			cache.Set(ctx, key, value, time.Minute)
			cache.Get(ctx, key)
			cache.Exists(ctx, key)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestMemoryCache_ConcurrentSetGet 测试并发设置和获取不同 key
func TestMemoryCache_ConcurrentSetGet(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(n int) {
			key := string(rune('A' + n%26))
			cache.Set(ctx, key, string(key), time.Minute)
			cache.Get(ctx, key)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestMemoryCache_CleanupGoroutine 测试 cleanup goroutine 在 Close 后停止
func TestMemoryCache_CleanupGoroutine(t *testing.T) {
	cache := NewMemoryCache()

	ctx := context.Background()
	cache.Set(ctx, "test", "value", 500*time.Millisecond)

	cache.Close()

	time.Sleep(600 * time.Millisecond)

}

