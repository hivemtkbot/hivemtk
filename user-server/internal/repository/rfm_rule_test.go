package repository

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"
	"time"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupRFMRuleTestDB 设置 RFM 规则测试数据库
func setupRFMRuleTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.RFMRule{},
		&model.UserRFM{},
	)
	db.SetTestDB(database)
	return database
}

// setupRFMRuleRepositories 创建测试用的仓库实例
func setupRFMRuleRepositories(t *testing.T) (*gorm.DB, *RFMRuleRepository, *UserRFMRepository) {
	database := setupRFMRuleTestDB(t)

	ruleRepo := NewRFMRuleRepository()
	userRepo := NewUserRFMRepository()

	return database, ruleRepo, userRepo
}

// TestRFMRuleRepository_Create 测试创建规则
func TestRFMRuleRepository_Create(t *testing.T) {
	_, ruleRepo, _ := setupRFMRuleRepositories(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		rule    *model.RFMRule
		wantErr bool
	}{
		{
			name: "create rule success",
			rule: &model.RFMRule{
				Name:     "Test RFM Rule",
				RDays1:   7,
				RDays2:   14,
				RDays3:   30,
				RDays4:   60,
				RDays5:   90,
				FCount1:  1,
				FCount2:  3,
				FCount3:  5,
				FCount4:  10,
				FCount5:  20,
				MAmount1: 10000, // 100 元 = 10000 分
				MAmount2: 50000, // 500 元 = 50000 分
				MAmount3: 100000,
				MAmount4: 500000,
				MAmount5: 1000000,
				IsActive: true,
			},
			wantErr: false,
		},
		{
			name: "create rule with minimal fields",
			rule: &model.RFMRule{
				Name: "Minimal Rule",
			},
			wantErr: false,
		},
		{
			name: "create inactive rule",
			rule: &model.RFMRule{
				Name:     "Inactive Rule",
				IsActive: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ruleRepo.Create(ctx, tt.rule)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.rule.ID == 0 {
				t.Error("Expected rule ID to be set after creation")
			}
		})
	}
}

// TestRFMRuleRepository_GetByID 测试根据 ID 获取规则
func TestRFMRuleRepository_GetByID(t *testing.T) {
	_, ruleRepo, _ := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建测试数据
	rule := &model.RFMRule{
		Name:     "GetByID Rule",
		IsActive: true,
	}
	ruleRepo.Create(ctx, rule)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing rule",
			id:      rule.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing rule",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ruleRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "GetByID Rule" {
					t.Errorf("Expected name 'GetByID Rule', got '%s'", result.Name)
				}
			}
		})
	}
}

