package selflearning

// asset_bundle_self_supervisor.go 资产包自我监督器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.2
//
// 职责：
//   1. 采集资产包 5 维专属监督指标（v1.1 §7.2 扩展）：
//        - asset_adoption      资产包采纳率（销冠采用比例 / 话术采用率）
//        - asset_conversion    资产包转化率（outcome=converted 的比例）
//        - asset_complaint     资产包投诉率（负反馈比例，越低越好）
//        - asset_freshness     资产包新鲜度（最近使用时间衰减分数）
//        - asset_ab_converge   A/B 实验收敛度（running 实验占比 vs converged 占比）
//   2. 按小时分桶聚合到 self_supervision_signals 表（target_type=asset, target_id=asset_id）
//   3. 超阈值自动告警（status=warning/alert），触发 SelfCorrectionDispatcher 派发资产包降级/回滚
//   4. 提供资产包监督看板查询（按 asset_id 维度）
//
// 与 RAGSelfSupervisor 的关系：
//   - RAGSelfSupervisor 采集 RAG 4 维指标（recall_precision/recall_coverage/generation_fidelity/answer_relevance）
//   - AssetBundleSelfSupervisor 采集资产包 5 维专属指标（adoption/conversion/complaint/freshness/ab_converge）
//   - 两者共享 asset_effectiveness 指标（由 RAGSelfSupervisor 采集，作为综合反馈维度）
//   - 两者都通过 SelfCorrectionDispatcher 派发修复策略
//
// 全自动执行约束（v1.1 §7.4）：
//   - 每次采集前检查 SwitchService.GetStatus().EnableAsset
//   - autonomous → 告警直接派发修复
//   - supervised → 写入 pending 动作待审
//   - manual → 仅记录告警，不派发

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"marketing/internal/dto"
	"marketing/internal/event"
	"marketing/internal/model"
	"marketing/internal/repository"
)

const (
	// 资产包监督指标默认阈值
	defaultThresholdAssetAdoption   = 0.6  // 采纳率阈值（≥60% 视为正常）
	defaultThresholdAssetConversion = 0.3  // 转化率阈值（≥30% 视为正常）
	defaultThresholdAssetComplaint  = 0.15 // 投诉率阈值（≤15% 视为正常）
	defaultThresholdAssetFreshness  = 0.5  // 新鲜度阈值（≥0.5 视为正常）
	defaultThresholdAssetABConverge = 0.5  // 收敛度阈值（≥0.5 视为健康）

	// 新鲜度计算参数
	defaultFreshnessHalfLifeDays = 14 // 新鲜度半衰期（天）
	freshnessBaseline            = 0.5
	freshnessMax                 = 1.0
	freshnessMin                 = 0.0

	// 告警/实验列表查询上限
	assetAlertListLimit        = 200
	assetDashboardAlertLimit   = 100
	assetABTestListLimit       = 100
	assetABTestRunningWeight   = 0.3
	assetABTestConvergedWeight = 1.0

	// 状态判定参数
	complaintWarningRatio = 0.7  // 投诉率告警阈值比例（value > threshold*0.7 → warning）
	metricAlertRatio      = 0.5  // 普通指标告警阈值比例（value < threshold*0.5 → alert）
	signalBaseline        = 0.5  // 信号基线值

	// 采集值默认参数
	adoptionDefault         = 0.5
	adoptionChampionAndAdopt = 0.95
	adoptionChampionOnly     = 0.85
	adoptionAdoptOnly        = 0.75
	adoptionRewardPositive   = 0.65
	adoptionRewardNegative   = 0.3
	conversionConverted      = 1.0
	conversionAbandoned      = 0.1
	conversionNeutral        = 0.5
	complaintReported        = 1.0
	complaintDislike         = 0.6
	complaintRewardNegative  = 0.4

	// 无数据时的中性收敛度
	neutralConvergence = 0.5
)

