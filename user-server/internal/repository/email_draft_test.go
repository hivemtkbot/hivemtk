package repository

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupEmailDraftTestDB 设置邮件草稿测试数据库
func setupEmailDraftTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.EmailDraft{},
	)
	db.SetTestDB(database)
	return database
}

// setupEmailDraftRepository 创建测试用的邮件草稿仓库实例
func setupEmailDraftRepository(t *testing.T) EmailDraftRepository {
	setupEmailDraftTestDB(t)
	return NewEmailDraftRepository()
}

// TestEmailDraftRepository_Create 测试创建邮件草稿
func TestEmailDraftRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailDraftRepository(t)

	tests := []struct {
		name    string
		draft   *model.EmailDraft
		wantErr bool
	}{
		{
			name: "create draft success",
			draft: &model.EmailDraft{
				Subject: "Test Subject",
				Content: "Test content",
			},
			wantErr: false,
		},
		{
			name: "create draft with attachments",
			draft: &model.EmailDraft{
				Subject:     "Draft with attachments",
				Content:     "Content with attachments",
				Attachments: "[\"file1.pdf\", \"file2.jpg\"]",
			},
			wantErr: false,
		},
		{
			name: "create draft with minimal fields",
			draft: &model.EmailDraft{
				Subject: "Minimal Draft",
				Content: "Minimal content",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.draft)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.draft.ID == uuid.Nil {
				t.Error("Expected draft ID to be set after creation")
			}
		})
	}
}

// TestEmailDraftRepository_GetByID 测试根据 ID 获取草稿
func TestEmailDraftRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailDraftRepository(t)

	// 创建测试数据
	draft := &model.EmailDraft{
		Subject: "GetByID Test",
		Content: "Test content",
	}
	repo.Create(draft)

	tests := []struct {
		name    string
		id      uuid.UUID
		wantErr bool
	}{
		{
			name:    "get existing draft",
			id:      draft.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing draft",
			id:      uuid.New(),
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
				if result.Content != "Test content" {
					t.Errorf("Expected content 'Test content', got '%s'", result.Content)
				}
				if result.Subject != "GetByID Test" {
					t.Errorf("Expected subject 'GetByID Test', got '%s'", result.Subject)
				}
			}
		})
	}
}

// TestEmailDraftRepository_List 测试获取草稿列表
func TestEmailDraftRepository_List(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailDraftRepository(t)

	// 创建测试数据
	for i := 1; i <= 5; i++ {
		repo.Create(ctx, &model.EmailDraft){
			Subject: "Draft " + string(rune('0'+i)),
			Content: "Content " + string(rune('0'+i)),
		})
	}

	results, err := repo.List(context.Background())
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 drafts, got %d", len(results))
	}
}

// TestEmailDraftRepository_Update 测试更新草稿
func TestEmailDraftRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailDraftRepository(t)

	// 创建测试数据
	draft := &model.EmailDraft{
		Subject: "Original Subject",
		Content: "Original content",
	}
	repo.Create(draft)

	// 更新
	draft.Subject = "Updated Subject"
	draft.Content = "Updated content"

	err := repo.Update(ctx, draft)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(draft.ID)
	if updated.Subject != "Updated Subject" {
		t.Errorf("Expected subject 'Updated Subject', got '%s'", updated.Subject)
	}
	if updated.Content != "Updated content" {
		t.Errorf("Expected content 'Updated content', got '%s'", updated.Content)
	}
}

// TestEmailDraftRepository_Delete 测试删除草稿
func TestEmailDraftRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailDraftRepository(t)

	// 创建测试数据
	draft := &model.EmailDraft{
		Subject: "To Delete",
		Content: "Delete content",
	}
	repo.Create(draft)

	err := repo.Delete(ctx, draft.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(draft.ID)
	if err == nil {
		t.Error("Expected draft to be deleted")
	}
}

// TestEmailDraftRepository_GetByID_NotFound 测试获取不存在的草稿
func TestEmailDraftRepository_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailDraftRepository(t)

	_, err := repo.GetByIDuuid.New())
	if err == nil {
		t.Error("Expected error when getting non-existing draft")
	}
}

// TestEmailDraftRepository_List_EmptyResult 测试获取空列表
func TestEmailDraftRepository_List_EmptyResult(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailDraftRepository(t)

	results, err := repo.List(context.Background())
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 drafts, got %d", len(results))
	}
}

// TestEmailDraftRepository_Update_WithUpdatedAt 测试更新时间戳
func TestEmailDraftRepository_Update_WithUpdatedAt(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailDraftRepository(t)

	// 创建测试数据
	draft := &model.EmailDraft{
		Subject: "Timestamp Test",
		Content: "Content",
	}
	repo.Create(draft)

	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)

	// 更新
	draft.Subject = "Updated Subject"
	err := repo.Update(ctx, draft)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(draft.ID)
	if updated.UpdatedAt.Before(draft.Create(dAt) {
		t.Error("Expected UpdatedAt to be after CreatedAt")
	}
}
