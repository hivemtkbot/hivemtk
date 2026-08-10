package service

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"math"
	"strconv"
	"time"
)

// RFMCalculatorService RFM 计算服务
type RFMCalculatorService struct {
	rfmRuleRepo *repository.RFMRuleRepository
	userRfmRepo *repository.UserRFMRepository
	orderRepo   repository.OrderRepository
	clueRepo    repository.ClueRepository
	userRepo    repository.UserRepository
}

// NewRFMCalculatorService 创建 RFM 计算服务
func NewRFMCalculatorService() *RFMCalculatorService {
	return &RFMCalculatorService{
		rfmRuleRepo: repository.NewRFMRuleRepository(),
		userRfmRepo: repository.NewUserRFMRepository(),
		orderRepo:   repository.NewOrderRepository(),
		clueRepo:    repository.NewClueRepository(),
		userRepo:    repository.NewUserRepository(),
	}
}

// UserStats 用户统计信息
type UserStats struct {
	UserID            uint
	LastTransactionAt *time.Time
	TransactionCount  int
	TotalAmount       int64 // 总消费金额（分）
	AvgAmount         int64 // 平均消费金额（分）
}

// CalculateRFM 计算单个用户的 RFM 值
func (s *RFMCalculatorService) CalculateRFM(ctx context.Context, userID uint, rule *model.RFMRule) (*model.UserRFM, error) {
	// 获取用户统计信息
	stats, err := s.getUserStats(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 计算 R、F、M 得分
	rScore := s.calcRScore(ctx, stats.LastTransactionAt, rule)
	fScore := s.calcFScore(ctx, stats.TransactionCount, rule)
	mScore := s.calcMScore(ctx, stats.TotalAmount, rule)

	totalScore := rScore + fScore + mScore

	// 确定用户分层
	layer := s.determineLayer(ctx, rScore, fScore, mScore, stats.LastTransactionAt)

	rfm := &model.UserRFM{
		UserID:            userID,
		RScore:            rScore,
		FScore:            fScore,
		MScore:            mScore,
		TotalScore:        totalScore,
		Layer:             string(layer),
		LastTransactionAt: stats.LastTransactionAt,
		TransactionCount:  stats.TransactionCount,
		TotalAmount:       stats.TotalAmount,
		AvgAmount:         stats.AvgAmount,
	}

	return rfm, nil
}

// getUserStats 获取用户统计信息
// userID 实际为用户的 TgID（转换为 uint），通过 TgID 关联订单数据
func (s *RFMCalculatorService) getUserStats(ctx context.Context, userID uint) (*UserStats, error) {
	tgID := int64(userID)
	orders, err := s.orderRepo.GetByTgID(ctx, tgID)
	if err != nil {
		return nil, err
	}

	stats := &UserStats{
		UserID:           userID,
		TransactionCount: 0,
		TotalAmount:      0,
		AvgAmount:        0,
	}

	for _, order := range orders {
		// 仅统计已支付订单（status=1 已支付, status=2 强制成功）
		if order.Status != 1 && order.Status != 2 {
			continue
		}
		stats.TransactionCount++

		// Price 是 string 类型（单位：元），解析后转换为分（int64）
		// 规范依据：MASTER_RULES.md「金额一律用 BIGINT 存「分」」
		amount, err := strconv.ParseFloat(order.Price, 64)
		if err == nil {
			// 元 → 分，四舍五入避免精度损失
			stats.TotalAmount += int64(math.Round(amount * 100))
		}

		// 记录最后交易时间（CreateTime 为 unix 时间戳）
		if order.CreateTime > 0 {
			t := time.Unix(order.CreateTime, 0)
			if stats.LastTransactionAt == nil || t.After(*stats.LastTransactionAt) {
				stats.LastTransactionAt = &t
			}
		}
	}

	if stats.TransactionCount > 0 {
		stats.AvgAmount = stats.TotalAmount / int64(stats.TransactionCount)
	}

	return stats, nil
}

// calcRScore 计算 R 得分（Recency - 最近一次消费时间）
func (s *RFMCalculatorService) calcRScore(ctx context.Context, lastTransactionAt *time.Time, rule *model.RFMRule) int {
	if lastTransactionAt == nil {
		return 1
	}

	days := int(time.Since(*lastTransactionAt).Hours() / 24)

	if days <= rule.RDays1 {
		return 5
	} else if days <= rule.RDays2 {
		return 4
	} else if days <= rule.RDays3 {
		return 3
	} else if days <= rule.RDays4 {
		return 2
	} else if days <= rule.RDays5 {
		return 1
	}
	return 1
}

// calcFScore 计算 F 得分（Frequency - 消费频次）
func (s *RFMCalculatorService) calcFScore(ctx context.Context, transactionCount int, rule *model.RFMRule) int {
	if transactionCount == 0 {
		return 1
	}

	if transactionCount >= rule.FCount5 {
		return 5
	} else if transactionCount >= rule.FCount4 {
		return 4
	} else if transactionCount >= rule.FCount3 {
		return 3
	} else if transactionCount >= rule.FCount2 {
		return 2
	} else if transactionCount >= rule.FCount1 {
		return 1
	}
	return 1
}

// calcMScore 计算 M 得分（Monetary - 消费金额）
// totalAmount 单位：分
func (s *RFMCalculatorService) calcMScore(ctx context.Context, totalAmount int64, rule *model.RFMRule) int {
	if totalAmount == 0 {
		return 1
	}

	if totalAmount >= rule.MAmount5 {
		return 5
	} else if totalAmount >= rule.MAmount4 {
		return 4
	} else if totalAmount >= rule.MAmount3 {
		return 3
	} else if totalAmount >= rule.MAmount2 {
		return 2
	} else if totalAmount >= rule.MAmount1 {
		return 1
	}
	return 1
}

// determineLayer 确定用户分层
func (s *RFMCalculatorService) determineLayer(ctx context.Context, rScore, fScore, mScore int, lastTransactionAt *time.Time) model.RFMLayer {
	// 检查是否为新用户
	if fScore == 1 && rScore >= 4 {
		return model.RFMLayerNew
	}

	// 检查是否流失
	if lastTransactionAt != nil {
		days := int(time.Since(*lastTransactionAt).Hours() / 24)
		if days > 90 {
			return model.RFMLayerLost
		} else if days > 60 {
			return model.RFMLayerSleep
		}
	}

	// 根据 RFM 得分确定分层
	// R 得分高表示最近有消费（<=30 天），F/M 得分高表示频次/金额高
	isHighR := rScore >= 3
	isHighF := fScore >= 3
	isHighM := mScore >= 3

	if isHighR && isHighF && isHighM {
		return model.RFMLayerImportantValue
	} else if !isHighR && isHighF && isHighM {
		return model.RFMLayerImportantKeep
	} else if isHighR && !isHighF && isHighM {
		return model.RFMLayerImportantDevelop
	} else if !isHighR && !isHighF && isHighM {
		return model.RFMLayerImportantStay
	} else if isHighR && isHighF && !isHighM {
		return model.RFMLayerGeneralValue
	} else if !isHighR && isHighF && !isHighM {
		return model.RFMLayerGeneralKeep
	} else if isHighR && !isHighF && !isHighM {
		return model.RFMLayerGeneralDevelop
	}
	return model.RFMLayerGeneralStay
}

// 独立部署版本：单租户，无 merchant_id
// CalculateAllUsers 计算所有用户的 RFM
func (s *RFMCalculatorService) CalculateAllUsers(ctx context.Context) (int, error) {
	// 获取活跃规则
	rule, err := s.rfmRuleRepo.GetActiveRule(ctx)
	if err != nil {
		// 如果没有活跃规则，使用默认规则
		rule = s.getDefaultRule(ctx)
	}

	// Get all users with transactions
	// In production, this would query users who have placed orders
	users, err := s.getAllUserIDs(ctx)
	if err != nil {
		return 0, err
	}

	updatedCount := 0
	for _, userID := range users {
		rfm, err := s.CalculateRFM(ctx, userID, rule)
		if err != nil {
			continue
		}

		// 保存或更新 RFM 记录
		existing, _ := s.userRfmRepo.GetByUserID(ctx, userID)
		if existing != nil {
			rfm.ID = existing.ID
			err = s.userRfmRepo.Update(ctx, rfm)
		} else {
			err = s.userRfmRepo.Create(ctx, rfm)
		}

		if err == nil {
			updatedCount++
		}
	}

	return updatedCount, nil
}

// getDefaultRule 获取默认规则
// 金额字段单位：分（100 元 = 10000 分）
func (s *RFMCalculatorService) getDefaultRule(ctx context.Context) *model.RFMRule {
	return &model.RFMRule{
		RDays1: 7, RDays2: 14, RDays3: 30, RDays4: 60, RDays5: 90,
		FCount1: 1, FCount2: 3, FCount3: 5, FCount4: 10, FCount5: 20,
		MAmount1: 10000, MAmount2: 50000, MAmount3: 100000, MAmount4: 500000, MAmount5: 1000000,
		IsActive: true,
	}
}

// getAllUserIDs 获取所有有交易记录的用户 ID
// 从已支付订单中提取不同的 TgID，转换为 uint 作为用户标识
func (s *RFMCalculatorService) getAllUserIDs(ctx context.Context) ([]uint, error) {
	tgIDs, err := s.orderRepo.GetDistinctPaidTgIDs(ctx)
	if err != nil {
		return nil, err
	}

	userIDs := make([]uint, 0, len(tgIDs))
	for _, tgID := range tgIDs {
		if tgID > 0 {
			userIDs = append(userIDs, uint(tgID))
		}
	}

	return userIDs, nil
}

// GetRFMRule 获取 RFM 规则
func (s *RFMCalculatorService) GetRFMRule(ctx context.Context) (*model.RFMRule, error) {
	return s.rfmRuleRepo.GetActiveRule(ctx)
}

// ListRFMRules 列出所有 RFM 规则（用于分群管理页）
func (s *RFMCalculatorService) ListRFMRules(ctx context.Context, page, pageSize int) ([]*model.RFMRule, int64, error) {
	all, err := s.rfmRuleRepo.GetAll(ctx)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(all))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []*model.RFMRule{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

// DeleteRFMRule 删除 RFM 规则
func (s *RFMCalculatorService) DeleteRFMRule(ctx context.Context, id uint) error {
	return s.rfmRuleRepo.Delete(ctx, id)
}

// SaveRFMRule 保存 RFM 规则
func (s *RFMCalculatorService) SaveRFMRule(ctx context.Context, req *SaveRFMRuleRequest) (*model.RFMRule, error) {
	rule := &model.RFMRule{
		Name:     req.Name,
		RDays1:   req.RDays1,
		RDays2:   req.RDays2,
		RDays3:   req.RDays3,
		RDays4:   req.RDays4,
		RDays5:   req.RDays5,
		FCount1:  req.FCount1,
		FCount2:  req.FCount2,
		FCount3:  req.FCount3,
		FCount4:  req.FCount4,
		FCount5:  req.FCount5,
		MAmount1: req.MAmount1,
		MAmount2: req.MAmount2,
		MAmount3: req.MAmount3,
		MAmount4: req.MAmount4,
		MAmount5: req.MAmount5,
		IsActive: req.IsActive,
	}

	// 如果规则无效，使用默认值
	if rule.RDays1 <= 0 {
		rule.RDays1 = 7
	}

	err := s.rfmRuleRepo.Create(ctx, rule)
	if err != nil {
		return nil, err
	}

	return rule, nil
}

// SaveRFMRuleRequest 保存 RFM 规则请求
// 金额字段单位：分（前端展示时 / 100 转元）
type SaveRFMRuleRequest struct {
	Name     string `json:"name"`
	RDays1   int    `json:"r_days_1"`
	RDays2   int    `json:"r_days_2"`
	RDays3   int    `json:"r_days_3"`
	RDays4   int    `json:"r_days_4"`
	RDays5   int    `json:"r_days_5"`
	FCount1  int    `json:"f_count_1"`
	FCount2  int    `json:"f_count_2"`
	FCount3  int    `json:"f_count_3"`
	FCount4  int    `json:"f_count_4"`
	FCount5  int    `json:"f_count_5"`
	MAmount1 int64  `json:"m_amount_1"`
	MAmount2 int64  `json:"m_amount_2"`
	MAmount3 int64  `json:"m_amount_3"`
	MAmount4 int64  `json:"m_amount_4"`
	MAmount5 int64  `json:"m_amount_5"`
	IsActive bool   `json:"is_active"`
}

// UpdateRFMRule 更新 RFM 规则
func (s *RFMCalculatorService) UpdateRFMRule(ctx context.Context, id uint, req *SaveRFMRuleRequest) (*model.RFMRule, error) {
	rule, err := s.rfmRuleRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("规则不存在")
	}

	// 独立部署版本：无权限校验
	rule.Name = req.Name
	rule.RDays1 = req.RDays1
	rule.RDays2 = req.RDays2
	rule.RDays3 = req.RDays3
	rule.RDays4 = req.RDays4
	rule.RDays5 = req.RDays5
	rule.FCount1 = req.FCount1
	rule.FCount2 = req.FCount2
	rule.FCount3 = req.FCount3
	rule.FCount4 = req.FCount4
	rule.FCount5 = req.FCount5
	rule.MAmount1 = req.MAmount1
	rule.MAmount2 = req.MAmount2
	rule.MAmount3 = req.MAmount3
	rule.MAmount4 = req.MAmount4
	rule.MAmount5 = req.MAmount5
	rule.IsActive = req.IsActive

	if err := s.rfmRuleRepo.Update(ctx, rule); err != nil {
		return nil, err
	}

	return rule, nil
}

// GetRFMStats 获取 RFM 统计
func (s *RFMCalculatorService) GetRFMStats(ctx context.Context) (map[string]any, error) {
	layerCount, err := s.userRfmRepo.GetLayerCount(ctx)
	if err != nil {
		return nil, err
	}

	totalUsers := 0
	for _, count := range layerCount {
		totalUsers += int(count)
	}

	return map[string]any{
		"total_users": totalUsers,
		"layer_count": layerCount,
		"layer_names": map[string]string{
			"important_value":   "重要价值用户",
			"important_keep":    "重要保持用户",
			"important_develop": "重要发展用户",
			"important_stay":    "重要挽留用户",
			"general_value":     "一般价值用户",
			"general_keep":      "一般保持用户",
			"general_develop":   "一般发展用户",
			"general_stay":      "一般挽留用户",
			"new":               "新用户",
			"sleep":             "沉睡用户",
			"lost":              "流失用户",
		},
	}, nil
}

// UserRFMWithUser 带用户信息的 RFM
type UserRFMWithUser struct {
	UserRFM   *model.UserRFM `json:"rfm"`
	UserName  string         `json:"user_name"`
	UserPhone string         `json:"user_phone"`
	UserEmail string         `json:"user_email"`
	LayerDesc string         `json:"layer_desc"`
}

// GetUsersByLayer 根据分层获取用户列表
func (s *RFMCalculatorService) GetUsersByLayer(ctx context.Context, layer string, page, pageSize int) ([]*UserRFMWithUser, int64, error) {
	rfms, total, err := s.userRfmRepo.GetByLayer(ctx, layer, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	return s.enrichUserData(ctx, rfms), total, nil
}

// GetRFMList 获取用户 RFM 列表
func (s *RFMCalculatorService) GetRFMList(ctx context.Context, page, pageSize int) ([]*UserRFMWithUser, int64, error) {
	rfms, total, err := s.userRfmRepo.GetAll(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	return s.enrichUserData(ctx, rfms), total, nil
}

// GetUserRFM 获取单个用户的 RFM
func (s *RFMCalculatorService) GetUserRFM(ctx context.Context, userID uint) (*UserRFMWithUser, error) {
	rfm, err := s.userRfmRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := s.enrichUserData(ctx, []*model.UserRFM{rfm})
	if len(result) == 0 {
		return nil, errors.New("未找到用户 RFM 信息")
	}

	return result[0], nil
}

// enrichUserData 丰富用户数据
// 通过 TgID 查询用户信息，填充姓名、电话、邮箱
func (s *RFMCalculatorService) enrichUserData(ctx context.Context, rfms []*model.UserRFM) []*UserRFMWithUser {
	result := make([]*UserRFMWithUser, 0, len(rfms))

	for _, rfm := range rfms {
		item := &UserRFMWithUser{
			UserRFM:   rfm,
			LayerDesc: model.GetLayerDescription(model.RFMLayer(rfm.Layer)),
		}

		// 通过 TgID 查询用户详情
		tgID := int64(rfm.UserID)
		if tgID > 0 {
			user, err := s.userRepo.GetByTgID(ctx, tgID)
			if err == nil && user != nil && user.ID != "" {
				if user.RealName != "" {
					item.UserName = user.RealName
				} else if user.FirstName != "" || user.LastName != "" {
					item.UserName = user.FirstName + " " + user.LastName
				} else if user.UserName != "" {
					item.UserName = user.UserName
				}
				item.UserPhone = user.Phone
				item.UserEmail = user.Email
			}
		}

		result = append(result, item)
	}

	return result
}
