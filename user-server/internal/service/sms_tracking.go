package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// smsMaxRetryCount 最大重试次数（合规要求：避免无限重试骚扰用户）
const smsMaxRetryCount = 3

// smsRetryableErrorPrefixes 可重试错误码前缀
//
// 判定规则：
//   - ERR_6xxx（网关超时 / 运营商临时故障）→ 可重试
//   - ERR_5xxx（运营商限流 / 内部错误）→ 可重试（需延时）
//   - ERR_4xxx（号码无效 / 黑名单 / 内容违规）→ 不重试
//   - 其他未知错误码 → 不重试（保守策略）
var smsRetryableErrorPrefixes = []string{"ERR_6", "ERR_5"}

// smsNonRetryableErrorPrefixes 不可重试错误码前缀
var smsNonRetryableErrorPrefixes = []string{"ERR_4"}

// DeliveryReportRequest 运营商状态报告 webhook 请求体
type DeliveryReportRequest struct {
	MessageID   string `json:"messageId" binding:"required"`
	Phone       string `json:"phone"`
	JobID       string `json:"jobId"`
	Provider    string `json:"provider"`
	Status      string `json:"status" binding:"required"` // delivered / failed / pending / sent
	ErrorCode   string `json:"errorCode"`
	ErrorMsg    string `json:"errorMsg"`
	SentAt      string `json:"sentAt"`
	DeliveredAt string `json:"deliveredAt"`
}

// SmsTrackingService 短信追踪服务
//
// 职责：
//   - RecordDeliveryReport：接收运营商状态报告 webhook
//   - IsRetryable：判断错误码是否可重试
//   - GetJobMetrics：返回任务的实时聚合指标
//   - RefreshJobMetrics：定时任务刷新任务指标
//   - RetryFailedMessages：重试可重试的失败消息
type SmsTrackingService struct {
	repo repository.SmsTrackingRepository
}

// NewSmsTrackingService 创建短信追踪服务
func NewSmsTrackingService(repo repository.SmsTrackingRepository) *SmsTrackingService {
	if repo == nil {
		repo = repository.NewSmsTrackingRepository(nil)
	}
	return &SmsTrackingService{repo: repo}
}

// RecordDeliveryReport 记录运营商状态报告
//
// 行为：
//  1. 若 message_id 已存在：更新状态（webhook 可能多次推送同一消息的中间状态）
//  2. 若 message_id 不存在：创建新记录
//  3. 根据错误码自动判定 is_retryable
//  4. failed 状态且 is_retryable=true 时，状态置为 retryable 等待定时任务重试
func (s *SmsTrackingService) RecordDeliveryReport(ctx context.Context, req *DeliveryReportRequest) error {
	if req.MessageID == "" {
		return errors.New("message_id 不能为空")
	}
	if req.Status == "" {
		return errors.New("status 不能为空")
	}

	// 规范化状态
	status := normalizeSmsStatus(req.Status)

	// 判定是否可重试
	isRetryable := false
	if status == model.SmsStatusFailed {
		isRetryable = s.IsRetryable(ctx, req.ErrorCode)
		// 失败但可重试 → 状态置为 retryable
		if isRetryable {
			status = model.SmsStatusRetryable
		}
	}

	now := time.Now()
	sentAt, _ := parseSmsTime(req.SentAt)
	deliveredAt, _ := parseSmsTime(req.DeliveredAt)

	// 已存在则更新
	existing, err := s.repo.GetByMessageID(ctx, req.MessageID)
	if err != nil {
		logger.Errorf("查询短信送达状态失败 message_id=%s: %v", req.MessageID, err)
		return err
	}

	if existing != nil {
		existing.Status = status
		existing.ErrorCode = req.ErrorCode
		existing.ErrorMsg = req.ErrorMsg
		existing.IsRetryable = isRetryable
		existing.Provider = req.Provider
		if req.Phone != "" {
			existing.Phone = NormalizePhone(req.Phone)
		}
		if req.JobID != "" {
			existing.JobID = req.JobID
		}
		if sentAt != nil {
			existing.SentAt = sentAt
		}
		if deliveredAt != nil {
			existing.DeliveredAt = deliveredAt
		}
		existing.ReceivedAt = now
		return s.repo.UpdateStatus(ctx, existing)
	}

	// 新记录
	record := &model.SmsDeliveryStatus{
		MessageID:   req.MessageID,
		Phone:       NormalizePhone(req.Phone),
		JobID:       req.JobID,
		Provider:    req.Provider,
		Status:      status,
		ErrorCode:   req.ErrorCode,
		ErrorMsg:    req.ErrorMsg,
		IsRetryable: isRetryable,
		RetryCount:  0,
		MaxRetry:    smsMaxRetryCount,
		SentAt:      sentAt,
		DeliveredAt: deliveredAt,
		ReceivedAt:  now,
	}
	return s.repo.CreateStatus(ctx, record)
}

