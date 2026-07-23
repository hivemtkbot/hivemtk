package repository

import (
	"context"
	"marketing/internal/model"
	"testing"
	"time"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupDomainPoolTestDB 设置域名池测试数据库
func setupDomainPoolTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.DomainPool{},
	)
}

// setupDomainPoolRepository 创建测试用的域名池仓库实例
func setupDomainPoolRepository(t *testing.T) DomainPoolRepository {
	database := setupDomainPoolTestDB(t)
	return NewDomainPoolRepository(database)
}

// TestDomainPoolRepository_Create 测试创建域名池记录
func TestDomainPoolRepository_Create(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	tests := []struct {
		name    string
		domain  *model.DomainPool
		wantErr bool
	}{
		{
			name: "create domain success",
			domain: &model.DomainPool{
				Domain:    "example.com",
				Port:      443,
				Purpose:   "Production API",
				Status:    1,
				LastCheck: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "create domain with minimal fields",
			domain: &model.DomainPool{
				Domain: "minimal.com",
				Port:   80,
				Status: 1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Creatett.domain)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.domain.ID == 0 {
				t.Error("Expected domain ID to be set after creation")
			}
		})
	}
}

// TestDomainPoolRepository_GetByID 测试根据 ID 获取域名
func TestDomainPoolRepository_GetByID(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	// 创建测试数据
	domain := &model.DomainPool{
		Domain:    "getbyid.com",
		Port:      8080,
		Purpose:   "Test Domain",
		Status:    1,
		LastCheck: time.Now(),
	}
	repo.Create(domain)

	tests := []struct {
		name    string
		id      int
		wantErr bool
	}{
		{
			name:    "get existing domain",
			id:      domain.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing domain",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByID(tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Domain != "getbyid.com" {
					t.Errorf("Expected domain 'getbyid.com', got '%s'", result.Domain)
				}
				if result.Port != 8080 {
					t.Errorf("Expected port 8080, got %d", result.Port)
				}
			}
		})
	}
}

// TestDomainPoolRepository_GetByDomain 测试根据域名获取
func TestDomainPoolRepository_GetByDomain(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	// 创建测试数据
	domain := &model.DomainPool{
		Domain:    "unique-domain.com",
		Port:      443,
		Purpose:   "Unique Test",
		Status:    1,
		LastCheck: time.Now(),
	}
	repo.Create(domain)

	tests := []struct {
		name    string
		domain  string
		wantErr bool
	}{
		{
			name:    "get existing domain",
			domain:  "unique-domain.com",
			wantErr: false,
		},
		{
			name:    "get non-existing domain",
			domain:  "non-existing.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByDomain(context.Background(), tt.domain)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByDomain() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Domain != tt.domain {
					t.Errorf("Expected domain '%s', got '%s'", tt.domain, result.Domain)
				}
			}
		})
	}
}

// TestDomainPoolRepository_List 测试获取域名列表
func TestDomainPoolRepository_List(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	// 创建测试数据
	for i := 1; i <= 5; i++ {
		repo.Create&model.DomainPool{
			Domain:    string(rune('a'+i-1)) + "domain.com",
			Port:      80 + i,
			Purpose:   "Test domain " + string(rune('0'+i)),
			Status:    1,
			LastCheck: time.Now(),
		})
	}

	tests := []struct {
		name      string
		page      int
		pageSize  int
		domain    string
		status    int
		wantCount int
		wantErr   bool
	}{
		{
			name:      "get first page",
			page:      1,
			pageSize:  3,
			domain:    "",
			status:    0,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "get second page",
			page:      2,
			pageSize:  3,
			domain:    "",
			status:    0,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "filter by status",
			page:      1,
			pageSize:  10,
			domain:    "",
			status:    1,
			wantCount: 5,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.List(context.Background(), tt.page, tt.pageSize, tt.domain, tt.status)

			if (err != nil) != tt.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if tt.page == 1 && tt.status == 0 && int(total) != 5 {
				t.Errorf("Expected total 5, got %d", total)
			}
		})
	}
}

// TestDomainPoolRepository_List_WithDomainFilter 测试带域名过滤的列表
func TestDomainPoolRepository_List_WithDomainFilter(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	// 创建测试数据
	repo.Create&model.DomainPool{
		Domain:    "test-example.com",
		Port:      443,
		Status:    1,
		LastCheck: time.Now(),
	})
	repo.Create&model.DomainPool{
		Domain:    "example-site.com",
		Port:      80,
		Status:    1,
		LastCheck: time.Now(),
	})
	repo.Create&model.DomainPool{
		Domain:    "other.com",
		Port:      8080,
		Status:    1,
		LastCheck: time.Now(),
	})

	results, total, err := repo.List(context.Background(), 1, 10, "example", 0)
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results with 'example' filter, got %d", len(results))
	}

	if int(total) != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}
}

// TestDomainPoolRepository_Update 测试更新域名
func TestDomainPoolRepository_Update(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	// 创建测试数据
	domain := &model.DomainPool{
		Domain:    "update-test.com",
		Port:      80,
		Purpose:   "Original Purpose",
		Status:    1,
		LastCheck: time.Now(),
	}
	repo.Create(domain)

	domain.Port = 443
	domain.Purpose = "Updated Purpose"

	err := repo.Updatedomain)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(domain.ID)
	if updated.Port != 443 {
		t.Errorf("Expected port 443, got %d", updated.Port)
	}
	if updated.Purpose != "Updated Purpose" {
		t.Errorf("Expected purpose 'Updated Purpose', got '%s'", updated.Purpose)
	}
}

// TestDomainPoolRepository_Delete 测试删除域名
func TestDomainPoolRepository_Delete(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	// 创建测试数据
	domain := &model.DomainPool{
		Domain:    "delete-test.com",
		Port:      80,
		Status:    1,
		LastCheck: time.Now(),
	}
	repo.Create(domain)

	err := repo.Deletedomain.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(domain.ID)
	if err == nil {
		t.Error("Expected domain to be deleted")
	}
}

// TestDomainPoolRepository_UpdateStatus 测试更新域名状态
func TestDomainPoolRepository_UpdateStatus(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	// 创建测试数据
	domain := &model.DomainPool{
		Domain:    "status-test.com",
		Port:      80,
		Status:    1,
		LastCheck: time.Now(),
	}
	repo.Create(domain)

	err := repo.UpdateStatus(context.Background(), domain.ID, 2)
	if err != nil {
		t.Errorf("UpdateStatus() error = %v", err)
	}

	updated, _ := repo.GetByID(domain.ID)
	if updated.Status != 2 {
		t.Errorf("Expected status 2, got %d", updated.Status)
	}
}

// TestDomainPoolRepository_UpdateLastCheck 测试更新最后检查时间
func TestDomainPoolRepository_UpdateLastCheck(t *testing.T) {
	repo := setupDomainPoolRepository(t)

	// 创建测试数据
	domain := &model.DomainPool{
		Domain:    "check-test.com",
		Port:      80,
		Status:    1,
		LastCheck: time.Time{},
	}
	repo.Create(domain)

	checkTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	err := repo.UpdateLastCheck(context.Background(), domain.ID, checkTime)
	if err != nil {
		t.Errorf("UpdateLastCheck() error = %v", err)
	}

	updated, _ := repo.GetByID(domain.ID)
	if updated.LastCheck.IsZero() {
		t.Error("Expected LastCheck to be updated")
	}
}
