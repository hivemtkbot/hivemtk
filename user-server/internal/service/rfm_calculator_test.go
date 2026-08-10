package service

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupRFMCalculatorServiceTestDB 设置测试数据库
func setupRFMCalculatorServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.RFMRule{},
		&model.UserRFM{},
		&model.Order{},
		&model.User{},
		&model.Clue{},
	)
	db.SetTestDB(database)
	return database
}

// setupRFMCalculatorService 设置测试服务
func setupRFMCalculatorService(t *testing.T) *RFMCalculatorService {
	setupRFMCalculatorServiceTestDB(t)
	return NewRFMCalculatorService()
}

// TestNewRFMCalculatorService 测试创建服务实例
func TestNewRFMCalculatorService(t *testing.T) {
	service := NewRFMCalculatorService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.rfmRuleRepo == nil {
		t.Error("Expected rfmRuleRepo to be initialized")
	}
	if service.userRfmRepo == nil {
		t.Error("Expected userRfmRepo to be initialized")
	}
}

// TestRFMCalculatorService_calcRScore 测试计算 R 得分
func TestRFMCalculatorService_calcRScore(t *testing.T) {
	service := setupRFMCalculatorService(t)
	now := time.Now()

	// 测试默认规则
	rule := &model.RFMRule{
		RDays1: 7, RDays2: 14, RDays3: 30, RDays4: 60, RDays5: 90,
	}

	tests := []struct {
		name     string
		lastTx   *time.Time
		expected int
	}{
		{"无交易记录", nil, 1},
		{"最近 7 天内", &now, 5},
		{"7-14 天", &[]time.Time{now.Add(-10 * 24 * time.Hour)}[0], 4},
		{"14-30 天", &[]time.Time{now.Add(-20 * 24 * time.Hour)}[0], 3},
		{"30-60 天", &[]time.Time{now.Add(-45 * 24 * time.Hour)}[0], 2},
		{"60-90 天", &[]time.Time{now.Add(-75 * 24 * time.Hour)}[0], 1},
		{"超过 90 天", &[]time.Time{now.Add(-100 * 24 * time.Hour)}[0], 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := service.calcRScore(context.Background(), tt.lastTx, rule)
			if score != tt.expected {
				t.Errorf("Expected R score %d, got %d", tt.expected, score)
			}
		})
	}
}

// TestRFMCalculatorService_calcFScore 测试计算 F 得分
func TestRFMCalculatorService_calcFScore(t *testing.T) {
	service := setupRFMCalculatorService(t)

	rule := &model.RFMRule{
		FCount1: 1, FCount2: 3, FCount3: 5, FCount4: 10, FCount5: 20,
	}

	tests := []struct {
		name     string
		count    int
		expected int
	}{
		{"0 次", 0, 1},
		{"1 次", 1, 1},
		{"3 次", 3, 2},
		{"5 次", 5, 3},
		{"10 次", 10, 4},
		{"20 次", 20, 5},
		{"超过 20 次", 25, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := service.calcFScore(context.Background(), tt.count, rule)
			if score != tt.expected {
				t.Errorf("Expected F score %d, got %d", tt.expected, score)
			}
		})
	}
}

// TestRFMCalculatorService_calcMScore 测试计算 M 得分
// 金额单位：分（100 元 = 10000 分）
func TestRFMCalculatorService_calcMScore(t *testing.T) {
	service := setupRFMCalculatorService(t)

	rule := &model.RFMRule{
		MAmount1: 10000, MAmount2: 50000, MAmount3: 100000, MAmount4: 500000, MAmount5: 1000000,
	}

	tests := []struct {
		name     string
		amount   int64
		expected int
	}{
		{"0 元", 0, 1},
		{"100 元", 10000, 1},
		{"500 元", 50000, 2},
		{"1000 元", 100000, 3},
		{"5000 元", 500000, 4},
		{"10000 元", 1000000, 5},
		{"超过 10000 元", 1500000, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := service.calcMScore(context.Background(), tt.amount, rule)
			if score != tt.expected {
				t.Errorf("Expected M score %d, got %d", tt.expected, score)
			}
		})
	}
}

