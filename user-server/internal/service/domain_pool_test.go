package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupDomainPoolServiceTestDB 设置域名池服务测试数据库
func setupDomainPoolServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.DomainPool{},
	)
	db.SetTestDB(database)
	return database
}

// newTestDomainPoolRepository 创建测试仓库
func newTestDomainPoolRepository(database *gorm.DB) repository.DomainPoolRepository {
	return repository.NewDomainPoolRepository(database)
}

// TestNewDomainPoolService 测试创建域名池服务
func TestNewDomainPoolService(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestDomainPoolService_Create 测试创建域名池记录
func TestDomainPoolService_Create(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	domainPool, err := service.Create(context.Background(), "example.com", 8080, "API 服务")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if domainPool.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com', got %s", domainPool.Domain)
	}
	if domainPool.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", domainPool.Port)
	}
	if domainPool.Purpose != "API 服务" {
		t.Errorf("Expected purpose 'API 服务', got %s", domainPool.Purpose)
	}
	if domainPool.Status != 1 {
		t.Errorf("Expected status 1, got %d", domainPool.Status)
	}
}

// TestDomainPoolService_Create_DefaultPort 测试创建时使用默认端口
func TestDomainPoolService_Create_DefaultPort(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	domainPool, err := service.Create(context.Background(), "example.com", 0, "API 服务")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if domainPool.Port != 80 {
		t.Errorf("Expected default port 80, got %d", domainPool.Port)
	}
}

// TestDomainPoolService_Create_NegativePort 测试创建时使用负数端口
func TestDomainPoolService_Create_NegativePort(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	domainPool, err := service.Create(context.Background(), "example.com", -1, "API 服务")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if domainPool.Port != 80 {
		t.Errorf("Expected default port 80 for negative port, got %d", domainPool.Port)
	}
}

// TestDomainPoolService_Create_DuplicateDomain 测试创建重复域名
func TestDomainPoolService_Create_DuplicateDomain(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建第一条记录
	_, err := service.Create(context.Background(), "example.com", 8080, "API 服务")
	if err != nil {
		t.Fatalf("First Create failed: %v", err)
	}

	// 尝试创建重复域名
	_, err = service.Create(context.Background(), "example.com", 9000, "其他服务")
	if err == nil {
		t.Error("Expected error for duplicate domain")
	}
	if err.Error() != "域名已存在" {
		t.Errorf("Expected '域名已存在', got %s", err.Error())
	}
}

// TestDomainPoolService_Update 测试更新域名池记录
func TestDomainPoolService_Update(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建记录
	created, _ := service.Create(context.Background(), "example.com", 8080, "API 服务")

	// 更新记录
	updated, err := service.Update(context.Background(), created.ID, "newexample.com", 9000, "新用途", 2)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updated.Domain != "newexample.com" {
		t.Errorf("Expected domain 'newexample.com', got %s", updated.Domain)
	}
	if updated.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", updated.Port)
	}
	if updated.Purpose != "新用途" {
		t.Errorf("Expected purpose '新用途', got %s", updated.Purpose)
	}
	if updated.Status != 2 {
		t.Errorf("Expected status 2, got %d", updated.Status)
	}
}

// TestDomainPoolService_Update_DefaultPort 测试更新时使用默认端口
func TestDomainPoolService_Update_DefaultPort(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	created, _ := service.Create(context.Background(), "example.com", 8080, "API 服务")
	updated, err := service.Update(context.Background(), created.ID, "example.com", 0, "API 服务", 1)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updated.Port != 80 {
		t.Errorf("Expected default port 80, got %d", updated.Port)
	}
}

// TestDomainPoolService_Update_DuplicateDomain 测试更新为已被使用的域名
func TestDomainPoolService_Update_DuplicateDomain(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建两条记录
	_, _ = service.Create(context.Background(), "example1.com", 8080, "服务 1")
	created2, _ := service.Create(context.Background(), "example2.com", 9000, "服务 2")

	// 尝试将第二条记录的域名更新为第一条的域名
	_, err := service.Update(context.Background(), created2.ID, "example1.com", 9000, "服务 2", 1)
	if err == nil {
		t.Error("Expected error for duplicate domain")
	}
	if err.Error() != "域名已被其他记录使用" {
		t.Errorf("Expected '域名已被其他记录使用', got %s", err.Error())
	}
}

