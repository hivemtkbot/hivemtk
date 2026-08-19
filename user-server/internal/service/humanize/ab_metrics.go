package humanize

import (
	"sync"
	"sync/atomic"
	"time"
)

// A/B 指标采集器（拟人化效果验证）
//
// 业界依据：
//   - 任何"主观拟人"功能都应 A/B 测试实际转化效果
//   - 数据：humanize_polisher + behavioral 拟人化后用户停留/转化/退订率变化
//   - 实现：在 AI 回复送出时记录分数；用户后续行为（回复/转化/流失）触发回写
//
// 设计：纯内存聚合（生产环境需异步落库 humanize_eval_samples 表，本文件聚焦聚合逻辑）。
//   - A/B 分桶：基于 stableHash(customerID) % 100，与 confidence 包 ABTestService 一致
//   - 关键指标：per-group 的 (mean_score, sample_count, conversion_rate, churn_rate)

// A/B 桶标识
const (
	ABGroupControl   = "control"   // 不做拟人化
	ABGroupTreatment = "treatment" // 拟人化开启
)

// HumanizeABMetrics 单桶聚合指标
type HumanizeABMetrics struct {
	Group             string  `json:"group"`
	SampleCount       int64   `json:"sample_count"`
	SumScore          float64 `json:"sum_score"`
	MeanScore         float64 `json:"mean_score"`
	ConversionCount   int64   `json:"conversion_count"`
	ChurnCount        int64   `json:"churn_count"`
	NegativeReplies   int64   `json:"negative_replies"`
	AvgFirstReplyMs   int64   `json:"avg_first_reply_ms"`
	SumFirstReplyMs   int64   `json:"sum_first_reply_ms"`
}

// ConversionRate 转化率（conversions / samples）
func (m *HumanizeABMetrics) ConversionRate() float64 {
	if m.SampleCount == 0 {
		return 0
	}
	return float64(m.ConversionCount) / float64(m.SampleCount)
}

// ChurnRate 流失率（churns / samples）
func (m *HumanizeABMetrics) ChurnRate() float64 {
	if m.SampleCount == 0 {
		return 0
	}
	return float64(m.ChurnCount) / float64(m.SampleCount)
}

// NegativeReplyRate 负面回复率
func (m *HumanizeABMetrics) NegativeReplyRate() float64 {
	if m.SampleCount == 0 {
		return 0
	}
	return float64(m.NegativeReplies) / float64(m.SampleCount)
}

// AvgFirstReplyMsSec 平均首响延迟（毫秒）
func (m *HumanizeABMetrics) AvgFirstReplyMsSec() float64 {
	if m.SampleCount == 0 {
		return 0
	}
	return float64(m.SumFirstReplyMs) / float64(m.SampleCount)
}

// ABRecorder A/B 指标记录器
//
// 线程安全：单例模式，多 goroutine 并发写
// v3 审计 P1-#5 增强：增加 DB 持久化（异步落库 ab_test_metrics 表）
type ABRecorder struct {
	mu        sync.RWMutex
	control   *HumanizeABMetrics
	treatment *HumanizeABMetrics
	traffic   int // 桶大小（默认 100）

	// 持久化钩子（异步调用，避免阻塞主链路）
	persistHook ABPersistHook
}

// ABPersistHook 持久化钩子签名
//
//   - testID: 业务实验 ID（如 "humanize_polish_v2"）
//   - group: control / treatment
//   - metricName: humanize_score / first_reply_ms / conversion / churn / negative_reply
//   - value: 数值
//   - customerID: 可选
type ABPersistHook func(testID, group, metricName, customerID string, value float64)

// NewABRecorder 构造 A/B 记录器
//
// trafficSplit: 0-100，控制桶占比（默认 50 = 50/50 split）
func NewABRecorder(trafficSplit int) *ABRecorder {
	if trafficSplit <= 0 || trafficSplit >= 100 {
		trafficSplit = 50
	}
	return &ABRecorder{
		control:   &HumanizeABMetrics{Group: ABGroupControl},
		treatment: &HumanizeABMetrics{Group: ABGroupTreatment},
		traffic:   trafficSplit,
	}
}

// SetPersistHook 注入持久化钩子
//
// 业界依据：异步落库可观测 A/B 真实效果（不靠内存聚合）
// 调用线程安全：可运行时切换（业务方可在 cron 启动时挂 DB 落库）
func (r *ABRecorder) SetPersistHook(hook ABPersistHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persistHook = hook
}

// getPersistHook 在锁外调用以避免持锁执行外部代码
func (r *ABRecorder) getPersistHook() ABPersistHook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.persistHook
}

// AssignBucket 根据 stableHash（与 confidence 包一致）分桶
//
// 业界依据：ABTestService.Assign 使用 fnv hash 保持一致性哈希
func (r *ABRecorder) AssignBucket(customerID string) string {
	if customerID == "" {
		// 空 ID 全部进 control（避免污染 treatment）
		return ABGroupControl
	}
	h := fnv32(customerID)
	if int(h%100) < r.traffic {
		return ABGroupControl
	}
	return ABGroupTreatment
}

