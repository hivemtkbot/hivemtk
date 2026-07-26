package confidence

// ab_test.go A/B 测试服务
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.14
//
// 支持流量分桶、定向规则、Mann-Whitney U 检验、Bootstrap CI
//
// 统计方法：
//   1. Mann-Whitney U 检验：非参数检验，不要求数据正态分布
//      H0: 两个样本来自相同分布
//      U = Σ_{i∈A,j∈B} [x_i < y_j] + 0.5*[x_i == y_j]
//      p 值通过正态近似计算（n > 20 时）
//   2. Bootstrap CI：重采样 10000 次估计均值差的 95% 置信区间

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sort"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// ABTestService A/B 测试服务
type ABTestService struct {
	repo *repository.ABTestRepository
	rng  *rand.Rand
}

// NewABTestService 创建 A/B 测试服务
func NewABTestService(repo *repository.ABTestRepository) *ABTestService {
	return &ABTestService{
		repo: repo,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Assign 分桶：根据 customer_id 哈希分配到 control / treatment
//
// 一致性哈希：同一 customer_id 始终分到同一组
// 测试不存在或非 running 状态时默认返回 control
func (s *ABTestService) Assign(ctx context.Context, testID, customerID string) (string, error) {
	test, err := s.repo.GetByTestID(ctx, testID)
	if err != nil || test == nil || test.Status != "running" {
		return "control", nil
	}
	// 一致性哈希
	h := stableHash(customerID)
	controlRatio := getFloat(test.TrafficSplit, "control")
	if h < controlRatio {
		return "control", nil
	}
	return "treatment", nil
}

// RecordMetric 记录单条指标样本
//
// 写入 ab_test_metrics 表
func (s *ABTestService) RecordMetric(ctx context.Context, testID, group, metricName string, value float64) error {
	m := &model.ABTestMetric{
		TestID:     testID,
		Group:      group,
		MetricName: metricName,
		Value:      value,
	}
	return s.repo.RecordMetric(ctx, m)
}

// Analyze 执行统计分析
//
// 1. Mann-Whitney U 检验
// 2. Bootstrap CI（重采样 10000 次估计均值的 95% 置信区间）
//
// 样本不足（<10）时返回 ErrInsufficientSamples
func (s *ABTestService) Analyze(ctx context.Context, testID, metricName string) (*dto.ABTestAnalysis, error) {
	controlSamples, err := s.repo.ListMetricSamples(ctx, testID, "control", metricName)
	if err != nil {
		return nil, err
	}
	treatmentSamples, err := s.repo.ListMetricSamples(ctx, testID, "treatment", metricName)
	if err != nil {
		return nil, err
	}
	if len(controlSamples) < 10 || len(treatmentSamples) < 10 {
		return nil, ErrInsufficientSamples
	}

	// 1. Mann-Whitney U
	u, p := mannWhitneyU(controlSamples, treatmentSamples)

	// 2. Bootstrap CI（treatment - control 的均值差）
	ciLower, ciUpper := bootstrapDifferenceCI(controlSamples, treatmentSamples, 10000, s.rng)

	// 3. 更新测试记录
	test, _ := s.repo.GetByTestID(ctx, testID)
	if test != nil {
		test.MannWhitneyU = u
		test.MannWhitneyP = p
		test.BootstrapCILower = ciLower
		test.BootstrapCIUpper = ciUpper
		test.BootstrapN = 10000
		_ = s.repo.Update(ctx, test)
	}

	return &dto.ABTestAnalysis{
		ControlMean:      mean(controlSamples),
		TreatmentMean:    mean(treatmentSamples),
		Difference:       mean(treatmentSamples) - mean(controlSamples),
		MannWhitneyU:     u,
		MannWhitneyP:     p,
		BootstrapCILower: ciLower,
		BootstrapCIUpper: ciUpper,
		Significant:      p < 0.05 && ciLower > 0,
	}, nil
}

// ErrInsufficientSamples 样本不足
var ErrInsufficientSamples = errors.New("insufficient samples for analysis (need >=10 per group)")

// ============================================================================
// 统计函数
// ============================================================================

// mannWhitneyU Mann-Whitney U 检验
//
// H0: 两个样本来自相同分布
// U = Σ_{i∈A,j∈B} [x_i < y_j] + 0.5*[x_i == y_j]
// p 值通过正态近似计算（n > 20 时）
//
// 返回 (U 统计量, p 值)
func mannWhitneyU(a, b []float64) (u, p float64) {
	n1 := len(a)
	n2 := len(b)
	if n1 == 0 || n2 == 0 {
		return 0, 1.0
	}

	// 合并排序
	type pair struct {
		val   float64
		group int // 0=a, 1=b
	}
	combined := make([]pair, 0, n1+n2)
	for _, v := range a {
		combined = append(combined, pair{v, 0})
	}
	for _, v := range b {
		combined = append(combined, pair{v, 1})
	}
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].val < combined[j].val
	})

	// 计算每个元素的 rank（处理 ties 用平均 rank）
	ranks := make([]float64, len(combined))
	i := 0
	for i < len(combined) {
		j := i
		for j+1 < len(combined) && combined[j+1].val == combined[i].val {
			j++
		}
		avgRank := float64(i+j+2) / 2 // rank 从 1 开始
		for k := i; k <= j; k++ {
			ranks[k] = avgRank
		}
		i = j + 1
	}

	// R1 = A 组 rank 之和
	r1 := 0.0
	for k, pr := range combined {
		if pr.group == 0 {
			r1 += ranks[k]
		}
	}
	u1 := r1 - float64(n1*(n1+1))/2
	u2 := float64(n1*n2) - u1
	u = math.Min(u1, u2)

	// 正态近似
	mu := float64(n1*n2) / 2
	sigma := math.Sqrt(float64(n1*n2*(n1+n2+1)) / 12)
	if sigma == 0 {
		return u, 1.0
	}
	z := (u - mu) / sigma
	p = 2 * (1 - normalCDF(math.Abs(z)))
	if p > 1 {
		p = 1
	}
	return u, p
}