// TestRFMCalculatorService_determineLayer 测试用户分层
func TestRFMCalculatorService_determineLayer(t *testing.T) {
	service := setupRFMCalculatorService(t)
	now := time.Now()

	tests := []struct {
		name     string
		rScore   int
		fScore   int
		mScore   int
		lastTx   *time.Time
		expected model.RFMLayer
	}{
		{"重要价值用户 (全高)", 5, 5, 5, &now, model.RFMLayerImportantValue},
		{"重要保持用户 (R 低 FM 高)", 2, 5, 5, &now, model.RFMLayerImportantKeep},
		{"重要发展用户 (F 低 RM 高)", 5, 2, 5, &now, model.RFMLayerImportantDevelop},
		{"重要挽留用户 (RF 低 M 高)", 2, 2, 5, &now, model.RFMLayerImportantStay},
		{"一般价值用户 (M 低 RF 高)", 5, 5, 2, &now, model.RFMLayerGeneralValue},
		{"一般保持用户 (RM 低 F 高)", 2, 5, 2, &now, model.RFMLayerGeneralKeep},
		{"一般发展用户 (FM 低 R 高)", 5, 2, 2, &now, model.RFMLayerGeneralDevelop},
		{"一般挽留用户 (全低)", 2, 2, 2, &now, model.RFMLayerGeneralStay},
		{"新用户", 5, 1, 1, &now, model.RFMLayerNew},
		{"沉睡用户 (60-90 天)", 3, 3, 3, &[]time.Time{now.Add(-70 * 24 * time.Hour)}[0], model.RFMLayerSleep},
		{"流失用户 (超过 90 天)", 3, 3, 3, &[]time.Time{now.Add(-100 * 24 * time.Hour)}[0], model.RFMLayerLost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer := service.determineLayer(context.Background(), tt.rScore, tt.fScore, tt.mScore, tt.lastTx)
			if layer != tt.expected {
				t.Errorf("Expected layer %s, got %s", tt.expected, layer)
			}
		})
	}
}

// TestRFMCalculatorService_CalculateRFM 测试计算单个用户 RFM
func TestRFMCalculatorService_CalculateRFM(t *testing.T) {
	service := setupRFMCalculatorService(t)

	rule := &model.RFMRule{
		RDays1: 7, RDays2: 14, RDays3: 30, RDays4: 60, RDays5: 90,
		FCount1: 1, FCount2: 3, FCount3: 5, FCount4: 10, FCount5: 20,
		MAmount1: 10000, MAmount2: 50000, MAmount3: 100000, MAmount4: 500000, MAmount5: 1000000,
	}

	// 由于 getUserStats 返回空数据，测试主要验证结构
	rfm, err := service.CalculateRFM(context.Background(), 1, rule)
	if err != nil {
		t.Fatalf("CalculateRFM failed: %v", err)
	}

	if rfm == nil {
		t.Fatal("Expected RFM to be returned")
	}

	if rfm.UserID != 1 {
		t.Errorf("Expected user_id 1, got %d", rfm.UserID)
	}
	// 由于没有交易数据，R/F/M 得分应该为 1
	if rfm.RScore != 1 {
		t.Errorf("Expected R score 1, got %d", rfm.RScore)
	}
	if rfm.FScore != 1 {
		t.Errorf("Expected F score 1, got %d", rfm.FScore)
	}
	if rfm.MScore != 1 {
		t.Errorf("Expected M score 1, got %d", rfm.MScore)
	}
}

// TestRFMCalculatorService_getDefaultRule 测试获取默认规则
// 金额单位：分（100 元 = 10000 分）
func TestRFMCalculatorService_getDefaultRule(t *testing.T) {
	service := setupRFMCalculatorService(t)

	rule := service.getDefaultRule(context.Background())
	if rule == nil {
		t.Fatal("Expected default rule to be returned")
	}
	if rule.RDays1 != 7 {
		t.Errorf("Expected RDays1 7, got %d", rule.RDays1)
	}
	if rule.RDays2 != 14 {
		t.Errorf("Expected RDays2 14, got %d", rule.RDays2)
	}
	if rule.RDays3 != 30 {
		t.Errorf("Expected RDays3 30, got %d", rule.RDays3)
	}
	if rule.FCount1 != 1 {
		t.Errorf("Expected FCount1 1, got %d", rule.FCount1)
	}
	if rule.MAmount1 != 10000 {
		t.Errorf("Expected MAmount1 10000 分, got %d", rule.MAmount1)
	}
	if rule.MAmount5 != 1000000 {
		t.Errorf("Expected MAmount5 1000000 分, got %d", rule.MAmount5)
	}
}

