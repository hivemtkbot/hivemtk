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

// setupEmailListTestDB 设置邮件列表测试数据库
func setupEmailListTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.EmailList{},
	)
	db.SetTestDB(database)
	return database
}

// setupEmailListRepository 创建测试用的邮件列表仓库实例
func setupEmailListRepository(t *testing.T) EmailListRepository {
	setupEmailListTestDB(t)
	return NewEmailListRepository()
}

// TestEmailListRepository_Create 测试创建邮件列表
func TestEmailListRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	tests := []struct {
		name    string
		list    *model.EmailList
		wantErr bool
	}{
		{
			name: "create email list success",
			list: &model.EmailList{
				To:       "user@example.com",
				Subject:  "Test Subject",
				Content:  "Test content",
				From:     "sender@example.com",
				TraceID:  uuid.New(),
				IsSend:   0,
				SendTime: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "create sent email list",
			list: &model.EmailList{
				To:       "sent@example.com",
				Subject:  "Sent Email",
				Content:  "Sent content",
				From:     "sender@example.com",
				TraceID:  uuid.New(),
				IsSend:   1,
				SendTime: time.Now().Add(-time.Hour),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.list)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.list.ID == uuid.Nil {
				t.Error("Expected list ID to be set after creation")
			}
		})
	}
}

// TestEmailListRepository_BatchCreate 测试批量创建邮件列表
func TestEmailListRepository_BatchCreate(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	lists := []*model.EmailList{
		{
			To:       "user1@example.com",
			Subject:  "Email 1",
			Content:  "Content 1",
			From:     "sender@example.com",
			TraceID:  uuid.New(),
			IsSend:   0,
			SendTime: time.Now(),
		},
		{
			To:       "user2@example.com",
			Subject:  "Email 2",
			Content:  "Content 2",
			From:     "sender@example.com",
			TraceID:  uuid.New(),
			IsSend:   0,
			SendTime: time.Now(),
		},
		{
			To:       "user3@example.com",
			Subject:  "Email 3",
			Content:  "Content 3",
			From:     "sender@example.com",
			TraceID:  uuid.New(),
			IsSend:   0,
			SendTime: time.Now(),
		},
	}

	count, err := repo.BatchCreate(ctx, lists)
	if err != nil {
		t.Errorf("BatchCreate() error = %v", err)
	}

	if count != 3 {
		t.Errorf("Expected to create 3 lists, got %d", count)
	}
}

// TestEmailListRepository_GetByID 测试根据 ID 获取邮件列表
func TestEmailListRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	// 创建测试数据
	list := &model.EmailList{
		To:      "getbyid@example.com",
		Subject: "GetByID Test",
		Content: "Test content",
		From:    "sender@example.com",
		TraceID: uuid.New(),
		IsSend:  0,
	}
	repo.Create(ctx, list)

	tests := []struct {
		name    string
		id      uuid.UUID
		wantErr bool
	}{
		{
			name:    "get existing list",
			id:      list.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing list",
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
				if result.To != "getbyid@example.com" {
					t.Errorf("Expected to 'getbyid@example.com', got '%s'", result.To)
				}
			}
		})
	}
}

