package repository

import (
	"context"
	"marketing/internal/model"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupDouyinCardTestDB 设置抖音卡片测试数据库
func setupDouyinCardTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.DouyinCard{},
	)
}

// setupDouyinCardRepository 创建测试用的抖音卡片仓库实例
func setupDouyinCardRepository(t *testing.T) DouyinCardRepository {
	database := setupDouyinCardTestDB(t)
	return NewDouyinCardRepository(database)
}

// TestDouyinCardRepository_Create 测试创建抖音卡片
func TestDouyinCardRepository_Create(t *testing.T) {
	repo := setupDouyinCardRepository(t)

	tests := []struct {
		name    string
		card    *model.DouyinCard
		wantErr bool
	}{
		{
			name: "create card success",
			card: &model.DouyinCard{
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
			card: &model.DouyinCard{
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

// TestDouyinCardRepository_GetByID 测试根据 ID 获取抖音卡片
func TestDouyinCardRepository_GetByID(t *testing.T) {
	repo := setupDouyinCardRepository(t)

	// 创建测试数据
	card := &model.DouyinCard{
		Title:       "GetByID Card",
		Description: "GetByID Description",
		IsActive:    true,
	}
	repo.Create(card)

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
			result, err := repo.GetByID(tt.id)

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

// TestDouyinCardRepository_GetList 测试获取抖音卡片列表
func TestDouyinCardRepository_GetList(t *testing.T) {
	database := setupDouyinCardTestDB(t)
	repo := NewDouyinCardRepository(database)

	// 创建测试数据
	activeTrue := true
	activeFalse := false

	// 创建 5 个活跃卡片
	for i := 1; i <= 5; i++ {
		card := &model.DouyinCard{
			Title:    "Active Card " + string(rune('A'+i-1)),
			IsActive: true,
		}
		if err := database.Create(card).Error; err != nil {
			t.Fatalf("Failed to create active card %d: %v", i, err)
		}
	}

	// 创建 1 个不活跃卡片 - 先创建再更新 IsActive 字段
	inactiveCard := &model.DouyinCard{
		Title:    "Inactive Card",
		IsActive: true, // 先设置为 true 让 GORM 创建记录
	}
	if err := database.Create(inactiveCard).Error; err != nil {
		t.Fatalf("Failed to create inactive card: %v", err)
	}
	// 然后用 Update 明确更新 IsActive 为 false
	if err := database.Model(&model.DouyinCard{}).Where("id = ?", inactiveCard.ID).Update("is_active", false).Error; err != nil {
		t.Fatalf("Failed to update inactive card IsActive: %v", err)
	}
	// 验证更新成功
	var verifyCard model.DouyinCard
	if err := database.First(&verifyCard, inactiveCard.ID).Error; err != nil {
		t.Fatalf("Failed to verify inactive card: %v", err)
	}
	if verifyCard.IsActive {
		t.Errorf("Expected inactive card IsActive to be false, got true")
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

// TestDouyinCardRepository_Update 测试更新抖音卡片
func TestDouyinCardRepository_Update(t *testing.T) {
	repo := setupDouyinCardRepository(t)

	// 创建测试数据
	card := &model.DouyinCard{
		Title:       "Original Title",
		Description: "Original Description",
		IsActive:    true,
		ShortLinkID: 0,
	}
	repo.Create(card)

	card.Title = "Updated Title"
	card.ShortLinkID = 123

	updated, err := repo.Updatecard)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", updated.Title)
	}
	if updated.ShortLinkID != 123 {
		t.Errorf("Expected ShortLinkID 123, got %d", updated.ShortLinkID)
	}
}

// TestDouyinCardRepository_Delete 测试删除抖音卡片
func TestDouyinCardRepository_Delete(t *testing.T) {
	repo := setupDouyinCardRepository(t)

	// 创建测试数据
	card := &model.DouyinCard{
		Title:    "To Delete",
		IsActive: true,
	}
	repo.Create(card)

	err := repo.Deletecard.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(card.ID)
	if err == nil {
		t.Error("Expected card to be deleted")
	}
}

// TestDouyinCardRepository_IncrementViewCount 测试增加浏览次数
func TestDouyinCardRepository_IncrementViewCount(t *testing.T) {
	repo := setupDouyinCardRepository(t)

	// 创建测试数据
	card := &model.DouyinCard{
		Title:     "View Count Test",
		ViewCount: 0,
	}
	repo.Create(card)

	updated, err := repo.IncrementViewCount(context.Background(), card.ID)
	if err != nil {
		t.Errorf("IncrementViewCount() error = %v", err)
	}

	if updated.ViewCount != 1 {
		t.Errorf("Expected ViewCount 1, got %d", updated.ViewCount)
	}
}

// TestDouyinCardRepository_IncrementShareCount 测试增加分享次数
func TestDouyinCardRepository_IncrementShareCount(t *testing.T) {
	repo := setupDouyinCardRepository(t)

	// 创建测试数据
	card := &model.DouyinCard{
		Title:      "Share Count Test",
		ShareCount: 0,
	}
	repo.Create(card)

	err := repo.IncrementShareCount(context.Background(), card.ID)
	if err != nil {
		t.Errorf("IncrementShareCount() error = %v", err)
	}

	updated, _ := repo.GetByID(card.ID)
	if updated.ShareCount != 1 {
		t.Errorf("Expected ShareCount 1, got %d", updated.ShareCount)
	}
}