// TestRFMCalculatorService_CalculateAllUsers 测试计算所有用户 RFM
func TestRFMCalculatorService_CalculateAllUsers(t *testing.T) {
	service := setupRFMCalculatorService(t)

	// 创建规则
	rule := &model.RFMRule{
		Name:   "Test Rule",
		RDays1: 7, RDays2: 14, RDays3: 30, RDays4: 60, RDays5: 90,
		FCount1: 1, FCount2: 3, FCount3: 5, FCount4: 10, FCount5: 20,
		MAmount1: 10000, MAmount2: 50000, MAmount3: 100000, MAmount4: 500000, MAmount5: 1000000,
		IsActive: true,
	}
	db.GetDB().Create(rule)

	// 由于 getAllUserIDs 返回空列表，应该返回 0
	count, err := service.CalculateAllUsers(context.Background())
	if err != nil {
		t.Fatalf("CalculateAllUsers failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 users updated, got %d", count)
	}
}

// TestRFMCalculatorService_SaveRFMRule 测试保存规则
func TestRFMCalculatorService_SaveRFMRule(t *testing.T) {
	service := setupRFMCalculatorService(t)

	req := &SaveRFMRuleRequest{
		Name:   "Test Rule",
		RDays1: 7, RDays2: 14, RDays3: 30, RDays4: 60, RDays5: 90,
		FCount1: 1, FCount2: 3, FCount3: 5, FCount4: 10, FCount5: 20,
		MAmount1: 10000, MAmount2: 50000, MAmount3: 100000, MAmount4: 500000, MAmount5: 1000000,
		IsActive: true,
	}

	rule, err := service.SaveRFMRule(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveRFMRule failed: %v", err)
	}

	if rule == nil {
		t.Fatal("Expected rule to be returned")
	}
	if rule.Name != "Test Rule" {
		t.Errorf("Expected name 'Test Rule', got '%s'", rule.Name)
	}
}

// TestRFMCalculatorService_SaveRFMRule_DefaultValues 测试保存规则使用默认值
func TestRFMCalculatorService_SaveRFMRule_DefaultValues(t *testing.T) {
	service := setupRFMCalculatorService(t)

	req := &SaveRFMRuleRequest{
		Name: "Test Rule",
		// RDays1 为 0，应该使用默认值 7
	}

	rule, err := service.SaveRFMRule(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveRFMRule failed: %v", err)
	}

	if rule.RDays1 != 7 {
		t.Errorf("Expected RDays1 to default to 7, got %d", rule.RDays1)
	}
}

// TestRFMCalculatorService_GetRFMRule 测试获取规则
func TestRFMCalculatorService_GetRFMRule(t *testing.T) {
	service := setupRFMCalculatorService(t)

	// 创建规则
	rule := &model.RFMRule{
		Name:     "Test Rule",
		IsActive: true,
	}
	db.GetDB().Create(rule)

	// 获取规则
	result, err := service.GetRFMRule(context.Background())
	if err != nil {
		t.Fatalf("GetRFMRule failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected rule to be returned")
	}
	if result.Name != "Test Rule" {
		t.Errorf("Expected name 'Test Rule', got '%s'", result.Name)
	}
}

// TestRFMCalculatorService_UpdateRFMRule 测试更新规则
func TestRFMCalculatorService_UpdateRFMRule(t *testing.T) {
	service := setupRFMCalculatorService(t)

	// 创建规则
	rule := &model.RFMRule{
		Name: "Old Name",
	}
	db.GetDB().Create(rule)

	// 更新规则
	req := &SaveRFMRuleRequest{
		Name:   "New Name",
		RDays1: 10,
	}

	updated, err := service.UpdateRFMRule(context.Background(), rule.ID, req)
	if err != nil {
		t.Fatalf("UpdateRFMRule failed: %v", err)
	}

	if updated == nil {
		t.Fatal("Expected updated rule to be returned")
	}
	if updated.Name != "New Name" {
		t.Errorf("Expected name 'New Name', got '%s'", updated.Name)
	}
	if updated.RDays1 != 10 {
		t.Errorf("Expected RDays1 10, got %d", updated.RDays1)
	}
}

