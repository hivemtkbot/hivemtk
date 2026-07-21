package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// ClueScoreService 线索评分服务
// 评分模型（5 维度加权，0-100）：
//
//	channel 25% / verify 20% / profile 20% / engagement 25% / recency 10%
type ClueScoreService struct {
	scoreRepo  repository.ClueScoreRepository
	engageRepo repository.ClueEngagementRepository
	clueRepo   repository.ClueRepository
}

// NewClueScoreService 创建线索评分服务实例
func NewClueScoreService() *ClueScoreService {
	return &ClueScoreService{
		scoreRepo:  repository.NewClueScoreRepository(),
		engageRepo: repository.NewClueEngagementRepository(),
		clueRepo:   repository.NewClueRepository(),
	}
}

// NewClueScoreServiceWithRepos 测试用 - 自定义依赖
func NewClueScoreServiceWithRepos(s repository.ClueScoreRepository, e repository.ClueEngagementRepository, c repository.ClueRepository) *ClueScoreService {
	return &ClueScoreService{scoreRepo: s, engageRepo: e, clueRepo: c}
}

// ScoreClue 评分单条线索（基础信息+互动事件）
func (s *ClueScoreService) ScoreClue(clue *model.Clue) (*model.ClueScore, error) {
	if clue == nil {
		return nil, errors.New("线索不能为空")
	}

	// 1. 渠道分 (0-100)
	channelScore := scoreChannel(clue.Type)

	// 2. 验证分 (0-100)
	verifyScore := 0
	if clue.IsVerify == 1 {
		verifyScore = 100
	} else if clue.IsVerify == 0 {
		verifyScore = 20 // 未验证给基础分
	}

	// 3. 资料完整度 (0-100)
	profileScore := scoreProfile(clue)

	// 4. 行为参与度 (0-100)
	engagementScore, err := s.scoreEngagement(clue.ID)
	if err != nil {
		// 互动事件查询失败不阻塞评分
		engagementScore = 0
	}

	// 5. 时效性 (0-100)
	recencyScore := clueScoreRecency(clue.CreateTime)

	// 加权汇总：25% / 20% / 20% / 25% / 10%
	total := int(math.Round(
		float64(channelScore)*0.25 +
			float64(verifyScore)*0.20 +
			float64(profileScore)*0.20 +
			float64(engagementScore)*0.25 +
			float64(recencyScore)*0.10,
	))
	if total < 0 {
		total = 0
	}
	if total > 100 {
		total = 100
	}

	// 置信度：维度完整度（5 维度，0-100）
	confidence := calcConfidence(channelScore, verifyScore, profileScore, engagementScore, recencyScore)

	factors := map[string]any{
		"channel":    channelScore,
		"verify":     verifyScore,
		"profile":    profileScore,
		"engagement": engagementScore,
		"recency":    recencyScore,
		"weights": map[string]float64{
			"channel":    0.25,
			"verify":     0.20,
			"profile":    0.20,
			"engagement": 0.25,
			"recency":    0.10,
		},
		"clue_type": clue.Type,
		"is_verify": clue.IsVerify,
	}
	factorsJSON, _ := json.Marshal(factors)

	score := &model.ClueScore{
		ClueID:          clue.ID,
		Account:         clue.Account,
		TotalScore:      total,
		Grade:           model.CalcGradeFromScore(total),
		Confidence:      confidence,
		ChannelScore:    channelScore,
		VerifyScore:     verifyScore,
		ProfileScore:    profileScore,
		EngagementScore: engagementScore,
		RecencyScore:    recencyScore,
		FactorsJSON:     string(factorsJSON),
		ModelVersion:    "h-score-1",
		ScoredAt:        time.Now(),
	}
	if err := s.scoreRepo.Upsert(score); err != nil {
		return nil, err
	}
	return score, nil
}

// ScoreAll 对所有线索进行评分（受 limit 限制）
func (s *ClueScoreService) ScoreAll(limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	clues, _, err := s.clueRepo.GetClueList(1, limit)
	if err != nil {
		return 0, err
	}
	success := 0
	for _, c := range clues {
		// clue.id 是 varchar，但 GetByID 接收 uint；忽略 ID 类型，按 list 返回对象评分
		if _, err := s.ScoreClue(c); err != nil {
			return success, fmt.Errorf("评分失败 (clue_id=%s): %w", c.ID, err)
		}
		success++
	}
	return success, nil
}

