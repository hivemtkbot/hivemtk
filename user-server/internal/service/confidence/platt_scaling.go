package confidence

import (
	"math"
)

// PlattScaling 二分类置信度 Platt 校准器。
//
// 学术依据：
//   - Platt, J. (1999). "Probabilistic Outputs for Support Vector Machines
//     and Comparisons to Regularized Likelihood Methods".
//   - 业界实现标准：scikit-learn `_SVC._predict_proba_lr` 用同样公式。
//   - 适用：二分类（如「这个 intent 分类是否正确」「这段 AI 回复是否要转人工」）。
//   - 与 Temperature Scaling 的关系：
//   - Temperature：单参数 T 缩放 logits 后 softmax（多分类）
//   - Platt：2 参数 (A, B)，对单 logit 做 sigmoid：P(y=1|x) = 1/(1+exp(A·z + B))
//
// 训练算法：Newton-Raphson MLE（Platt 原论文 §2.2）。
//   - 目标：最小化 NLL of sigmoid(A·z_i + B) vs label y_i ∈ {0,1}
//   - 优点：2 参数拟合极快（O(N) 迭代 1-10 次收敛），不需要交叉验证
//   - 缺点：仅限二分类（多分类需 OvR 或 one-vs-rest）
//
// 数值稳定：
//   - log1p 防止 sigmoid 在极端值处下溢/上溢
//   - loss 限幅避免 log(0) → -Inf
type PlattScaling struct {
	A float64 // logit 缩放系数
	B float64 // bias 偏置
}

// NewPlattScaling 创建 Platt 校准器（默认 A=1, B=0 = 无校准）
func NewPlattScaling() *PlattScaling {
	return &PlattScaling{A: 1.0, B: 0.0}
}

// PlattSample Platt 校准样本
type PlattSample struct {
	DecisionValue float64 // 模型输出（logit 或分数，越大越倾向 y=1）
	Label         int     // 真实标签：1 = 正例，0 = 反例
}

// Fit 用 Newton-Raphson MLE 拟合 (A, B)。
//
// 数学推导（来自 Platt 1999 论文）：
//
//	令 z_i = A·f_i + B，σ(z) = 1/(1+exp(-z))
//	NLL = -Σ [ t_i · log(σ(z_i)) + (1-t_i) · log(1-σ(z_i)) ]
//	∂NLL/∂A = Σ (σ(z_i) - t_i) · f_i    （err · f）
//	∂NLL/∂B = Σ (σ(z_i) - t_i)           （err）
//	∂²NLL/∂A² = Σ σ(z_i)·(1-σ(z_i)) · f_i²  （正定）
//	∂²NLL/∂B² = Σ σ(z_i)·(1-σ(z_i))
//	∂²NLL/∂A∂B = Σ σ(z_i)·(1-σ(z_i)) · f_i
//
// Newton 更新（最小化 NLL）：
//
//	[A, B] -= H^(-1) · ∇NLL
//	H = [[hAA, hAB], [hAB, hBB]] 全部为正（与原 platt_scaling.go 草稿不同）
//
// 返回：训练后的 PlattScaling 实例（链式调用友好）。
func (p *PlattScaling) Fit(samples []PlattSample) *PlattScaling {
	if len(samples) == 0 {
		return p
	}
	// 初始化为 identity（A=1, B=0）
	a, b := 1.0, 0.0

	// Newton-Raphson 迭代（Platt 1999 推荐 10 次足够收敛）
	const maxIter = 50
	const tol = 1e-7
	for iter := 0; iter < maxIter; iter++ {
		gA, gB := 0.0, 0.0
		hAA, hBB, hAB := 0.0, 0.0, 0.0
		for _, s := range samples {
			z := a*s.DecisionValue + b
			// 数值稳定 sigmoid
			sig := stableSigmoid(z)
			err := sig - float64(s.Label)
			gA += err * s.DecisionValue
			gB += err
			sig1ms := sig * (1 - sig)
			// Hessian of NLL（正定；NLL 是凸函数）
			hAA += sig1ms * s.DecisionValue * s.DecisionValue
			hBB += sig1ms
			hAB += sig1ms * s.DecisionValue
		}

		// 解 2x2 线性系统 H · d = g，即 d = H^(-1) · g
		// H^(-1) = (1/det) [[hBB, -hAB], [-hAB, hAA]]
		// dA = (hBB*gA - hAB*gB) / det
		// dB = (hAA*gB - hAB*gA) / det
		det := hAA*hBB - hAB*hAB
		if det < 1e-12 {
			// Hessian 奇异（样本同值或 N 极小），停止
			break
		}
		dA := (hBB*gA - hAB*gB) / det
		dB := (hAA*gB - hAB*gA) / det
		a -= dA
		b -= dB

		// 收敛：步长 < tol
		if math.Abs(dA) < tol && math.Abs(dB) < tol {
			break
		}
	}

	p.A = a
	p.B = b
	return p
}

// Predict 校准后的 y=1 概率
func (p *PlattScaling) Predict(decisionValue float64) float64 {
	z := p.A*decisionValue + p.B
	return stableSigmoid(z)
}

// PredictBatch 批量校准（性能优化）
func (p *PlattScaling) PredictBatch(decisionValues []float64) []float64 {
	out := make([]float64, len(decisionValues))
	for i, z := range decisionValues {
		out[i] = p.Predict(z)
	}
	return out
}

// ECE 计算校准误差（用于监控/调参）
//
// 与 temperature_scaler.go::Calibrator.evaluate 同等语义（15 分桶）。
// 业界标准定义（Guo 2017, Naeini 2015）：ECE = Σ (|B_m|/N) · |acc(B_m) - conf(B_m)|
func (p *PlattScaling) ECE(samples []PlattSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	const bins = 15
	binConf := make([]float64, bins)
	binAcc := make([]float64, bins)
	binCount := make([]int, bins)
	for _, s := range samples {
		conf := p.Predict(s.DecisionValue)
		binIdx := int(conf * float64(bins))
		if binIdx >= bins {
			binIdx = bins - 1
		}
		if binIdx < 0 {
			binIdx = 0
		}
		binConf[binIdx] += conf
		binAcc[binIdx] += float64(s.Label)
		binCount[binIdx]++
	}
	n := float64(len(samples))
	ece := 0.0
	for i := 0; i < bins; i++ {
		if binCount[i] == 0 {
			continue
		}
		avgConf := binConf[i] / float64(binCount[i])
		avgAcc := binAcc[i] / float64(binCount[i])
		ece += (float64(binCount[i]) / n) * math.Abs(avgAcc-avgConf)
	}
	return ece
}

// Parameters 返回拟合后的 (A, B)，便于序列化/调参
func (p *PlattScaling) Parameters() (A, B float64) {
	return p.A, p.B
}

// stableSigmoid 数值稳定 sigmoid 实现（避免 exp 溢出）
//
// 标准 sigmoid: σ(z) = 1 / (1 + exp(-z))
// 当 z > 0 时: σ(z) = 1 / (1 + exp(-z)) 但 exp(-z) 在 z 大时易下溢
// 改写: σ(z) = exp(z) / (1 + exp(z))，对 z < 0 用此形式
func stableSigmoid(z float64) float64 {
	if z >= 0 {
		return 1.0 / (1.0 + math.Exp(-z))
	}
	ex := math.Exp(z)
	return ex / (1.0 + ex)
}
