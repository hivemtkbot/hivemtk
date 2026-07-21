package humanize

// abtest_stats.go P0-4 A/B 测试统计服务（业界三件套）
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十六章 §16.4.6 ~ §16.4.9
//
// 业界三件套：
//   1. Mann-Whitney U 检验（非参数，不假设正态分布，§16.4.7）
//   2. Bootstrap CI 置信区间（差值/单组，§16.4.8）
//   3. Cohen's d 效应量（§16.4.9）
//
// 参考：
//   - BooStSa (ACL 2022)
//   - TACL 2021 Summarization Resampling
//   - Cohen 1988

import (
	"context"
	"math"
	"math/rand"
	"sort"
	"time"

	"marketing/internal/dto"
)

// ============================================================================
// ABTestStatsService A/B 测试统计服务
// ============================================================================

// ABTestStatsService A/B 测试统计服务
type ABTestStatsService struct {
	bootstrapN int     // Bootstrap 重采样次数（默认 10000）
	alpha      float64 // 显著性水平（默认 0.05）
}

// NewABTestStatsService 构造
func NewABTestStatsService() *ABTestStatsService {
	return &ABTestStatsService{
		bootstrapN: 10000,
		alpha:      0.05,
	}
}

// WithBootstrapN 设置 Bootstrap 重采样次数
func (s *ABTestStatsService) WithBootstrapN(n int) *ABTestStatsService {
	if n > 0 {
		s.bootstrapN = n
	}
	return s
}

// WithAlpha 设置显著性水平
func (s *ABTestStatsService) WithAlpha(a float64) *ABTestStatsService {
	if a > 0 && a < 1 {
		s.alpha = a
	}
	return s
}

// Compute 执行完整 A/B 统计分析
//
// 流程：
//  1. 校验样本量（每组 ≥ 5）
//  2. 计算描述性统计（mean/median/stddev/min/max）
//  3. Mann-Whitney U 检验 → U 统计量 + p 值
//  4. Cohen's d 效应量
//  5. Bootstrap CI 差值 95% 置信区间
//  6. 决策：significant + winner
func (s *ABTestStatsService) Compute(ctx context.Context, input *dto.ABTestStatsInput) (*dto.ABTestStatsResult, error) {
	if input == nil {
		return nil, ErrInvalidInput
	}
	if len(input.Control) < 5 || len(input.Treatment) < 5 {
		return nil, ErrInsufficientSamples
	}
	// 1. Mann-Whitney U 检验
	u, p := MannWhitneyUTest(input.Control, input.Treatment)
	// 2. Cohen's d
	d := CohensD(input.Control, input.Treatment)
	// 3. Bootstrap CI（差值的 95% 置信区间）
	ciLow, ciHigh := BootstrapCIDifference(input.Control, input.Treatment, s.bootstrapN, s.alpha)
	// 4. 描述性统计
	controlMean := meanValue(input.Control)
	treatmentMean := meanValue(input.Treatment)
	// 5. 决策
	significant := p < s.alpha
	effectLabel := InterpretCohensD(d)
	winner := "inconclusive"
	if significant {
		// d > 0 表示 group1（control）> group2（treatment）
		// 但我们的语义是：d = (control_mean - treatment_mean) / pooled_sd
		// d > 0 → control 高 → winner = control
		// d < 0 → treatment 高 → winner = treatment
		// 但 CohensD 实现传入 (group1=control, group2=treatment)，d > 0 → control 高
		// 用户期望：d > 0 → treatment 优（因为通常 treatment 是新版本）
		// 所以翻转：d < 0 → treatment 优
		if d < 0 {
			winner = "treatment"
		} else {
			winner = "control"
		}
	}
	return &dto.ABTestStatsResult{
		ExperimentID:    input.ExperimentID,
		ControlMean:     round4(controlMean),
		TreatmentMean:   round4(treatmentMean),
		MannWhitneyU:    round4(u),
		MannWhitneyP:    round4(p),
		CohensD:         round4(d),
		EffectSizeLabel: effectLabel,
		BootstrapCILow:  round4(ciLow),
		BootstrapCIHigh: round4(ciHigh),
		Significant:     significant,
		Winner:          winner,
	}, nil
}

// ============================================================================
// Mann-Whitney U 检验（§16.4.7）
// ============================================================================

