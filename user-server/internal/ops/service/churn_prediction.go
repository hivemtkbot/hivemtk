package service

import (
	"encoding/json"
	"errors"
	"marketing/internal/ops/model"
	"marketing/internal/ops/repository"
	"math"
	"time"

	"gorm.io/gorm"
)

// ChurnPredictionService 流失预警服务
type ChurnPredictionService struct {
	predictionRepo *repository.ChurnPredictionRepository
	warningRepo    *repository.ChurnWarningRepository
	configRepo     *repository.ChurnModelConfigRepository
	statsRepo      *repository.ChurnStatisticsRepository
}

// NewChurnPredictionService 创建流失预警服务实例
func NewChurnPredictionService() *ChurnPredictionService {
	return &ChurnPredictionService{
		predictionRepo: repository.NewChurnPredictionRepository(),
		warningRepo:    repository.NewChurnWarningRepository(),
		configRepo:     repository.NewChurnModelConfigRepository(),
		statsRepo:      repository.NewChurnStatisticsRepository(),
	}
}

// NewChurnPredictionServiceWithDB 创建指定数据库连接的流失预警服务实例（用于测试）
func NewChurnPredictionServiceWithDB(db *gorm.DB) *ChurnPredictionService {
	return &ChurnPredictionService{
		predictionRepo: repository.NewChurnPredictionRepositoryWithDB(db),
		warningRepo:    repository.NewChurnWarningRepositoryWithDB(db),
		configRepo:     repository.NewChurnModelConfigRepositoryWithDB(db),
		statsRepo:      repository.NewChurnStatisticsRepositoryWithDB(db),
	}
}

// GetChurnPrediction 获取用户的流失预测
func (s *ChurnPredictionService) GetChurnPrediction(userID string) (*model.ChurnPrediction, error) {
	prediction, err := s.predictionRepo.GetByUserID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 用户尚未生成流失预测时，返回默认的零风险预测，而不是 404
			return &model.ChurnPrediction{
				UserID:            userID,
				ChurnScore:        0,
				ChurnRisk:         "low",
				RiskFactors:       "[]",
				DaysSinceActive:   0,
				PurchaseFreq:      0,
				AverageOrderValue: 0,
			}, nil
		}
		return nil, err
	}
	return prediction, nil
}

// GetChurnPredictions 获取流失预测列表
func (s *ChurnPredictionService) GetChurnPredictions(page, pageSize int) ([]*model.ChurnPrediction, int64, error) {
	return s.predictionRepo.GetAll(page, pageSize)
}

// GetHighRiskUsers 获取高风险用户列表
func (s *ChurnPredictionService) GetHighRiskUsers(limit int) ([]*model.ChurnPrediction, error) {
	return s.predictionRepo.GetHighRiskUsers(limit)
}

// CalculateChurnPrediction 计算用户流失分数
func (s *ChurnPredictionService) CalculateChurnPrediction(userID string, userData map[string]any) error {
	// 获取配置
	config, err := s.configRepo.GetCurrent()
	if err != nil {
		// 使用默认配置
		config = s.getDefaultConfig()
	}

	// 计算各维度分数
	inactiveScore := s.calculateInactiveScore(userData, config.InactiveThreshold)
	purchaseFreqScore := s.calculatePurchaseFreqScore(userData, config.PurchaseThreshold)
	orderValueScore := s.calculateOrderValueScore(userData)
	engagementScore := s.calculateEngagementScore(userData)

	// 加权计算总分
	churnScore := inactiveScore*config.InactiveDaysWeight +
		purchaseFreqScore*config.PurchaseFreqWeight +
		orderValueScore*config.OrderValueWeight +
		engagementScore*config.EngagementWeight

	// 确定风险等级
	var churnRisk string
	if churnScore >= config.CriticalRiskScore {
		churnRisk = "critical"
	} else if churnScore >= config.HighRiskScore {
		churnRisk = "high"
	} else if churnScore >= 50 {
		churnRisk = "medium"
	} else {
		churnRisk = "low"
	}

	// 识别风险因素
	riskFactors := s.identifyRiskFactors(inactiveScore, purchaseFreqScore, orderValueScore, engagementScore)
	riskFactorsJSON, _ := json.Marshal(riskFactors)

	// 保存预测
	prediction := &model.ChurnPrediction{
		UserID:            userID,
		ChurnScore:        churnScore,
		ChurnRisk:         churnRisk,
		RiskFactors:       string(riskFactorsJSON),
		DaysSinceActive:   int(inactiveScore),
		PurchaseFreq:      purchaseFreqScore,
		AverageOrderValue: orderValueScore,
	}

	// 从 userData 中提取时间信息
	if lastActivity, ok := userData["last_activity_at"].(time.Time); ok {
		prediction.LastActivityAt = &lastActivity
	}
	if lastPurchase, ok := userData["last_purchase_at"].(time.Time); ok {
		prediction.LastPurchaseAt = &lastPurchase
	}

	return s.predictionRepo.Upsert(prediction)
}