// AssetBundleSelfSupervisor 资产包自我监督器
type AssetBundleSelfSupervisor struct {
	switchSvc     *SwitchService
	signalRepo    repository.SelfSupervisionSignalRepository
	logRepo       repository.SelfLearningLogRepository
	actionRepo    repository.SelfCorrectionActionRepository
	abTestRepo    repository.AssetBundleABTestRepository
	assetRepo     AssetBundleRepository
	dispatcher    *SelfCorrectionDispatcher

	// 资产包指标阈值（从开关读取，初始化时取默认值）
	// 注意：asset_complaint 越低越好，threshold 为上限
	defaultThresholds map[string]float64

	// 新鲜度衰减参数
	freshnessHalfLifeDays int // 新鲜度半衰期（默认 14 天）
}

// NewAssetBundleSelfSupervisor 创建资产包自我监督器
func NewAssetBundleSelfSupervisor(
	switchSvc *SwitchService,
	signalRepo repository.SelfSupervisionSignalRepository,
	logRepo repository.SelfLearningLogRepository,
	actionRepo repository.SelfCorrectionActionRepository,
	abTestRepo repository.AssetBundleABTestRepository,
	assetRepo AssetBundleRepository,
	dispatcher *SelfCorrectionDispatcher,
) *AssetBundleSelfSupervisor {
	return &AssetBundleSelfSupervisor{
		switchSvc:             switchSvc,
		signalRepo:            signalRepo,
		logRepo:               logRepo,
		actionRepo:            actionRepo,
		abTestRepo:            abTestRepo,
		assetRepo:             assetRepo,
		dispatcher:            dispatcher,
		defaultThresholds: map[string]float64{
			model.SupervisionMetricAssetAdoption:   defaultThresholdAssetAdoption,
			model.SupervisionMetricAssetConversion: defaultThresholdAssetConversion,
			model.SupervisionMetricAssetComplaint:  defaultThresholdAssetComplaint,
			model.SupervisionMetricAssetFreshness:  defaultThresholdAssetFreshness,
			model.SupervisionMetricAssetABConverge: defaultThresholdAssetABConverge,
		},
		freshnessHalfLifeDays: defaultFreshnessHalfLifeDays,
	}
}

// ============================================================================
// 5 维资产包监督指标采集（dialogue.ended 触发）
// ============================================================================

