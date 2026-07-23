package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// CustomerRFMService 客户 RFM 服务
type CustomerRFMService struct {
	rfmRepo		repository.CustomerRFMRepository
	customerRepo	repository.CustomerRepository
	orderRepo	repository.OrderRepository
	recoveryRepo	repository.RecoveryQueueRepository
	nowFunc		func() time.Time
}

// NewCustomerRFMService 创建 RFM 服务
func NewCustomerRFMService() *CustomerRFMService {
	return &CustomerRFMService{
		rfmRepo:	repository.NewCustomerRFMRepository(),
		customerRepo:	repository.NewCustomerRepository(),
		orderRepo:	repository.NewOrderRepository(),
		recoveryRepo:	repository.NewRecoveryQueueRepository(),
		nowFunc:	time.Now,
	}
}

// NewCustomerRFMServiceWithRepos 测试用
func NewCustomerRFMServiceWithRepos(
	rfm repository.CustomerRFMRepository,
	cust repository.CustomerRepository,
	ord repository.OrderRepository,
	rec repository.RecoveryQueueRepository,
) *CustomerRFMService {
	return &CustomerRFMService{
		rfmRepo:	rfm,
		customerRepo:	cust,
		orderRepo:	ord,
		recoveryRepo:	rec,
		nowFunc:	time.Now,
	}
}

// RFMConfig RFM 评分配置
type RFMConfig struct {
	// R (Recency) 阈值：距今 <= N 天 → 5 分，依次类推
	RecencyBuckets	[]int	// [7, 30, 90, 180] 默认；<=7→5, <=30→4, <=90→3, <=180→2, >180→1
	// F (Frequency) 阈值：>= N 次 → 5 分
	FrequencyBuckets	[]int	// [10, 5, 3, 1] 默认
	// M (Monetary) 阈值：>= N 分 → 5 分
	MonetaryBuckets	[]int64	// [500000, 100000, 30000, 5000] 默认
	// 流失阈值：recency_days >= N 视为流失
	ChurnRecencyThreshold	int	// 默认 180
	// 自动入挽回队列开关
	AutoEnqueueRecovery	bool
}

// DefaultRFMConfig 默认 RFM 配置
func DefaultRFMConfig() RFMConfig {
	return RFMConfig{
		RecencyBuckets:		[]int{7, 30, 90, 180},
		FrequencyBuckets:	[]int{10, 5, 3, 1},
		MonetaryBuckets:	[]int64{500000, 100000, 30000, 5000},	// 5000元/1000元/300元/50元（分单位）
		ChurnRecencyThreshold:	180,
		AutoEnqueueRecovery:	true,
	}
}

// ComputeForCustomer 计算单个客户的 RFM
func (s *CustomerRFMService) ComputeForCustomer(ctx context.Context, customerID string, cfg RFMConfig) (*model.CustomerRFM, error) {
	if customerID == "" {
		return nil, errors.New("customer_id 不能为空")
	}
	cust, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if cust == nil {
		return nil, errors.New("客户不存在")
	}

	// 解析 account_id（用于 order 关联）
	accountID := customerID
	if cust.Phone != "" {
		accountID = cust.Phone
	}

	// 计算 R / F / M（基于 order 表 + account_id）
	r, f, m, lastActive, err := s.computeRawMetrics(ctx, accountID)
	if err != nil {
		return nil, err
	}

	// 1-5 分映射
	rs := rfmScoreRecency(r, cfg.RecencyBuckets)
	fs := rfmScoreFrequency(f, cfg.FrequencyBuckets)
	ms := rfmScoreMonetary(m, cfg.MonetaryBuckets)

	// 综合 0-100：R 30% + F 30% + M 40%
	composite := int(float64(rs)*30 + float64(fs)*30 + float64(ms)*40)
	if composite < 0 {
		composite = 0
	}
	if composite > 100 {
		composite = 100
	}

	segment := determineSegment(rs, fs, ms, r, cfg.ChurnRecencyThreshold)
	churnRisk, churnScore := calcChurnRisk(r, f, m, cfg)

	avgOrder := int64(0)
	if f > 0 {
		avgOrder = m / int64(f)
	}

	rfm := &model.CustomerRFM{
		CustomerID:	customerID,
		UnifiedID:	cust.UnifiedID,
		RecencyDays:	r,
		Frequency:	f,
		MonetaryTotal:	m,
		AvgOrderValue:	avgOrder,
		RScore:		rs,
		FScore:		fs,
		MScore:		ms,
		CompositeScore:	composite,
		Segment:	segment,
		ChurnRiskLevel:	churnRisk,
		ChurnScore:	churnScore,
		LastActiveAt:	lastActive,
		ComputedAt:	s.nowFunc(),
	}
	if err := s.rfmRepo.Upsert(ctx, rfm); err != nil {
		return nil, err
	}

	// 流失客户自动入挽回队列
	if cfg.AutoEnqueueRecovery && segment == model.RFMSegmentChurn {
		s.enqueueRecovery(ctx, rfm)
	}
	return rfm, nil
}