// calculateInactiveScore 计算未活跃分数（0-100）
func (s *ChurnPredictionService) calculateInactiveScore(userData map[string]any, threshold int) float64 {
	days := 0
	if d, ok := userData["days_since_active"].(int); ok {
		days = d
	}

	// 超过阈值天数为 100 分
	if days >= threshold*2 {
		return 100
	}
	if days >= threshold {
		return 50 + float64(days-threshold)*50/float64(threshold)
	}
	return float64(days) * 50 / float64(threshold)
}

// calculatePurchaseFreqScore 计算购买频率分数
func (s *ChurnPredictionService) calculatePurchaseFreqScore(userData map[string]any, threshold int) float64 {
	days := 0
	if d, ok := userData["days_since_purchase"].(int); ok {
		days = d
	}

	if days >= threshold*2 {
		return 100
	}
	if days >= threshold {
		return 50 + float64(days-threshold)*50/float64(threshold)
	}
	return float64(days) * 50 / float64(threshold)
}

// calculateOrderValueScore 计算订单金额分数（简化版本）
func (s *ChurnPredictionService) calculateOrderValueScore(userData map[string]any) float64 {
	aov := 0.0
	if v, ok := userData["average_order_value"].(float64); ok {
		aov = v
	}

	// 订单金额越低，流失风险越高（简化逻辑）
	if aov <= 0 {
		return 100
	}
	if aov < 50 {
		return 80
	}
	if aov < 100 {
		return 50
	}
	if aov < 500 {
		return 30
	}
	return 10
}

// calculateEngagementScore 计算互动频率分数
func (s *ChurnPredictionService) calculateEngagementScore(userData map[string]any) float64 {
	interactions := 0
	if i, ok := userData["interactions_30d"].(int); ok {
		interactions = i
	}

	// 互动越少，流失风险越高
	if interactions == 0 {
		return 100
	}
	if interactions < 3 {
		return 70
	}
	if interactions < 10 {
		return 40
	}
	return 20
}

// identifyRiskFactors 识别风险因素
func (s *ChurnPredictionService) identifyRiskFactors(inactiveScore, purchaseFreqScore, orderValueScore, engagementScore float64) []string {
	var factors []string

	if inactiveScore >= 70 {
		factors = append(factors, "长期未活跃")
	}
	if purchaseFreqScore >= 70 {
		factors = append(factors, "购买频率下降")
	}
	if orderValueScore >= 70 {
		factors = append(factors, "订单金额偏低")
	}
	if engagementScore >= 70 {
		factors = append(factors, "互动频率降低")
	}

	return factors
}

// CreateChurnWarning 创建流失预警
func (s *ChurnPredictionService) CreateChurnWarning(userID string, prediction *model.ChurnPrediction) error {
	var warningLevel, warningType, description, suggestion string

	switch prediction.ChurnRisk {
	case "critical":
		warningLevel = "critical"
		warningType = "churn_critical"
		description = "用户流失风险极高，需要立即关注"
		suggestion = "建议立即联系用户，提供专属优惠或关怀活动"
	case "high":
		warningLevel = "high"
		warningType = "churn_high"
		description = "用户流失风险较高，需要重点关注"
		suggestion = "建议发送个性化营销内容，提高用户活跃度"
	case "medium":
		warningLevel = "medium"
		warningType = "churn_medium"
		description = "用户存在一定的流失风险"
		suggestion = "建议关注用户行为，适时进行用户触达"
	default:
		return nil // 低风险不创建预警
	}

	warning := &model.ChurnWarning{
		UserID:       userID,
		WarningLevel: warningLevel,
		WarningType:  warningType,
		Description:  description,
		Suggestion:   suggestion,
		IsHandled:    false,
	}

	return s.warningRepo.Create(warning)
}

// GetChurnWarnings 获取流失预警列表
func (s *ChurnPredictionService) GetChurnWarnings(page, pageSize int) ([]*model.ChurnWarning, int64, error) {
	return s.warningRepo.GetAll(page, pageSize)
}

// GetUnhandledWarnings 获取未处理的流失预警
func (s *ChurnPredictionService) GetUnhandledWarnings(page, pageSize int) ([]*model.ChurnWarning, int64, error) {
	return s.warningRepo.GetUnhandled(page, pageSize)
}

// MarkWarningHandled 标记预警为已处理
func (s *ChurnPredictionService) MarkWarningHandled(id, handledBy uint, note string) error {
	return s.warningRepo.MarkHandled(id, handledBy, note)
}

// InterveneWarning 对流失预警进行干预，记录干预类型
func (s *ChurnPredictionService) InterveneWarning(id, handledBy uint, interventionType string) error {
	note := "干预类型: " + interventionType
	return s.warningRepo.MarkHandled(id, handledBy, note)
}

// GetModelConfig 获取模型配置
func (s *ChurnPredictionService) GetModelConfig() (*model.ChurnModelConfig, error) {
	config, err := s.configRepo.GetCurrent()
	if err != nil {
		return s.getDefaultConfig(), nil
	}
	return config, nil
}