// TestEmailListRepository_List 测试获取邮件列表
func TestEmailListRepository_List(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	// 创建测试数据
	for i := 1; i <= 15; i++ {
		repo.Create(ctx, &model.EmailList{
			To:      "user" + string(rune('0'+i)) + "@example.com",
			Subject: "Email " + string(rune('0'+i)),
			Content: "Content " + string(rune('0'+i)),
			From:    "sender@example.com",
			TraceID: uuid.New(),
			IsSend:  0,
		})
	}

	tests := []struct {
		name      string
		page      int
		pageSize  int
		wantCount int
		wantTotal int64
	}{
		{
			name:      "get all lists",
			page:      1,
			pageSize:  20,
			wantCount: 15,
			wantTotal: 15,
		},
		{
			name:      "get first page",
			page:      1,
			pageSize:  10,
			wantCount: 10,
			wantTotal: 15,
		},
		{
			name:      "get second page",
			page:      2,
			pageSize:  10,
			wantCount: 5,
			wantTotal: 15,
		},
		{
			name:      "get third page (empty)",
			page:      3,
			pageSize:  10,
			wantCount: 0,
			wantTotal: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.List(ctx, tt.page, tt.pageSize)

			if err != nil {
				t.Errorf("List() error = %v", err)
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

// TestEmailListRepository_Update 测试更新邮件列表
func TestEmailListRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	// 创建测试数据
	list := &model.EmailList{
		To:      "update@example.com",
		Subject: "Original Subject",
		Content: "Original content",
		From:    "sender@example.com",
		TraceID: uuid.New(),
		IsSend:  0,
	}
	repo.Create(ctx, list)

	// 更新
	list.Subject = "Updated Subject"
	list.Content = "Updated content"
	list.IsSend = 1

	err := repo.Update(ctx, list)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, list.ID)
	if updated.Subject != "Updated Subject" {
		t.Errorf("Expected subject 'Updated Subject', got '%s'", updated.Subject)
	}
	if updated.IsSend != 1 {
		t.Errorf("Expected IsSend 1, got %d", updated.IsSend)
	}
}

// TestEmailListRepository_Delete 测试删除邮件列表
func TestEmailListRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	// 创建测试数据
	list := &model.EmailList{
		To:      "delete@example.com",
		Subject: "To Delete",
		Content: "Delete content",
		From:    "sender@example.com",
		TraceID: uuid.New(),
		IsSend:  0,
	}
	repo.Create(ctx, list)

	err := repo.Delete(ctx, list.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, list.ID)
	if err == nil {
		t.Error("Expected list to be deleted")
	}
}

// TestEmailListRepository_GetUnsentEmailList 测试获取未发送的邮件列表
func TestEmailListRepository_GetUnsentEmailList(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	// 创建未发送的邮件
	repo.Create(ctx, &model.EmailList{
		To:      "unsent1@example.com",
		Subject: "Unsent Email 1",
		Content: "Content 1",
		From:    "sender@example.com",
		TraceID: uuid.New(),
		IsSend:  0,
	})

	repo.Create(ctx, &model.EmailList{
		To:      "unsent2@example.com",
		Subject: "Unsent Email 2",
		Content: "Content 2",
		From:    "sender@example.com",
		TraceID: uuid.New(),
		IsSend:  0,
	})

	// 创建已发送的邮件
	repo.Create(ctx, &model.EmailList{
		To:      "sent@example.com",
		Subject: "Sent Email",
		Content: "Sent content",
		From:    "sender@example.com",
		TraceID: uuid.New(),
		IsSend:  1,
	})

	results, err := repo.GetUnsentEmailList(ctx, 10)
	if err != nil {
		t.Errorf("GetUnsentEmailList() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 unsent emails, got %d", len(results))
	}
}

// TestEmailListRepository_GetTodayCountByFrom 测试获取今日发送数量
func TestEmailListRepository_GetTodayCountByFrom(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	// 创建今日发送的邮件
	repo.Create(ctx, &model.EmailList{
		To:       "today1@example.com",
		Subject:  "Today Email 1",
		Content:  "Content 1",
		From:     "test_sender",
		TraceID:  uuid.New(),
		IsSend:   1,
		SendTime: time.Now(),
	})

	repo.Create(ctx, &model.EmailList{
		To:       "today2@example.com",
		Subject:  "Today Email 2",
		Content:  "Content 2",
		From:     "test_sender",
		TraceID:  uuid.New(),
		IsSend:   1,
		SendTime: time.Now(),
	})

	// 创建其他发送者的邮件
	repo.Create(ctx, &model.EmailList{
		To:       "other@example.com",
		Subject:  "Other Email",
		Content:  "Other content",
		From:     "other_sender",
		TraceID:  uuid.New(),
		IsSend:   1,
		SendTime: time.Now(),
	})

	count, err := repo.GetTodayCountByFrom(ctx, "test_sender")
	if err != nil {
		t.Errorf("GetTodayCountByFrom() error = %v", err)
	}

	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

// TestEmailListRepository_GetByTraceID 测试根据 TraceID 获取邮件
func TestEmailListRepository_GetByTraceID(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	traceID := uuid.New()
	list := &model.EmailList{
		To:      "trace@example.com",
		Subject: "Trace Test",
		Content: "Trace content",
		From:    "sender@example.com",
		TraceID: traceID,
		IsSend:  0,
	}
	repo.Create(ctx, list)

	tests := []struct {
		name    string
		traceID uuid.UUID
		wantErr bool
	}{
		{
			name:    "get existing trace",
			traceID: traceID,
			wantErr: false,
		},
		{
			name:    "get non-existing trace",
			traceID: uuid.New(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByTraceID(ctx, tt.traceID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByTraceID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.To != "trace@example.com" {
					t.Errorf("Expected to 'trace@example.com', got '%s'", result.To)
				}
			}
		})
	}
}

// TestEmailListRepository_GetByID_NotFound 测试获取不存在的邮件列表
func TestEmailListRepository_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	_, err := repo.GetByID(ctx, uuid.New())
	if err == nil {
		t.Error("Expected error when getting non-existing list")
	}
}

// TestEmailListRepository_List_EmptyResult 测试获取空列表
func TestEmailListRepository_List_EmptyResult(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailListRepository(t)

	results, total, err := repo.List(ctx, 1, 10)
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}