// TestRFMCalculatorService_UpdateRFMRule_NotFound 测试更新不存在的规则
func TestRFMCalculatorService_UpdateRFMRule_NotFound(t *testing.T) {
	service := setupRFMCalculatorService(t)

	req := &SaveRFMRuleRequest{
		Name: "Test",
	}

	_, err := service.UpdateRFMRule(context.Background(), 999, req)
	if err == nil {
		t.Error("Expected error for non-existent rule")
	}
}

// TestRFMCalculatorService_UpdateRFMRule_NoPermission 测试无权限更新
// 独立部署模式下无权限校验，此测试验证更新一个不存在的规则会返回错误
func TestRFMCalculatorService_UpdateRFMRule_NoPermission(t *testing.T) {
	service := setupRFMCalculatorService(t)

	// 独立部署模式（无多租户）：无权限校验，所有合法操作都允许
	// 此测试场景：使用不存在的规则 ID，验证返回规则不存在错误
	req := &SaveRFMRuleRequest{
		Name: "Updated",
	}

	_, err := service.UpdateRFMRule(context.Background(), 99999, req)
	if err == nil {
		t.Error("Expected error for non-existent rule")
	}
}

// TestRFMCalculatorService_GetRFMStats 测试获取统计
func TestRFMCalculatorService_GetRFMStats(t *testing.T) {
	service := setupRFMCalculatorService(t)

	stats, err := service.GetRFMStats(context.Background())
	if err != nil {
		t.Fatalf("GetRFMStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats to be returned")
	}
	if stats["total_users"] != 0 {
		t.Errorf("Expected 0 total users, got %v", stats["total_users"])
	}
}

// TestRFMCalculatorService_GetUsersByLayer 测试根据分层获取用户
func TestRFMCalculatorService_GetUsersByLayer(t *testing.T) {
	service := setupRFMCalculatorService(t)

	// 创建测试数据
	rfm := &model.UserRFM{
		UserID: 1,
		Layer:  "important_value",
		RScore: 5, FScore: 5, MScore: 5,
	}
	db.GetDB().Create(rfm)

	users, total, err := service.GetUsersByLayer(context.Background(), "important_value", 1, 10)
	if err != nil {
		t.Fatalf("GetUsersByLayer failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}
	if len(users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(users))
	}
	if users[0].LayerDesc == "" {
		t.Error("Expected layer description to be populated")
	}
}

// TestRFMCalculatorService_GetRFMList 测试获取用户列表
func TestRFMCalculatorService_GetRFMList(t *testing.T) {
	service := setupRFMCalculatorService(t)

	// 创建测试数据
	rfm := &model.UserRFM{
		UserID: 1,
		Layer:  "important_value",
	}
	db.GetDB().Create(rfm)

	users, total, err := service.GetRFMList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetRFMList failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}
	if len(users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(users))
	}
}

// TestRFMCalculatorService_GetUserRFM 测试获取单个用户 RFM
func TestRFMCalculatorService_GetUserRFM(t *testing.T) {
	service := setupRFMCalculatorService(t)

	// 创建测试数据
	rfm := &model.UserRFM{
		UserID: 1,
		Layer:  "important_value",
		RScore: 5, FScore: 5, MScore: 5,
		TotalScore: 15,
	}
	db.GetDB().Create(rfm)

	result, err := service.GetUserRFM(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUserRFM failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected RFM to be returned")
	}
	if result.UserRFM.UserID != 1 {
		t.Errorf("Expected user_id 1, got %d", result.UserRFM.UserID)
	}
}

// TestRFMCalculatorService_GetUserRFM_NotFound 测试获取不存在的用户
func TestRFMCalculatorService_GetUserRFM_NotFound(t *testing.T) {
	service := setupRFMCalculatorService(t)

	_, err := service.GetUserRFM(context.Background(), 999)
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}

// TestRFMCalculatorService_enrichUserData 测试丰富用户数据
func TestRFMCalculatorService_enrichUserData(t *testing.T) {
	service := setupRFMCalculatorService(t)

	rfms := []*model.UserRFM{
		{
			UserID: 1,
			Layer:  "important_value",
		},
	}

	result := service.enrichUserData(context.Background(), rfms)
	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result))
	}
	if result[0].LayerDesc == "" {
		t.Error("Expected layer description to be populated")
	}
}