func TestRFMRuleRepository_GetActiveRule(t *testing.T) {
	_, ruleRepo, _ := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建测试数据
	ruleRepo.Create(ctx, &model.RFMRule{
		Name:     "Active Rule",
		IsActive: true,
	})
	ruleRepo.Create(ctx, &model.RFMRule{
		Name:     "Inactive Rule",
		IsActive: false,
	})

	// 私域部署下,GetActiveRule 直接返回 IsActive=true 的规则
	result, err := ruleRepo.GetActiveRule(context.Background())
	if err != nil {
		t.Errorf("GetActiveRule() unexpected error = %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Name != "Active Rule" {
		t.Errorf("Expected name 'Active Rule', got '%s'", result.Name)
	}
}

// TestRFMRuleRepository_Update 测试更新规则
func TestRFMRuleRepository_Update(t *testing.T) {
	_, ruleRepo, _ := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建测试数据
	rule := &model.RFMRule{
		Name:     "Original Name",
		IsActive: true,
	}
	ruleRepo.Create(ctx, rule)

	rule.Name = "Updated Name"
	rule.IsActive = false

	err := ruleRepo.Update(ctx, rule)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := ruleRepo.GetByID(ctx, rule.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.IsActive {
		t.Error("Expected IsActive to be false")
	}
}

// TestRFMRuleRepository_Delete 测试删除规则
func TestRFMRuleRepository_Delete(t *testing.T) {
	_, ruleRepo, _ := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建测试数据
	rule := &model.RFMRule{
		Name:     "Delete Rule",
		IsActive: true,
	}
	ruleRepo.Create(ctx, rule)

	err := ruleRepo.Delete(ctx, rule.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = ruleRepo.GetByID(ctx, rule.ID)
	if err == nil {
		t.Error("Expected rule to be deleted")
	}
}

// TestUserRFMRepository_Create 测试创建用户 RFM 记录
func TestUserRFMRepository_Create(t *testing.T) {
	_, _, userRepo := setupRFMRuleRepositories(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		rfm     *model.UserRFM
		wantErr bool
	}{
		{
			name: "create user rfm success",
			rfm: &model.UserRFM{
				UserID:           1,
				RScore:           5,
				FScore:           4,
				MScore:           5,
				TotalScore:       14,
				Layer:            "important_value",
				TransactionCount: 20,
				TotalAmount:      1000000, // 10000 元 = 1000000 分
				AvgAmount:        50000,   // 500 元 = 50000 分
			},
			wantErr: false,
		},
		{
			name: "create user rfm with minimal fields",
			rfm: &model.UserRFM{
				UserID: 2,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := userRepo.Create(ctx, tt.rfm)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.rfm.ID == 0 {
				t.Error("Expected RFM ID to be set after creation")
			}
		})
	}
}

// TestUserRFMRepository_GetByUserID 测试根据用户 ID 获取 RFM
func TestUserRFMRepository_GetByUserID(t *testing.T) {
	_, _, userRepo := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建测试数据
	rfm := &model.UserRFM{
		UserID:     100,
		RScore:     5,
		FScore:     4,
		MScore:     5,
		TotalScore: 14,
		Layer:      "important_value",
	}
	userRepo.Create(ctx, rfm)

	tests := []struct {
		name      string
		userID    uint
		wantErr   bool
		wantLayer string
	}{
		{
			name:      "get existing user rfm",
			userID:    100,
			wantErr:   false,
			wantLayer: "important_value",
		},
		{
			name:    "get non-existing user rfm",
			userID:  999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := userRepo.GetByUserID(context.Background(), tt.userID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByUserID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && result.Layer != tt.wantLayer {
				t.Errorf("Expected layer '%s', got '%s'", tt.wantLayer, result.Layer)
			}
		})
	}
}

// TestUserRFMRepository_GetByLayer 测试根据分层获取用户列表
func TestUserRFMRepository_GetByLayer(t *testing.T) {
	_, _, userRepo := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建测试数据
	layers := []string{"important_value", "important_value", "general_value", "general_keep", "important_value"}
	for i, layer := range layers {
		userRepo.Create(ctx, &model.UserRFM{
			UserID:     uint(i + 1),
			RScore:     5,
			FScore:     5,
			MScore:     5,
			TotalScore: 15,
			Layer:      layer,
		})
	}

	tests := []struct {
		name       string
		merchantID string
		layer      string
		page       int
		pageSize   int
		wantCount  int
		wantTotal  int64
	}{
		{
			name: "get important_value users",

			layer:     "important_value",
			page:      1,
			pageSize:  10,
			wantCount: 3,
			wantTotal: 3,
		},
		{
			name: "get general_value users",

			layer:     "general_value",
			page:      1,
			pageSize:  10,
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name: "get non-existing layer",

			layer:     "non_existing",
			page:      1,
			pageSize:  10,
			wantCount: 0,
			wantTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := userRepo.GetByLayer(context.Background(), tt.layer, tt.page, tt.pageSize)

			if err != nil {
				t.Errorf("GetByLayer() error = %v", err)
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

// TestUserRFMRepository_GetLayerCount 测试获取各分层用户数量
func TestUserRFMRepository_GetLayerCount(t *testing.T) {
	_, _, userRepo := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建测试数据
	layers := map[string]int{
		"important_value": 3,
		"important_keep":  2,
		"general_value":   1,
		"general_keep":    1,
	}

	for layer, count := range layers {
		for i := 0; i < count; i++ {
			userRepo.Create(ctx, &model.UserRFM{
				UserID:     uint(len(layers)*100 + layerCount(layer, i)),
				RScore:     5,
				FScore:     5,
				MScore:     5,
				TotalScore: 15,
				Layer:      layer,
			})
		}
	}

	counts, err := userRepo.GetLayerCount(context.Background())
	if err != nil {
		t.Errorf("GetLayerCount() error = %v", err)
	}

	for layer, expectedCount := range layers {
		if counts[layer] != int64(expectedCount) {
			t.Errorf("Expected layer %s count %d, got %d", layer, expectedCount, counts[layer])
		}
	}
}

// layerCount is a helper to generate unique user IDs
func layerCount(layer string, index int) int {
	return len(layer)*10 + index
}

// TestUserRFMRepository_DeleteByUserID 测试删除用户 RFM 记录
func TestUserRFMRepository_DeleteByUserID(t *testing.T) {
	_, _, userRepo := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建测试数据
	rfm := &model.UserRFM{
		UserID:     100,
		RScore:     5,
		FScore:     4,
		MScore:     5,
		TotalScore: 14,
		Layer:      "important_value",
	}
	userRepo.Create(ctx, rfm)

	err := userRepo.DeleteByUserID(context.Background(), 100)
	if err != nil {
		t.Errorf("DeleteByUserID() error = %v", err)
	}

	_, err = userRepo.GetByUserID(context.Background(), 100)
	if err == nil {
		t.Error("Expected RFM record to be deleted")
	}
}

// TestUserRFMRepository_BatchUpsert 测试批量 upsert 用户 RFM 记录
func TestUserRFMRepository_BatchUpsert(t *testing.T) {
	_, _, userRepo := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 先创建一条记录
	existingRFM := &model.UserRFM{
		UserID:     1,
		RScore:     3,
		FScore:     3,
		MScore:     3,
		TotalScore: 9,
		Layer:      "general",
	}
	userRepo.Create(ctx, existingRFM)

	// 准备批量 upsert 数据
	rfms := []*model.UserRFM{
		{
			UserID:     1, // 已存在的用户
			RScore:     5,
			FScore:     5,
			MScore:     5,
			TotalScore: 15,
			Layer:      "important_value",
		},
		{
			UserID:     2, // 新用户
			RScore:     4,
			FScore:     4,
			MScore:     4,
			TotalScore: 12,
			Layer:      "value",
		},
	}

	err := userRepo.BatchUpsert(context.Background(), rfms)
	if err != nil {
		t.Errorf("BatchUpsert() error = %v", err)
	}

	// 验证已存在的用户被更新
	updated, _ := userRepo.GetByUserID(context.Background(), 1)
	if updated.TotalScore != 15 {
		t.Errorf("Expected existing user TotalScore 15, got %d", updated.TotalScore)
	}
	if updated.Layer != "important_value" {
		t.Errorf("Expected existing user layer 'important_value', got '%s'", updated.Layer)
	}

	// 验证新用户被创建
	newUser, _ := userRepo.GetByUserID(context.Background(), 2)
	if newUser.TotalScore != 12 {
		t.Errorf("Expected new user TotalScore 12, got %d", newUser.TotalScore)
	}
}

// TestUserRFMRepository_GetNeedUpdateUsers 测试获取需要更新的 RFM 用户
func TestUserRFMRepository_GetNeedUpdateUsers(t *testing.T) {
	database, _, userRepo := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建一个需要更新的用户（100 天前的更新时间）
	oldRFM := &model.UserRFM{
		UserID:     1,
		RScore:     3,
		FScore:     3,
		MScore:     3,
		TotalScore: 9,
		Layer:      "general",
	}
	userRepo.Create(ctx, oldRFM)
	// Use raw SQL to set the old UpdatedAt (GORM autoUpdateTime overrides it)
	database.Exec("UPDATE user_rfms SET updated_at = ? WHERE id = ?", time.Now().AddDate(0, 0, -100), oldRFM.ID)

	// 创建一个需要更新的用户（400 天前的更新时间）
	veryOldRFM := &model.UserRFM{
		UserID:     2,
		RScore:     4,
		FScore:     4,
		MScore:     4,
		TotalScore: 12,
		Layer:      "value",
	}
	userRepo.Create(ctx, veryOldRFM)
	// Use raw SQL to set the very old UpdatedAt
	database.Exec("UPDATE user_rfms SET updated_at = ? WHERE id = ?", time.Now().AddDate(0, 0, -400), veryOldRFM.ID)

	// 创建一个不需要更新的用户（新的更新时间）
	newRFM := &model.UserRFM{
		UserID:     3,
		RScore:     5,
		FScore:     5,
		MScore:     5,
		TotalScore: 15,
		Layer:      "important_value",
	}
	userRepo.Create(ctx, newRFM)

	tests := []struct {
		name       string
		merchantID string
		days       int
		wantCount  int
	}{
		{
			name: "get users need update (30 days)",

			days:      30,
			wantCount: 2, // 100 days and 400 days records
		},
		{
			name: "get users need update (365 days)",

			days:      365,
			wantCount: 1, // only 400 days record
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := userRepo.GetNeedUpdateUsers(context.Background(), tt.days)

			if err != nil {
				t.Errorf("GetNeedUpdateUsers() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d users need update, got %d", tt.wantCount, len(results))
			}
		})
	}
}

// TestUserRFMRepository_GetNeedUpdateUsers_EmptyResult 测试获取空结果
func TestUserRFMRepository_GetNeedUpdateUsers_EmptyResult(t *testing.T) {
	_, _, userRepo := setupRFMRuleRepositories(t)
	ctx := context.Background()

	// 创建所有用户都是最近更新的
	for i := 1; i <= 3; i++ {
		userRepo.Create(ctx, &model.UserRFM{
			UserID:     uint(i),
			RScore:     5,
			FScore:     5,
			MScore:     5,
			TotalScore: 15,
			Layer:      "vip",
			UpdatedAt:  time.Now(),
		})
	}

	results, err := userRepo.GetNeedUpdateUsers(context.Background(), 30)
	if err != nil {
		t.Errorf("GetNeedUpdateUsers() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 users need update, got %d", len(results))
	}
}
