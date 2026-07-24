package repository

import (
	"context"
	"marketing/internal/model"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupLiveCodeTestDB 设置活码测试数据库
func setupLiveCodeTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.LiveCode{},
		&model.DomainPool{},
	)
}

// setupLiveCodeRepository 创建测试用的活码仓库实例
func setupLiveCodeRepository(t *testing.T) LiveCodeRepository {
	database := setupLiveCodeTestDB(t)
	return NewLiveCodeRepository(database)
}

// TestLiveCodeRepository_Create 测试创建活码
func TestLiveCodeRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeRepository(t)

	tests := []struct {
		name    string
		card    *model.LiveCode
		wantErr bool
	}{
		{
			name: "create live code success",
			card: &model.LiveCode{
				Name:            "Test Live Code",
				ShortLink:       "abc123",
				ShortDomainID:   1,
				EntryDomainID:   2,
				LandingDomainID: 3,
				Status:          1,
				ImageURL:        "https://example.com/image.jpg",
				EntryURL:        "https://example.com/entry",
				LandingURL:      "https://example.com/landing",
			},
			wantErr: false,
		},
		{
			name: "create live code with minimal fields",
			card: &model.LiveCode{
				Name:            "Minimal Live Code",
				ShortLink:       "min123",
				ShortDomainID:   1,
				EntryDomainID:   1,
				LandingDomainID: 1,
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
				if tt.card.ID == "" {
					t.Error("Expected live code ID to be set after creation")
				}
			}
		})
	}
}

// TestLiveCodeRepository_GetByID 测试根据 ID 获取活码
func TestLiveCodeRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeRepository(t)

	// 创建测试数据
	liveCode := &model.LiveCode{
		Name:            "GetByID Live Code",
		ShortLink:       "test123",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
		Status:          1,
	}
	repo.Create(ctx, liveCode)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "get existing live code",
			id:      liveCode.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing live code",
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
				if result.Name != "GetByID Live Code" {
					t.Errorf("Expected name 'GetByID Live Code', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestLiveCodeRepository_GetByShortLink 测试根据短链获取活码
func TestLiveCodeRepository_GetByShortLink(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeRepository(t)

	// 创建测试数据
	liveCode := &model.LiveCode{
		Name:            "GetByShortLink Live Code",
		ShortLink:       "short123",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
		Status:          1,
	}
	repo.Create(ctx, liveCode)

	tests := []struct {
		name      string
		shortLink string
		wantErr   bool
		wantName  string
	}{
		{
			name:      "get existing live code by short link",
			shortLink: "short123",
			wantErr:   false,
			wantName:  "GetByShortLink Live Code",
		},
		{
			name:      "get non-existing live code by short link",
			shortLink: "non-existing",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByShortLink(context.Background(), tt.shortLink)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByShortLink() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && result.Name != tt.wantName {
				t.Errorf("Expected name '%s', got '%s'", tt.wantName, result.Name)
			}
		})
	}
}

// TestLiveCodeRepository_Update 测试更新活码
func TestLiveCodeRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeRepository(t)

	// 创建测试数据
	liveCode := &model.LiveCode{
		Name:            "Original Name",
		ShortLink:       "update123",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
		Status:          1,
	}
	repo.Create(ctx, liveCode)

	liveCode.Name = "Updated Name"
	liveCode.Status = 0

	err := repo.Update(ctx, liveCode)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, liveCode.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Status != 0 {
		t.Errorf("Expected status 0, got %d", updated.Status)
	}
}

// TestLiveCodeRepository_Delete 测试删除活码
func TestLiveCodeRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeRepository(t)

	// 创建测试数据
	liveCode := &model.LiveCode{
		Name:            "To Delete",
		ShortLink:       "delete123",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
		Status:          1,
	}
	repo.Create(ctx, liveCode)

	err := repo.Delete(ctx, liveCode.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, liveCode.ID)
	if err == nil {
		t.Error("Expected live code to be deleted")
	}
}