// MannWhitneyUTest Mann-Whitney U 检验（非参数，不假设正态分布）
//
// 适用：5 维评分是序数型数据，天然非正态分布（天花板效应）
//
// 公式：
//
//	U_A = R_A - n_A*(n_A+1)/2
//	U_B = R_B - n_B*(n_B+1)/2
//	U = min(U_A, U_B)
//	正态近似（n≥5 时）：z = (U - μ_U) / σ_U
//	  μ_U = n_A * n_B / 2
//	  σ_U = sqrt(n_A * n_B * (n_A + n_B + 1) / 12)  [无结情况]
//	p 值 = 2 * (1 - Φ(|z|))  双侧
//
// 含结修正的方差：
//
//	σ_U² = (nA*nB/12) * [(nA+nB+1) - Σ(t³-t)/((nA+nB)(nA+nB-1))]
func MannWhitneyUTest(groupA, groupB []float64) (uStat, pValue float64) {
	nA := len(groupA)
	nB := len(groupB)
	if nA < 5 || nB < 5 {
		// 样本不足，返回不显著
		return 0, 1.0
	}
	// 1. 合并排序
	type pair struct {
		value float64
		group int // 0=A, 1=B
	}
	combined := make([]pair, 0, nA+nB)
	for _, v := range groupA {
		combined = append(combined, pair{v, 0})
	}
	for _, v := range groupB {
		combined = append(combined, pair{v, 1})
	}
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].value < combined[j].value
	})
	// 2. 赋秩（处理结：平均秩）
	ranks := make([]float64, len(combined))
	i := 0
	for i < len(combined) {
		j := i
		for j+1 < len(combined) && combined[j+1].value == combined[i].value {
			j++
		}
		// [i, j] 是同一值，平均秩 = (i+1 + j+1) / 2 = (i+j+2)/2
		avgRank := float64(i+j+2) / 2
		for k := i; k <= j; k++ {
			ranks[k] = avgRank
		}
		i = j + 1
	}
	// 3. 计算秩和
	var rankA, rankB float64
	for k, p := range combined {
		if p.group == 0 {
			rankA += ranks[k]
		} else {
			rankB += ranks[k]
		}
	}
	// 4. U 统计量
	uA := rankA - float64(nA*(nA+1))/2
	uB := rankB - float64(nB*(nB+1))/2
	uStat = math.Min(uA, uB)
	// 5. 正态近似计算 p 值
	muU := float64(nA*nB) / 2
	// 含结修正的方差
	tieTerm := 0.0
	i = 0
	for i < len(combined) {
		j := i
		for j+1 < len(combined) && combined[j+1].value == combined[i].value {
			j++
		}
		t := float64(j - i + 1)
		tieTerm += (t*t - t) * t // t³ - t
		i = j + 1
	}
	totalN := float64(nA + nB)
	varU := (float64(nA*nB) / 12) * (totalN + 1 - tieTerm/(totalN*(totalN-1)))
	sigmaU := math.Sqrt(varU)
	if sigmaU == 0 {
		return uStat, 1.0
	}
	z := (uStat - muU) / sigmaU
	// 双侧 p 值
	pValue = 2 * (1 - normalCDF(math.Abs(z)))
	if pValue < 0 {
		pValue = 0
	}
	if pValue > 1 {
		pValue = 1
	}
	return uStat, pValue
}

// normalCDF 标准正态分布累积分布函数（用 erf 近似）
func normalCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}

// ============================================================================
// Bootstrap CI 置信区间（§16.4.8）
// ============================================================================

