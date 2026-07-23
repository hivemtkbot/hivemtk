package repository

import (
	"context"
	"marketing/internal/model"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupXiaohongshuCardTestDB 设置小红书卡片测试数据库
func setupXiaohongshuCardTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.XiaohongshuCard{},
		&model.XiaohongshuCardActivity{},
	)
}

// setupXiaohongshuCardRepository 创建测试用的小红书卡片仓库实例
func setupXiaohongshuCardRepository(t *testing.T) XiaohongshuCardRepository {
	database := setupXiaohongshuCardTestDB(t)
	return NewXiaohongshuCardRepository(database)
}

// TestXiaohongshuCardRepository_Create 测试创建小红书卡片
func TestXiaohongshuCardRepository_Create(t *testing.T) {
	repo := setupXiaohongshuCardRepository(t)

	tests := []struct {
		name    string
		card    *model.XiaohongshuCard
		wantErr bool
	}{
		{
			name: "create card success",
			card: &model.XiaohongshuCard{
				Title:       "Test Card",
				Description: "Test Description",
				ImageURL:    "https://example.com/image.jpg",
				RedirectURL: "https://example.com",
				ShareURL:    "https://example.com/share",
				Tags:        "tag1,tag2",
				IsActive:    true,
			},
			wantErr: false,
		},
		{
			name: "create card with minimal fields",
			card: &model.XiaohongshuCard{
				Title:    "Minimal Card",
				IsActive: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.Creatett.card)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID == 0 {
					t.Error("Expected card ID to be set after creation")
				}
				if result.Title != tt.card.Title {
					t.Errorf("Expected title '%s', got '%s'", tt.card.Title, result.Title)
				}
			}
		})
	}
}

// TestXiaohongshuCardRepository_GetByID 测试根据 ID 获取小红书卡片
func TestXiaohongshuCardRepository_GetByID(t *testing.T) {
	repo := setupXiaohongshuCardRepository(t)

	// 创建测试数据
	card := &model.XiaohongshuCard{
		Title:       "GetByID Card",
		Description: "GetByID Description",
		IsActive:    true,
	}
	repo.Createcard)

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
			result, err := repo.GetByIDtt.id)

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

// TestXiaohongshuCardRepository_GetList 测试获取小红书卡片列表
func TestXiaohongshuCardRepository_GetList(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	repo := NewXiaohongshuCardRepository(database)

	// 创建测试数据
	activeTrue := true
	activeFalse := false

	// 创建 5 个活跃卡片
	for i := 1; i <= 5; i++ {
		card := &model.XiaohongshuCard{
			Title:    "Active Card " + string(rune('A'+i-1)),
			IsActive: true,
		}
		if err := database.Create(card).Error; err != nil {
			t.Fatalf("Failed to create active card %d: %v", i, err)
		}
	}

	// 创建 1 个不活跃卡片 - 先创建再更新 IsActive 字段
	inactiveCard := &model.XiaohongshuCard{
		Title:    "Inactive Card",
		IsActive: true,
	}
	if err := database.Create(inactiveCard).Error; err != nil {
		t.Fatalf("Failed to create inactive card: %v", err)
	}
	if err := database.Model(&model.XiaohongshuCard{}).Where("id = ?", inactiveCard.ID).Update("is_active", false).Error; err != nil {
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
			results, total, err := repo.GetListtt.req)

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

// TestXiaohongshuCardRepository_Update 测试更新小红书卡片
func TestXiaohongshuCardRepository_Update(t *testing.T) {
	repo := setupXiaohongshuCardRepository(t)

	// 创建测试数据
	card := &model.XiaohongshuCard{
		Title:       "Original Title",
		Description: "Original Description",
		IsActive:    true,
	}
	repo.Createcard)

	card.Title = "Updated Title"
	shortLinkID := uint(123)
	card.ShortLinkID = &shortLinkID

	updated, err := repo.Updatecard)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", updated.Title)
	}
	if updated.ShortLinkID == nil || *updated.ShortLinkID != 123 {
		t.Errorf("Expected ShortLinkID 123, got %v", updated.ShortLinkID)
	}
}

// TestXiaohongshuCardRepository_Delete 测试删除小红书卡片
func TestXiaohongshuCardRepository_Delete(t *testing.T) {
	repo := setupXiaohongshuCardRepository(t)

	// 创建测试数据
	card := &model.XiaohongshuCard{
		Title:    "To Delete",
		IsActive: true,
	}
	repo.Createcard)

	err := repo.Deletecard.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByIDcard.ID)
	if err == nil {
		t.Error("Expected card to be deleted")
	}
}

// TestXiaohongshuCardRepository_IncrementViewCount 测试增加浏览次数
func TestXiaohongshuCardRepository_IncrementViewCount(t *testing.T) {
	repo := setupXiaohongshuCardRepository(t)

	// 创建测试数据
	card := &model.XiaohongshuCard{
		Title:     "View Count Test",
		ViewCount: 0,
	}
	repo.Createcard)

	updated, err := repo.IncrementViewCount(context.Background(), card.ID)
	if err != nil {
		t.Errorf("IncrementViewCount() error = %v", err)
	}

	if updated.ViewCount != 1 {
		t.Errorf("Expected ViewCount 1, got %d", updated.ViewCount)
	}
}

// TestXiaohongshuCardRepository_CreateActivity 测试创建活动记录
func TestXiaohongshuCardRepository_CreateActivity(t *testing.T) {
	repo := setupXiaohongshuCardRepository(t)

	// 创建测试卡片
	card := &model.XiaohongshuCard{
		Title:    "Activity Test Card",
		IsActive: true,
	}
	repo.Createcard)

	// 创建测试数据
	activity := &model.XiaohongshuCardActivity{
		CardID:       card.ID,
		UserID:       1,
		Username:     "test_user",
		ActivityType: "view",
		IPAddress:    "192.168.1.1",
		Content:      "Test content",
	}

	err := repo.CreateActivity(context.Background(), activity)
	if err != nil {
		t.Errorf("CreateActivity() error = %v", err)
	}

	if activity.ID == 0 {
		t.Error("Expected activity ID to be set after creation")
	}
	if activity.CardID != card.ID {
		t.Errorf("Expected CardID %d, got %d", card.ID, activity.CardID)
	}
}

// TestXiaohongshuCardRepository_UpdateShortLinkID 测试更新短链 ID
func TestXiaohongshuCardRepository_UpdateShortLinkID(t *testing.T) {
	repo := setupXiaohongshuCardRepository(t)

	// 创建测试数据
	card := &model.XiaohongshuCard{
		Title: "ShortLink Test",
	}
	repo.Createcard)

	shortLinkID := uint(456)
	err := repo.UpdateShortLinkID(context.Background(), card.ID, &shortLinkID)
	if err != nil {
		t.Errorf("UpdateShortLinkID() error = %v", err)
	}

	updated, _ := repo.GetByIDcard.ID)
	if updated.ShortLinkID == nil || *updated.ShortLinkID != 456 {
		t.Errorf("Expected ShortLinkID 456, got %v", updated.ShortLinkID)
	}
}
