package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// setupEmailJobsTestDB 设置邮件任务测试数据库
func setupEmailJobsTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.EmailJobs{},
	)
	db.SetTestDB(database)
	return database
}

// setupEmailJobsRepository 创建测试用的邮件任务仓库实例
func setupEmailJobsRepository(t *testing.T) EmailJobsRepository {
	setupEmailJobsTestDB(t)
	return NewEmailJobsRepository()
}

// TestEmailJobsRepository_Create 测试创建邮件任务
func TestEmailJobsRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailJobsRepository(t)

	tests := []struct {
		name    string
		job     *model.EmailJobs
		wantErr bool
	}{
		{
			name: "create email job success",
			job: &model.EmailJobs{
				Subject:      "Test Email Subject",
				SendTotal:    0,
				EmailTotal:   100,
				SuccessTotal: 0,
				FailTotal:    0,
				ReadTotal:    0,
			},
			wantErr: false,
		},
		{
			name: "create email job with large total",
			job: &model.EmailJobs{
				Subject:      "Bulk Email Campaign",
				SendTotal:    0,
				EmailTotal:   10000,
				SuccessTotal: 0,
				FailTotal:    0,
				ReadTotal:    0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.job)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if tt.job.ID == uuid.Nil {
					t.Error("Expected job ID to be set after creation")
				}
			}
		})
	}
}

// TestEmailJobsRepository_GetByID 测试根据 ID 获取邮件任务
func TestEmailJobsRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailJobsRepository(t)

	job := &model.EmailJobs{
		Subject:      "GetByID Test",
		EmailTotal:   50,
		SuccessTotal: 10,
		FailTotal:    2,
		ReadTotal:    5,
	}
	repo.Create(ctx, job)

	tests := []struct {
		name    string
		id      uuid.UUID
		wantErr bool
	}{
		{
			name:    "get existing job",
			id:      job.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing job",
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
				if result.Subject != "GetByID Test" {
					t.Errorf("Expected subject 'GetByID Test', got '%s'", result.Subject)
				}
			}
		})
	}
}

// TestEmailJobsRepository_List 测试获取邮件任务列表
func TestEmailJobsRepository_List(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailJobsRepository(t)

	for i := 1; i <= 15; i++ {
		repo.Create(ctx, &model.EmailJobs{
			Subject:      "Email Job " + string(rune('A'+i-1)),
			EmailTotal:   int64(i * 10),
			SuccessTotal: int64(i * 8),
			FailTotal:    int64(i * 2),
			ReadTotal:    int64(i * 5),
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
			name:      "get all jobs",
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

// TestEmailJobsRepository_Update 测试更新邮件任务
func TestEmailJobsRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailJobsRepository(t)

	job := &model.EmailJobs{
		Subject:      "Original Subject",
		EmailTotal:   100,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}
	repo.Create(ctx, job)

	job.SendTotal = 50
	job.SuccessTotal = 45
	job.FailTotal = 5
	job.ReadTotal = 20

	err := repo.Update(ctx, job)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, job.ID)
	if updated.SendTotal != 50 {
		t.Errorf("Expected SendTotal 50, got %d", updated.SendTotal)
	}
	if updated.SuccessTotal != 45 {
		t.Errorf("Expected SuccessTotal 45, got %d", updated.SuccessTotal)
	}
	if updated.FailTotal != 5 {
		t.Errorf("Expected FailTotal 5, got %d", updated.FailTotal)
	}
	if updated.ReadTotal != 20 {
		t.Errorf("Expected ReadTotal 20, got %d", updated.ReadTotal)
	}
}

// TestEmailJobsRepository_Delete 测试删除邮件任务
func TestEmailJobsRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailJobsRepository(t)

	job := &model.EmailJobs{
		Subject:      "To Delete",
		EmailTotal:   10,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}
	repo.Create(ctx, job)

	err := repo.Delete(ctx, job.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, job.ID)
	if err == nil {
		t.Error("Expected job to be deleted")
	}
}

// TestEmailJobsRepository_GetByID_NotFound 测试获取不存在的任务
func TestEmailJobsRepository_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailJobsRepository(t)

	_, err := repo.GetByID(ctx, uuid.New())
	if err == nil {
		t.Error("Expected error when getting non-existing job")
	}
}

// TestEmailJobsRepository_List_EmptyResult 测试获取空结果
func TestEmailJobsRepository_List_EmptyResult(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailJobsRepository(t)

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

// TestEmailJobsRepository_Create_WithStats 测试创建带统计数据的任务
func TestEmailJobsRepository_Create_WithStats(t *testing.T) {
	ctx := context.Background()
	repo := setupEmailJobsRepository(t)

	job := &model.EmailJobs{
		Subject:      "Campaign with stats",
		EmailTotal:   1000,
		SendTotal:    1000,
		SuccessTotal: 950,
		FailTotal:    50,
		ReadTotal:    600,
	}

	err := repo.Create(ctx, job)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	result, _ := repo.GetByID(ctx, job.ID)
	if result.EmailTotal != 1000 {
		t.Errorf("Expected EmailTotal 1000, got %d", result.EmailTotal)
	}
	if result.SuccessTotal != 950 {
		t.Errorf("Expected SuccessTotal 950, got %d", result.SuccessTotal)
	}
	if result.FailTotal != 50 {
		t.Errorf("Expected FailTotal 50, got %d", result.FailTotal)
	}
	if result.ReadTotal != 600 {
		t.Errorf("Expected ReadTotal 600, got %d", result.ReadTotal)
	}
}

