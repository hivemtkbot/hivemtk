package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupKuaishouCardTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.KuaishouCard{},
		&model.KuaishouCardActivity{},
	)
}

func setupKuaishouCardRepository(t *testing.T) KuaishouCardRepository {
	database := setupKuaishouCardTestDB(t)
	return NewKuaishouCardRepository(database)
}

// TestKuaishouCardRepository_Create 测试创建快手卡片
func TestKuaishouCardRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	tests := []struct {
		name    string
		card    *model.KuaishouCard
		wantErr bool
	}{
		{
			name: "create card success",
			card: &model.KuaishouCard{
				Title:        "Test Card",
				Description:  "Test Description",
				ImageURL:     "https://example.com/image.jpg",
				RedirectURL:  "https://example.com",
				DomainPoolID: 1,
				Tags:         "tag1,tag2",
				IsActive:     true,
			},
			wantErr: false,
		},
		{
			name: "create card with minimal fields",
			card: &model.KuaishouCard{
				Title:    "Minimal Card",
				IsActive: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.Create(ctx, tt.card)

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

// TestKuaishouCardRepository_GetByID 测试根据 ID 获取快手卡片
func TestKuaishouCardRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	card := &model.KuaishouCard{
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

// TestKuaishouCardRepository_GetList 测试获取快手卡片列表
func TestKuaishouCardRepository_GetList(t *testing.T) {
	ctx := context.Background()
	database := setupKuaishouCardTestDB(t)
	repo := NewKuaishouCardRepository(database)

	activeTrue := true
	activeFalse := false

	for i := 1; i <= 5; i++ {
		card := &model.KuaishouCard{
			Title:    "Active Card " + string(rune('A'+i-1)),
			IsActive: true,
		}
		if err := database.Create(card).Error; err != nil {
			t.Fatalf("Failed to create active card %d: %v", i, err)
		}
	}

	inactiveCard := &model.KuaishouCard{
		Title:    "Inactive Card",
		IsActive: true,
	}
	if err := database.Create(inactiveCard).Error; err != nil {
		t.Fatalf("Failed to create inactive card: %v", err)
	}
	if err := database.Model(&model.KuaishouCard{}).Where("id = ?", inactiveCard.ID).Update("is_active", false).Error; err != nil {
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

// TestKuaishouCardRepository_Update 测试更新快手卡片
func TestKuaishouCardRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	card := &model.KuaishouCard{
		Title:       "Original Title",
		Description: "Original Description",
		IsActive:    true,
	}
	repo.Create(ctx, card)

	card.Title = "Updated Title"
	shortLinkID := uint(123)
	card.ShortLinkID = &shortLinkID

	updated, err := repo.Update(ctx, card)
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

// TestKuaishouCardRepository_Delete 测试删除快手卡片
func TestKuaishouCardRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	card := &model.KuaishouCard{
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

// TestKuaishouCardRepository_IncrementViewCount 测试增加浏览次数
func TestKuaishouCardRepository_IncrementViewCount(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	card := &model.KuaishouCard{
		Title:     "View Count Test",
		ViewCount: 0,
	}
	repo.Create(ctx, card)

	updated, err := repo.IncrementViewCount(context.Background(), card.ID)
	if err != nil {
		t.Errorf("IncrementViewCount() error = %v", err)
	}

	if updated.ViewCount != 1 {
		t.Errorf("Expected ViewCount 1, got %d", updated.ViewCount)
	}
}

// TestKuaishouCardRepository_IncrementLikeCount 测试增加点赞次数
func TestKuaishouCardRepository_IncrementLikeCount(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	card := &model.KuaishouCard{
		Title:     "Like Count Test",
		LikeCount: 0,
	}
	repo.Create(ctx, card)

	err := repo.IncrementLikeCount(context.Background(), card.ID)
	if err != nil {
		t.Errorf("IncrementLikeCount() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, card.ID)
	if updated.LikeCount != 1 {
		t.Errorf("Expected LikeCount 1, got %d", updated.LikeCount)
	}
}

// TestKuaishouCardRepository_IncrementShareCount 测试增加分享次数
func TestKuaishouCardRepository_IncrementShareCount(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	card := &model.KuaishouCard{
		Title:      "Share Count Test",
		ShareCount: 0,
	}
	repo.Create(ctx, card)

	err := repo.IncrementShareCount(context.Background(), card.ID)
	if err != nil {
		t.Errorf("IncrementShareCount() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, card.ID)
	if updated.ShareCount != 1 {
		t.Errorf("Expected ShareCount 1, got %d", updated.ShareCount)
	}
}

// TestKuaishouCardRepository_CreateActivity 测试创建活动记录
func TestKuaishouCardRepository_CreateActivity(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	card := &model.KuaishouCard{
		Title:    "Activity Test Card",
		IsActive: true,
	}
	repo.Create(ctx, card)

	activity := &model.KuaishouCardActivity{
		CardID:       card.ID,
		UserID:       1,
		Username:     "test_user",
		ActivityType: "view",
		IPAddress:    "192.168.1.1",
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

// TestKuaishouCardRepository_UpdateShortLinkID 测试更新短链 ID
func TestKuaishouCardRepository_UpdateShortLinkID(t *testing.T) {
	ctx := context.Background()
	repo := setupKuaishouCardRepository(t)

	card := &model.KuaishouCard{
		Title: "ShortLink Test",
	}
	repo.Create(ctx, card)

	shortLinkID := uint(456)
	err := repo.UpdateShortLinkID(context.Background(), card.ID, &shortLinkID)
	if err != nil {
		t.Errorf("UpdateShortLinkID() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, card.ID)
	if updated.ShortLinkID == nil || *updated.ShortLinkID != 456 {
		t.Errorf("Expected ShortLinkID 456, got %v", updated.ShortLinkID)
	}
}
