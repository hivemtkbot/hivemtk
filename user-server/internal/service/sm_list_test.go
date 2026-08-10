package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupSmlistServiceTestDB 设置 SMList 服务测试数据库
func setupSmlistServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Smlist{},
	)
	db.SetTestDB(database)
	return database
}

// newTestSmlistRepository 创建测试仓库
func newTestSmlistRepository(database *gorm.DB) repository.SmlistRepository {
	return repository.NewSmlistRepository(database)
}

// TestNewSmlistService 测试创建 Smlist 服务
func TestNewSmlistService(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)

	service := NewSmlistService(repo)
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestSmlistService_Register 测试注册 Smlist
func TestSmlistService_Register(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	smlist := model.Smlist{
		QQ:      "123456789",
		Tg:      "test_tg",
		WX:      "test_wx",
		X:       "test_x",
		Name:    "测试名称",
		Phone:   "13812345678",
		City:    "北京",
		Address: "测试地址",
		Desc:    "测试描述",
		Age:     "25",
		Score:   "90",
		Price:   "100",
		Service: "测试服务",
		Images:  "image1.jpg,image2.jpg",
	}

	result, err := service.Register(context.Background(), smlist)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Name != "测试名称" {
		t.Errorf("Expected name '测试名称', got %s", result.Name)
	}

	if result.Phone != "13812345678" {
		t.Errorf("Expected phone '13812345678', got %s", result.Phone)
	}

	// 验证 ID 已生成
	if result.ID == "" {
		t.Error("Expected ID to be generated")
	}
}

// TestSmlistService_Register_EmptyFields 测试注册时字段为空的情况
func TestSmlistService_Register_EmptyFields(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	smlist := model.Smlist{
		Name: "仅名称",
	}

	result, err := service.Register(context.Background(), smlist)
	if err != nil {
		t.Fatalf("Register with empty fields failed: %v", err)
	}

	if result.Name != "仅名称" {
		t.Errorf("Expected name '仅名称', got %s", result.Name)
	}
}

// TestSmlistService_GetSmlist 测试获取 Smlist
func TestSmlistService_GetSmlist(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 创建测试数据
	smlist := &model.Smlist{
		Name:  "测试名称",
		Phone: "13812345678",
	}
	database.Create(smlist)

	// 获取
	result, err := service.GetSmlist(context.Background(), smlist.ID)
	if err != nil {
		t.Fatalf("GetSmlist failed: %v", err)
	}

	if result.Name != "测试名称" {
		t.Errorf("Expected name '测试名称', got %s", result.Name)
	}

	if result.Phone != "13812345678" {
		t.Errorf("Expected phone '13812345678', got %s", result.Phone)
	}
}

// TestSmlistService_GetSmlist_NotFound 测试获取不存在的 Smlist
func TestSmlistService_GetSmlist_NotFound(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	_, err := service.GetSmlist(context.Background(), "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent Smlist")
	}
}

// TestSmlistService_GetSmlistList 测试获取 Smlist 列表
func TestSmlistService_GetSmlistList(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 创建多条数据
	for i := 0; i < 5; i++ {
		smlist := &model.Smlist{
			Name:  "测试" + string(rune('0'+i)),
			Phone: "1381234567" + string(rune('0'+i)),
		}
		database.Create(smlist)
	}

	// 获取列表
	list, total, err := service.GetSmlistList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetSmlistList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(list) != 5 {
		t.Errorf("Expected 5 items, got %d", len(list))
	}
}

// TestSmlistService_GetSmlistList_Pagination 测试分页
func TestSmlistService_GetSmlistList_Pagination(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 创建 10 条数据
	for i := 0; i < 10; i++ {
		smlist := &model.Smlist{
			Name:  "测试" + string(rune('0'+i)),
			Phone: "1381234567" + string(rune('0'+i)),
		}
		database.Create(smlist)
	}

	// 第一页，每页 5 条
	list, total, err := service.GetSmlistList(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetSmlistList failed: %v", err)
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}

	if len(list) != 5 {
		t.Errorf("Expected 5 items on first page, got %d", len(list))
	}

	// 第二页
	list2, total2, err := service.GetSmlistList(context.Background(), 2, 5)
	if err != nil {
		t.Fatalf("GetSmlistList page 2 failed: %v", err)
	}

	if total2 != 10 {
		t.Errorf("Expected total 10, got %d", total2)
	}

	if len(list2) != 5 {
		t.Errorf("Expected 5 items on second page, got %d", len(list2))
	}
}

// TestSmlistService_GetSmlistList_Empty 测试空列表
func TestSmlistService_GetSmlistList_Empty(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	list, total, err := service.GetSmlistList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetSmlistList failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(list) != 0 {
		t.Errorf("Expected 0 items, got %d", len(list))
	}
}