// IsRetryable 判断错误码是否可重试
//
// 规则：
//   - ERR_6xxx（网关超时）→ 可重试
//   - ERR_5xxx（运营商内部错误 / 限流）→ 可重试
//   - ERR_4xxx（号码无效 / 黑名单 / 内容违规）→ 不重试
//   - 空错误码 → 不重试（保守策略）
//   - 其他未知错误码 → 不重试（保守策略）
func (s *SmsTrackingService) IsRetryable(ctx context.Context, errorCode string) bool {
	if errorCode == "" {
		return false
	}
	for _, prefix := range smsNonRetryableErrorPrefixes {
		if strings.HasPrefix(errorCode, prefix) {
			return false
		}
	}
	for _, prefix := range smsRetryableErrorPrefixes {
		if strings.HasPrefix(errorCode, prefix) {
			return true
		}
	}
	return false
}

// RetryFailedMessages 重试可重试的失败消息
//
// 由定时任务调用（每 5 分钟一次）
// 返回值：实际重试的消息数 + 错误（如果有）
// 注意：本方法只标记 retry_count 自增和状态变更，实际重新发送由调用方（短信服务）执行
func (s *SmsTrackingService) RetryFailedMessages(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	statuses, err := s.repo.ListRetryableStatuses(ctx, batchSize)
	if err != nil {
		logger.Errorf("查询可重试短信失败: %v", err)
		return 0, err
	}

	retried := 0
	for _, st := range statuses {
		// 达到最大重试次数 → 标记为 failed，不再重试
		if st.RetryCount >= st.MaxRetry {
			st.Status = model.SmsStatusFailed
			st.IsRetryable = false
			st.ErrorMsg = "已达最大重试次数：" + st.ErrorMsg
			if err := s.repo.UpdateStatus(ctx, st); err != nil {
				logger.Errorf("更新短信重试状态失败 message_id=%s: %v", st.MessageID, err)
				continue
			}
			continue
		}

		// 标记重试中：retry_count + 1
		st.RetryCount++
		// 状态保持为 retryable，等待下一次 webhook 报告（成功 → delivered / 失败 → retryable 或 failed）
		// 注：实际重新发送由调用方通过 SmsService.ResendSms 完成
		if err := s.repo.UpdateStatus(ctx, st); err != nil {
			logger.Errorf("更新短信重试计数失败 message_id=%s: %v", st.MessageID, err)
			continue
		}
		retried++
	}

	return retried, nil
}