// normalCDF 标准正态分布 CDF
//
// 使用误差函数 erf 实现
func normalCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt(2)))
}

// bootstrapDifferenceCI Bootstrap 重采样估计均值差的 95% CI
//
// 重采样 n 次，每次从 a 和 b 中有放回采样，计算均值差
// 返回 (lower, upper) 95% 置信区间
func bootstrapDifferenceCI(a, b []float64, n int, rng *rand.Rand) (float64, float64) {
	if len(a) == 0 || len(b) == 0 || n <= 0 {
		return 0, 0
	}
	diffs := make([]float64, n)
	for i := 0; i < n; i++ {
		sa := resample(a, rng)
		sb := resample(b, rng)
		diffs[i] = mean(sb) - mean(sa)
	}
	sort.Float64s(diffs)
	loIdx := int(0.025 * float64(n))
	hiIdx := int(0.975 * float64(n))
	if loIdx >= n {
		loIdx = n - 1
	}
	if hiIdx >= n {
		hiIdx = n - 1
	}
	return diffs[loIdx], diffs[hiIdx]
}

// resample 有放回重采样
func resample(src []float64, rng *rand.Rand) []float64 {
	dst := make([]float64, len(src))
	for i := range dst {
		dst[i] = src[rng.Intn(len(src))]
	}
	return dst
}

// mean 均值
func mean(a []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range a {
		sum += v
	}
	return sum / float64(len(a))
}

// stableHash 稳定哈希 [0, 1)
//
// 使用 FNV-1a 哈希，对同一字符串始终返回同一值
func stableHash(s string) float64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return float64(h%10000) / 10000.0
}

// getFloat 从 JSONMap 中读取 float64
//
// 默认返回 0.5
func getFloat(m map[string]any, key string) float64 {
	if m == nil {
		return 0.5
	}
	if v, ok := m[key]; ok {
		switch x := v.(type) {
		case float64:
			return x
		case int:
			return float64(x)
		case int64:
			return float64(x)
		}
	}
	return 0.5
}
