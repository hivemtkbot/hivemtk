package service

import (
	"context"
	"encoding/json"
	"marketing/internal/ops/model"
	"marketing/internal/pkg/utils/db"
	"testing"
	"time"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupChurnPredictionServiceTestDB 设置测试数据库
func setupChurnPredictionServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.ChurnPrediction{},
		&model.ChurnWarning{},
		&model.ChurnModelConfig{},
		&model.ChurnStatistics{},
	)
	db.SetTestDB(database)
	return database
}

// setupChurnPredictionService 设置测试服务
func setupChurnPredictionService(t *testing.T) *ChurnPredictionService {
	setupChurnPredictionServiceTestDB(t)
	return NewChurnPredictionService()
}

// TestNewChurnPredictionService 测试创建服务实例
func TestNewChurnPredictionService(t *testing.T) {
	service := NewChurnPredictionService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.predictionRepo == nil {
		t.Error("Expected predictionRepo to be initialized")
	}
	if service.warningRepo == nil {
		t.Error("Expected warningRepo to be initialized")
	}
	if service.configRepo == nil {
		t.Error("Expected configRepo to be initialized")
	}
	if service.statsRepo == nil {
		t.Error("Expected statsRepo to be initialized")
	}
}

// TestChurnPredictionService_CalculateChurnPrediction_LowRisk 测试计算低风险用户
func TestChurnPredictionService_CalculateChurnPrediction_LowRisk(t *testing.T) {
	service := setupChurnPredictionService(t)

	userData := map[string]any{
		"user_id":             "user_123",
		"days_since_active":   5,
		"days_since_purchase": 10,
		"average_order_value": 200.0,
		"interactions_30d":    15,
		"last_activity_at":    time.Now().AddDate(0, 0, -5),
		"last_purchase_at":    time.Now().AddDate(0, 0, -10),
	}

	err := service.CalculateChurnPrediction("user_123", userData)
	if err != nil {
		t.Fatalf("CalculateChurnPrediction failed: %v", err)
	}

	// 验证预测已保存
	prediction, err := service.GetChurnPrediction("user_123")
	if err != nil {
		t.Fatalf("GetChurnPrediction failed: %v", err)
	}

	if prediction.ChurnRisk != "low" {
		t.Errorf("Expected churn_risk 'low', got '%s'", prediction.ChurnRisk)
	}
	// DaysSinceActive 存储的是 inactiveScore 而不是原始天数
	// 5 天 < 30 天阈值，所以分数应该是 5/30 * 50 = 8.33 -> 8
	if prediction.DaysSinceActive < 5 || prediction.DaysSinceActive > 10 {
		t.Errorf("Expected days_since_active around 8 (score for 5 days), got %d", prediction.DaysSinceActive)
	}
}

// TestChurnPredictionService_CalculateChurnPrediction_HighRisk 测试计算高风险用户
func TestChurnPredictionService_CalculateChurnPrediction_HighRisk(t *testing.T) {
	service := setupChurnPredictionService(t)

	userData := map[string]any{
		"user_id":             "user_456",
		"days_since_active":   90,
		"days_since_purchase": 120,
		"average_order_value": 30.0,
		"interactions_30d":    0,
	}

	err := service.CalculateChurnPrediction("user_456", userData)
	if err != nil {
		t.Fatalf("CalculateChurnPrediction failed: %v", err)
	}

	prediction, err := service.GetChurnPrediction("user_456")
	if err != nil {
		t.Fatalf("GetChurnPrediction failed: %v", err)
	}

	if prediction.ChurnRisk != "critical" && prediction.ChurnRisk != "high" {
		t.Errorf("Expected high/critical risk, got '%s'", prediction.ChurnRisk)
	}
}

