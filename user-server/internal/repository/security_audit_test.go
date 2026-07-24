package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupSecurityAuditTestDB 设置测试数据库
func setupSecurityAuditTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SecurityAuditResult{},
	)
	db.SetTestDB(database)
	return database
}

// TestSecurityAuditRepository_Create 创建审计记录
func TestSecurityAuditRepository_Create(t *testing.T) {
	database := setupSecurityAuditTestDB(t)
	repo := NewSecurityAuditRepository()
	ctx := context.Background()

	now := time.Now()
	record := &model.SecurityAuditResult{
		AuditName: "test_audit",
		Status:    "running",
		StartedAt: &now,
	}

	err := repo.Create(ctx, record)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if record.ID == 0 {
		t.Error("expected ID to be set after Create")
	}

	// 验证 DB
	var dbRecord model.SecurityAuditResult
	database.First(&dbRecord, record.ID)
	if dbRecord.AuditName != "test_audit" {
		t.Errorf("expected AuditName 'test_audit', got '%s'", dbRecord.AuditName)
	}
	if dbRecord.Status != "running" {
		t.Errorf("expected Status 'running', got '%s'", dbRecord.Status)
	}
}

// TestSecurityAuditRepository_UpdateResults 更新审计结果
func TestSecurityAuditRepository_UpdateResults(t *testing.T) {
	database := setupSecurityAuditTestDB(t)
	repo := NewSecurityAuditRepository()
	ctx := context.Background()

	now := time.Now()
	record := &model.SecurityAuditResult{
		AuditName: "test_audit",
		Status:    "running",
		StartedAt: &now,
	}
	repo.Create(ctx, record)

	updates := map[string]any{
		"status":        "completed",
		"total_checks":  10,
		"passed_count":  8,
		"failed_count":  1,
		"warning_count": 1,
		"score":         90,
	}

	err := repo.UpdateResults(context.Background(), record.ID, updates)
	if err != nil {
		t.Fatalf("UpdateResults failed: %v", err)
	}

	var dbRecord model.SecurityAuditResult
	database.First(&dbRecord, record.ID)
	if dbRecord.Status != "completed" {
		t.Errorf("expected Status 'completed', got '%s'", dbRecord.Status)
	}
	if dbRecord.TotalChecks != 10 {
		t.Errorf("expected TotalChecks 10, got %d", dbRecord.TotalChecks)
	}
	if dbRecord.Score != 90 {
		t.Errorf("expected Score 90, got %f", dbRecord.Score)
	}
}

// TestSecurityAuditRepository_GetByID 按 ID 查询
func TestSecurityAuditRepository_GetByID(t *testing.T) {
	setupSecurityAuditTestDB(t)
	repo := NewSecurityAuditRepository()
	ctx := context.Background()

	now := time.Now()
	record := &model.SecurityAuditResult{
		AuditName: "get_test",
		Status:    "completed",
		StartedAt: &now,
	}
	repo.Create(ctx, record)

	result, err := repo.GetByID(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if result.AuditName != "get_test" {
		t.Errorf("expected AuditName 'get_test', got '%s'", result.AuditName)
	}
}

// TestSecurityAuditRepository_GetByID_NotFound 不存在
func TestSecurityAuditRepository_GetByID_NotFound(t *testing.T) {
	setupSecurityAuditTestDB(t)
	repo := NewSecurityAuditRepository()
	ctx := context.Background()

	_, err := repo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
}

// TestSecurityAuditRepository_List 列表+分页
func TestSecurityAuditRepository_List(t *testing.T) {
	setupSecurityAuditTestDB(t)
	repo := NewSecurityAuditRepository()
	ctx := context.Background()

	// 创建 5 条记录
	for i := 0; i < 5; i++ {
		now := time.Now()
		repo.Create(ctx, &model.SecurityAuditResult{
			AuditName: fmt.Sprintf("audit_%d", i),
			Status:    "completed",
			StartedAt: &now,
		})
	}

	// 第一页 3 条
	list, total, err := repo.List(context.Background(), 1, 3)
	if err != nil {
		t.Fatalf("List page 1 failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 items on page 1, got %d", len(list))
	}

	// 第二页 2 条
	list2, _, err := repo.List(context.Background(), 2, 3)
	if err != nil {
		t.Fatalf("List page 2 failed: %v", err)
	}
	if len(list2) != 2 {
		t.Errorf("expected 2 items on page 2, got %d", len(list2))
	}
}

// TestSecurityAuditRepository_List_Empty 空列表
func TestSecurityAuditRepository_List_Empty(t *testing.T) {
	setupSecurityAuditTestDB(t)
	repo := NewSecurityAuditRepository()

	list, total, err := repo.List(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("List empty failed: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 items, got %d", len(list))
	}
}
