package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupShortLinkTestDB 设置短链测试数据库
func setupShortLinkTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.ShortLink{},
	)
}

// setupShortLinkRepository 创建测试用的短链仓库实例
func setupShortLinkRepository(t *testing.T) ShortLinkRepository {
	database := setupShortLinkTestDB(t)
	return NewShortLinkRepository(database)
}

// TestShortLinkRepository_Create 测试创建短链
func TestShortLinkRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	tests := []struct {
		name    string
		link    *model.ShortLink
		wantErr bool
	}{
		{
			name: "create short link success",
			link: &model.ShortLink{
				ShortCode:   "abc123",
				OriginalURL: "https://example.com/very/long/url",
				Title:       "Test Link",
				Description: "Test Description",
				DomainID:    1,
				Status:      1,
			},
			wantErr: false,
		},
		{
			name: "create short link with minimal fields",
			link: &model.ShortLink{
				ShortCode:   "min123",
				OriginalURL: "https://example.com",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.link)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.link.ID == 0 {
				t.Error("Expected short link ID to be set after creation")
			}
		})
	}
}

// TestShortLinkRepository_GetByID 测试根据 ID 获取短链
func TestShortLinkRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	link := &model.ShortLink{
		ShortCode:   "getbyid",
		OriginalURL: "https://example.com",
		Title:       "GetByID Link",
	}
	repo.Create(ctx, link)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing link",
			id:      link.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing link",
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
				if result.ShortCode != "getbyid" {
					t.Errorf("Expected short code 'getbyid', got '%s'", result.ShortCode)
				}
			}
		})
	}
}

// TestShortLinkRepository_GetByShortCode 测试根据短码获取短链
func TestShortLinkRepository_GetByShortCode(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	link := &model.ShortLink{
		ShortCode:   "testcode",
		OriginalURL: "https://example.com/test",
		Title:       "Test Link",
	}
	repo.Create(ctx, link)

	tests := []struct {
		name      string
		shortCode string
		wantErr   bool
		wantTitle string
	}{
		{
			name:      "get existing link by short code",
			shortCode: "testcode",
			wantErr:   false,
			wantTitle: "Test Link",
		},
		{
			name:      "get non-existing link by short code",
			shortCode: "nonexistent",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByShortCode(context.Background(), tt.shortCode)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByShortCode() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && result.Title != tt.wantTitle {
				t.Errorf("Expected title '%s', got '%s'", tt.wantTitle, result.Title)
			}
		})
	}
}

// TestShortLinkRepository_GetList 测试获取短链列表
func TestShortLinkRepository_GetList(t *testing.T) {
	ctx := context.Background()
	database := setupShortLinkTestDB(t)
	repo := NewShortLinkRepository(database)

	for i := 1; i <= 5; i++ {
		database.Create(&model.ShortLink{
			ShortCode:   "code" + string(rune('0'+i)),
			OriginalURL: "https://example.com/" + string(rune('0'+i)),
			Title:       "Link " + string(rune('A'+i-1)),
			Status:      1,
		})
	}

	database.Create(&model.ShortLink{
		ShortCode:   "disabled",
		OriginalURL: "https://example.com/disabled",
		Title:       "Disabled Link",
		Status:      2,
	})

	tests := []struct {
		name        string
		page        int
		pageSize    int
		shortCode   string
		originalURL string
		status      int
		wantCount   int
		wantTotal   int64
	}{
		{
			name:      "get all links",
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
			name:      "filter by short code",
			page:      1,
			pageSize:  10,
			shortCode: "code1",
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:        "filter by original URL",
			page:        1,
			pageSize:    10,
			originalURL: "example.com/1",
			wantCount:   1,
			wantTotal:   1,
		},
		{
			name:      "filter by status (active)",
			page:      1,
			pageSize:  10,
			status:    1,
			wantCount: 5,
			wantTotal: 5,
		},
		{
			name:      "filter by status (disabled)",
			page:      1,
			pageSize:  10,
			status:    2,
			wantCount: 1,
			wantTotal: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.GetList(ctx, tt.page, tt.pageSize, tt.shortCode, tt.originalURL, tt.status)

			if err != nil {
				t.Errorf("GetList() error = %v", err)
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

// TestShortLinkRepository_Update 测试更新短链
func TestShortLinkRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	link := &model.ShortLink{
		ShortCode:   "update",
		OriginalURL: "https://example.com/original",
		Title:       "Original Title",
	}
	repo.Create(ctx, link)

	link.Title = "Updated Title"
	link.Description = "New Description"

	err := repo.Update(ctx, link)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, link.ID)
	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", updated.Title)
	}
	if updated.Description != "New Description" {
		t.Errorf("Expected description 'New Description', got '%s'", updated.Description)
	}
}

// TestShortLinkRepository_Delete 测试删除短链
func TestShortLinkRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	link := &model.ShortLink{
		ShortCode:   "delete",
		OriginalURL: "https://example.com/delete",
	}
	repo.Create(ctx, link)

	err := repo.Delete(ctx, link.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, link.ID)
	if err == nil {
		t.Error("Expected link to be deleted")
	}
}