// TestLiveCodeRepository_GetList 测试获取活码列表
func TestLiveCodeRepository_GetList(t *testing.T) {
	ctx := context.Background()
	database := setupLiveCodeTestDB(t)
	repo := NewLiveCodeRepository(database)

	// 创建测试数据
	for i := 1; i <= 5; i++ {
		database.Create(&model.LiveCode{
			Name:            "Live Code " + string(rune('A'+i-1)),
			ShortLink:       "code" + string(rune('0'+i)),
			ShortDomainID:   1,
			EntryDomainID:   2,
			LandingDomainID: 3,
			Status:          1,
		})
	}

	// 创建 1 个禁用的活码
	database.Create(&model.LiveCode{
		Name:            "Disabled Live Code",
		ShortLink:       "disabled",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
		Status:          0,
	})

	tests := []struct {
		name       string
		page       int
		pageSize   int
		nameFilter string
		wantCount  int
		wantTotal  int64
	}{
		{
			name:      "get all live codes",
			page:      1,
			pageSize:  10,
			wantCount: 6,
			wantTotal: 6,
		},
		{
			name:      "get first page",
			page:      1,
			pageSize:  3,
			wantCount: 3,
			wantTotal: 6,
		},
		{
			name:       "filter by name",
			page:       1,
			pageSize:   10,
			nameFilter: "Live Code A",
			wantCount:  1,
			wantTotal:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.GetList(ctx, tt.page, tt.pageSize, tt.nameFilter, "")

			if err != nil {
				t.Errorf("GetList() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if total != int64(tt.wantTotal) {
				t.Errorf("Expected total %d, got %d", tt.wantTotal, total)
			}
		})
	}
}

// TestLiveCodeRepository_GetAvailableLiveCodes 测试获取可用的活码
func TestLiveCodeRepository_GetAvailableLiveCodes(t *testing.T) {
	ctx := context.Background()
	database := setupLiveCodeTestDB(t)
	repo := NewLiveCodeRepository(database)

	// 创建 3 个可用的活码（status=1, 7 天内创建）
	for i := 1; i <= 3; i++ {
		liveCode := &model.LiveCode{
			Name:            "Available " + string(rune('0'+i)),
			ShortLink:       "avail" + string(rune('0'+i)),
			ShortDomainID:   1,
			EntryDomainID:   2,
			LandingDomainID: 3,
			Status:          1,
		}
		database.Create(liveCode)
	}

	// 创建 1 个禁用的活码 - 先创建再更新 Status 字段
	disabledCode := &model.LiveCode{
		Name:            "Disabled Code",
		ShortLink:       "disabled",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
		Status:          1, // GORM default might override, set to 1 first
	}
	database.Create(disabledCode)
	// Then update to status 0
	database.Model(&model.LiveCode{}).Where("id = ?", disabledCode.ID).Update("status", 0)

	availableCodes, err := repo.GetAvailableLiveCodes(ctx)
	if err != nil {
		t.Errorf("GetAvailableLiveCodes() error = %v", err)
	}

	// 应该返回 3 个可用的活码（status=1 且 7 天内创建的）
	// Note: All records created in this test are within 7 days, so we expect 3
	if len(availableCodes) != 3 {
		t.Errorf("Expected 3 available live codes, got %d", len(availableCodes))
	}
}

// TestLiveCodeRepository_GetByID_NotFound 测试获取不存在的活码
func TestLiveCodeRepository_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeRepository(t)

	_, err := repo.GetByID(ctx, "non-existing-id")
	if err == nil {
		t.Error("Expected error when getting non-existing live code")
	}
}

// TestLiveCodeRepository_Delete_NotFound 测试删除不存在的活码
func TestLiveCodeRepository_Delete_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeRepository(t)

	err := repo.Delete(ctx, "non-existing-id")
	if err != nil {
		// GORM Delete 不会返回错误，即使没有记录被删除
		// 这里只检查是否 panic
		t.Errorf("Delete() panicked = %v", err)
	}
}