// TestChurnPredictionService_CalculateChurnPrediction_WithCustomConfig 测试使用自定义配置计算
func TestChurnPredictionService_CalculateChurnPrediction_WithCustomConfig(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 创建自定义配置
	config := &model.ChurnModelConfig{
		InactiveDaysWeight: 0.5,
		PurchaseFreqWeight: 0.3,
		OrderValueWeight:   0.1,
		EngagementWeight:   0.1,
		InactiveThreshold:  15,
		PurchaseThreshold:  30,
		HighRiskScore:      60,
		CriticalRiskScore:  80,
	}
	service.SaveModelConfig(config)

	userData := map[string]any{
		"user_id":             "user_789",
		"days_since_active":   20,
		"days_since_purchase": 40,
		"average_order_value": 150.0,
		"interactions_30d":    5,
	}

	err := service.CalculateChurnPrediction("user_789", userData)
	if err != nil {
		t.Fatalf("CalculateChurnPrediction failed: %v", err)
	}

	prediction, _ := service.GetChurnPrediction("user_789")
	if prediction == nil {
		t.Error("Expected prediction to be created")
	}
}

// TestChurnPredictionService_GetChurnPredictions 测试获取流失预测列表
func TestChurnPredictionService_GetChurnPredictions(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 创建多个预测
	for i := 1; i <= 5; i++ {
		userData := map[string]any{
			"user_id":             "user_" + string(rune('0'+i)),
			"days_since_active":   i * 10,
			"days_since_purchase": i * 15,
			"average_order_value": float64(i * 50),
			"interactions_30d":    i,
		}
		service.CalculateChurnPrediction("user_"+string(rune('0'+i)), userData)
	}

	// 获取列表
	predictions, total, err := service.GetChurnPredictions(1, 10)
	if err != nil {
		t.Fatalf("GetChurnPredictions failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(predictions) != 5 {
		t.Errorf("Expected 5 predictions, got %d", len(predictions))
	}
}

// TestChurnPredictionService_GetHighRiskUsers 测试获取高风险用户
func TestChurnPredictionService_GetHighRiskUsers(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 创建高风险用户
	userData := map[string]any{
		"user_id":             "user_high",
		"days_since_active":   100,
		"days_since_purchase": 150,
		"average_order_value": 20.0,
		"interactions_30d":    0,
	}
	service.CalculateChurnPrediction("user_high", userData)

	// 创建低风险用户
	userDataLow := map[string]any{
		"user_id":             "user_low",
		"days_since_active":   5,
		"days_since_purchase": 10,
		"average_order_value": 200.0,
		"interactions_30d":    20,
	}
	service.CalculateChurnPrediction("user_low", userDataLow)

	// 获取高风险用户
	highRiskUsers, err := service.GetHighRiskUsers(10)
	if err != nil {
		t.Fatalf("GetHighRiskUsers failed: %v", err)
	}

	if len(highRiskUsers) < 1 {
		t.Errorf("Expected at least 1 high risk user, got %d", len(highRiskUsers))
	}
}

// TestChurnPredictionService_CreateChurnWarning 测试创建流失预警
func TestChurnPredictionService_CreateChurnWarning(t *testing.T) {
	service := setupChurnPredictionService(t)

	prediction := &model.ChurnPrediction{
		UserID:      "user_warn",
		ChurnScore:  85.0,
		ChurnRisk:   "critical",
		RiskFactors: `["长期未活跃", "购买频率下降"]`,
	}

	err := service.CreateChurnWarning("user_warn", prediction)
	if err != nil {
		t.Fatalf("CreateChurnWarning failed: %v", err)
	}

	// 验证预警已创建
	warnings, total, _ := service.GetChurnWarnings(1, 10)
	if total != 1 {
		t.Errorf("Expected 1 warning, got %d", total)
	}
	if len(warnings) != 1 {
		t.Errorf("Expected 1 warning in list, got %d", len(warnings))
	}
}

// TestChurnPredictionService_CreateChurnWarning_LowRisk 测试低风险不创建预警
func TestChurnPredictionService_CreateChurnWarning_LowRisk(t *testing.T) {
	service := setupChurnPredictionService(t)

	prediction := &model.ChurnPrediction{
		UserID:      "user_low",
		ChurnScore:  30.0,
		ChurnRisk:   "low",
		RiskFactors: `[]`,
	}

	err := service.CreateChurnWarning("user_low", prediction)
	if err != nil {
		t.Fatalf("CreateChurnWarning failed: %v", err)
	}

	// 验证没有创建预警
	_, total, _ := service.GetChurnWarnings(1, 10)
	if total != 0 {
		t.Errorf("Expected 0 warnings for low risk, got %d", total)
	}
}

// TestChurnPredictionService_GetUnhandledWarnings 测试获取未处理预警
func TestChurnPredictionService_GetUnhandledWarnings(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 创建预警
	prediction := &model.ChurnPrediction{
		UserID:      "user_unhandled",
		ChurnScore:  75.0,
		ChurnRisk:   "high",
		RiskFactors: `["长期未活跃"]`,
	}
	service.CreateChurnWarning("user_unhandled", prediction)

	// 获取未处理预警
	warnings, total, err := service.GetUnhandledWarnings(1, 10)
	if err != nil {
		t.Fatalf("GetUnhandledWarnings failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected 1 unhandled warning, got %d", total)
	}
	if len(warnings) != 1 {
		t.Errorf("Expected 1 warning in list, got %d", len(warnings))
	}
}

// TestChurnPredictionService_MarkWarningHandled 测试标记预警已处理
func TestChurnPredictionService_MarkWarningHandled(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 创建预警
	prediction := &model.ChurnPrediction{
		UserID:      "user_handle",
		ChurnScore:  80.0,
		ChurnRisk:   "high",
		RiskFactors: `["购买频率下降"]`,
	}
	service.CreateChurnWarning("user_handle", prediction)

	// 获取预警
	warnings, _, _ := service.GetChurnWarnings(1, 10)
	if len(warnings) == 0 {
		t.Fatal("No warnings created")
	}
	warningID := warnings[0].ID

	// 标记为已处理
	err := service.MarkWarningHandled(warningID, 123, "已联系用户")
	if err != nil {
		t.Fatalf("MarkWarningHandled failed: %v", err)
	}

	// 验证状态
	updated, _, _ := service.GetUnhandledWarnings(1, 10)
	if len(updated) != 0 {
		t.Error("Expected warning to be handled")
	}
}

// TestChurnPredictionService_GetModelConfig 测试获取模型配置
func TestChurnPredictionService_GetModelConfig(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 获取默认配置
	config, err := service.GetModelConfig()
	if err != nil {
		t.Fatalf("GetModelConfig failed: %v", err)
	}

	if config.InactiveThreshold != 30 {
		t.Errorf("Expected inactive_threshold 30, got %d", config.InactiveThreshold)
	}
	if config.InactiveDaysWeight != 0.3 {
		t.Errorf("Expected inactive_days_weight 0.3, got %f", config.InactiveDaysWeight)
	}
}

// TestChurnPredictionService_SaveModelConfig 测试保存模型配置
func TestChurnPredictionService_SaveModelConfig(t *testing.T) {
	service := setupChurnPredictionService(t)

	config := &model.ChurnModelConfig{
		InactiveDaysWeight: 0.4,
		PurchaseFreqWeight: 0.3,
		OrderValueWeight:   0.2,
		EngagementWeight:   0.1,
		InactiveThreshold:  20,
		PurchaseThreshold:  45,
		HighRiskScore:      65,
		CriticalRiskScore:  80,
	}

	err := service.SaveModelConfig(config)
	if err != nil {
		t.Fatalf("SaveModelConfig failed: %v", err)
	}

	// 验证配置已保存
	retrieved, err := service.GetModelConfig()
	if err != nil {
		t.Fatalf("GetModelConfig failed: %v", err)
	}

	if retrieved.InactiveThreshold != 20 {
		t.Errorf("Expected inactive_threshold 20, got %d", retrieved.InactiveThreshold)
	}
}

// TestChurnPredictionService_CalculateDailyStatistics 测试计算每日统计
func TestChurnPredictionService_CalculateDailyStatistics(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 创建不同风险的预测
	for i := 1; i <= 10; i++ {
		risk := "low"
		score := 30.0
		if i <= 2 {
			risk = "critical"
			score = 90.0
		} else if i <= 4 {
			risk = "high"
			score = 75.0
		}

		prediction := &model.ChurnPrediction{
			UserID:      "user_stat_" + string(rune('0'+i)),
			ChurnScore:  score,
			ChurnRisk:   risk,
			RiskFactors: `[]`,
		}
		service.predictionRepo.Upsert(prediction)
	}

	// 计算统计
	date := time.Now().Format("2006-01-02")
	err := service.CalculateDailyStatistics(date)
	if err != nil {
		t.Fatalf("CalculateDailyStatistics failed: %v", err)
	}

	// 验证统计
	stats, err := service.statsRepo.GetLatest()
	if err != nil {
		t.Fatalf("GetLatest stats failed: %v", err)
	}

	if stats.TotalUsers != 10 {
		t.Errorf("Expected total_users 10, got %d", stats.TotalUsers)
	}
	if stats.HighRiskUsers < 2 {
		t.Errorf("Expected at least 2 high risk users, got %d", stats.HighRiskUsers)
	}
	if stats.CriticalUsers < 2 {
		t.Errorf("Expected at least 2 critical users, got %d", stats.CriticalUsers)
	}
}

// TestChurnPredictionService_GetRiskDistribution 测试获取风险分布
func TestChurnPredictionService_GetRiskDistribution(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 创建不同风险的预测
	risks := []string{"low", "low", "medium", "high", "critical"}
	for i, risk := range risks {
		score := 30.0
		switch risk {
		case "medium":
			score = 55.0
		case "high":
			score = 75.0
		case "critical":
			score = 90.0
		}

		prediction := &model.ChurnPrediction{
			UserID:      "user_dist_" + string(rune('0'+i)),
			ChurnScore:  score,
			ChurnRisk:   risk,
			RiskFactors: `[]`,
		}
		service.predictionRepo.Upsert(prediction)
	}

	// 获取分布
	distribution, err := service.GetRiskDistribution()
	if err != nil {
		t.Fatalf("GetRiskDistribution failed: %v", err)
	}

	if distribution["low"] != 2 {
		t.Errorf("Expected 2 low risk, got %d", distribution["low"])
	}
	if distribution["medium"] != 1 {
		t.Errorf("Expected 1 medium risk, got %d", distribution["medium"])
	}
	if distribution["high"] != 1 {
		t.Errorf("Expected 1 high risk, got %d", distribution["high"])
	}
	if distribution["critical"] != 1 {
		t.Errorf("Expected 1 critical risk, got %d", distribution["critical"])
	}
}

// TestChurnPredictionService_GenerateSuggestion 测试生成建议
func TestChurnPredictionService_GenerateSuggestion(t *testing.T) {
	service := setupChurnPredictionService(t)

	prediction := &model.ChurnPrediction{
		ChurnRisk:   "high",
		RiskFactors: `["长期未活跃", "购买频率下降"]`,
	}

	userData := map[string]any{}
	suggestion := service.GenerateSuggestion(prediction, userData)

	if suggestion == "" {
		t.Error("Expected suggestion to be generated")
	}
	if len(suggestion) < 10 {
		t.Errorf("Expected meaningful suggestion, got '%s'", suggestion)
	}
}

// TestChurnPredictionService_GenerateSuggestion_LowRisk 测试低风险用户建议
func TestChurnPredictionService_GenerateSuggestion_LowRisk(t *testing.T) {
	service := setupChurnPredictionService(t)

	prediction := &model.ChurnPrediction{
		ChurnRisk:   "low",
		RiskFactors: `[]`,
	}

	suggestion := service.GenerateSuggestion(prediction, nil)
	if suggestion != "用户风险较低，保持正常运营即可" {
		t.Errorf("Expected low risk suggestion, got '%s'", suggestion)
	}
}

// TestChurnPredictionService_calculateInactiveScore 测试未活跃分数计算
func TestChurnPredictionService_calculateInactiveScore(t *testing.T) {
	service := setupChurnPredictionService(t)

	tests := []struct {
		name      string
		days      int
		threshold int
		wantMin   float64
		wantMax   float64
	}{
		{"零天", 0, 30, 0, 0},
		{"15 天 (50%)", 15, 30, 24, 26},
		{"30 天 (阈值)", 30, 30, 49, 51},
		{"45 天 (超阈值)", 45, 30, 74, 76},
		{"60 天 (2 倍阈值)", 60, 30, 99, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userData := map[string]any{"days_since_active": tt.days}
			score := service.calculateInactiveScore(userData, tt.threshold)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("calculateInactiveScore() = %v, want [%v, %v]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestChurnPredictionService_calculatePurchaseFreqScore 测试购买频率分数计算
func TestChurnPredictionService_calculatePurchaseFreqScore(t *testing.T) {
	service := setupChurnPredictionService(t)

	tests := []struct {
		name      string
		days      int
		threshold int
		wantMin   float64
		wantMax   float64
	}{
		{"零天", 0, 60, 0, 0},
		{"30 天 (50%)", 30, 60, 24, 26},
		{"60 天 (阈值)", 60, 60, 49, 51},
		{"90 天 (超阈值)", 90, 60, 74, 76},
		{"120 天 (2 倍阈值)", 120, 60, 99, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userData := map[string]any{"days_since_purchase": tt.days}
			score := service.calculatePurchaseFreqScore(userData, tt.threshold)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("calculatePurchaseFreqScore() = %v, want [%v, %v]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestChurnPredictionService_calculateOrderValueScore 测试订单金额分数计算
func TestChurnPredictionService_calculateOrderValueScore(t *testing.T) {
	service := setupChurnPredictionService(t)

	tests := []struct {
		name    string
		aov     float64
		wantMin float64
		wantMax float64
	}{
		{"零金额", 0, 99, 100},
		{"低金额 (<50)", 30, 79, 81},
		{"中金额 (50-100)", 75, 49, 51},
		{"高金额 (100-500)", 200, 29, 31},
		{"超高金额 (>=500)", 600, 9, 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userData := map[string]any{"average_order_value": tt.aov}
			score := service.calculateOrderValueScore(userData)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("calculateOrderValueScore() = %v, want [%v, %v]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestChurnPredictionService_calculateEngagementScore 测试互动频率分数计算
func TestChurnPredictionService_calculateEngagementScore(t *testing.T) {
	service := setupChurnPredictionService(t)

	tests := []struct {
		name         string
		interactions int
		wantMin      float64
		wantMax      float64
	}{
		{"零互动", 0, 99, 100},
		{"低互动 (<3)", 2, 69, 71},
		{"中互动 (3-10)", 5, 39, 41},
		{"高互动 (>=10)", 15, 19, 21},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userData := map[string]any{"interactions_30d": tt.interactions}
			score := service.calculateEngagementScore(userData)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("calculateEngagementScore() = %v, want [%v, %v]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestChurnPredictionService_identifyRiskFactors 测试风险因素识别
func TestChurnPredictionService_identifyRiskFactors(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 所有因素都高风险
	factors := service.identifyRiskFactors(80, 80, 80, 80)
	if len(factors) != 4 {
		t.Errorf("Expected 4 risk factors, got %d", len(factors))
	}

	// 所有因素都低风险
	factorsLow := service.identifyRiskFactors(30, 30, 30, 30)
	if len(factorsLow) != 0 {
		t.Errorf("Expected 0 risk factors, got %d", len(factorsLow))
	}
}

// TestChurnPredictionService_RunChurnCalculation 测试批量流失计算
func TestChurnPredictionService_RunChurnCalculation(t *testing.T) {
	service := setupChurnPredictionService(t)

	users := []map[string]any{
		{
			"user_id":             "user_batch_1",
			"days_since_active":   50,
			"days_since_purchase": 70,
			"average_order_value": 100.0,
			"interactions_30d":    3,
		},
		{
			"user_id":             "user_batch_2",
			"days_since_active":   100,
			"days_since_purchase": 150,
			"average_order_value": 50.0,
			"interactions_30d":    0,
		},
	}

	err := service.RunChurnCalculation(users)
	if err != nil {
		t.Fatalf("RunChurnCalculation failed: %v", err)
	}

	// 验证预测已创建
	pred1, _ := service.GetChurnPrediction("user_batch_1")
	if pred1 == nil {
		t.Error("Expected prediction for user_batch_1")
	}

	pred2, _ := service.GetChurnPrediction("user_batch_2")
	if pred2 == nil {
		t.Error("Expected prediction for user_batch_2")
	}

	// 验证预警已创建（高风险用户）
	_, total, _ := service.GetUnhandledWarnings(1, 10)
	if total < 1 {
		t.Errorf("Expected at least 1 warning, got %d", total)
	}
}

// TestCalculateConfidence 测试置信度计算函数
func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		name       string
		rate       float64
		sampleSize int
		wantMin    float64
		wantMax    float64
	}{
		{"零样本", 50.0, 0, 0, 0},
		{"高转化率大样本", 70.0, 100, 0.8, 1.0},
		{"50% 转化率", 50.0, 100, 0.3, 0.7},
		{"低转化率", 20.0, 100, 0, 0.2},
		{"小样本", 60.0, 10, 0.1, 0.9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := CalculateConfidence(tt.rate, tt.sampleSize)
			if confidence < tt.wantMin || confidence > tt.wantMax {
				t.Errorf("CalculateConfidence(%v, %d) = %v, want [%v, %v]",
					tt.rate, tt.sampleSize, confidence, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestChurnPredictionService_GetChurnRate 测试获取流失率
func TestChurnPredictionService_GetChurnRate(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 先创建统计数据
	stats := &model.ChurnStatistics{
		StatDate:      time.Now().Format("2006-01-02"),
		TotalUsers:    100,
		ChurnUsers:    25,
		ChurnRate:     25.0,
		HighRiskUsers: 10,
		CriticalUsers: 5,
	}
	service.statsRepo.Create(stats)

	// 获取流失率
	rate, err := service.GetChurnRate()
	if err != nil {
		t.Fatalf("GetChurnRate failed: %v", err)
	}

	if rate != 25.0 {
		t.Errorf("Expected churn rate 25.0, got %f", rate)
	}
}

// TestChurnPredictionService_GetChurnWarnings_Pagination 测试分页获取预警
func TestChurnPredictionService_GetChurnWarnings_Pagination(t *testing.T) {
	service := setupChurnPredictionService(t)

	// 创建多个预警
	for i := 1; i <= 15; i++ {
		prediction := &model.ChurnPrediction{
			UserID:      "user_page_" + string(rune('0'+i%10)),
			ChurnScore:  float64(70 + i),
			ChurnRisk:   "high",
			RiskFactors: `["长期未活跃"]`,
		}
		service.CreateChurnWarning("user_page_"+string(rune('0'+i%10)), prediction)
	}

	// 获取第一页
	warnings, total, err := service.GetChurnWarnings(1, 10)
	if err != nil {
		t.Fatalf("GetChurnWarnings failed: %v", err)
	}

	if total < 1 {
		t.Errorf("Expected warnings, got %d", total)
	}
	if len(warnings) < 1 {
		t.Errorf("Expected warnings on page 1, got %d", len(warnings))
	}
}

// TestChurnPredictionService_CalculateChurnPrediction_RiskFactorsJSON 测试风险因素 JSON 序列化
func TestChurnPredictionService_CalculateChurnPrediction_RiskFactorsJSON(t *testing.T) {
	service := setupChurnPredictionService(t)

	userData := map[string]any{
		"user_id":             "user_json",
		"days_since_active":   90,
		"days_since_purchase": 120,
		"average_order_value": 30.0,
		"interactions_30d":    0,
	}

	err := service.CalculateChurnPrediction("user_json", userData)
	if err != nil {
		t.Fatalf("CalculateChurnPrediction failed: %v", err)
	}

	prediction, _ := service.GetChurnPrediction("user_json")

	// 验证风险因素可以正确解析
	var factors []string
	err = json.Unmarshal([]byte(prediction.RiskFactors), &factors)
	if err != nil {
		t.Errorf("Failed to unmarshal risk factors: %v", err)
	}

	if len(factors) < 1 {
		t.Errorf("Expected at least 1 risk factor, got %d", len(factors))
	}
}
