package service

// sms_delivery_tracker.go 短信到达率追踪服务（E 域 P1 缺口 #2）
//
// 五层架构归属: L3 业务层
// 设计依据: docs/marketing-features/sms-config.md + 核心链路优化 §15.2
//
// 与 sms_tracking.go（通用送达状态记录）的关系：
//   - sms_tracking.go         通用状态记录：成功/失败/可重试 + 重试
//   - 本文件                  专注于"到达率"维度：
//                              * 携号转网（Number Portability）追踪
//                              * 黑名单维度（运营商级 / 业务级）
//                              * 到达率 / 失败率批量聚合
//                              * 多家运营商回执兼容（阿里云/腾讯云/华为云）
//
// 私域独立部署: 无 merchant_id 字段
//
// 合规要点：
//   - 携号转网：用户携号转网后，运营商回执的 carrier 字段会变化；
//     本服务追踪 carrier 变化以发现"号码已换运营商"，对运营触达策略很重要。
//   - 黑名单：分运营商级（如 ERR_4002）与业务级（用户主动退订），分别记录。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// ----------------------------------------------------------------------------
// 常量
// ----------------------------------------------------------------------------

// SmsCarrier 运营商
type SmsCarrier string

const (
	SmsCarrierMobile  SmsCarrier = "mobile"  // 中国移动
	SmsCarrierUnicom  SmsCarrier = "unicom"  // 中国联通
	SmsCarrierTelecom SmsCarrier = "telecom" // 中国电信
	SmsCarrierUnknown SmsCarrier = "unknown" // 未知 / 携号转网过渡
)

// SmsBlockType 黑名单类型
type SmsBlockType string

const (
	SmsBlockCarrier    SmsBlockType = "carrier"    // 运营商级黑名单（如 ERR_4002）
	SmsBlockBusiness   SmsBlockType = "business"   // 业务级（用户主动退订）
	SmsBlockRegulatory SmsBlockType = "regulatory" // 监管/合规黑名单
	SmsBlockContent    SmsBlockType = "content"    // 内容违规
)