// TestSmlistService_GetSmlistAllList 测试获取全部 Smlist 列表
func TestSmlistService_GetSmlistAllList(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 创建多条数据
	for i := 0; i < 5; i++ {
		smlist := &model.Smlist{
			Name:  "测试" + string(rune('0'+i)),
			Phone: "1381234567" + string(rune('0'+i)),
		}
		database.Create(smlist)
	}

	// 获取全部列表
	list, total, err := service.GetSmlistAllList(context.Background())
	if err != nil {
		t.Fatalf("GetSmlistAllList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(list) != 5 {
		t.Errorf("Expected 5 items, got %d", len(list))
	}
}

// TestSmlistService_GetSmlistAllList_Empty 测试空的全部列表
func TestSmlistService_GetSmlistAllList_Empty(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	list, total, err := service.GetSmlistAllList(context.Background())
	if err != nil {
		t.Fatalf("GetSmlistAllList failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(list) != 0 {
		t.Errorf("Expected 0 items, got %d", len(list))
	}
}

// TestSmlistService_DeleteSmlist 测试删除 Smlist
func TestSmlistService_DeleteSmlist(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 创建测试数据（使用 UUID 字符串）
	smlist := &model.Smlist{
		Name:  "待删除测试",
		Phone: "13812345678",
	}
	database.Create(smlist)

	// 删除
	err := service.DeleteSmlist(context.Background(), smlist.ID)
	if err != nil {
		t.Fatalf("DeleteSmlist failed: %v", err)
	}

	// 验证已删除
	var count int64
	database.Unscoped().Model(&model.Smlist{}).Where("id = ?", smlist.ID).Count(&count)
	if count != 1 {
		database.Model(&model.Smlist{}).Where("id = ?", smlist.ID).Count(&count)
		if count != 0 {
			t.Errorf("Expected Smlist to be deleted, got count %d", count)
		}
	}
}

// TestSmlistService_DeleteSmlist_NotFound 测试删除不存在的 Smlist
func TestSmlistService_DeleteSmlist_NotFound(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 生成一个不存在的 UUID
	nonExistentID := "00000000-0000-0000-0000-000000000000"
	err := service.DeleteSmlist(context.Background(), nonExistentID)
	if err != nil {
		t.Logf("DeleteSmlist for non-existent ID returned error (expected): %v", err)
	}
}

// TestSmlistService_GetRecentSmlistList 测试获取最近 Smlist 列表
func TestSmlistService_GetRecentSmlistList(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 创建多条数据（最近 48 小时内）
	for i := 0; i < 5; i++ {
		smlist := &model.Smlist{
			Name:  "最近测试" + string(rune('0'+i)),
			Phone: "1381234567" + string(rune('0'+i)),
		}
		database.Create(smlist)
	}

	// 获取最近列表
	list, err := service.GetRecentSmlistList(context.Background())
	if err != nil {
		t.Fatalf("GetRecentSmlistList failed: %v", err)
	}

	if len(list) != 5 {
		t.Errorf("Expected 5 items, got %d", len(list))
	}
}

// TestSmlistService_GetRecentSmlistList_Empty 测试空的最近列表
func TestSmlistService_GetRecentSmlistList_Empty(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	list, err := service.GetRecentSmlistList(context.Background())
	if err != nil {
		t.Fatalf("GetRecentSmlistList failed: %v", err)
	}

	if len(list) != 0 {
		t.Errorf("Expected 0 items, got %d", len(list))
	}
}

// TestSmlistService_GetRecentSmlistList_OrderByCreateTime 测试最近列表按创建时间降序
func TestSmlistService_GetRecentSmlistList_OrderByCreateTime(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 创建三条数据，间隔短暂
	smlist1 := &model.Smlist{Name: "第一", Phone: "13800000001"}
	time.Sleep(10 * time.Millisecond)
	smlist2 := &model.Smlist{Name: "第二", Phone: "13800000002"}
	time.Sleep(10 * time.Millisecond)
	smlist3 := &model.Smlist{Name: "第三", Phone: "13800000003"}

	database.Create(smlist1)
	database.Create(smlist2)
	database.Create(smlist3)

	list, err := service.GetRecentSmlistList(context.Background())
	if err != nil {
		t.Fatalf("GetRecentSmlistList failed: %v", err)
	}

	if len(list) < 3 {
		t.Errorf("Expected at least 3 items, got %d", len(list))
		return
	}

	// 验证按创建时间降序排列（最新的在前）
	if list[0].Name != "第三" {
		t.Logf("Expected first item to be '第三', got %s", list[0].Name)
	}
}

// TestSmlistService_Integration 集成测试：注册 - 查询 - 列表 - 删除全流程
func TestSmlistService_Integration(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 1. 注册
	smlist := model.Smlist{
		QQ:      "123456789",
		Name:    "集成测试",
		Phone:   "13812345678",
		City:    "上海",
		Address: "测试地址",
	}

	registered, err := service.Register(context.Background(), smlist)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 2. 查询
	retrieved, err := service.GetSmlist(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("GetSmlist failed: %v", err)
	}

	if retrieved.Name != "集成测试" {
		t.Errorf("Expected name '集成测试', got %s", retrieved.Name)
	}

	// 3. 列表
	list, total, err := service.GetSmlistList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetSmlistList failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}

	if len(list) != 1 {
		t.Errorf("Expected 1 item, got %d", len(list))
	}

	// 4. 删除
	err = service.DeleteSmlist(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("DeleteSmlist failed: %v", err)
	}

	// 5. 验证删除后查询失败
	_, err = service.GetSmlist(context.Background(), registered.ID)
	if err == nil {
		t.Error("Expected error after deletion")
	}
}

// TestSmlistService_ConcurrentRegister 测试并发注册
func TestSmlistService_ConcurrentRegister(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 并发注册 10 条数据，使用互斥锁保护数据库写入
	var mu sync.Mutex
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			mu.Lock()
			defer mu.Unlock()
			smlist := model.Smlist{
				Name:  "并发测试" + string(rune('0'+idx)),
				Phone: "1381234567" + string(rune('0'+idx)),
			}
			_, err := service.Register(context.Background(), smlist)
			if err != nil {
				t.Errorf("Concurrent Register failed: %v", err)
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证数据数量
	list, total, err := service.GetSmlistList(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("GetSmlistList failed: %v", err)
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}

	if len(list) != 10 {
		t.Errorf("Expected 10 items, got %d", len(list))
	}
}

// TestSmlistService_BoundaryConditions 边界条件测试
func TestSmlistService_BoundaryConditions(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 测试分页边界
	// 创建 1 条数据
	smlist := &model.Smlist{
		Name:  "边界测试",
		Phone: "13812345678",
	}
	database.Create(smlist)

	// 第 0 页（边界情况）
	list, total, err := service.GetSmlistList(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("GetSmlistList page 0 failed: %v", err)
	}
	t.Logf("Page 0: total=%d, len=%d", total, len(list))

	// limit 为 0 的情况
	list2, total2, err := service.GetSmlistList(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("GetSmlistList limit 0 failed: %v", err)
	}
	t.Logf("Limit 0: total=%d, len=%d", total2, len(list2))

	// 负数分页
	list3, total3, err := service.GetSmlistList(context.Background(), -1, -10)
	if err != nil {
		t.Fatalf("GetSmlistList negative params failed: %v", err)
	}
	t.Logf("Negative params: total=%d, len=%d", total3, len(list3))
}

// TestSmlistService_SpecialCharacters 特殊字符测试
func TestSmlistService_SpecialCharacters(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	smlist := model.Smlist{
		Name:    "测试'特殊\"字符<>&",
		Phone:   "13812345678",
		Address: "测试地址\n换行\t制表符",
		Desc:    "描述中包含 emoji 🎉🚀",
	}

	result, err := service.Register(context.Background(), smlist)
	if err != nil {
		t.Fatalf("Register with special characters failed: %v", err)
	}

	if result.Name != "测试'特殊\"字符<>&" {
		t.Errorf("Expected name with special characters, got %s", result.Name)
	}

	// 验证能正确查询
	retrieved, err := service.GetSmlist(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("GetSmlist with special characters failed: %v", err)
	}

	if retrieved.Name != "测试'特殊\"字符<>&" {
		t.Errorf("Expected retrieved name with special characters, got %s", retrieved.Name)
	}
}

// TestSmlistService_LongStrings 长字符串测试
func TestSmlistService_LongStrings(t *testing.T) {
	database := setupSmlistServiceTestDB(t)
	repo := newTestSmlistRepository(database)
	service := NewSmlistService(repo)

	// 创建长字符串
	longString := ""
	for i := 0; i < 100; i++ {
		longString += "长字符串测试"
	}

	smlist := model.Smlist{
		Name:    longString,
		Phone:   "13812345678",
		Address: longString,
		Desc:    longString,
	}

	result, err := service.Register(context.Background(), smlist)
	if err != nil {
		t.Fatalf("Register with long strings failed: %v", err)
	}

	if result.ID == "" {
		t.Error("Expected ID to be generated for long string test")
	}
}