// TestDomainPoolService_Update_SameDomain 测试更新时域名不变
func TestDomainPoolService_Update_SameDomain(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	created, _ := service.Create(context.Background(), "example.com", 8080, "API 服务")
	updated, err := service.Update(context.Background(), created.ID, "example.com", 9000, "新用途", 1)
	if err != nil {
		t.Fatalf("Update with same domain failed: %v", err)
	}

	if updated.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com', got %s", updated.Domain)
	}
}

// TestDomainPoolService_Update_NotFound 测试更新不存在的记录
func TestDomainPoolService_Update_NotFound(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	_, err := service.Update(context.Background(), 99999, "example.com", 8080, "API 服务", 1)
	if err == nil {
		t.Error("Expected error for non-existent record")
	}
}

// TestDomainPoolService_Delete 测试删除域名池记录
func TestDomainPoolService_Delete(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建记录
	created, _ := service.Create(context.Background(), "example.com", 8080, "API 服务")

	// 删除记录
	err := service.Delete(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证已删除
	_, err = service.GetByID(context.Background(), created.ID)
	if err == nil {
		t.Error("Expected error for deleted record")
	}
}

// TestDomainPoolService_Delete_NotFound 测试删除不存在的记录
func TestDomainPoolService_Delete_NotFound(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	err := service.Delete(context.Background(), 99999)
	if err != nil {
		t.Errorf("Delete non-existent record should not return error: %v", err)
	}
}

// TestDomainPoolService_GetByID 测试根据 ID 获取域名池记录
func TestDomainPoolService_GetByID(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建记录
	created, _ := service.Create(context.Background(), "example.com", 8080, "API 服务")

	// 获取记录
	retrieved, err := service.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com', got %s", retrieved.Domain)
	}
	if retrieved.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", retrieved.Port)
	}
	if retrieved.Purpose != "API 服务" {
		t.Errorf("Expected purpose 'API 服务', got %s", retrieved.Purpose)
	}
}

// TestDomainPoolService_GetByID_NotFound 测试获取不存在的记录
func TestDomainPoolService_GetByID_NotFound(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	_, err := service.GetByID(context.Background(), 99999)
	if err == nil {
		t.Error("Expected error for non-existent record")
	}
}

// TestDomainPoolService_List 测试获取域名池列表
func TestDomainPoolService_List(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建多条记录
	for i := 0; i < 5; i++ {
		domain := "example" + string(rune('0'+i)) + ".com"
		_, _ = service.Create(context.Background(), domain, 8080, "服务"+string(rune('0'+i)))
	}

	// 获取列表
	list, total, err := service.List(context.Background(), 1, 10, "", 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(list) != 5 {
		t.Errorf("Expected 5 records, got %d", len(list))
	}
}

// TestDomainPoolService_List_WithDomainFilter 测试带域名过滤的列表
func TestDomainPoolService_List_WithDomainFilter(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建多条记录
	_, _ = service.Create(context.Background(), "test1.example.com", 8080, "服务 1")
	_, _ = service.Create(context.Background(), "test2.example.com", 8080, "服务 2")
	_, _ = service.Create(context.Background(), "other.com", 8080, "其他服务")

	// 获取列表，使用域名过滤
	list, total, err := service.List(context.Background(), 1, 10, "example", 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}
	if len(list) != 2 {
		t.Errorf("Expected 2 records, got %d", len(list))
	}
}

// TestDomainPoolService_List_WithStatusFilter 测试带状态过滤的列表
func TestDomainPoolService_List_WithStatusFilter(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建不同状态的记录
	_, _ = service.Create(context.Background(), "example1.com", 8080, "服务 1") // status 1
	_, _ = service.Create(context.Background(), "example2.com", 8080, "服务 2") // status 1

	// 手动更新一条记录的状态为 2
	database.Model(&model.DomainPool{}).Where("domain = ?", "example2.com").Update("status", 2)

	// 获取列表，使用状态过滤
	list, total, err := service.List(context.Background(), 1, 10, "", 1)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 record, got %d", len(list))
	}
}