// ComputeAll 计算所有客户 RFM（limit 限制）
func (s *CustomerRFMService) ComputeAll(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	customers, _, err := s.customerRepo.List(ctx, 1, limit)
	if err != nil {
		return 0, err
	}
	cfg := DefaultRFMConfig()
	success := 0
	for _, c := range customers {
		if _, err := s.ComputeForCustomer(ctx, c.ID, cfg); err != nil {
			return success, err
		}
		success++
	}
	return success, nil
}

// GetByCustomerID 查询客户 RFM
func (s *CustomerRFMService) GetByCustomerID(ctx context.Context, customerID string) (*model.CustomerRFM, error) {
	return s.rfmRepo.GetByCustomerID(ctx, customerID)
}

// ListBySegment 分层分页查询
func (s *CustomerRFMService) ListBySegment(ctx context.Context, segment string, page, pageSize int) ([]*model.CustomerRFM, int64, error) {
	return s.rfmRepo.ListBySegment(ctx, segment, page, pageSize)
}

// Distribution 分层统计
func (s *CustomerRFMService) Distribution(ctx context.Context) (map[string]int64, error) {
	return s.rfmRepo.CountBySegment(ctx)
}

// computeRawMetrics 计算 R/F/M + 最后活跃时间
// 简化版：使用 orderRepo.GetDistinctPaidTgIDs / GetByTgID 走通全流程
// 注意：本系统 order 表使用 account_id 关联客户（独立部署）
func (s *CustomerRFMService) computeRawMetrics(ctx context.Context, accountID string) (recencyDays, frequency int, monetary int64, lastActive *time.Time, err error) {
	if accountID == "" {
		recencyDays = 9999
		return
	}
	// 1) 直接按 account_id 拉取该客户订单（GetByStringID 不可用，order 表无 string id 索引）
	// 简化：走 GetByTgID 仅当 accountID 可解析为 int64；否则返回 0
	tgID, _ := strconv.ParseInt(accountID, 10, 64)
	if tgID > 0 {
		orders, oerr := s.orderRepo.GetByTgID(ctx, tgID)
		if oerr != nil {
			err = oerr
			return
		}
		frequency = len(orders)
		for _, o := range orders {
			if o.Price == "" {
				continue
			}
			price, perr := strconv.ParseInt(o.Price, 10, 64)
			if perr != nil {
				continue
			}
			monetary += price
			if o.CreateTime > 0 {
				t := time.Unix(o.CreateTime, 0)
				if lastActive == nil || t.After(*lastActive) {
					lastActive = &t
				}
			}
		}
		if lastActive != nil {
			recencyDays = int(time.Since(*lastActive).Hours() / 24)
			if recencyDays < 0 {
				recencyDays = 0
			}
		}
		return
	}
	recencyDays = 9999
	return
}