// GetJobMetrics 返回任务的实时聚合指标
func (s *SmsTrackingService) GetJobMetrics(ctx context.Context, jobID string) (*model.SmsJobMetric, error) {
	if jobID == "" {
		return nil, errors.New("job_id 不能为空")
	}

	// 优先读取已聚合的指标
	metric, err := s.repo.GetJobMetric(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if metric == nil {
		metric = &model.SmsJobMetric{JobID: jobID}
	}

	// 实时统计
	sent, err := s.repo.CountByJob(ctx, jobID, "")
	if err != nil {
		return nil, err
	}
	delivered, err := s.repo.CountByJob(ctx, jobID, model.SmsStatusDelivered)
	if err != nil {
		return nil, err
	}
	failed, err := s.repo.CountByJob(ctx, jobID, model.SmsStatusFailed)
	if err != nil {
		return nil, err
	}
	retryable, err := s.repo.CountByJob(ctx, jobID, model.SmsStatusRetryable)
	if err != nil {
		return nil, err
	}

	metric.TotalSent = sent
	metric.TotalDelivered = delivered
	metric.TotalFailed = failed + retryable // failed 含永久失败 + 待重试
	metric.TotalRetried = retryable

	if sent > 0 {
		metric.DeliveryRate = round2(float64(delivered) / float64(sent) * 100)
		metric.FailureRate = round2(float64(metric.TotalFailed) / float64(sent) * 100)
	} else {
		metric.DeliveryRate = 0
		metric.FailureRate = 0
	}

	return metric, nil
}

// GetRangeMetrics 聚合时间区间内的短信指标
func (s *SmsTrackingService) GetRangeMetrics(ctx context.Context, start, end time.Time) (*model.SmsJobMetric, error) {
	if start.IsZero() || end.IsZero() {
		return nil, errors.New("start / end 不能为空")
	}
	if end.Before(start) {
		return nil, errors.New("end 必须大于 start")
	}

	sent, err := s.repo.CountByRange(ctx, start, end, "")
	if err != nil {
		return nil, err
	}
	delivered, err := s.repo.CountByRange(ctx, start, end, model.SmsStatusDelivered)
	if err != nil {
		return nil, err
	}
	failed, err := s.repo.CountByRange(ctx, start, end, model.SmsStatusFailed)
	if err != nil {
		return nil, err
	}
	retryable, err := s.repo.CountByRange(ctx, start, end, model.SmsStatusRetryable)
	if err != nil {
		return nil, err
	}

	metric := &model.SmsJobMetric{
		JobID:          "range",
		TotalSent:      sent,
		TotalDelivered: delivered,
		TotalFailed:    failed + retryable,
		TotalRetried:   retryable,
	}
	if sent > 0 {
		metric.DeliveryRate = round2(float64(delivered) / float64(sent) * 100)
		metric.FailureRate = round2(float64(metric.TotalFailed) / float64(sent) * 100)
	}
	return metric, nil
}

// RefreshJobMetrics 刷新任务指标（定时任务调用，每 5 分钟一次）
func (s *SmsTrackingService) RefreshJobMetrics(ctx context.Context, jobID string) error {
	metric, err := s.GetJobMetrics(ctx, jobID)
	if err != nil {
		return err
	}
	return s.repo.UpsertJobMetric(ctx, metric)
}

// ListJobStatuses 分页查询任务的送达状态
func (s *SmsTrackingService) ListJobStatuses(ctx context.Context, jobID string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 20
	}
	return s.repo.ListStatusesByJob(ctx, jobID, page, limit)
}

// ListPhoneStatuses 分页查询手机号的送达状态
func (s *SmsTrackingService) ListPhoneStatuses(ctx context.Context, phone string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 20
	}
	phone = NormalizePhone(phone)
	return s.repo.ListStatusesByPhone(ctx, phone, page, limit)
}

// normalizeSmsStatus 规范化状态字符串
//
// 兼容主流运营商状态：
//   - DELIV / DELIVERED / SUCCESS / OK → delivered
//   - FAIL / FAILED / ERROR → failed
//   - PENDING / WAITING / SENDING → pending
//   - SENT / ACCEPTD → sent
func normalizeSmsStatus(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "DELIV", "DELIVERED", "SUCCESS", "OK", "REACH":
		return model.SmsStatusDelivered
	case "FAIL", "FAILED", "ERROR", "UNDELIV", "UNDELIVERED":
		return model.SmsStatusFailed
	case "PENDING", "WAITING", "SENDING":
		return model.SmsStatusPending
	case "SENT", "ACCEPTD", "ACCEPTED":
		return model.SmsStatusSent
	default:
		// 未知状态保持原值小写
		return strings.ToLower(status)
	}
}

// parseSmsTime 解析运营商 webhook 时间字符串
//
// 支持格式：
//   - RFC3339: 2006-01-02T15:04:05Z07:00
//   - 2006-01-02 15:04:05
//   - 2006-01-02
//   - Unix 时间戳（秒）
func parseSmsTime(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return &t, nil
		}
	}

	// 失败返回 nil（不阻塞主流程）
	return nil, errors.New("unsupported time format")
}

// smsTrackingServiceWithDB 内部辅助：使用指定 db 创建服务（测试用）
func smsTrackingServiceWithDB(db *gorm.DB) *SmsTrackingService {
	return NewSmsTrackingService(repository.NewSmsTrackingRepository(db))
}