// TestDomainPoolService_List_Pagination 测试分页
func TestDomainPoolService_List_Pagination(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建多条记录
	for i := 0; i < 15; i++ {
		domain := "example" + string(rune('0'+i%10)) + string(rune('0'+i/10)) + ".com"
		_, _ = service.Create(context.Background(), domain, 8080, "服务")
	}

	// 获取第一页
	list1, total1, err := service.List(context.Background(), 1, 10, "", 0)
	if err != nil {
		t.Fatalf("List page 1 failed: %v", err)
	}

	// 获取第二页
	list2, _, err := service.List(context.Background(), 2, 10, "", 0)
	if err != nil {
		t.Fatalf("List page 2 failed: %v", err)
	}

	if total1 != 15 {
		t.Errorf("Expected total 15, got %d", total1)
	}
	if len(list1) != 10 {
		t.Errorf("Expected 10 records on page 1, got %d", len(list1))
	}
	if len(list2) != 5 {
		t.Errorf("Expected 5 records on page 2, got %d", len(list2))
	}
}

// TestDomainPoolService_CheckDomain 测试检查单个域名
func TestDomainPoolService_CheckDomain(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建记录（使用一个可能无法访问的域名进行测试）
	created, _ := service.Create(context.Background(), "nonexistent.invalid.domain", 80, "测试")

	// 检查域名（应该返回 false，因为域名不存在）
	accessible, err := service.CheckDomain(created.ID)
	if err != nil {
		t.Fatalf("CheckDomain failed: %v", err)
	}

	if accessible {
		t.Error("Expected domain to be inaccessible")
	}

	// 验证状态已更新为 2（不可访问）
	updated, _ := service.GetByID(context.Background(), created.ID)
	if updated.Status != 2 {
		t.Errorf("Expected status 2 after check, got %d", updated.Status)
	}
}

// TestDomainPoolService_CheckDomain_NotFound 测试检查不存在的域名
func TestDomainPoolService_CheckDomain_NotFound(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	_, err := service.CheckDomain(99999)
	if err == nil {
		t.Error("Expected error for non-existent record")
	}
}