// TestShortLinkRepository_GetTotalCount 测试获取短链总数
func TestShortLinkRepository_GetTotalCount(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	for i := 1; i <= 5; i++ {
		repo.Create(ctx, &model.ShortLink{
			ShortCode:   "code" + string(rune('0'+i)),
			OriginalURL: "https://example.com",
		})
	}

	count, err := repo.GetTotalCount(context.Background())
	if err != nil {
		t.Errorf("GetTotalCount() error = %v", err)
	}

	if count != 5 {
		t.Errorf("Expected count 5, got %d", count)
	}
}

// TestShortLinkRepository_IncreaseClickCount 测试增加点击次数
func TestShortLinkRepository_IncreaseClickCount(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	link := &model.ShortLink{
		ShortCode:   "click",
		OriginalURL: "https://example.com",
		ClickCount:  0,
	}
	repo.Create(ctx, link)

	err := repo.IncreaseClickCount(context.Background(), link.ID)
	if err != nil {
		t.Errorf("IncreaseClickCount() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, link.ID)
	if updated.ClickCount != 1 {
		t.Errorf("Expected ClickCount 1, got %d", updated.ClickCount)
	}

	err = repo.IncreaseClickCount(context.Background(), link.ID)
	if err != nil {
		t.Errorf("IncreaseClickCount() error = %v", err)
	}

	updated2, _ := repo.GetByID(ctx, link.ID)
	if updated2.ClickCount != 2 {
		t.Errorf("Expected ClickCount 2, got %d", updated2.ClickCount)
	}
}

// TestShortLinkRepository_GetByID_NotFound 测试获取不存在的短链
func TestShortLinkRepository_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	_, err := repo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("Expected error when getting non-existing link")
	}
}

// TestShortLinkRepository_GetByShortCode_NotFound 测试获取不存在的短码
func TestShortLinkRepository_GetByShortCode_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	_, err := repo.GetByShortCode(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existing short code")
	}
}

// TestShortLinkRepository_GetList_EmptyResult 测试获取空结果
func TestShortLinkRepository_GetList_EmptyResult(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	results, total, err := repo.GetList(ctx, 1, 10, "", "", 0)
	if err != nil {
		t.Errorf("GetList() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

// TestShortLinkRepository_WithExpireTime 测试有过期时间的短链
func TestShortLinkRepository_WithExpireTime(t *testing.T) {
	ctx := context.Background()
	repo := setupShortLinkRepository(t)

	futureTime := time.Now().Add(24 * time.Hour)
	link := &model.ShortLink{
		ShortCode:   "expiring",
		OriginalURL: "https://example.com",
		ExpireTime:  &futureTime,
	}
	err := repo.Create(ctx, link)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	updated, err := repo.GetByID(ctx, link.ID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}

	if updated.ExpireTime == nil {
		t.Error("Expected ExpireTime to be set")
	}
}
