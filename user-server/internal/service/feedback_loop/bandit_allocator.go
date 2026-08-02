package feedbackloop

// bandit_allocator.go Multi-Armed Bandit 流量分配器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十七章 §17.4.5
//
// 算法：Thompson Sampling on Beta(α, β) posterior
//   1. 每个 variant v 维护 Beta(alpha_v, beta_v) 后验
//   2. 采样：theta_v ~ Beta(alpha_v, beta_v)
//   3. 选择：argmax_v(theta_v)
//   4. 更新：成功 → alpha_v += 1；失败 → beta_v += 1
//
// 冷启动保护：
//   - 探索期（每臂样本 < MinSamplesForExploit）强制均匀随机分配
//   - 利用期（≥ MinSamplesForExploit）使用 Thompson Sampling
//   - 流量上限保护：单臂流量超过 TrafficCeiling 时强制探索其他臂
//   - 最低流量保障：每臂至少 ExplorationFloor（10%）
//
// 收敛判定：
//   - 蒙特卡洛采样 PosteriorSamples 次计算每臂胜出概率
//   - 若 P(arm_best) ≥ ConvergenceThreshold 且每臂样本 ≥ MinSamplesForPromote，则收敛
//
// Beta 采样实现：
//   beta(α,β) = gamma(α,1) / (gamma(α,1) + gamma(β,1))
//   Gamma 用 Marsaglia-Tsang 方法（标准数值算法，无需 gonum 依赖）

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// BanditAllocator Multi-Armed Bandit 流量分配器
type BanditAllocator struct {
	banditRepo *repository.FeedbackLoopRepository
	mu         sync.RWMutex
	cache      map[string][]*model.BanditArm // experimentID → arms（内存缓存）
	rng        *rand.Rand                    // 独立 PRNG（线程安全由 mu 保护）
	config     BanditConfig
}