// SmsNumberPortabilityRecord 携号转网记录
type SmsNumberPortabilityRecord struct {
	ID              int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Phone           string     `gorm:"column:phone;size:20;not null;index" json:"phone"`
	OriginalCarrier SmsCarrier `gorm:"column:original_carrier;size:32" json:"original_carrier"`
	CurrentCarrier  SmsCarrier `gorm:"column:current_carrier;size:32" json:"current_carrier"`
	DetectedAt      time.Time  `gorm:"column:detected_at;not null;index:idx_sms_np_detected_at,sort:desc" json:"detected_at"`
	Source          string     `gorm:"column:source;size:32;not null;default:'webhook'" json:"source"`
	RawPayload      string     `gorm:"column:raw_payload;type:text" json:"raw_payload"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

// TableName 表名
func (SmsNumberPortabilityRecord) TableName() string { return "sms_number_portability_logs" }

// ----------------------------------------------------------------------------
// 服务结构
// ----------------------------------------------------------------------------

// SmsDeliveryTrackerService 短信到达率追踪服务
type SmsDeliveryTrackerService struct {
	tracking *SmsTrackingService
	repo     repository.SmsTrackingRepository
	db       *gorm.DB

	// 携号转网内存缓存：phone → 最新运营商（避免每次 webhook 走 DB）
	carrierMu     sync.RWMutex
	carrierCache  map[string]SmsCarrier
	carrierLoaded bool
}

// NewSmsDeliveryTrackerService 创建短信到达率追踪服务
func NewSmsDeliveryTrackerService(db *gorm.DB, tracking *SmsTrackingService, repo repository.SmsTrackingRepository) *SmsDeliveryTrackerService {
	if tracking == nil {
		tracking = NewSmsTrackingService(repo)
	}
	if repo == nil {
		repo = repository.NewSmsTrackingRepository(db)
	}
	return &SmsDeliveryTrackerService{
		db:            db,
		tracking:      tracking,
		repo:          repo,
		carrierCache:  make(map[string]SmsCarrier),
		carrierLoaded: false,
	}
}

// ----------------------------------------------------------------------------
// 携号转网追踪
// ----------------------------------------------------------------------------

// DetectCarrierFromPhone 简单规则识别运营商（前 7 位号段）
//
// 真实生产应使用第三方号段库（如 https://github.com/ls0f/phone）
// 此处为不引入新依赖的简化版本，覆盖主流号段。
func DetectCarrierFromPhone(phone string) SmsCarrier {
	phone = NormalizePhone(phone)
	if len(phone) < 7 {
		return SmsCarrierUnknown
	}
	prefix := phone[:7]
	switch {
	case hasPrefix(prefix, []string{"134", "135", "136", "137", "138", "139", "150", "151", "152", "157", "158", "159", "182", "183", "184", "187", "188", "198"}):
		return SmsCarrierMobile
	case hasPrefix(prefix, []string{"130", "131", "132", "155", "156", "166", "175", "176", "185", "186"}):
		return SmsCarrierUnicom
	case hasPrefix(prefix, []string{"133", "149", "153", "173", "174", "177", "180", "181", "189", "199"}):
		return SmsCarrierTelecom
	}
	return SmsCarrierUnknown
}

// hasPrefix 判断 prefix 是否匹配任一前缀列表
func hasPrefix(prefix string, candidates []string) bool {
	for _, c := range candidates {
		if len(c) <= len(prefix) && prefix[:len(c)] == c {
			return true
		}
	}
	return false
}

// DetectAndRecordPortability 检测并记录携号转网
//
// 流程：
//  1. 读取运营商 webhook 推送的 carrier（权威）
//  2. 与内存缓存的"上一次运营商"对比
//  3. 若发生变化 → 写入 sms_number_portability_logs
func (s *SmsDeliveryTrackerService) DetectAndRecordPortability(ctx context.Context, phone, webhookCarrier string) error {
	phone = NormalizePhone(phone)
	if phone == "" {
		return errors.New("phone 不能为空")
	}

	// 1) 优先使用 webhook 推送的 carrier
	var newCarrier SmsCarrier
	switch strings.ToLower(strings.TrimSpace(webhookCarrier)) {
	case "mobile", "中国移动", "cmcc", "yidong":
		newCarrier = SmsCarrierMobile
	case "unicom", "中国联通", "cu", "liantong":
		newCarrier = SmsCarrierUnicom
	case "telecom", "中国电信", "ct", "dianxin":
		newCarrier = SmsCarrierTelecom
	default:
		// webhook 未给 → 用号段兜底
		newCarrier = DetectCarrierFromPhone(phone)
	}
	if newCarrier == SmsCarrierUnknown {
		return nil // 无法识别就不记录
	}

	// 2) 与缓存对比
	s.carrierMu.Lock()
	original, exists := s.carrierCache[phone]
	s.carrierCache[phone] = newCarrier
	s.carrierMu.Unlock()

	if !exists {
		// 首次记录：从 DB 加载历史
		s.loadCarrierCache()
		s.carrierMu.RLock()
		original, exists = s.carrierCache[phone]
		s.carrierMu.RUnlock()
		s.carrierCache[phone] = newCarrier // 还原
	}

	if exists && original == newCarrier {
		return nil // 运营商未变化
	}

	// 3) 写入转网记录
	rec := &SmsNumberPortabilityRecord{
		Phone:           phone,
		OriginalCarrier: original,
		CurrentCarrier:  newCarrier,
		DetectedAt:      time.Now(),
		Source:          "webhook",
		RawPayload:      fmt.Sprintf(`{"phone":%q,"carrier":%q}`, phone, webhookCarrier),
		CreatedAt:       time.Now(),
	}
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).Create(rec).Error
}

// loadCarrierCache 加载最近一次运营商快照
func (s *SmsDeliveryTrackerService) loadCarrierCache() {
	if s.carrierLoaded || s.db == nil {
		return
	}
	rows := []SmsNumberPortabilityRecord{}
	if err := s.db.Order("detected_at DESC").Limit(10000).Find(&rows).Error; err != nil {
		logger.Errorf("[SmsDeliveryTracker] load carrier cache: %v", err)
		s.carrierLoaded = true
		return
	}
	s.carrierMu.Lock()
	for _, r := range rows {
		if _, ok := s.carrierCache[r.Phone]; !ok {
			s.carrierCache[r.Phone] = r.CurrentCarrier
		}
	}
	s.carrierMu.Unlock()
	s.carrierLoaded = true
}

// GetCurrentCarrier 查询号码当前归属运营商
func (s *SmsDeliveryTrackerService) GetCurrentCarrier(phone string) SmsCarrier {
	phone = NormalizePhone(phone)
	s.carrierMu.RLock()
	if c, ok := s.carrierCache[phone]; ok {
		s.carrierMu.RUnlock()
		return c
	}
	s.carrierMu.RUnlock()

	// fallback：从号段识别
	return DetectCarrierFromPhone(phone)
}

// ListPortabilityRecords 列出携号转网记录（分页 + 手机号过滤）
//
// 设计：直接查询 sms_number_portability_logs，不走内存缓存。
// 限流：limit 上限 200，避免一次拉全表。
func (s *SmsDeliveryTrackerService) ListPortabilityRecords(ctx context.Context, phone string, page, limit int) ([]SmsNumberPortabilityRecord, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, errors.New("service or db is nil")
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 20
	}

	q := s.db.WithContext(ctx).Model(&SmsNumberPortabilityRecord{})
	if phone != "" {
		q = q.Where("phone = ?", NormalizePhone(phone))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count portability: %w", err)
	}

	var rows []SmsNumberPortabilityRecord
	if err := q.Order("detected_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list portability: %w", err)
	}
	return rows, total, nil
}

// ----------------------------------------------------------------------------
// 黑名单维度追踪
// ----------------------------------------------------------------------------

// SmsBlacklistRecord 黑名单记录（聚合）
type SmsBlacklistRecord struct {
	Phone     string       `json:"phone"`
	BlockType SmsBlockType `json:"block_type"`
	Reason    string       `json:"reason"`
	ErrorCode string       `json:"error_code"`
	BlockedAt time.Time    `json:"blocked_at"`
	JobID     string       `json:"job_id"`
	MessageID string       `json:"message_id"`
}

// RecordBlacklistEvent 记录黑名单事件
//
// 触发场景：
//   - 运营商回执 ERR_4002 → carrier 级黑名单
//   - 用户回复 TD 退订   → business 级黑名单
//   - 内容审核违规       → content / regulatory 级
func (s *SmsDeliveryTrackerService) RecordBlacklistEvent(ctx context.Context, phone, errorCode, errorMsg, jobID, messageID string) {
	phone = NormalizePhone(phone)
	if phone == "" {
		return
	}
	rec := SmsBlacklistRecord{
		Phone:     phone,
		ErrorCode: errorCode,
		Reason:    errorMsg,
		BlockedAt: time.Now(),
		JobID:     jobID,
		MessageID: messageID,
	}
	switch {
	case strings.HasPrefix(errorCode, "ERR_4002"):
		rec.BlockType = SmsBlockCarrier
	case strings.HasPrefix(errorCode, "ERR_4003"):
		rec.BlockType = SmsBlockContent
	case strings.HasPrefix(errorCode, "ERR_4004"):
		rec.BlockType = SmsBlockRegulatory
	default:
		rec.BlockType = SmsBlockBusiness
	}
	logger.Infof("[SmsDeliveryTracker] blacklist: %+v", rec)
}

// ----------------------------------------------------------------------------
// 到达率聚合
// ----------------------------------------------------------------------------

// DeliveryRateMetrics 到达率指标
type DeliveryRateMetrics struct {
	WindowStart  time.Time              `json:"window_start"`
	WindowEnd    time.Time              `json:"window_end"`
	TotalSent    int64                  `json:"total_sent"`
	Delivered    int64                  `json:"delivered"`
	Failed       int64                  `json:"failed"`
	Retryable    int64                  `json:"retryable"`
	Blacklisted  int64                  `json:"blacklisted"`   // 黑名单触达失败
	Portability  int64                  `json:"portability"`   // 携号转网触达失败
	DeliveryRate float64                `json:"delivery_rate"` // delivered / total * 100
	FailureRate  float64                `json:"failure_rate"`
	ByCarrier    map[string]CarrierStat `json:"by_carrier"`
}

// CarrierStat 单运营商统计
type CarrierStat struct {
	Total        int64   `json:"total"`
	Delivered    int64   `json:"delivered"`
	Failed       int64   `json:"failed"`
	DeliveryRate float64 `json:"delivery_rate"`
}

// GetDeliveryRateMetrics 聚合时间窗口内的到达率
func (s *SmsDeliveryTrackerService) GetDeliveryRateMetrics(ctx context.Context, start, end time.Time) (*DeliveryRateMetrics, error) {
	if start.IsZero() || end.IsZero() {
		return nil, errors.New("start / end 不能为空")
	}
	if end.Before(start) {
		return nil, errors.New("end 必须大于 start")
	}
	if s.db == nil {
		return nil, errors.New("db is nil")
	}

	m := &DeliveryRateMetrics{
		WindowStart: start,
		WindowEnd:   end,
		ByCarrier:   make(map[string]CarrierStat),
	}

	// 1) 基础聚合
	type aggRow struct {
		Total     int64
		Delivered int64
		Failed    int64
		Retryable int64
	}
	var row aggRow
	if err := s.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ?", start, end).
		Select(`
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed,
			COUNT(*) FILTER (WHERE status = 'retryable') AS retryable
		`).
		Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("query delivery aggregate: %w", err)
	}
	m.TotalSent = row.Total
	m.Delivered = row.Delivered
	m.Failed = row.Failed
	m.Retryable = row.Retryable
	if m.TotalSent > 0 {
		m.DeliveryRate = round2(float64(m.Delivered) / float64(m.TotalSent) * 100)
		m.FailureRate = round2(float64(m.Failed+m.Retryable) / float64(m.TotalSent) * 100)
	}

	// 2) 黑名单 / 携号转网触达失败
	var blacklisted int64
	_ = s.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ? AND error_code LIKE ?", start, end, "ERR_4002%").
		Count(&blacklisted).Error
	m.Blacklisted = blacklisted

	// 携号转网触达失败（号码已不存在/换运营商）
	var port int64
	_ = s.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ? AND (error_code = 'ERR_4005' OR error_msg LIKE '%携号转网%')", start, end).
		Count(&port).Error
	m.Portability = port

	// 3) 按运营商维度统计
	type carrierRow struct {
		Provider  string
		Total     int64
		Delivered int64
		Failed    int64
	}
	var crows []carrierRow
	if err := s.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ?", start, end).
		Select(`
			COALESCE(provider, 'unknown') AS provider,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed
		`).
		Group("provider").
		Scan(&crows).Error; err == nil {
		for _, cr := range crows {
			cs := CarrierStat{
				Total:     cr.Total,
				Delivered: cr.Delivered,
				Failed:    cr.Failed,
			}
			if cr.Total > 0 {
				cs.DeliveryRate = round2(float64(cr.Delivered) / float64(cr.Total) * 100)
			}
			m.ByCarrier[cr.Provider] = cs
		}
	}

	return m, nil
}

// ----------------------------------------------------------------------------
// 多家运营商回执兼容
// ----------------------------------------------------------------------------

// ProviderDeliveryReport 兼容的运营商回执统一格式
type ProviderDeliveryReport struct {
	MessageID   string `json:"messageId"`
	Phone       string `json:"phone"`
	JobID       string `json:"jobId"`
	Provider    string `json:"provider"`
	Carrier     string `json:"carrier"` // 携号转网维度
	Status      string `json:"status"`
	ErrorCode   string `json:"errorCode"`
	ErrorMsg    string `json:"errorMsg"`
	SentAt      string `json:"sentAt"`
	DeliveredAt string `json:"deliveredAt"`
	RawPayload  string `json:"rawPayload"`
}

// RecordFromProvider 统一的运营商回执接收入口
//
// 自动：
//  1. 转换为内部 DeliveryReportRequest 走原 tracking 路径
//  2. 检测并记录携号转网
//  3. 记录黑名单事件
func (s *SmsDeliveryTrackerService) RecordFromProvider(ctx context.Context, r *ProviderDeliveryReport) error {
	if r == nil {
		return errors.New("report is nil")
	}
	if r.MessageID == "" {
		return errors.New("messageId 不能为空")
	}

	// 1) 走原 tracking 路径
	req := &DeliveryReportRequest{
		MessageID:   r.MessageID,
		Phone:       r.Phone,
		JobID:       r.JobID,
		Provider:    r.Provider,
		Status:      r.Status,
		ErrorCode:   r.ErrorCode,
		ErrorMsg:    r.ErrorMsg,
		SentAt:      r.SentAt,
		DeliveredAt: r.DeliveredAt,
	}
	if err := s.tracking.RecordDeliveryReport(ctx, req); err != nil {
		logger.Errorf("[SmsDeliveryTracker] record delivery report: %v", err)
	}

	// 2) 检测携号转网（异步不阻塞主流程）
	go func(phone, carrier string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.DetectAndRecordPortability(bgCtx, phone, carrier)
	}(r.Phone, r.Carrier)

	// 3) 黑名单事件
	if r.ErrorCode != "" {
		s.RecordBlacklistEvent(ctx, r.Phone, r.ErrorCode, r.ErrorMsg, r.JobID, r.MessageID)
	}

	return nil
}

// ----------------------------------------------------------------------------
// JSON 序列化辅助
// ----------------------------------------------------------------------------

// MarshalReport 序列化原始回执（用于持久化到 raw_payload 字段）
func MarshalReport(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