// CollectMetrics 采集本次会话涉及的资产包 5 维监督指标
//
// 触发：dialogue.ended 事件
// 输入：DialogueEndedPayload（包含 used_asset_ids / outcome / aggregated_reward / signal_breakdown）
// 输出：每个 asset_id 写入 5 个 SupervisionSignal
//
// 采集策略：
//   1. asset_adoption      基于 signal_breakdown.champion_mark / script_adopt 信号
//   2. asset_conversion    基于 outcome=converted
//   3. asset_complaint     基于 signal_breakdown.complaint / dislike 信号
//   4. asset_freshness     基于资产包 updated_at 与当前时间差（指数衰减）
//   5. asset_ab_converge   基于 asset_id 关联的 A/B 实验状态（converged 比例）
func (s *AssetBundleSelfSupervisor) CollectMetrics(ctx context.Context, payload *event.DialogueEndedPayload) error {
	if payload == nil {
		return fmt.Errorf("payload is nil")
	}
	snap, err := s.switchSvc.GetStatus(ctx)
	if err != nil {
		return err
	}
	if !snap.EnableAsset {
		return nil
	}
	if len(payload.UsedAssetIDs) == 0 {
		return nil // 未使用资产包，无需采集
	}

	bucketHour := time.Now().Truncate(time.Hour)

	// 解析反馈信号明细
	signalBreakdown := payload.SignalBreakdown
	hasChampionMark := hasSignalKey(signalBreakdown, "champion_mark")
	hasScriptAdopt := hasSignalKey(signalBreakdown, "script_adopt")
	hasComplaint := hasSignalKey(signalBreakdown, "complaint")
	hasDislike := hasSignalKey(signalBreakdown, "dislike")
	hasConversion := payload.Outcome == "converted"

	// 逐个资产包采集 5 维指标
	for _, assetID := range payload.UsedAssetIDs {
		// 1. asset_adoption（销冠采用率：0.0-1.0）
		adoption := adoptionDefault // 默认中性
		if hasChampionMark && hasScriptAdopt {
			adoption = adoptionChampionAndAdopt // 销冠标记 + 话术采用
		} else if hasChampionMark {
			adoption = adoptionChampionOnly
		} else if hasScriptAdopt {
			adoption = adoptionAdoptOnly
		} else if payload.AggregatedReward > 0 {
			adoption = adoptionRewardPositive
		} else if payload.AggregatedReward < 0 {
			adoption = adoptionRewardNegative
		}
		if err := s.upsertSignal(ctx, model.SupervisionTargetAsset, assetID, model.SupervisionMetricAssetAdoption, bucketHour, adoption, s.getThreshold(model.SupervisionMetricAssetAdoption, snap), 1, payload.TraceID); err != nil {
			log.Printf("[asset_supervisor] upsert asset_adoption failed: asset=%s err=%v", assetID, err)
		}

		// 2. asset_conversion（转化率：0.0-1.0）
		conversion := 0.0
		if hasConversion {
			conversion = conversionConverted
		} else if payload.Outcome == "abandoned" {
			conversion = conversionAbandoned
		} else {
			conversion = conversionNeutral // 中性
		}
		if err := s.upsertSignal(ctx, model.SupervisionTargetAsset, assetID, model.SupervisionMetricAssetConversion, bucketHour, conversion, s.getThreshold(model.SupervisionMetricAssetConversion, snap), 1, payload.TraceID); err != nil {
			log.Printf("[asset_supervisor] upsert asset_conversion failed: asset=%s err=%v", assetID, err)
		}

		// 3. asset_complaint（投诉率：0.0-1.0，越低越好）
		complaint := 0.0
		if hasComplaint {
			complaint = complaintReported
		} else if hasDislike {
			complaint = complaintDislike
		} else if payload.AggregatedReward < 0 {
			complaint = complaintRewardNegative
		}
		if err := s.upsertSignal(ctx, model.SupervisionTargetAsset, assetID, model.SupervisionMetricAssetComplaint, bucketHour, complaint, s.getThreshold(model.SupervisionMetricAssetComplaint, snap), 1, payload.TraceID); err != nil {
			log.Printf("[asset_supervisor] upsert asset_complaint failed: asset=%s err=%v", assetID, err)
		}

		// 4. asset_freshness（新鲜度：基于资产包 updated_at 的指数衰减）
		freshness := s.computeFreshness(ctx, assetID)
		if err := s.upsertSignal(ctx, model.SupervisionTargetAsset, assetID, model.SupervisionMetricAssetFreshness, bucketHour, freshness, s.getThreshold(model.SupervisionMetricAssetFreshness, snap), 1, payload.TraceID); err != nil {
			log.Printf("[asset_supervisor] upsert asset_freshness failed: asset=%s err=%v", assetID, err)
		}

		// 5. asset_ab_converge（A/B 实验收敛度：0.0-1.0）
		converge := s.computeABConvergence(ctx, assetID)
		if err := s.upsertSignal(ctx, model.SupervisionTargetAsset, assetID, model.SupervisionMetricAssetABConverge, bucketHour, converge, s.getThreshold(model.SupervisionMetricAssetABConverge, snap), 1, payload.TraceID); err != nil {
			log.Printf("[asset_supervisor] upsert asset_ab_converge failed: asset=%s err=%v", assetID, err)
		}
	}
	return nil
}

// ============================================================================
// 监督告警扫描（cron.hourly 触发）
// ============================================================================

