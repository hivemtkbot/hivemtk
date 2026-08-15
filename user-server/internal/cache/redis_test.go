package cache

import (
	"context"
	"testing"
	"time"
)

// TestRedisCache_New 测试创建 Redis 缓存
func TestRedisCache_New(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Password: "",
		DB:       0,
		PoolSize: 10,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	if cache == nil {
		t.Fatal("NewRedisCache() 返回 nil")
	}
	if cache.client == nil {
		t.Error("NewRedisCache() client 未初始化")
	}

	cache.Close()
}

// TestRedisCache_New_ConnectionFailure 测试连接失败
func TestRedisCache_New_ConnectionFailure(t *testing.T) {
	config := RedisConfig{
		Host: "nonexistent_host",
		Port: 6379,
	}

	_, err := NewRedisCache(config)
	if err == nil {
		t.Error("NewRedisCache() 连接不存在的 host 应该返回错误")
	}
}

// TestRedisCache_Get 测试获取 Redis 缓存
func TestRedisCache_Get(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	err = cache.Set(ctx, "test_key", "test_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	result, err := cache.Get(ctx, "test_key")
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != "test_value" {
		t.Errorf("Get() = %v, 期望 test_value", result)
	}
}

// TestRedisCache_GetNonExistent 测试获取不存在的 Redis 缓存
func TestRedisCache_GetNonExistent(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	result, err := cache.Get(ctx, "non_existent_key")
	if err == nil {
		t.Error("Get() 非existent key 应该返回错误")
	}
	if result != "" {
		t.Errorf("Get() 非existent key 应该返回空字符串，得到：%v", result)
	}
}

// TestRedisCache_Set 测试设置 Redis 缓存
func TestRedisCache_Set(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	err = cache.Set(ctx, "key1", "value1", 5*time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	result, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != "value1" {
		t.Errorf("Set() 后 Get() = %v, 期望 value1", result)
	}
}

// TestRedisCache_Delete 测试删除 Redis 缓存
func TestRedisCache_Delete(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	err = cache.Set(ctx, "to_delete", "delete_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	err = cache.Delete(ctx, "to_delete")
	if err != nil {
		t.Fatalf("Delete() 返回错误：%v", err)
	}

	_, err = cache.Get(ctx, "to_delete")
	if err == nil {
		t.Error("Delete() 后 Get() 应该返回错误")
	}
}

// TestRedisCache_Exists 测试检查 Redis 缓存是否存在
func TestRedisCache_Exists(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	err = cache.Set(ctx, "exists_key", "exists_value", time.Minute)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	exists, err := cache.Exists(ctx, "exists_key")
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

// TestRedisCache_GetJSON 测试获取 Redis JSON 缓存
func TestRedisCache_GetJSON(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	value := map[string]any{
		"name": "test",
		"age":  25,
	}

	err = cache.SetJSON(ctx, "json_key", value, time.Minute)
	if err != nil {
		t.Fatalf("SetJSON() 返回错误：%v", err)
	}

	// 获取 JSON 缓存
	var result map[string]any
	err = cache.GetJSON(ctx, "json_key", &result)
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

// TestRedisCache_SetJSON 测试设置 Redis JSON 缓存
func TestRedisCache_SetJSON(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	value := TestStruct{
		Name:  "test_struct",
		Value: 42,
	}

	err = cache.SetJSON(ctx, "struct_key", value, time.Minute)
	if err != nil {
		t.Fatalf("SetJSON() 返回错误：%v", err)
	}

	// 获取并反序列化
	var result TestStruct
	err = cache.GetJSON(ctx, "struct_key", &result)
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

// TestRedisCache_Clear 测试清空 Redis 缓存
func TestRedisCache_Clear(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
		DB:   1, 
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	err = cache.Set(ctx, "key1", "value1", time.Minute)
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

	exists, _ := cache.Exists(ctx, "key1")
	if exists {
		t.Error("Clear() 后 key1 应该不存在")
	}
	exists, _ = cache.Exists(ctx, "key2")
	if exists {
		t.Error("Clear() 后 key2 应该不存在")
	}
}

// TestRedisCache_Close 测试关闭 Redis 连接
func TestRedisCache_Close(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}

	err = cache.Close()
	if err != nil {
		t.Errorf("Close() 返回错误：%v", err)
	}
}

// TestRedisCache_StructConversion 测试 struct 转换
func TestRedisCache_StructConversion(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	user := User{
		ID:   1,
		Name: "test_user",
	}

	err = cache.SetJSON(ctx, "user_key", user, time.Minute)
	if err != nil {
		t.Fatalf("SetJSON() 返回错误：%v", err)
	}

	// 获取 JSON 缓存到相同的 struct 类型
	var result User
	err = cache.GetJSON(ctx, "user_key", &result)
	if err != nil {
		t.Fatalf("GetJSON() 返回错误：%v", err)
	}

	if result.ID != 1 {
		t.Errorf("GetJSON() ID = %v, 期望 1", result.ID)
	}
	if result.Name != "test_user" {
		t.Errorf("GetJSON() Name = %v, 期望 test_user", result.Name)
	}
}

// TestRedisCache_WithExpiration 测试带过期时间的缓存
func TestRedisCache_WithExpiration(t *testing.T) {
	t.Skip("Skipping test that requires Redis server")

	config := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("NewRedisCache() 返回错误：%v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	err = cache.Set(ctx, "expiring_key", "expiring_value", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Set() 返回错误：%v", err)
	}

	result, err := cache.Get(ctx, "expiring_key")
	if err != nil {
		t.Fatalf("Get() 返回错误：%v", err)
	}
	if result != "expiring_value" {
		t.Errorf("Get() = %v, 期望 expiring_value", result)
	}

	time.Sleep(200 * time.Millisecond)

	_, err = cache.Get(ctx, "expiring_key")
	if err == nil {
		t.Error("Get() 过期后应该返回错误")
	}
}

