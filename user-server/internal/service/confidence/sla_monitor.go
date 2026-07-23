package confidence

// sla_monitor.go SLA 监控服务
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.13
//
// 每分钟聚合一次，计算 6 个核心指标：
//   1. 自动回复率         目标 60-70%   告警 <40% 或 >85%
//   2. 转人工率           目标 15-25%   告警 >35%
//   3. 审核队列超时率     目标 <10%     告警 >20%
//   4. 平均座席分配时长   目标 <30s     告警 >60s
//   5. 转人工后客户接受率 目标 >60%     告警 <40%
//   6. 校准 ECE           目标 <0.05   告警 >0.10

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// SLAMonitorService SLA 监控服务
type SLAMonitorService struct {
	repo		*repository.SLAMonitorRepository
	signalRepo	*repository.ConfidenceSignalRepository
	handoffRepo	*repository.HandoffDecisionRepository
	reviewRepo	*repository.ReviewQueueRepository
	calibRepo	*repository.ConfidenceCalibrationRepository
}

// NewSLAMonitorService 创建 SLA 监控服务
func NewSLAMonitorService(
	repo *repository.SLAMonitorRepository,
	signalRepo *repository.ConfidenceSignalRepository,
	handoffRepo *repository.HandoffDecisionRepository,
	reviewRepo *repository.ReviewQueueRepository,
	calibRepo *repository.ConfidenceCalibrationRepository,
) *SLAMonitorService {
	return &SLAMonitorService{
		repo:		repo,
		signalRepo:	signalRepo,
		handoffRepo:	handoffRepo,
		reviewRepo:	reviewRepo,
		calibRepo:	calibRepo,
	}
}

// AggregateMinute 聚合上一分钟的指标（cron 每分钟调用）
//
// 桶时间：[now-1min, now)
// 计算流程：
//  1. 查询桶内所有 confidence_signals
//  2. 按 decision_band 统计 auto_reply / handoff / review
//  3. 查询 handoff_decisions 桶内记录，计算分配时长、客户接受率
//  4. 查询 review_queue 中 auto_released 的数量，计算超时率
//  5. 查询 confidence_calibrations 取最近 active 的 ece_after
//  6. 检测告警
//  7. 写入 sla_monitors 表
func (s *SLAMonitorService) AggregateMinute(ctx context.Context) error {
	now := time.Now()
	bucketStart := now.Truncate(time.Minute).Add(-time.Minute)
	bucketEnd := bucketStart.Add(time.Minute)

	// 1. 查询桶内所有信号
	signals, err := s.signalRepo.ListByTimeRange(ctx, bucketStart, bucketEnd)
	if err != nil {
		return err
	}
	total := len(signals)
	if total == 0 {
		return nil
	}

	// 2. 按 band 统计
	var autoReply, handoff, reviewTimeout int
	for _, sig := range signals {
		switch sig.DecisionBand {
		case "auto", "llm_fallback":
			autoReply++
		case "handoff":
			handoff++
		case "review":
			// 查 review_queue 是否超时
			item, _ := s.reviewRepo.GetBySignalID(ctx, sig.SignalID)
			if item != nil && item.AutoReleased {
				reviewTimeout++
			}
		}
	}

	// 3. 查转人工记录的分配时长
	handoffRecords, _ := s.handoffRepo.ListByTimeRange(ctx, bucketStart, bucketEnd)
	assignSum := 0.0
	acceptedCount := 0
	assignCount := 0
	for _, h := range handoffRecords {
		if h.AssignedAt != nil && h.CreatedAt.Before(*h.AssignedAt) {
			assignSum += h.AssignedAt.Sub(h.CreatedAt).Seconds()
			assignCount++
		}
		if h.CustomerAccepted {
			acceptedCount++
		}
	}

	// 4. 构造监控记录
	monitor := &model.SLAMonitor{
		MonitorID:		uuid.NewString(),
		BucketMinute:		bucketStart,
		AutoReplyRate:		safeDivF(float64(autoReply), float64(total)),
		HandoffRate:		safeDivF(float64(handoff), float64(total)),
		ReviewTimeoutRate:	safeDivF(float64(reviewTimeout), float64(total)),
		AvgAssignmentSeconds:	safeDivF(assignSum, float64(max1(assignCount))),
		PostHandoffAcceptRate:	safeDivF(float64(acceptedCount), float64(max1(handoff))),
		ECE:			s.computeECE(ctx),
		TotalMessages:		total,
	}
	monitor.AlertsTriggered = s.detectAlerts(ctx, monitor)
	return s.repo.Create(ctx, monitor)
}

// detectAlerts 检测告警
//
// 返回逗号分隔的告警名（无告警返回空字符串）
func (s *SLAMonitorService) detectAlerts(ctx context.Context, m *model.SLAMonitor) string {
	var alerts []string
	if m.AutoReplyRate < 0.40 || m.AutoReplyRate > 0.85 {
		alerts = append(alerts, "auto_reply_rate_out_of_range")
	}
	if m.HandoffRate > 0.35 {
		alerts = append(alerts, "handoff_rate_too_high")
	}
	if m.ReviewTimeoutRate > 0.20 {
		alerts = append(alerts, "review_timeout_too_high")
	}
	if m.AvgAssignmentSeconds > 60 {
		alerts = append(alerts, "assignment_too_slow")
	}
	if m.PostHandoffAcceptRate < 0.40 {
		alerts = append(alerts, "post_handoff_accept_too_low")
	}
	if m.ECE > 0.10 {
		alerts = append(alerts, "calibration_degraded")
	}
	return strings.Join(alerts, ",")
}

// computeECE 计算当前 ECE
//
// 从 confidence_calibrations 表取最近 active 记录的 ece_after
// 失败返回 0
func (s *SLAMonitorService) computeECE(ctx context.Context) float64 {
	if s.calibRepo == nil {
		return 0.0
	}
	record, err := s.calibRepo.GetActive(ctx, "intent_conf")
	if err != nil || record == nil {
		return 0.0
	}
	return record.ECEAfter
}

// ListLatest 取最近 N 条监控记录
func (s *SLAMonitorService) ListLatest(ctx context.Context, limit int) ([]model.SLAMonitor, error) {
	return s.repo.ListLatest(ctx, limit)
}

// safeDivF 安全除法（b=0 返回 0）
func safeDivF(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// max1 保证至少为 1（用于除法兜底）
func max1(a int) int {
	if a == 0 {
		return 1
	}
	return a
}
