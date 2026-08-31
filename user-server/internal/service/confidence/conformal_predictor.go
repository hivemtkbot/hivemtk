package confidence

import (
	"math"
	"sort"
)

// ConformalPredictor 共形预测器（Conformal Prediction）
//
// 学术依据：
//   - Shafer, G. & Vovk, V. (2008). "A Tutorial on Conformal Prediction".
//   - 业界实现：scikit-learn `mapie`、Python `nonconformist`、R `conformalInference`。
//   - 适用：selective prediction（高风险 abstention）—— 当 LLM 置信度 < 阈值时主动转人工。
//
// 核心思想（distribution-free）：
//   - 给定校准集 {(x_i, y_i)} 和任意非一致性度量（non-conformity score）s(x, y)
//   - 计算每个样本的非一致性分数 α_i = s(x_i, y_i)
//   - 排序 α_1 ≤ α_2 ≤ ... ≤ α_n
//   - 给定目标覆盖率 1-δ，取 q = ceil((n+1)·(1-δ)) / n 分位数作为阈值
//   - 预测时：输出预测集 {y : s(x, y) ≤ q}
//
// 关键性质：对于任意新样本 (X, Y)，P(Y ∈ C(X)) ≥ 1-δ
//   - 这是**有限样本保证**（不依赖数据分布假设）
//   - 比"经验校准"（如 Platt/Temperature）更强——后者只保证渐近
//
// 业界应用：
//   - Selective prediction for medical/legal：高风险 abstention 转人工专家
//   - 客服场景：置信度 < 阈值 → 转人工
//   - LLM 输出：单次 LLM 输出的事实性可声明 coverage guarantee
type ConformalPredictor struct {
	calibrationScores []float64 // 校准集的非一致性分数（升序）
	delta             float64   // 显著性水平（如 0.1 → 90% 覆盖率）
}

// NewConformalPredictor 构造共形预测器
//
// scores: 校准集的非一致性分数（任意非负实数）
// delta: 显著性水平；0.1 表示目标 90% 覆盖率
//
// 业界参数选择：
//   - 客服/医疗场景：delta=0.1（90% 覆盖率，宁可过转人工不可漏转）
//   - 推荐系统：delta=0.05（95% 覆盖率，更激进）
//   - 高频交易：delta=0.01（99% 覆盖率，极保守）
func NewConformalPredictor(scores []float64, delta float64) *ConformalPredictor {
	if delta <= 0 || delta >= 1 {
		delta = 0.1 // 默认 90% 覆盖率
	}
	sorted := make([]float64, len(scores))
	copy(sorted, scores)
	sort.Float64s(sorted)
	return &ConformalPredictor{
		calibrationScores: sorted,
		delta:             delta,
	}
}

// Quantile 计算 1-delta 覆盖率下的非一致性分数阈值
//
// 公式（Vovk et al. 2005, "Algorithmic Learning in a Random World" §4.1）：
//
//	q = ⌈(n+1)·(1-δ)⌉ / n 分位数
//
// 例子：n=100, δ=0.1
//
//	q = ⌈101·0.9⌉ / 100 = ⌈90.9⌉ / 100 = 91 / 100 = 0.91
//	即排序后第 91 个分数作为阈值
//
// 返回：阈值；新样本的非一致性分数 ≤ 阈值 → 预测可信
func (c *ConformalPredictor) Quantile() float64 {
	n := len(c.calibrationScores)
	if n == 0 {
		return math.Inf(1) // 无校准数据 → 永远 abstention
	}
	// Vovk's formula: q = ceil((n+1)*(1-δ)) / n
	// 索引：1-based
	idx := int(math.Ceil(float64(n+1) * (1 - c.delta)))
	if idx < 1 {
		idx = 1
	}
	if idx > n {
		idx = n
	}
	// 1-based to 0-based
	return c.calibrationScores[idx-1]
}

// PredictSet 根据非一致性分数输出预测集合
//
// scores: 对每个候选 label 计算的非一致性分数（如 s(x, y_candidate)）
// 返回：1-based 索引数组，索引 i 表示"labels[i] 在预测集中"
//
// 用法：给定输入 x，对每个候选 label y_i 计算 s(x, y_i)，
//
//	返回 s(x, y_i) ≤ Quantile() 的所有 y_i
func (c *ConformalPredictor) PredictSet(scores []float64) []int {
	threshold := c.Quantile()
	out := make([]int, 0, len(scores))
	for i, s := range scores {
		if s <= threshold {
			out = append(out, i+1) // 1-based
		}
	}
	return out
}

// CoverageGuarantee 报告当前校准集的有限样本覆盖率保证
//
// 返回：1-δ（即 "至少 X% 的未来样本会被正确覆盖"）
func (c *ConformalPredictor) CoverageGuarantee() float64 {
	return 1 - c.delta
}

// CalibrateOnline 在线校准：添加新样本后重新计算分位数
//
// 业界做法：滑动窗口校准（rolling window），让分布漂移得到修正。
// 例如：保留最近 1000 个真实 (prediction, label) 样本，每日 recalibrate。
func (c *ConformalPredictor) CalibrateOnline(newScore float64, maxRetained int) {
	c.calibrationScores = append(c.calibrationScores, newScore)
	if maxRetained > 0 && len(c.calibrationScores) > maxRetained {
		// 滑动窗口：删除最早的
		c.calibrationScores = c.calibrationScores[len(c.calibrationScores)-maxRetained:]
	}
	sort.Float64s(c.calibrationScores)
}

// CoverageEmpirical 经验覆盖率：用于 A/B test 校准质量
//
// 比较：经验覆盖率 vs 1-δ 保证值
// 经验 ≈ 保证值时说明校准无偏；
// 经验 << 保证值时说明模型欠置信（over-conservative）→ 阈值偏低；
// 经验 >> 保证值时说明校准不严格（但 conformal 保证仍为下界）
func CoverageEmpirical(predictions, labels []int) float64 {
	if len(predictions) == 0 || len(predictions) != len(labels) {
		return 0
	}
	covered := 0
	for i := range predictions {
		if predictions[i] == labels[i] {
			covered++
		}
	}
	return float64(covered) / float64(len(predictions))
}

// BrierScore Brier 分数：均方概率误差，衡量概率预测质量
//
// 公式：BS = (1/N) Σ (p_i - y_i)²
// 范围：[0, 1]，0 = 完美预测
//
// 与 conformal prediction 配合使用：A/B test 哪种校准方法 Brier 更低
func BrierScore(probabilities []float64, labels []int) float64 {
	if len(probabilities) == 0 || len(probabilities) != len(labels) {
		return 0
	}
	sum := 0.0
	for i, p := range probabilities {
		y := float64(labels[i])
		d := p - y
		sum += d * d
	}
	return sum / float64(len(probabilities))
}

// SelectivePredict 决定是否"高风险 abstention"（转人工）
//
// threshold: 1-δ 保证下量化阈值（越小越保守）
// score: 当前样本的非一致性分数
// 返回：true = 应当 abstention（转人工）；false = 模型可自主回答
//
// 业界用法：
//   - LLM 输出 factuality score < threshold → "我不确定，请联系人工"
//   - Intent 分类 non-conformity > threshold → 转人工
func SelectivePredict(threshold, score float64) bool {
	return score > threshold
}