// ScanAlerts 扫描资产包告警（每小时）
//
// 行为：
//   1. 查询最近 1h 的 warning/alert 资产包信号
//   2. 对 alert 状态的信号，触发 SelfCorrectionDispatcher 派发修复
//   3. 写入 self_learning_logs（scenario=asset_supervision）
func (s *AssetBundleSelfSupervisor) ScanAlerts(ctx context.Context) (int, error) {
	snap, err := s.switchSvc.GetStatus(ctx)
	if err != nil {
		return 0, err
	}
	if !snap.EnableAsset {
		return 0, nil
	}
	since := time.Now().Add(-1 * time.Hour)
	alerts, err := s.signalRepo.ListAlerts(ctx, since, assetAlertListLimit)
	if err != nil {
		return 0, err
	}
	dispatchedCount := 0
	for _, alert := range alerts {
		// 仅处理资产包类告警
		if alert.TargetType != model.SupervisionTargetAsset {
			continue
		}
		if !IsAssetMetric(alert.MetricName) {
			continue
		}
		if alert.Status != model.SupervisionStatusAlert {
			continue
		}
		// A/B 收敛度低不派发修复（实验未收敛是正常状态）
		if alert.MetricName == model.SupervisionMetricAssetABConverge {
			log.Printf("[asset_supervisor] skip ab_converge alert (waiting for bandit): asset=%s", alert.TargetID)
			continue
		}
		// 触发 dispatcher
		if s.dispatcher != nil {
			if err := s.dispatcher.DispatchFromSignal(ctx, alert); err != nil {
				log.Printf("[asset_supervisor] dispatch failed: signal=%s asset=%s err=%v", alert.SignalID, alert.TargetID, err)
				continue
			}
			dispatchedCount++
		}
	}
	if dispatchedCount > 0 {
		log.Printf("[asset_supervisor] dispatched %d asset correction actions from alerts", dispatchedCount)
	}
	return dispatchedCount, nil
}

// ============================================================================
// 资产包监督看板（供 Controller 调用）
// ============================================================================

// GetAssetDashboard 获取资产包监督看板
//
// range: "24h" / "7d" / "30d"
// 按 asset_id 维度展示 5 维监督指标
func (s *AssetBundleSelfSupervisor) GetAssetDashboard(ctx context.Context, rangeStr string) (*dto.AssetSupervisionDashboardResponse, error) {
	var from time.Time
	now := time.Now()
	switch rangeStr {
	case "7d":
		from = now.Add(-7 * 24 * time.Hour)
	case "30d":
		from = now.Add(-30 * 24 * time.Hour)
	default:
		from = now.Add(-24 * time.Hour)
		rangeStr = "24h"
	}

	// 收集 5 维资产包指标
	metricNames := []string{
		model.SupervisionMetricAssetAdoption,
		model.SupervisionMetricAssetConversion,
		model.SupervisionMetricAssetComplaint,
		model.SupervisionMetricAssetFreshness,
		model.SupervisionMetricAssetABConverge,
	}
	metrics := make([]*dto.SupervisionMetricItem, 0, len(metricNames))
	for _, metricName := range metricNames {
		avg, count, err := s.signalRepo.AggregateByRange(ctx, model.SupervisionTargetAsset, metricName, from, now)
		if err != nil {
			continue
		}
		metrics = append(metrics, &dto.SupervisionMetricItem{
			Name:         metricName,
			DisplayName:  assetMetricDisplayName(metricName),
			Value:        avg,
			Threshold:    s.defaultThresholds[metricName],
			SampleCount:  count,
			LastSampleAt: now,
		})
	}

	// 查询当前告警中的资产包信号
	alerts, lerr := s.signalRepo.ListAlerts(ctx, from, assetDashboardAlertLimit)
	if lerr != nil {
		log.Printf("[asset_supervisor] dashboard list_alerts failed: err=%v", lerr)
	}
	alertItems := make([]*dto.SupervisionAlertItem, 0, len(alerts))
	for _, a := range alerts {
		if a.TargetType != model.SupervisionTargetAsset {
			continue
		}
		if !IsAssetMetric(a.MetricName) {
			continue
		}
		severity := "warning"
		if a.Status == model.SupervisionStatusAlert {
			severity = "critical"
		}
		alertItems = append(alertItems, &dto.SupervisionAlertItem{
			AlertID:     a.SignalID,
			MetricName:  a.MetricName,
			Severity:    severity,
			Message:     fmt.Sprintf("asset=%s metric=%s value=%.3f threshold=%.3f", a.TargetID, a.MetricName, a.Value, a.Threshold),
			TriggeredAt: a.BucketHour,
		})
	}

	// A/B 实验状态汇总
	abStats := s.getABTestStats(ctx)

	return &dto.AssetSupervisionDashboardResponse{
		Range:         rangeStr,
		From:          from,
		To:            now,
		AssetMetrics:  metrics,
		Alerts:        alertItems,
		ABTestSummary: abStats,
		UpdatedAt:     now,
	}, nil
}