// scoreRecency 评分 R：days 越小分越高
func rfmScoreRecency(days int, buckets []int) int {
	if len(buckets) == 0 {
		return 1
	}
	score := 1
	for _, b := range buckets {
		if days <= b {
			score++
		}
	}
	if score > 5 {
		score = 5
	}
	return score
}

// scoreFrequency 评分 F：次数越多分越高
func rfmScoreFrequency(freq int, buckets []int) int {
	if len(buckets) == 0 {
		return 1
	}
	score := 1
	for _, b := range buckets {
		if freq >= b {
			score++
		}
	}
	if score > 5 {
		score = 5
	}
	return score
}

// scoreMonetary 评分 M：金额越多分越高
func rfmScoreMonetary(m int64, buckets []int64) int {
	if len(buckets) == 0 {
		return 1
	}
	score := 1
	for _, b := range buckets {
		if m >= b {
			score++
		}
	}
	if score > 5 {
		score = 5
	}
	return score
}

// determineSegment 分层判定
// 优先级：champion > loyal > at_risk > churn > potential
func determineSegment(r, f, m, recencyDays, churnThreshold int) string {
	if recencyDays >= churnThreshold || (r <= 1 && f <= 1) {
		return model.RFMSegmentChurn
	}
	if r == 5 && f >= 4 {
		return model.RFMSegmentChampion
	}
	if r >= 4 && f >= 3 {
		return model.RFMSegmentLoyal
	}
	if r == 2 || (f >= 2 && m >= 3) {
		return model.RFMSegmentAtRisk
	}
	return model.RFMSegmentPotential
}

// calcChurnRisk 流失风险评估
//
//	churn_risk_level: low / medium / high
//	churn_score: 0-100，越高越可能流失
func calcChurnRisk(recencyDays, freq int, monetary int64, cfg RFMConfig) (string, int) {
	score := 0
	// recency 贡献 60 分（超过阈值直接加满 70）
	if recencyDays >= cfg.ChurnRecencyThreshold {
		score += 70
	} else if recencyDays >= cfg.ChurnRecencyThreshold/2 {
		score += 40
	} else if recencyDays >= 30 {
		score += 15
	}
	// frequency 反向贡献（次数越多越不会流失，最多减 25 分）
	if freq >= 5 {
		score -= 25
	} else if freq >= 2 {
		score -= 10
	}
	// monetary 反向贡献（最多减 15 分）
	if monetary >= 100000 {
		score -= 15
	} else if monetary >= 10000 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	switch {
	case score >= 70:
		return "high", score
	case score >= 40:
		return "medium", score
	default:
		return "low", score
	}
}

// enqueueRecovery 流失客户自动入挽回队列
func (s *CustomerRFMService) enqueueRecovery(ctx context.Context, rfm *model.CustomerRFM) {
	if s.recoveryRepo == nil || rfm == nil {
		return
	}
	// 1) 不重复入队
	if existing, _ := s.recoveryRepo.GetActiveByCustomerID(ctx, rfm.CustomerID); existing != nil {
		return
	}
	// 2) 推导优先级：churn_score 越高，priority 越小（越优先）
	priority := 5
	if rfm.ChurnScore >= 80 {
		priority = 1
	} else if rfm.ChurnScore >= 60 {
		priority = 2
	}
	// 3) 写入
	item := &model.RecoveryQueue{
		CustomerID:	rfm.CustomerID,
		UnifiedID:	rfm.UnifiedID,
		Reason:		"churn",
		Strategy:	"sms_coupon",
		Priority:	priority,
		Stage:		model.RecoveryStageQueued,
	}
	_ = s.recoveryRepo.Create(ctx, item)
}

// daysSinceUnix 计算 unix 时间戳距今的天数
// 保留以备外部调用方使用（_ 前缀避免 unused 告警）
func _daysSinceUnix(unixSec int64) int {
	if unixSec <= 0 {
		return 9999
	}
	t := time.Unix(unixSec, 0)
	days := int(time.Since(t).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}
