package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupSmlistTestDB 设置短信列表测试数据库
func setupSmlistTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Smlist{},
	)
	db.SetTestDB(database)
	return database
}

// setupSmlistRepository 创建测试用的短信列表仓库实例
func setupSmlistRepository(t *testing.T) SmlistRepository {
	setupSmlistTestDB(t)
	return NewSmlistRepository()
}

// TestSmlistRepository_Create 测试创建短信列表
func TestSmlistRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupSmlistRepository(t)

	tests := []struct {
		name    string
		smlist  *model.Smlist
		wantErr bool
	}{
		{
			name: "create smlist success",
			smlist: &model.Smlist{
				QQ:    "12345678",
				Tg:    "@telegram_user",
				WX:    "wechat_user",
				X:     "twitter_user",
				Name:  "Test User",
				Phone: "13800138000",
				City:  "Beijing",
				Desc:  "Test description",
			},
			wantErr: false,
		},
		{
			name: "create smlist with minimal fields",
			smlist: &model.Smlist{
				Name:  "Minimal User",
				Phone: "13900139000",
			},
			wantErr: false,
		},
		{
			name: "create smlist with all fields",
			smlist: &model.Smlist{
				QQ:      "87654321",
				Tg:      "@another_telegram",
				WX:      "another_wechat",
				X:       "another_twitter",
				Name:    "Complete User",
				Phone:   "13700137000",
				City:    "Shanghai",
				Address: "No. 1 Test Road",
				Desc:    "Complete description",
				Age:     "25",
				Score:   "100",
				Price:   "500",
				Service: "VIP",
				Images:  "[\"img1.jpg\", \"img2.jpg\"]",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.smlist)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.smlist.ID == "" {
				t.Error("Expected smlist ID to be set after creation")
			}
		})
	}
}

// TestSmlistRepository_GetByID 测试根据 ID 获取短信列表
func TestSmlistRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupSmlistRepository(t)

	smlist := &model.Smlist{
		QQ:    "12345678",
		Name:  "GetByID User",
		Phone: "13800138000",
		City:  "Beijing",
	}
	repo.Create(ctx, smlist)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "get existing smlist",
			id:      smlist.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing smlist",
			id:      "non-existing-id",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "GetByID User" {
					t.Errorf("Expected name 'GetByID User', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestSmlistRepository_GetSmlistList 测试获取短信列表（分页）
func TestSmlistRepository_GetSmlistList(t *testing.T) {
	ctx := context.Background()
	repo := setupSmlistRepository(t)

	for i := 1; i <= 15; i++ {
		repo.Create(ctx, &model.Smlist{
			QQ:    "qq" + string(rune('0'+i)),
			Name:  "User " + string(rune('0'+i)),
			Phone: "1380013800" + string(rune('0'+(i%10))),
			City:  "Beijing",
		})
	}

	tests := []struct {
		name      string
		page      int
		limit     int
		wantCount int
		wantTotal int64
	}{
		{
			name:      "get all lists",
			page:      1,
			limit:     20,
			wantCount: 15,
			wantTotal: 15,
		},
		{
			name:      "get first page",
			page:      1,
			limit:     10,
			wantCount: 10,
			wantTotal: 15,
		},
		{
			name:      "get second page",
			page:      2,
			limit:     10,
			wantCount: 5,
			wantTotal: 15,
		},
		{
			name:      "get third page (empty)",
			page:      3,
			limit:     10,
			wantCount: 0,
			wantTotal: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.GetSmlistList(context.Background(), tt.page, tt.limit)

			if err != nil {
				t.Errorf("GetSmlistList() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if total != tt.wantTotal {
				t.Errorf("Expected total %d, got %d", tt.wantTotal, total)
			}
		})
	}
}

// TestSmlistRepository_GetSmlistAllList 测试获取所有短信列表
func TestSmlistRepository_GetSmlistAllList(t *testing.T) {
	ctx := context.Background()
	repo := setupSmlistRepository(t)

	for i := 1; i <= 8; i++ {
		repo.Create(ctx, &model.Smlist{
			QQ:    "qq" + string(rune('0'+i)),
			Name:  "All User " + string(rune('0'+i)),
			Phone: "1380013800" + string(rune('0'+(i%10))),
			City:  "Shanghai",
		})
	}

	results, total, err := repo.GetSmlistAllList(context.Background())
	if err != nil {
		t.Errorf("GetSmlistAllList() error = %v", err)
	}

	if len(results) != 8 {
		t.Errorf("Expected 8 results, got %d", len(results))
	}

	if total != 8 {
		t.Errorf("Expected total 8, got %d", total)
	}
}

// TestSmlistRepository_Delete 测试删除短信列表
func TestSmlistRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupSmlistRepository(t)

	smlist := &model.Smlist{
		QQ:    "12345678",
		Name:  "To Delete",
		Phone: "13800138000",
		City:  "Beijing",
	}
	repo.Create(ctx, smlist)

	err := repo.Delete(ctx, smlist.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, smlist.ID)
	if err == nil {
		t.Error("Expected smlist to be deleted")
	}
}

// TestSmlistRepository_GetRecentSmlistList 测试获取最近的短信列表
func TestSmlistRepository_GetRecentSmlistList(t *testing.T) {
	ctx := context.Background()
	repo := setupSmlistRepository(t)

	repo.Create(ctx, &model.Smlist{
		QQ:    "recent1",
		Name:  "Recent User 1",
		Phone: "13800138001",
		City:  "Beijing",
	})

	repo.Create(ctx, &model.Smlist{
		QQ:    "recent2",
		Name:  "Recent User 2",
		Phone: "13800138002",
		City:  "Shanghai",
	})

	oldSmlist := &model.Smlist{
		QQ:    "old1",
		Name:  "Old User 1",
		Phone: "13800138003",
		City:  "Guangzhou",
	}
	repo.Create(ctx, oldSmlist)

	db.GetDB().Model(&model.Smlist{}).Where("id = ?", oldSmlist.ID).Update("created_at", time.Now().Add(-time.Hour*72))

	results, err := repo.GetRecentSmlistList(context.Background())
	if err != nil {
		t.Errorf("GetRecentSmlistList() error = %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 recent results, got %d", len(results))
	}
}

// TestSmlistRepository_GetByID_NotFound 测试获取不存在的短信列表
func TestSmlistRepository_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupSmlistRepository(t)

	_, err := repo.GetByID(ctx, "non-existing-id")
	if err == nil {
		t.Error("Expected error when getting non-existing smlist")
	}
}

// TestSmlistRepository_GetSmlistList_Empty 测试获取空列表
func TestSmlistRepository_GetSmlistList_Empty(t *testing.T) {
	ctx := context.Background()
	repo := setupSmlistRepository(t)

	results, total, err := repo.GetSmlistList(ctx, 1, 10)
	if err != nil {
		t.Errorf("GetSmlistList() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