// SaveModelConfig 保存模型配置
func (s *ChurnPredictionService) SaveModelConfig(config *model.ChurnModelConfig) error {
	return s.configRepo.Upsert(config)
}

// getDefaultConfig 获取默认配置
func (s *ChurnPredictionService) getDefaultConfig() *model.ChurnModelConfig {
	return &model.ChurnModelConfig{
		InactiveDaysWeight: 0.3,
		PurchaseFreqWeight: 0.3,
		OrderValueWeight:   0.2,
		EngagementWeight:   0.2,
		InactiveThreshold:  30,
		PurchaseThreshold:  60,
		HighRiskScore:      70,
		CriticalRiskScore:  85,
	}
}

// GetChurnStatistics 获取流失统计
func (s *ChurnPredictionService) GetChurnStatistics(startDate, endDate string) ([]*model.ChurnStatistics, error) {
	return s.statsRepo.GetAll(startDate, endDate)
}

// CalculateDailyStatistics 计算每日统计
func (s *ChurnPredictionService) CalculateDailyStatistics(date string) error {
	// 获取当天的预测数据
	predictions, _, _ := s.predictionRepo.GetAll(1, 10000)

	totalUsers := len(predictions)
	churnUsers := 0
	highRiskUsers := 0
	criticalUsers := 0

	for _, p := range predictions {
		if p.ChurnRisk == "high" {
			highRiskUsers++
		} else if p.ChurnRisk == "critical" {
			criticalUsers++
		}
		if p.ChurnScore >= 70 {
			churnUsers++
		}
	}

	churnRate := 0.0
	if totalUsers > 0 {
		churnRate = float64(churnUsers) / float64(totalUsers) * 100
	}

	stats := &model.ChurnStatistics{
		StatDate:      date,
		TotalUsers:    totalUsers,
		ChurnUsers:    churnUsers,
		ChurnRate:     churnRate,
		HighRiskUsers: highRiskUsers,
		CriticalUsers: criticalUsers,
	}

	return s.statsRepo.Create(stats)
}

// RunChurnCalculation 运行流失计算（定时任务）
func (s *ChurnPredictionService) RunChurnCalculation(users []map[string]any) error {
	for _, user := range users {
		userID, ok := user["user_id"].(string)
		if !ok {
			continue
		}

		// 计算流失分数
		if err := s.CalculateChurnPrediction(userID, user); err != nil {
			continue
		}

		// 获取预测结果
		prediction, _ := s.predictionRepo.GetByUserID(userID)
		if prediction != nil {
			// 创建预警
			s.CreateChurnWarning(userID, prediction)
		}
	}

	// 更新配置的最后计算时间
	s.configRepo.UpdateCalcTime()

	// 计算统计
	date := time.Now().Format("2006-01-02")
	s.CalculateDailyStatistics(date)

	return nil
}

// GetChurnRate 获取流失率
func (s *ChurnPredictionService) GetChurnRate() (float64, error) {
	stats, err := s.statsRepo.GetLatest()
	if err != nil {
		return 0, err
	}
	return stats.ChurnRate, nil
}

// GetRiskDistribution 获取风险分布
func (s *ChurnPredictionService) GetRiskDistribution() (map[string]int, error) {
	predictions, _, _ := s.predictionRepo.GetAll(1, 10000)

	distribution := map[string]int{
		"low":      0,
		"medium":   0,
		"high":     0,
		"critical": 0,
	}

	for _, p := range predictions {
		distribution[p.ChurnRisk]++
	}

	return distribution, nil
}

// GenerateSuggestion 生成挽回建议
func (s *ChurnPredictionService) GenerateSuggestion(prediction *model.ChurnPrediction, userData map[string]any) string {
	var riskFactors []string
	json.Unmarshal([]byte(prediction.RiskFactors), &riskFactors)

	suggestions := []string{}

	for _, factor := range riskFactors {
		switch factor {
		case "长期未活跃":
			suggestions = append(suggestions, "发送活跃提醒消息，提供登录奖励")
		case "购买频率下降":
			suggestions = append(suggestions, "推送个性化商品推荐，提供限时优惠")
		case "订单金额偏低":
			suggestions = append(suggestions, "提供满减优惠券，鼓励提高客单价")
		case "互动频率降低":
			suggestions = append(suggestions, "邀请参与互动活动，增加品牌曝光")
		}
	}

	if len(suggestions) == 0 {
		return "用户风险较低，保持正常运营即可"
	}

	return "建议措施：" + joinStrings(suggestions, "；")
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// CalculateConfidence 计算置信度（使用正态分布）
func CalculateConfidence(rate float64, sampleSize int) float64 {
	if sampleSize == 0 {
		return 0
	}

	p := rate / 100
	n := float64(sampleSize)

	// 标准误
	se := math.Sqrt(p * (1 - p) / n)

	if se == 0 {
		return 0
	}

	// Z 分数
	z := (p - 0.5) / se

	// 置信度
	confidence := 0.5 * (1 + math.Erf(z/math.Sqrt2))

	return confidence
}