// NewBanditAllocator 构造分配器
//
// 参数：
//
//	db     - GORM DB（用于读写 bandit_arms 表，内部下沉到 repository 层使用）
//	cfg    - 配置（零值字段会用默认值填充）
//	seed   - PRNG 种子（测试时可固定；生产用 time.Now().UnixNano()）
func NewBanditAllocator(db *gorm.DB, cfg BanditConfig, seed int64) *BanditAllocator {
	if cfg.MinSamplesForExploit == 0 {
		cfg.MinSamplesForExploit = 30
	}
	if cfg.ExplorationFloor == 0 {
		cfg.ExplorationFloor = 0.10
	}
	if cfg.TrafficCeiling == 0 {
		cfg.TrafficCeiling = 0.60
	}
	if cfg.ConvergenceThreshold == 0 {
		cfg.ConvergenceThreshold = 0.95
	}
	if cfg.MinSamplesForPromote == 0 {
		cfg.MinSamplesForPromote = 100
	}
	if cfg.PosteriorSamples == 0 {
		cfg.PosteriorSamples = 1000
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &BanditAllocator{
		banditRepo: repository.NewFeedbackLoopRepositoryWithDB(db),
		cache:      make(map[string][]*model.BanditArm),
		rng:        rand.New(rand.NewSource(seed)),
		config:     cfg,
	}
}

// SelectArm 为指定实验选择一个臂（Thompson Sampling）
//
// 冷启动期（每臂样本 < MinSamplesForExploit）强制均匀随机分配
// 利用期使用 Thompson Sampling
// 流量上限保护：单臂流量超过 TrafficCeiling 时强制探索其他臂
//
// 返回：
//
//	armKey  - 选中的 arm key
//	strategy - cold_start_uniform / thompson_sampling / forced_explore
//	err     - 错误（无臂/DB 错误）
func (b *BanditAllocator) SelectArm(ctx context.Context, experimentID string) (armKey, strategy string, err error) {
	if experimentID == "" {
		return "", "", ErrInvalidInput
	}
	arms, err := b.loadArms(ctx, experimentID)
	if err != nil {
		return "", "", fmt.Errorf("load arms: %w", err)
	}
	if len(arms) == 0 {
		return "", "", ErrNoArms
	}
	if len(arms) == 1 {
		return arms[0].ArmKey, "single_arm", nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// 计算总样本数（active 臂：exploring/exploiting）
	activeArms := make([]*model.BanditArm, 0, len(arms))
	totalSamples := int64(0)
	for _, a := range arms {
		if a.Status == model.BanditArmStatusExploring || a.Status == model.BanditArmStatusExploiting {
			activeArms = append(activeArms, a)
			totalSamples += a.TotalTrials
		}
	}
	if len(activeArms) == 0 {
		// 所有臂都已 promoted/retired，返回第一个臂
		return arms[0].ArmKey, "no_active", nil
	}

	// 冷启动期：每臂样本数 < MinSamplesForExploit → 均匀随机
	minSamples := int64(b.config.MinSamplesForExploit)
	if totalSamples < minSamples*int64(len(activeArms)) {
		idx := b.rng.Intn(len(activeArms))
		go b.markSampledAsync(experimentID, activeArms[idx].ArmKey)
		return activeArms[idx].ArmKey, "cold_start_uniform", nil
	}

	// 利用期：Thompson Sampling
	bestKey := ""
	bestSample := -1.0
	for _, a := range activeArms {
		sample := b.betaSample(a.Alpha, a.Beta)
		if sample > bestSample {
			bestSample = sample
			bestKey = a.ArmKey
		}
	}
	if bestKey == "" {
		return activeArms[0].ArmKey, "fallback_first", nil
	}
	strategy = "thompson_sampling"

	// 流量上限保护：单臂流量超过 TrafficCeiling 时强制探索非超限臂
	if forcedKey, needExplore := b.enforceTrafficCeiling(activeArms, bestKey, totalSamples); needExplore {
		bestKey = forcedKey
		strategy = "forced_explore"
	}

	// 异步标记采样时间（不阻塞主链路）
	go b.markSampledAsync(experimentID, bestKey)

	return bestKey, strategy, nil
}

// SelectPrompt 便捷方法：为 SOP 节点选择 Prompt 候选
//
// 查找当前 running 的 prompt 实验，调用 SelectArm，并返回关联的 PromptCandidateID
// 若无运行中的实验或无可用臂，返回 0 表示使用默认 Prompt
func (b *BanditAllocator) SelectPrompt(ctx context.Context, sopID uint, nodeID string) (uint, string, error) {
	test, err := b.banditRepo.GetRunningPromptABTestBySOPNode(ctx, sopID, nodeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 无运行中的实验，返回 0 表示使用默认 Prompt
			return 0, "", nil
		}
		return 0, "", fmt.Errorf("query running ab test: %w", err)
	}
	armKey, _, err := b.SelectArm(ctx, test.ExperimentID)
	if err != nil {
		return 0, "", err
	}
	if armKey == "" {
		return 0, "", nil
	}
	// arm_key → prompt_candidate_id
	arm, err := b.banditRepo.GetBanditArmByExperimentAndKey(ctx, test.ExperimentID, armKey)
	if err != nil {
		return 0, armKey, fmt.Errorf("query arm: %w", err)
	}
	return arm.PromptCandidateID, armKey, nil
}

// UpdateReward 更新臂的奖励（成功/失败）
//
// 在 FeedbackCollector 写入 feedback_signals 后异步调用
// 成功：alpha += 1, success_trials += 1
// 失败：beta += 1
// 通用：total_trials += 1, sum_reward += reward, avg_reward = sum_reward / total_trials
func (b *BanditAllocator) UpdateReward(ctx context.Context, experimentID, armKey string, success bool, reward float64) error {
	if experimentID == "" || armKey == "" {
		return ErrInvalidInput
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.banditRepo.UpdateBanditArmReward(ctx, experimentID, armKey, success, reward); err != nil {
		return fmt.Errorf("update bandit arm: %w", err)
	}
	// 失效缓存
	delete(b.cache, experimentID)
	return nil
}

// CheckConvergence 检查实验是否收敛
//
// 收敛条件：
//  1. 每臂样本数 ≥ MinSamplesForPromote
//  2. 蒙特卡洛采样 PosteriorSamples 次，P(winner 最优) ≥ ConvergenceThreshold
//
// 返回：
//
//	winnerKey - 胜出臂 key（未收敛时为空字符串）
//	ok        - 是否收敛
func (b *BanditAllocator) CheckConvergence(ctx context.Context, experimentID string) (string, bool) {
	arms, err := b.loadArms(ctx, experimentID)
	if err != nil || len(arms) < 2 {
		return "", false
	}
	// 检查最小样本数（每臂）
	for _, a := range arms {
		if a.TotalTrials < int64(b.config.MinSamplesForPromote) {
			return "", false
		}
	}
	// 蒙特卡洛采样计算每臂胜出概率
	winCounts := make(map[string]int)
	samples := b.config.PosteriorSamples
	for s := 0; s < samples; s++ {
		bestKey := ""
		bestTheta := -1.0
		for _, a := range arms {
			theta := b.betaSample(a.Alpha, a.Beta)
			if theta > bestTheta {
				bestTheta = theta
				bestKey = a.ArmKey
			}
		}
		if bestKey != "" {
			winCounts[bestKey]++
		}
	}
	// 找到胜出概率最高的臂
	var winnerKey string
	winnerProb := 0.0
	for k, v := range winCounts {
		prob := float64(v) / float64(samples)
		if prob > winnerProb {
			winnerProb = prob
			winnerKey = k
		}
	}
	if winnerProb >= b.config.ConvergenceThreshold {
		return winnerKey, true
	}
	return "", false
}

// PromoteArm 提升胜出臂，淘汰其他臂
//
// 事务操作：
//  1. winner → status=promoted, promoted_at=NOW()
//  2. 其他 → status=retired, retired_at=NOW()
func (b *BanditAllocator) PromoteArm(ctx context.Context, experimentID, winnerKey string) error {
	if experimentID == "" || winnerKey == "" {
		return ErrInvalidInput
	}
	return b.banditRepo.PromoteBanditArmWinner(ctx, experimentID, winnerKey)
}

// InvalidateCache 失效指定实验的缓存（外部数据变更后调用）
func (b *BanditAllocator) InvalidateCache(experimentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.cache, experimentID)
}

// ----------------------------------------------------------------------------
// 内部方法
// ----------------------------------------------------------------------------

// loadArms 加载臂（带缓存）
func (b *BanditAllocator) loadArms(ctx context.Context, experimentID string) ([]*model.BanditArm, error) {
	b.mu.RLock()
	if arms, ok := b.cache[experimentID]; ok {
		b.mu.RUnlock()
		return arms, nil
	}
	b.mu.RUnlock()

	arms, err := b.banditRepo.ListActiveBanditArms(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.cache[experimentID] = arms
	b.mu.Unlock()
	return arms, nil
}

// enforceTrafficCeiling 流量上限保护
//
// 若当前 bestKey 臂的流量比例已超过 TrafficCeiling，强制选择流量比例最低的非 bestKey 臂
// 返回：(forcedKey, needExplore)
func (b *BanditAllocator) enforceTrafficCeiling(arms []*model.BanditArm, bestKey string, totalTrials int64) (string, bool) {
	if totalTrials == 0 {
		return bestKey, false
	}
	// 检查 bestKey 是否超上限
	for _, a := range arms {
		if a.ArmKey == bestKey {
			if float64(a.TotalTrials)/float64(totalTrials) > b.config.TrafficCeiling {
				// 选择流量比例最低的非 bestKey 臂
				return pickLowestTrafficArm(arms, bestKey), true
			}
			return bestKey, false
		}
	}
	return bestKey, false
}

// pickLowestTrafficArm 选择流量比例最低的非 excludeKey 臂
func pickLowestTrafficArm(arms []*model.BanditArm, excludeKey string) string {
	sort.Slice(arms, func(i, j int) bool {
		return arms[i].TotalTrials < arms[j].TotalTrials
	})
	for _, a := range arms {
		if a.ArmKey != excludeKey {
			return a.ArmKey
		}
	}
	return excludeKey
}

// markSampledAsync 异步标记采样时间（不阻塞主链路）
func (b *BanditAllocator) markSampledAsync(experimentID, armKey string) {
	// 用独立 ctx，避免主链路 ctx 取消导致标记失败
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = b.banditRepo.UpdateBanditArmLastSampled(ctx, experimentID, armKey, time.Now())
}

// ----------------------------------------------------------------------------
// Beta / Gamma 采样（Marsaglia-Tsang 方法）
// ----------------------------------------------------------------------------

// betaSample Beta(α, β) 采样
//
// 公式：beta(α,β) = gamma(α,1) / (gamma(α,1) + gamma(β,1))
// 边界保护：α ≤ 0 或 β ≤ 0 返回 0.5（中性）
func (b *BanditAllocator) betaSample(alpha, beta float64) float64 {
	if alpha <= 0 || beta <= 0 {
		return 0.5
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	x := gammaSample(alpha, 1.0, b.rng)
	y := gammaSample(beta, 1.0, b.rng)
	sum := x + y
	if sum <= 0 {
		return 0.5
	}
	return x / sum
}

// gammaSample Gamma(α, θ) 采样（Marsaglia-Tsang 方法）
//
// 参考：Marsaglia, G. & Tsang, W. W. (2000) "A simple method for generating gamma variables"
// 当 α < 1 时使用增强公式：Gamma(α) = Gamma(α+1) * U^(1/α)
// 当 α ≥ 1 时使用标准 Marsaglia-Tsang 接受-拒绝采样
//
// 注意：rng 的并发安全由调用方（betaSample）通过 b.mu 保护
func gammaSample(alpha, theta float64, rng *rand.Rand) float64 {
	if alpha < 1 {
		// Gamma(α) = Gamma(α+1) * U^(1/α)
		u := rng.Float64()
		if u <= 0 {
			u = 1e-10
		}
		return gammaSample(alpha+1, theta, rng) * math.Pow(u, 1.0/alpha)
	}
	d := alpha - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for attempts := 0; attempts < 1000; attempts++ {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		// 快速接受
		if u < 1-0.0331*x*x*x*x {
			return d * v * theta
		}
		// 慢速接受
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v * theta
		}
	}
	// 1000 次未收敛，退化为均值（防止极端情况下死循环）
	return alpha * theta
}

// sqrtF32 float32 平方根（保留供 ChampionDialogueAnalyzer 使用）
func sqrtF32(f float32) float32 {
	return float32(math.Sqrt(float64(f)))
}