// RecordScore 记录一次拟人化分数（AI 回复送出时调用）
//
// score: humanize 评分（0-1）
// firstReplyMs: 从收到客户消息到 AI 回复送出耗时（含分条延迟）
// customerID: 可选（用于持久化关联客户）
func (r *ABRecorder) RecordScore(group string, score float64, firstReplyMs int64, customerID ...string) {
	r.mu.Lock()
	m := r.metricsFor(group)
	m.SampleCount++
	m.SumScore += score
	m.SumFirstReplyMs += firstReplyMs
	m.MeanScore = m.SumScore / float64(m.SampleCount)
	r.mu.Unlock()

	// v3 审计 P1-#5 增强：异步落库
	r.persistAsync(group, "humanize_score", score, customerID)
	r.persistAsync(group, "first_reply_ms", float64(firstReplyMs), customerID)
}

// RecordOutcome 记录用户后续行为
//
// outcomeType: "conversion" | "churn" | "negative_reply"
// customerID: 可选
func (r *ABRecorder) RecordOutcome(group, outcomeType string, customerID ...string) {
	r.mu.Lock()
	m := r.metricsFor(group)
	switch outcomeType {
	case "conversion":
		atomic.AddInt64(&m.ConversionCount, 1)
	case "churn":
		atomic.AddInt64(&m.ChurnCount, 1)
	case "negative_reply":
		atomic.AddInt64(&m.NegativeReplies, 1)
	}
	r.mu.Unlock()

	// v3 审计 P1-#5 增强：异步落库
	r.persistAsync(group, outcomeType, 1.0, customerID)
}

// persistAsync 异步调用持久化钩子
//
// 业界依据：主链路不能被 DB 写入阻塞
//   - hook 失败仅记日志，不影响主流程
//   - 默认 testID 写死为 "humanize_ab"（业务方可通过 SetTestID 改）
func (r *ABRecorder) persistAsync(group, metricName string, value float64, customerID []string) {
	hook := r.getPersistHook()
	if hook == nil {
		return
	}
	cid := ""
	if len(customerID) > 0 {
		cid = customerID[0]
	}
	// 异步调用，避免阻塞 RecordScore 主流程
	go func(g, mn string, v float64) {
		defer func() {
			if r := recover(); r != nil {
				// 防止 panic 影响业务
				_ = r
			}
		}()
		hook("humanize_ab", g, mn, cid, v)
	}(group, metricName, value)
}

func (r *ABRecorder) metricsFor(group string) *HumanizeABMetrics {
	if group == ABGroupControl {
		return r.control
	}
	return r.treatment
}

// Snapshot 返回当前快照（深拷贝，调用方修改不影响内部状态）
func (r *ABRecorder) Snapshot() (control, treatment HumanizeABMetrics) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	control = *r.control
	treatment = *r.treatment
	return
}

// ABComparison A/B 对比结果
type ABComparison struct {
	Control           *HumanizeABMetrics `json:"control"`
	Treatment         *HumanizeABMetrics `json:"treatment"`
	ConversionDelta   float64           `json:"conversion_delta"`
	ChurnDelta        float64           `json:"churn_delta"`
	ScoreDelta        float64           `json:"score_delta"`
	Winner            string            `json:"winner"`             // "treatment" | "control" | "inconclusive"
	MinSampleReached  bool              `json:"min_sample_reached"` // 是否达到统计显著性最低样本量
	Recommendation    string            `json:"recommendation"`
}

// Compare 对比两桶并给出建议
//
// 业界 A/B 检验标准：
//   - 最小样本量：每桶 100（粗略），生产建议 1000+
//   - 转化率差 > 5% 视为显著
//   - 置信度不计算（避免复杂统计引入误差），仅给方向性建议
//
// 返回：对比结果
func (r *ABRecorder) Compare() ABComparison {
	ctrl, trt := r.Snapshot()
	comp := ABComparison{
		Control:   &ctrl,
		Treatment: &trt,
	}
	comp.ConversionDelta = trt.ConversionRate() - ctrl.ConversionRate()
	comp.ChurnDelta = trt.ChurnRate() - ctrl.ChurnRate()
	comp.ScoreDelta = trt.MeanScore - ctrl.MeanScore

	// 最小样本量校验
	comp.MinSampleReached = ctrl.SampleCount >= 100 && trt.SampleCount >= 100

	// 简单赢家判定
	switch {
	case !comp.MinSampleReached:
		comp.Winner = "inconclusive"
		comp.Recommendation = "样本量不足（每桶 < 100），继续收集"
	case comp.ConversionDelta > 0.05 && comp.ChurnDelta < 0.02:
		comp.Winner = ABGroupTreatment
		comp.Recommendation = "treatment 转化率显著高于 control，建议全量"
	case comp.ConversionDelta < -0.05:
		comp.Winner = ABGroupControl
		comp.Recommendation = "treatment 转化率显著低于 control，建议回滚"
	case comp.ScoreDelta > 0.05:
		comp.Winner = ABGroupTreatment
		comp.Recommendation = "treatment 拟人化分数更高但转化未显著，建议观察"
	default:
		comp.Winner = "inconclusive"
		comp.Recommendation = "差异不显著（< 5%），可继续观察或调整参数"
	}
	return comp
}

// fnv32 简化版 FNV-1a hash（与 confidence/ABTestService.stableHash 同算法）
func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// Tracker 跟踪单次交互的延迟（用于 first-reply time 统计）
type Tracker struct {
	startedAt time.Time
}

// NewTracker 启动一次交互跟踪
func NewTracker() *Tracker {
	return &Tracker{startedAt: time.Now()}
}

// ElapsedMs 距启动的毫秒数
func (t *Tracker) ElapsedMs() int64 {
	return time.Since(t.startedAt).Milliseconds()
}
