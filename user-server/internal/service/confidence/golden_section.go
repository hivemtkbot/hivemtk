package confidence

// golden_section.go 黄金分割搜索一维优化器
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.4
//
// 用于在 [a, b] 区间内寻找使目标函数最小的 T
// 适用前提：目标函数单峰凸（NLL 对 T 满足，Guo 2017 Lemma 1）
//
// 黄金比例 φ = (sqrt(5) - 1) / 2 ≈ 0.618
// 每次迭代区间缩小 38.2%，收敛条件：区间长度 < tolerance

import "math"

// GoldenSectionSearcher 黄金分割搜索器
type GoldenSectionSearcher struct {
	tolerance float64
	maxIter   int
}

// NewGoldenSectionSearcher 创建搜索器
//
// tolerance: 区间收敛阈值（默认 1e-4）
// maxIter:   最大迭代次数（默认 100）
func NewGoldenSectionSearcher(tolerance float64, maxIter int) *GoldenSectionSearcher {
	if tolerance <= 0 {
		tolerance = 1e-4
	}
	if maxIter <= 0 {
		maxIter = 100
	}
	return &GoldenSectionSearcher{tolerance: tolerance, maxIter: maxIter}
}

// Minimize 在 [a, b] 上最小化目标函数 f，返回最优 x*
//
// 算法：
//  1. 取 x1 = b - φ*(b-a), x2 = a + φ*(b-a)
//  2. 比较 f(x1) 和 f(x2)：
//     - f(x1) < f(x2)：最优点在 [a, x2]，淘汰右段
//     - f(x1) >= f(x2)：最优点在 [x1, b]，淘汰左段
//  3. 重复直到区间 < tolerance 或达到 maxIter
//  4. 返回 (a+b)/2
func (g *GoldenSectionSearcher) Minimize(f func(float64) float64, a, b float64) float64 {
	if a >= b {
		return (a + b) / 2
	}
	// 黄金比例 φ = (sqrt(5) - 1) / 2 ≈ 0.618
	phi := (math.Sqrt(5) - 1) / 2

	// 初始两个内点
	x1 := b - phi*(b-a)
	x2 := a + phi*(b-a)
	f1 := f(x1)
	f2 := f(x2)

	for i := 0; i < g.maxIter && (b-a) > g.tolerance; i++ {
		if f1 < f2 {
			// 最优点在 [a, x2]，淘汰右段
			b = x2
			x2 = x1
			f2 = f1
			x1 = b - phi*(b-a)
			f1 = f(x1)
		} else {
			// 最优点在 [x1, b]，淘汰左段
			a = x1
			x1 = x2
			f1 = f2
			x2 = a + phi*(b-a)
			f2 = f(x2)
		}
	}
	return (a + b) / 2
}