// GetAssetMetricHistory 查询单个资产包的指标历史
//
// 用途：资产包详情页 - 展示某资产包 5 维指标的历史趋势
func (s *AssetBundleSelfSupervisor) GetAssetMetricHistory(ctx context.Context, assetID string, from, to time.Time) ([]*dto.SupervisionMetricItem, error) {
	if assetID == "" {
		return nil, fmt.Errorf("asset_id is empty")
	}
	metricNames := []string{
		model.SupervisionMetricAssetAdoption,
		model.SupervisionMetricAssetConversion,
		model.SupervisionMetricAssetComplaint,
		model.SupervisionMetricAssetFreshness,
		model.SupervisionMetricAssetABConverge,
	}
	out := make([]*dto.SupervisionMetricItem, 0, len(metricNames))
	for _, metricName := range metricNames {
		avg, count, err := s.signalRepo.AggregateByRange(ctx, model.SupervisionTargetAsset, metricName, from, to)
		if err != nil {
			continue
		}
		// 查询该 asset_id 该指标的最新信号（取阈值）
		signals, serr := s.signalRepo.ListByTarget(ctx, model.SupervisionTargetAsset, assetID, from, to, 1)
		if serr != nil {
			log.Printf("[asset_supervisor] detail list_by_target failed: asset=%s metric=%s err=%v", assetID, metricName, serr)
		}
		threshold := s.defaultThresholds[metricName]
		if len(signals) > 0 {
			threshold = signals[0].Threshold
		}
		out = append(out, &dto.SupervisionMetricItem{
			Name:         metricName,
			DisplayName:  assetMetricDisplayName(metricName),
			Value:        avg,
			Threshold:    threshold,
			SampleCount:  count,
			LastSampleAt: to,
		})
	}
	return out, nil
}

// ============================================================================
// 内部方法
// ============================================================================

// upsertSignal 写入或更新资产包监督信号
func (s *AssetBundleSelfSupervisor) upsertSignal(ctx context.Context, targetType model.SupervisionTargetType, targetID, metricName string, bucket time.Time, value, threshold float64, sampleCount int, traceID string) error {
	// 投诉率越低越好，状态判定逻辑反转
	status := model.SupervisionStatusNormal
	if metricName == model.SupervisionMetricAssetComplaint {
		if value > threshold {
			status = model.SupervisionStatusAlert
		} else if value > threshold*complaintWarningRatio {
			status = model.SupervisionStatusWarning
		}
	} else {
		// 其他指标：低于阈值告警
		if value < threshold*metricAlertRatio {
			status = model.SupervisionStatusAlert
		} else if value < threshold {
			status = model.SupervisionStatusWarning
		}
	}
	signal := &model.SelfSupervisionSignal{
		SignalID:    GenSignalID(targetType, targetID, metricName, bucket),
		TargetType:  targetType,
		TargetID:    targetID,
		MetricName:  metricName,
		BucketHour:  bucket,
		Value:       value,
		Baseline:    signalBaseline,
		Threshold:   threshold,
		SampleCount: int64(sampleCount),
		Status:      status,
		TraceIDs:    []string{traceID},
	}
	return s.signalRepo.UpsertSignal(ctx, signal)
}

// computeFreshness 计算资产包新鲜度（指数衰减）
//
// 公式：freshness = 0.5 * (1 + 2^(-days_since_update / half_life))
//   - days=0   → freshness=1.0（刚更新）
//   - days=14  → freshness=0.75（半衰期）
//   - days=28  → freshness=0.625
//   - days=∞   → freshness=0.5（基线）
func (s *AssetBundleSelfSupervisor) computeFreshness(ctx context.Context, assetID string) float64 {
	if s.assetRepo == nil {
		return freshnessBaseline
	}
	asset, err := s.assetRepo.FindByAssetID(ctx, assetID)
	if err != nil || asset == nil {
		return freshnessBaseline
	}
	days := time.Since(asset.UpdatedAt).Hours() / 24
	halfLife := float64(s.freshnessHalfLifeDays)
	if halfLife <= 0 {
		halfLife = defaultFreshnessHalfLifeDays
	}
	freshness := freshnessBaseline * (1 + math.Pow(2, -days/halfLife))
	if freshness > freshnessMax {
		freshness = freshnessMax
	}
	if freshness < freshnessMin {
		freshness = freshnessMin
	}
	return freshness
}

