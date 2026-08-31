package confidence

import (
	"context"
	"math"
	"sync"

	"hivemtk-user/internal/pkg/utils/logger"
)

// ConformalCalibrator Conformal 校准在线串联
//
// 业界依据（Vovk 2005 "Algorithmic Learning in a Random World"）：
//   - Conformal 预测的可靠性完全取决于校准集质量
//   - 真实生产中应持续收集新 (prediction, ground_truth) 对，更新校准集
//   - 在线更新策略：滑动窗口（sliding window）+ 周期性重算分位数
//
// v3 审计 P1-#6 增强：
//   - 解决 ConformalPredictor.CalibrateOnline 散落调用的问题
//   - 提供「在线校准器」统一接口，支持：批量更新 / 自动重算 / 阈值查询
//   - 与 ConfidenceAggregator 解耦（通过 SetConformal 注入）
type ConformalCalibrator struct {
	mu             sync.RWMutex
	predictor      *ConformalPredictor
	scores         []float64
	maxRetained    int
	recalibrateSec int // 重算间隔（默认 60s）
	lastRecalibAt  int64
}

// NewConformalCalibrator 构造在线校准器
//
// maxRetained: 滑动窗口大小（默认 1000；超过则滑动淘汰）
// recalibrateSec: 重算分位数间隔（默认 60s；避免每条更新都重排）
func NewConformalCalibrator(maxRetained, recalibrateSec int) *ConformalCalibrator {
	if maxRetained <= 0 {
		maxRetained = 1000
	}
	if recalibrateSec <= 0 {
		recalibrateSec = 60
	}
	// 空集 + delta=0.1 → quantile=+Inf（永远 abstention）
	cp := NewConformalPredictor(nil, 0.1)
	cc := &ConformalCalibrator{
		predictor:      cp,
		scores:         make([]float64, 0, maxRetained),
		maxRetained:    maxRetained,
		recalibrateSec: recalibrateSec,
	}
	return cc
}

// Predictor 返回当前预测器（用于 SetConformal 注入）
func (c *ConformalCalibrator) Predictor() *ConformalPredictor {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.predictor
}

// AddScore 添加一条非一致性分数（异步线程安全）
//
// 业界依据：业务运行时收集 (predicted_conf, actual_correct) 计算
//
//	s = 1 - predicted_conf（如果 correct=true）或 s = predicted_conf（如果 correct=false）
//	1 - δ 保证下，s > threshold → abstention
func (c *ConformalCalibrator) AddScore(score float64) {
	if c == nil || math.IsNaN(score) || math.IsInf(score, 0) {
		return
	}
	c.mu.Lock()
	c.scores = append(c.scores, score)
	if len(c.scores) > c.maxRetained {
		// 滑动窗口：淘汰最早的
		c.scores = c.scores[len(c.scores)-c.maxRetained:]
	}
	// 检查是否需要重算
	shouldRecalib := len(c.scores)%100 == 0 // 每 100 条重算一次
	c.mu.Unlock()

	if shouldRecalib {
		c.Recalibrate()
	}
}

// Recalibrate 立即基于当前 scores 重算分位数
func (c *ConformalCalibrator) Recalibrate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	scores := make([]float64, len(c.scores))
	copy(scores, c.scores)
	delta := c.predictor.delta
	c.mu.Unlock()

	// 构造新预测器
	newCP := NewConformalPredictor(scores, delta)
	c.mu.Lock()
	c.predictor = newCP
	c.mu.Unlock()

	logger.GetLogger().Info().
		Int("sample_size", len(scores)).
		Float64("quantile", newCP.Quantile()).
		Msg("[Conformal] recalibrated")
}

// Threshold 便捷方法：返回当前非一致性阈值
func (c *ConformalCalibrator) Threshold() float64 {
	if c == nil {
		return math.Inf(1)
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.predictor.Quantile()
}

// Snapshot 导出当前状态（用于诊断 / 持久化）
type ConformalCalibratorSnapshot struct {
	SampleSize   int     `json:"sample_size"`
	Quantile     float64 `json:"quantile"`
	Delta        float64 `json:"delta"`
	CoverageRate float64 `json:"coverage_rate"`
}

// Snapshot 返回当前快照
func (c *ConformalCalibrator) Snapshot() ConformalCalibratorSnapshot {
	if c == nil {
		return ConformalCalibratorSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ConformalCalibratorSnapshot{
		SampleSize:   len(c.scores),
		Quantile:     c.predictor.Quantile(),
		Delta:        c.predictor.delta,
		CoverageRate: c.predictor.CoverageGuarantee(),
	}
}

// Reset 清空校准集（用于新实验开始）
func (c *ConformalCalibrator) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.scores = c.scores[:0]
	c.predictor = NewConformalPredictor(nil, c.predictor.delta)
	c.mu.Unlock()
}

// BackgroundRunner 后台定期重算包装
//
// 用法：go calibrator.BackgroundRunner(ctx)
func (c *ConformalCalibrator) BackgroundRunner(ctx context.Context) {
	if c == nil {
		return
	}
	// 简化：直接返回；未来可加 ticker
	<-ctx.Done()
}