// RecordEngagement 记录一次互动事件
func (s *ClueScoreService) RecordEngagement(clueID, eventType, channel string, payload any) error {
	if clueID == "" {
		return errors.New("clue_id 不能为空")
	}
	if eventType == "" {
		return errors.New("event_type 不能为空")
	}
	payloadBytes, _ := json.Marshal(payload)
	evt := &model.ClueEngagementEvent{
		ClueID:    clueID,
		EventType: eventType,
		Channel:   channel,
		Payload:   string(payloadBytes),
	}
	return s.engageRepo.Create(evt)
}

// GetByClueID 查询线索评分
func (s *ClueScoreService) GetByClueID(clueID string) (*model.ClueScore, error) {
	return s.scoreRepo.GetByClueID(clueID)
}

// ListByGrade 按等级分页查询
func (s *ClueScoreService) ListByGrade(grade string, page, pageSize int) ([]*model.ClueScore, int64, error) {
	return s.scoreRepo.ListByGrade(grade, page, pageSize)
}

// ListTopByScore 查询 top N 评分
func (s *ClueScoreService) ListTopByScore(limit int) ([]*model.ClueScore, error) {
	return s.scoreRepo.ListTopByScore(limit)
}

// LoadClueForScoring 加载线索对象（用于评分）
// 通过 clue list 拉取（兼容 string ID 主键）
func (s *ClueScoreService) LoadClueForScoring(clueID string) (*model.Clue, error) {
	if clueID == "" {
		return nil, errors.New("clue_id 不能为空")
	}
	// 通过 list 全量查找（数据量小，性能可接受）
	clues, _, err := s.clueRepo.GetClueList(1, 500)
	if err != nil {
		return nil, err
	}
	for _, c := range clues {
		if c.ID == clueID {
			return c, nil
		}
	}
	return nil, errors.New("线索不存在")
}

// scoreChannel 渠道质量评分（按 6 种类型给定基础分）
// 依据：电话/微信/Whatsapp 触达成功率高于纯社交账号
func scoreChannel(clueType int64) int {
	switch clueType {
	case 1: // QQ
		return 60
	case 2: // 微信
		return 85
	case 3: // 电话
		return 95
	case 4: // Telegram
		return 80
	case 5: // Whatsapp
		return 90
	case 6: // twitter
		return 55
	default:
		return 50
	}
}

// scoreProfile 资料完整度评分（4 个字段，每字段 25 分）
func scoreProfile(clue *model.Clue) int {
	score := 0
	if clue.Name != "" {
		score += 25
	}
	if clue.City != "" {
		score += 25
	}
	if clue.Address != "" {
		score += 25
	}
	if clue.Desc != "" {
		score += 25
	}
	return score
}

// scoreEngagement 行为参与度评分
//
//	事件权重：reply 20 / click 15 / call 25 / visit 10；近 7 天有效
//	总分上限 100
func (s *ClueScoreService) scoreEngagement(clueID string) (int, error) {
	if clueID == "" {
		return 0, nil
	}
	since := time.Now().Add(-7 * 24 * time.Hour)
	count, err := s.engageRepo.CountByClueID(clueID, since)
	if err != nil {
		return 0, err
	}
	if count > 8 {
		count = 8
	}
	return int(count) * 12, nil
}

// scoreRecency 时效性评分
//
//	<= 24h → 100, <= 7d → 70, <= 30d → 40, > 30d → 0
func clueScoreRecency(createTime int64) int {
	if createTime <= 0 {
		return 50
	}
	t := time.Unix(createTime, 0)
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days <= 1:
		return 100
	case days <= 7:
		return 70
	case days <= 30:
		return 40
	default:
		return 0
	}
}

// calcConfidence 置信度（基于维度数据完整度）
func calcConfidence(channel, verify, profile, engagement, recency int) int {
	// 每维度 20 分（5 维度）
	score := 0
	if channel > 0 {
		score += 20
	}
	if verify > 0 {
		score += 20
	}
	if profile > 0 {
		score += 20
	}
	if engagement > 0 {
		score += 20
	}
	if recency > 0 {
		score += 20
	}
	return score
}

// FormatCreateTime 辅助：把字符串 create_time 转 int64（兼容 list 中已格式化字符串）
func FormatCreateTime(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