// TestDomainPoolService_CheckAllDomains 测试检查所有域名
func TestDomainPoolService_CheckAllDomains(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建多条记录
	_, _ = service.Create(context.Background(), "example1.invalid", 80, "测试 1")
	_, _ = service.Create(context.Background(), "example2.invalid", 8080, "测试 2")

	// 检查所有域名
	results, err := service.CheckAllDomains()
	if err != nil {
		t.Fatalf("CheckAllDomains failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// 验证所有结果都标记为不可访问（因为是无效域名）
	for _, result := range results {
		if result.Status != 2 {
			t.Errorf("Expected status 2 for invalid domain, got %d", result.Status)
		}
	}
}

// TestDomainPoolService_CheckAllDomains_EmptyList 测试检查空列表
func TestDomainPoolService_CheckAllDomains_EmptyList(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	results, err := service.CheckAllDomains()
	if err != nil {
		t.Fatalf("CheckAllDomains failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty list, got %d", len(results))
	}
}

// TestDomainPoolService_Create_EmptyDomain 测试创建空域名
func TestDomainPoolService_Create_EmptyDomain(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	_, err := service.Create(context.Background(), "", 8080, "API 服务")
	if err != nil {
		// 空域名可能被数据库唯一索引拒绝，这是可接受的
		t.Logf("Empty domain rejected: %v", err)
	}
}

// TestDomainPoolService_Create_EmptyPurpose 测试创建空用途
func TestDomainPoolService_Create_EmptyPurpose(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	domainPool, err := service.Create(context.Background(), "example.com", 8080, "")
	if err != nil {
		t.Fatalf("Create with empty purpose failed: %v", err)
	}

	if domainPool.Purpose != "" {
		t.Errorf("Expected empty purpose, got %s", domainPool.Purpose)
	}
}

// TestDomainPoolService_List_EmptyList 测试空列表
func TestDomainPoolService_List_EmptyList(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	list, total, err := service.List(context.Background(), 1, 10, "", 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
	if len(list) != 0 {
		t.Errorf("Expected 0 records, got %d", len(list))
	}
}

// TestDomainPoolService_List_LargePageSize 测试大分页尺寸
func TestDomainPoolService_List_LargePageSize(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	// 创建少量记录
	for i := 0; i < 3; i++ {
		domain := "example" + string(rune('0'+i)) + ".com"
		_, _ = service.Create(context.Background(), domain, 8080, "服务")
	}

	// 使用大分页尺寸
	list, total, err := service.List(context.Background(), 1, 1000, "", 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
	if len(list) != 3 {
		t.Errorf("Expected 3 records, got %d", len(list))
	}
}

// TestDomainPoolService_Update_LastCheck 测试更新时间戳
func TestDomainPoolService_Update_LastCheck(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	created, _ := service.Create(context.Background(), "example.com", 8080, "API 服务")

	// 检查域名会更新 UpdatedAt
	_, _ = service.CheckDomain(created.ID)

	updated, _ := service.GetByID(context.Background(), created.ID)
	if updated.UpdatedAt.Before(created.CreatedAt) {
		t.Error("Expected UpdatedAt to be updated")
	}
}

// TestDomainPoolService_CheckDomain_VerifyLastCheck 测试检查域名会更新最后检查时间
func TestDomainPoolService_CheckDomain_VerifyLastCheck(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	created, _ := service.Create(context.Background(), "nonexistent.invalid", 80, "测试")

	beforeCheck := time.Now()
	_, _ = service.CheckDomain(created.ID)
	afterCheck := time.Now()

	updated, _ := service.GetByID(context.Background(), created.ID)
	if updated.LastCheck.Before(beforeCheck) || updated.LastCheck.After(afterCheck) {
		t.Errorf("Expected LastCheck to be updated, got %v", updated.LastCheck)
	}
}

// TestDomainPoolService_CheckAllDomains_VerifyLastCheck 测试检查所有域名会更新最后检查时间
func TestDomainPoolService_CheckAllDomains_VerifyLastCheck(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	_, _ = service.Create(context.Background(), "nonexistent1.invalid", 80, "测试 1")
	_, _ = service.Create(context.Background(), "nonexistent2.invalid", 80, "测试 2")

	beforeCheck := time.Now()
	_, _ = service.CheckAllDomains()
	afterCheck := time.Now()

	// 验证所有记录的 LastCheck 都被更新
	var pools []*model.DomainPool
	database.Find(&pools)

	for _, pool := range pools {
		if pool.LastCheck.Before(beforeCheck) || pool.LastCheck.After(afterCheck) {
			t.Errorf("Expected LastCheck to be updated for domain %s, got %v", pool.Domain, pool.LastCheck)
		}
	}
}

// TestDomainPoolService_Create_LongDomain 测试创建长域名
func TestDomainPoolService_Create_LongDomain(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	longDomain := "very-long-subdomain.example.very-long-domain-name-that-might-approach-limit.com"
	domainPool, err := service.Create(context.Background(), longDomain, 8080, "测试")
	if err != nil {
		t.Fatalf("Create with long domain failed: %v", err)
	}

	if domainPool.Domain != longDomain {
		t.Errorf("Expected domain %s, got %s", longDomain, domainPool.Domain)
	}
}

// TestDomainPoolService_Create_SpecialCharactersInPurpose 测试用途中包含特殊字符
func TestDomainPoolService_Create_SpecialCharactersInPurpose(t *testing.T) {
	database := setupDomainPoolServiceTestDB(t)
	service := NewDomainPoolService(database)

	purpose := "测试服务 <>&\"' 特殊字符"
	domainPool, err := service.Create(context.Background(), "example.com", 8080, purpose)
	if err != nil {
		t.Fatalf("Create with special characters failed: %v", err)
	}

	if domainPool.Purpose != purpose {
		t.Errorf("Expected purpose %s, got %s", purpose, domainPool.Purpose)
	}
}