// computeABConvergence 计算资产包关联的 A/B 实验收敛度
//
// 收敛度定义：
//   - 无实验           → 0.5（中性，无数据）
//   - 全部 converged   → 1.0（已收敛）
//   - 全部 running     → 0.3（实验中，待收敛）
//   - running/converged 混合 → 加权平均
func (s *AssetBundleSelfSupervisor) computeABConvergence(ctx context.Context, assetID string) float64 {
	if s.abTestRepo == nil {
		return neutralConvergence
	}
	// 查询以该 asset_id 为 baseline 的所有实验
	// 简化实现：通过 ListByStatus 查询 running 与 converged 的实验
	runningTests, err := s.abTestRepo.ListByStatus(ctx, model.ABTestStatusRunning, assetABTestListLimit)
	if err != nil {
		return neutralConvergence
	}
	convergedTests, err := s.abTestRepo.ListByStatus(ctx, model.ABTestStatusConverged, assetABTestListLimit)
	if err != nil {
		return neutralConvergence
	}
	// 过滤出 baseline=assetID 的实验
	var running, converged int
	for _, t := range runningTests {
		if t.BaselineAssetID == assetID {
			running++
		}
	}
	for _, t := range convergedTests {
		if t.BaselineAssetID == assetID {
			converged++
		}
	}
	total := running + converged
	if total == 0 {
		return neutralConvergence // 无实验
	}
	// converged 权重 1.0，running 权重 0.3
	score := (float64(converged)*assetABTestConvergedWeight + float64(running)*assetABTestRunningWeight) / float64(total)
	if score > freshnessMax {
		score = freshnessMax
	}
	return score
}

// getABTestStats 获取 A/B 实验状态汇总（供看板展示）
func (s *AssetBundleSelfSupervisor) getABTestStats(ctx context.Context) *dto.AssetABTestSummary {
	if s.abTestRepo == nil {
		return nil
	}
	counts, err := s.abTestRepo.CountByStatus(ctx)
	if err != nil {
		return nil
	}
	summary := &dto.AssetABTestSummary{}
	for status, count := range counts {
		switch status {
		case model.ABTestStatusRunning:
			summary.RunningCount = count
		case model.ABTestStatusConverged:
			summary.ConvergedCount = count
		case model.ABTestStatusCompleted:
			summary.CompletedCount = count
		case model.ABTestStatusRolledBack:
			summary.RolledBackCount = count
		}
	}
	summary.TotalCount = summary.RunningCount + summary.ConvergedCount + summary.CompletedCount + summary.RolledBackCount
	if summary.TotalCount > 0 {
		summary.ConvergeRate = float64(summary.ConvergedCount+summary.CompletedCount) / float64(summary.TotalCount)
	}
	return summary
}

// getThreshold 获取指标阈值
//
// 注意：当前实现使用 defaultThresholds，snap 参数预留给未来从开关读取动态阈值。
func (s *AssetBundleSelfSupervisor) getThreshold(metricName string, snap *SwitchSnapshot) float64 {
	_ = snap // 预留：未来从 SwitchSnapshot 读取动态阈值
	if v, ok := s.defaultThresholds[metricName]; ok {
		return v
	}
	return signalBaseline
}

// hasSignalKey 检查 signal_breakdown 中是否存在指定信号键
func hasSignalKey(breakdown map[string]any, key string) bool {
	if breakdown == nil {
		return false
	}
	v, ok := breakdown[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val > 0
	case string:
		return strings.ToLower(val) == "true" || val == "1"
	}
	return false
}

// assetMetricDisplayName 资产包指标中文名
func assetMetricDisplayName(metricName string) string {
	switch metricName {
	case model.SupervisionMetricAssetAdoption:
		return "资产包采纳率"
	case model.SupervisionMetricAssetConversion:
		return "资产包转化率"
	case model.SupervisionMetricAssetComplaint:
		return "资产包投诉率"
	case model.SupervisionMetricAssetFreshness:
		return "资产包新鲜度"
	case model.SupervisionMetricAssetABConverge:
		return "A/B 实验收敛度"
	case model.SupervisionMetricAssetEffectiveness:
		return "资产包综合效能"
	}
	return metricName
}
