package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupXianyuCardTestDB 设置闲鱼卡片测试数据库
func setupXianyuCardTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.XianyuCard{},
	)
}

// setupXianyuCardRepository 创建测试用的闲鱼卡片仓库实例
func setupXianyuCardRepository(t *testing.T) XianyuCardRepository {
	database := setupXianyuCardTestDB(t)
	return NewXianyuCardRepository(database)
}

// TestXianyuCardRepository_Create 测试创建闲鱼卡片
func TestXianyuCardRepository_Create(t *testing.T) {
	repo := setupXianyuCardRepository(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		card    *model.XianyuCard
		wantErr bool
	}{
		{
			name: "create card success",
			card: &model.XianyuCard{
				Title:        "Test Card",
				Description:  "Test Description",
				ImageURL:     "https://example.com/image.jpg",
				RedirectURL:  "https://example.com",
				ShortLinkID:  1,
				DomainPoolID: 1,
				Tags:         "tag1,tag2",
				IsActive:     true,
			},
			wantErr: false,
		},
		{
			name: "create card with minimal fields",
			card: &model.XianyuCard{
				Title:    "Minimal Card",
				IsActive: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.card)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if tt.card.ID == 0 {
					t.Error("Expected card ID to be set after creation")
				}
			}
		})
	}
}

// TestXianyuCardRepository_GetByID 测试根据 ID 获取闲鱼卡片
func TestXianyuCardRepository_GetByID(t *testing.T) {
	repo := setupXianyuCardRepository(t)
	ctx := context.Background()

	// 创建测试数据
	card := &model.XianyuCard{
		Title:       "GetByID Card",
		Description: "GetByID Description",
		IsActive:    true,
	}
	repo.Create(ctx, card)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing card",
			id:      card.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing card",
			id:      99999,
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
				if result.Title != "GetByID Card" {
					t.Errorf("Expected title 'GetByID Card', got '%s'", result.Title)
				}
			}
		})
	}
}

// TestXianyuCardRepository_GetList 测试获取闲鱼卡片列表
func TestXianyuCardRepository_GetList(t *testing.T) {
	database := setupXianyuCardTestDB(t)
	repo := NewXianyuCardRepository(database)
	ctx := context.Background()

	// 创建测试数据
	activeTrue := true
	activeFalse := false

	// 创建 5 个活跃卡片
	for i := 1; i <= 5; i++ {
		card := &model.XianyuCard{
			Title:    "Active Card " + string(rune('A'+i-1)),
			IsActive: true,
		}
		if err := database.Create(card).Error; err != nil {
			t.Fatalf("Failed to create active card %d: %v", i, err)
		}
	}

	// 创建 1 个不活跃卡片 - 先创建再更新 IsActive 字段
	inactiveCard := &model.XianyuCard{
		Title:    "Inactive Card",
		IsActive: true,
	}
	if err := database.Create(inactiveCard).Error; err != nil {
		t.Fatalf("Failed to create inactive card: %v", err)
	}
	if err := database.Model(&model.XianyuCard{}).Where("id = ?", inactiveCard.ID).Update("is_active", false).Error; err != nil {
		t.Fatalf("Failed to update inactive card IsActive: %v", err)
	}

	tests := []struct {
		name      string
		req       CardListFilter
		wantCount int
		wantErr   bool
	}{
		{
			name: "get all cards",
			req: CardListFilter{
				Page:     1,
				PageSize: 10,
			},
			wantCount: 6,
			wantErr:   false,
		},
		{
			name: "get first page",
			req: CardListFilter{
				Page:     1,
				PageSize: 3,
			},
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "filter by active status (true)",
			req: CardListFilter{
				Page:     1,
				PageSize: 10,
				IsActive: &activeTrue,
			},
			wantCount: 5,
			wantErr:   false,
		},
		{
			name: "filter by active status (false)",
			req: CardListFilter{
				Page:     1,
				PageSize: 10,
				IsActive: &activeFalse,
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "filter by keyword",
			req: CardListFilter{
				Page:     1,
				PageSize: 10,
				Keyword:  "Active Card A",
			},
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.GetList(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetList() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if tt.name == "get all cards" && int(total) != 6 {
				t.Errorf("Expected total 6, got %d", total)
			}
		})
	}
}

// TestXianyuCardRepository_Update 测试更新闲鱼卡片
func TestXianyuCardRepository_Update(t *testing.T) {
	repo := setupXianyuCardRepository(t)
	ctx := context.Background()

	// 创建测试数据
	card := &model.XianyuCard{
		Title:       "Original Title",
		Description: "Original Description",
		IsActive:    true,
		ShortLinkID: 0,
	}
	repo.Create(ctx, card)

	card.Title = "Updated Title"
	card.ShortLinkID = 123

	err := repo.Update(ctx, card)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, card.ID)
	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", updated.Title)
	}
	if updated.ShortLinkID != 123 {
		t.Errorf("Expected ShortLinkID 123, got %d", updated.ShortLinkID)
	}
}

// TestXianyuCardRepository_Delete 测试删除闲鱼卡片
func TestXianyuCardRepository_Delete(t *testing.T) {
	repo := setupXianyuCardRepository(t)
	ctx := context.Background()

	// 创建测试数据
	card := &model.XianyuCard{
		Title:    "To Delete",
		IsActive: true,
	}
	repo.Create(ctx, card)

	err := repo.Delete(ctx, card.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, card.ID)
	if err == nil {
		t.Error("Expected card to be deleted")
	}
}

// TestXianyuCardRepository_Delete_NotFound 测试删除不存在的卡片
func TestXianyuCardRepository_Delete_NotFound(t *testing.T) {
	repo := setupXianyuCardRepository(t)
	ctx := context.Background()

	err := repo.Delete(ctx, 99999)
	if err == nil {
		t.Error("Expected error when deleting non-existing card")
	}
}

// TestXianyuCardRepository_GetByID_NotFound 测试获取不存在的卡片
func TestXianyuCardRepository_GetByID_NotFound(t *testing.T) {
	repo := setupXianyuCardRepository(t)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("Expected error when getting non-existing card")
	}
}