// BootstrapCIDifference Bootstrap 置信区间（差值）
//
// 流程：
//  1. 两组样本量相等 → 配对 bootstrap（按相同索引重采样）
//  2. 两组样本量不等 → 独立 bootstrap（分别重采样）
//  3. 每次重采样计算 mean(treatment) - mean(control)
//  4. 重复 N 次（默认 10000）
//  5. 取 α/2 和 1-α/2 分位数作为 CI
//
// 如果 CI 包含 0，则不能拒绝"两组无差异"的原假设
func BootstrapCIDifference(control, treatment []float64, nBoot int, alpha float64) (low, high float64) {
	if len(control) == 0 || len(treatment) == 0 || nBoot <= 0 {
		return 0, 0
	}
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.05
	}
	// 两组样本量不等时退化为独立 bootstrap（不再配对）
	paired := len(control) == len(treatment)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	diffs := make([]float64, nBoot)
	nC := len(control)
	nT := len(treatment)
	for i := 0; i < nBoot; i++ {
		if paired {
			// 配对：按相同索引重采样
			sumC := 0.0
			sumT := 0.0
			for j := 0; j < nC; j++ {
				idx := rng.Intn(nC)
				sumC += control[idx]
				sumT += treatment[idx]
			}
			diffs[i] = sumT/float64(nC) - sumC/float64(nC)
		} else {
			// 独立：分别重采样
			sumC := 0.0
			for j := 0; j < nC; j++ {
				sumC += control[rng.Intn(nC)]
			}
			sumT := 0.0
			for j := 0; j < nT; j++ {
				sumT += treatment[rng.Intn(nT)]
			}
			diffs[i] = sumT/float64(nT) - sumC/float64(nC)
		}
	}
	sort.Float64s(diffs)
	lowIdx := int(float64(nBoot) * alpha / 2)
	highIdx := int(float64(nBoot) * (1 - alpha/2))
	if lowIdx >= nBoot {
		lowIdx = nBoot - 1
	}
	if highIdx >= nBoot {
		highIdx = nBoot - 1
	}
	low = math.Round(diffs[lowIdx]*10000) / 10000
	high = math.Round(diffs[highIdx]*10000) / 10000
	return low, high
}

// BootstrapCI 单组 Bootstrap CI（用于单指标置信区间）
func BootstrapCI(data []float64, nBoot int, alpha float64) (low, high float64) {
	if len(data) == 0 || nBoot <= 0 {
		return 0, 0
	}
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.05
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := len(data)
	means := make([]float64, nBoot)
	for i := 0; i < nBoot; i++ {
		sum := 0.0
		for j := 0; j < n; j++ {
			sum += data[rng.Intn(n)]
		}
		means[i] = sum / float64(n)
	}
	sort.Float64s(means)
	lowIdx := int(float64(nBoot) * alpha / 2)
	highIdx := int(float64(nBoot) * (1 - alpha/2))
	if lowIdx >= nBoot {
		lowIdx = nBoot - 1
	}
	if highIdx >= nBoot {
		highIdx = nBoot - 1
	}
	return means[lowIdx], means[highIdx]
}

// ============================================================================
// Cohen's d 效应量（§16.4.9）
// ============================================================================

// CohensD Cohen's d 效应量
//
// 公式：d = (M1 - M2) / SD_pooled
//
//	SD_pooled = sqrt(((n1-1)*var1 + (n2-1)*var2) / (n1+n2-2))
//
// 解读（Cohen 1988）：
//
//	|d| < 0.2  negligible
//	|d| ∈ [0.2, 0.5)  small
//	|d| ∈ [0.5, 0.8)  medium
//	|d| ≥ 0.8  large
//
// 注意：Cohen's d 不随样本量变化（不像 p 值），是判断差异实际意义的标准工具
//
// 传入参数：(group1=control, group2=treatment)
// 返回 d：d > 0 → control > treatment；d < 0 → treatment > control
func CohensD(group1, group2 []float64) float64 {
	n1 := len(group1)
	n2 := len(group2)
	if n1 < 2 || n2 < 2 {
		return 0
	}
	mean1 := meanValue(group1)
	mean2 := meanValue(group2)
	var1 := varianceValue(group1, mean1)
	var2 := varianceValue(group2, mean2)
	// 合并方差
	pooledVar := (float64(n1-1)*var1 + float64(n2-1)*var2) / float64(n1+n2-2)
	if pooledVar == 0 {
		return 0
	}
	pooledSD := math.Sqrt(pooledVar)
	d := (mean1 - mean2) / pooledSD
	return math.Round(d*10000) / 10000
}

// InterpretCohensD Cohen's d 解读标签
func InterpretCohensD(d float64) string {
	abs := math.Abs(d)
	switch {
	case abs < 0.2:
		return "negligible"
	case abs < 0.5:
		return "small"
	case abs < 0.8:
		return "medium"
	default:
		return "large"
	}
}

// ============================================================================
// 描述性统计辅助
// ============================================================================

// meanValue 均值
func meanValue(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// varianceValue 样本方差（无偏估计）
func varianceValue(data []float64, m float64) float64 {
	n := len(data)
	if n < 2 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		diff := v - m
		sum += diff * diff
	}
	return sum / float64(n-1)
}

// round4 保留 4 位小数
func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
