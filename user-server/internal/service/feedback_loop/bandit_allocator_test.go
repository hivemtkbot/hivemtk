package feedbackloop

// bandit_allocator_test.go P0-5 Multi-Armed Bandit 流量分配器测试
//
// 覆盖：
//  A. 纯算法单元测试（不需 PG）
//     1. betaSample 范围 [0,1]
//     2. betaSample 边界（α≤0 / β≤0 返回 0.5）
//     3. gammaSample Marsaglia-Tsang 正确性
//     4. pickLowestTrafficArm 选流量最低的非排除臂
//     5. enforceTrafficCeiling 流量上限保护
//  B. PG 集成测试
//     1. SelectArm 冷启动均匀分配
//     2. SelectArm 单臂场景
//     3. SelectArm 无臂错误
//     4. UpdateReward 成功/失败增量更新
//     5. CheckConvergence 收敛判定（样本不足 / 已收敛）
//     6. PromoteArm 提升胜出臂 + 退役其他
//     7. InvalidateCache 失效缓存
//     8. SelectPrompt 便捷方法

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"marketing/internal/model"
)

// ============================================================================
// A. 纯算法单元测试
// ============================================================================

// TestBetaSample_RangeInUnitInterval Beta 采样结果应在 [0,1]
func TestBetaSample_RangeInUnitInterval(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	cases := []struct{ alpha, beta float64 }{
		{1, 1}, {2, 2}, {10, 5}, {5, 10}, {100, 100}, {0.5, 0.5},
	}
	for _, c := range cases {
		for i := 0; i < 100; i++ {
			sample := b.betaSample(c.alpha, c.beta)
			if sample < 0 || sample > 1 {
				t.Errorf("betaSample(α=%v, β=%v) = %v 超出 [0,1]", c.alpha, c.beta, sample)
			}
		}
	}
}

// TestBetaSample_BoundaryConditions 边界条件
func TestBetaSample_BoundaryConditions(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	// α≤0 或 β≤0 返回 0.5
	if got := b.betaSample(0, 5); !approxEqualF64(got, 0.5) {
		t.Errorf("betaSample(0, 5) = %v want 0.5", got)
	}
	if got := b.betaSample(5, 0); !approxEqualF64(got, 0.5) {
		t.Errorf("betaSample(5, 0) = %v want 0.5", got)
	}
	if got := b.betaSample(-1, 5); !approxEqualF64(got, 0.5) {
		t.Errorf("betaSample(-1, 5) = %v want 0.5", got)
	}
	if got := b.betaSample(5, -1); !approxEqualF64(got, 0.5) {
		t.Errorf("betaSample(5, -1) = %v want 0.5", got)
	}
}

// TestBetaSample_MeanConverges Beta(α,β) 均值应近似 α/(α+β)
//
// 大数定律：N 次采样的均值 → 理论均值 α/(α+β)
func TestBetaSample_MeanConverges(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	cases := []struct {
		alpha, beta float64
	}{
		{2, 8},   // 理论均值 0.2
		{5, 5},   // 理论均值 0.5
		{8, 2},   // 理论均值 0.8
		{50, 50}, // 理论均值 0.5
	}
	const N = 5000
	for _, c := range cases {
		var sum float64
		for i := 0; i < N; i++ {
			sum += b.betaSample(c.alpha, c.beta)
		}
		mean := sum / N
		expected := c.alpha / (c.alpha + c.beta)
		if math.Abs(mean-expected) > 0.05 {
			t.Errorf("betaSample(α=%v, β=%v) 均值=%v want≈%v (N=%d)",
				c.alpha, c.beta, mean, expected, N)
		}
	}
}

// TestGammaSample_PositiveOutput Gamma 采样应返回正数
func TestGammaSample_PositiveOutput(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	rng := b.rng
	cases := []struct{ alpha, theta float64 }{
		{0.5, 1}, {1, 1}, {2, 1}, {5, 2}, {10, 0.5}, {0.1, 1},
	}
	for _, c := range cases {
		for i := 0; i < 100; i++ {
			sample := gammaSample(c.alpha, c.theta, rng)
			if sample <= 0 {
				t.Errorf("gammaSample(α=%v, θ=%v) = %v 应为正数", c.alpha, c.theta, sample)
			}
		}
	}
}

// TestGammaSample_MeanConverges Gamma(α,θ) 均值应近似 α*θ
func TestGammaSample_MeanConverges(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	rng := b.rng
	cases := []struct {
		alpha, theta float64
	}{
		{1, 1}, {2, 1}, {5, 2}, {10, 0.5}, {0.5, 1},
	}
	const N = 5000
	for _, c := range cases {
		var sum float64
		for i := 0; i < N; i++ {
			sum += gammaSample(c.alpha, c.theta, rng)
		}
		mean := sum / N
		expected := c.alpha * c.theta
		// 容差 10%（Gamma 分布方差较大）
		if math.Abs(mean-expected)/expected > 0.10 {
			t.Errorf("gammaSample(α=%v, θ=%v) 均值=%v want≈%v (N=%d)",
				c.alpha, c.theta, mean, expected, N)
		}
	}
}

// TestPickLowestTrafficArm 选流量最低的非排除臂
func TestPickLowestTrafficArm(t *testing.T) {
	arms := []*model.BanditArm{
		{ArmKey: "arm_a", TotalTrials: 100},
		{ArmKey: "arm_b", TotalTrials: 30},
		{ArmKey: "arm_c", TotalTrials: 50},
	}
	// 排除 arm_a，应选 arm_b（流量最低）
	got := pickLowestTrafficArm(arms, "arm_a")
	if got != "arm_b" {
		t.Errorf("pickLowestTrafficArm exclude=arm_a = %q want arm_b", got)
	}
	// 排除 arm_b，应选 arm_c（流量最低的非 arm_b）
	got = pickLowestTrafficArm(arms, "arm_b")
	if got != "arm_c" {
		t.Errorf("pickLowestTrafficArm exclude=arm_b = %q want arm_c", got)
	}
	// 全部都被排除时退化为 excludeKey
	got = pickLowestTrafficArm([]*model.BanditArm{{ArmKey: "only", TotalTrials: 1}}, "only")
	if got != "only" {
		t.Errorf("pickLowestTrafficArm single arm = %q want only", got)
	}
}

// TestEnforceTrafficCeiling_NoCeilingReached 未达上限时不强制探索
func TestEnforceTrafficCeiling_NoCeilingReached(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	arms := []*model.BanditArm{
		{ArmKey: "arm_a", TotalTrials: 50},
		{ArmKey: "arm_b", TotalTrials: 50},
	}
	// arm_a 占 50%，未超 60% 上限，应不强制
	forced, needExplore := b.enforceTrafficCeiling(arms, "arm_a", 100)
	if needExplore {
		t.Errorf("arm_a 占 50%% 未超上限，不应强制探索")
	}
	if forced != "arm_a" {
		t.Errorf("未强制时 forced = %q want arm_a", forced)
	}
}

// TestEnforceTrafficCeiling_CeilingReached 达上限时强制探索其他臂
func TestEnforceTrafficCeiling_CeilingReached(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	arms := []*model.BanditArm{
		{ArmKey: "arm_a", TotalTrials: 80}, // 占 80%，超 60% 上限
		{ArmKey: "arm_b", TotalTrials: 20},
	}
	forced, needExplore := b.enforceTrafficCeiling(arms, "arm_a", 100)
	if !needExplore {
		t.Errorf("arm_a 占 80%% 超上限，应强制探索")
	}
	if forced != "arm_b" {
		t.Errorf("强制探索时 forced = %q want arm_b", forced)
	}
}

// TestEnforceTrafficCeiling_ZeroTotalTrials 总样本为 0 时不强制
func TestEnforceTrafficCeiling_ZeroTotalTrials(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	arms := []*model.BanditArm{
		{ArmKey: "arm_a", TotalTrials: 0},
		{ArmKey: "arm_b", TotalTrials: 0},
	}
	forced, needExplore := b.enforceTrafficCeiling(arms, "arm_a", 0)
	if needExplore {
		t.Errorf("总样本为 0 时不应强制探索")
	}
	if forced != "arm_a" {
		t.Errorf("不强制时 forced = %q want arm_a", forced)
	}
}

// ============================================================================
// B. PG 集成测试
// ============================================================================

// TestBanditAllocator_SelectArm_ColdStartUniform 冷启动期均匀随机分配
//
// 验证：
//   - 每臂样本 < MinSamplesForExploit 时使用 cold_start_uniform 策略
//   - 多次调用后每个臂都被选中过（均匀性）
func TestBanditAllocator_SelectArm_ColdStartUniform(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_cold_start"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a", Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_b", Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_c", Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring},
	}
	if err := db.Create(&arms).Error; err != nil {
		t.Fatalf("seed arms: %v", err)
	}

	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)

	// 冷启动期：每臂 0 trials < 30，应使用 cold_start_uniform
	selected := make(map[string]int)
	for i := 0; i < 60; i++ {
		armKey, _, err := b.SelectArm(ctx, expID)
		if err != nil {
			t.Fatalf("SelectArm[%d] failed: %v", i, err)
		}
		selected[armKey]++
	}
	// 3 个臂都应被选中过（均匀性）
	if len(selected) < 3 {
		t.Errorf("冷启动期应均匀分配到 3 个臂，实际选中 %d 个: %v", len(selected), selected)
	}
}

// TestBanditAllocator_SelectArm_SingleArm 单臂场景直接返回
func TestBanditAllocator_SelectArm_SingleArm(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_single_arm"
	arm := model.BanditArm{
		ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_only",
		Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring,
	}
	if err := db.Create(&arm).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	armKey, strategy, err := b.SelectArm(context.Background(), expID)
	if err != nil {
		t.Fatalf("SelectArm: %v", err)
	}
	if armKey != "arm_only" {
		t.Errorf("armKey = %q want arm_only", armKey)
	}
	if strategy != "single_arm" {
		t.Errorf("strategy = %q want single_arm", strategy)
	}
}

// TestBanditAllocator_SelectArm_NoArms 无臂错误
func TestBanditAllocator_SelectArm_NoArms(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	_, _, err := b.SelectArm(context.Background(), "non_existent_experiment")
	if !errors.Is(err, ErrNoArms) {
		t.Errorf("SelectArm 无臂应返回 ErrNoArms, got %v", err)
	}
}

// TestBanditAllocator_SelectArm_EmptyExperimentID 空 experimentID
func TestBanditAllocator_SelectArm_EmptyExperimentID(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	_, _, err := b.SelectArm(context.Background(), "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 experimentID 应返回 ErrInvalidInput, got %v", err)
	}
}

// TestBanditAllocator_UpdateReward_Success 成功更新 alpha
func TestBanditAllocator_UpdateReward_Success(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_update_reward"
	arm := model.BanditArm{
		ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a",
		Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring,
	}
	if err := db.Create(&arm).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)

	// 成功：alpha + 1
	if err := b.UpdateReward(ctx, expID, "arm_a", true, 1.5); err != nil {
		t.Fatalf("UpdateReward: %v", err)
	}
	var updated model.BanditArm
	if err := db.First(&updated, arm.ID).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if updated.Alpha != 3 {
		t.Errorf("Alpha = %v want 3", updated.Alpha)
	}
	if updated.Beta != 2 {
		t.Errorf("Beta = %v want 2", updated.Beta)
	}
	if updated.TotalTrials != 1 {
		t.Errorf("TotalTrials = %v want 1", updated.TotalTrials)
	}
	if updated.SuccessTrials != 1 {
		t.Errorf("SuccessTrials = %v want 1", updated.SuccessTrials)
	}
	if !approxEqualF64(updated.SumReward, 1.5) {
		t.Errorf("SumReward = %v want 1.5", updated.SumReward)
	}
}

// TestBanditAllocator_UpdateReward_Failure 失败更新 beta
func TestBanditAllocator_UpdateReward_Failure(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_update_reward_fail"
	arm := model.BanditArm{
		ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a",
		Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring,
	}
	if err := db.Create(&arm).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	if err := b.UpdateReward(context.Background(), expID, "arm_a", false, -0.5); err != nil {
		t.Fatalf("UpdateReward: %v", err)
	}
	var updated model.BanditArm
	_ = db.First(&updated, arm.ID).Error
	if updated.Alpha != 2 {
		t.Errorf("Alpha = %v want 2", updated.Alpha)
	}
	if updated.Beta != 3 {
		t.Errorf("Beta = %v want 3 (失败时 beta+1)", updated.Beta)
	}
	if updated.SuccessTrials != 0 {
		t.Errorf("SuccessTrials = %v want 0", updated.SuccessTrials)
	}
}

// TestBanditAllocator_UpdateReward_InvalidInput 空参数返回错误
func TestBanditAllocator_UpdateReward_InvalidInput(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	if err := b.UpdateReward(context.Background(), "", "arm_a", true, 1.0); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 experimentID 应返回 ErrInvalidInput, got %v", err)
	}
	if err := b.UpdateReward(context.Background(), "exp", "", true, 1.0); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 armKey 应返回 ErrInvalidInput, got %v", err)
	}
}

// TestBanditAllocator_CheckConvergence_InsufficientSamples 样本不足不收敛
func TestBanditAllocator_CheckConvergence_InsufficientSamples(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_convergence_insufficient"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a", Alpha: 50, Beta: 10, TotalTrials: 50, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_b", Alpha: 10, Beta: 50, TotalTrials: 50, Status: model.BanditArmStatusExploring},
	}
	if err := db.Create(&arms).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	// 默认 MinSamplesForPromote=100，每臂只有 50 trials
	winner, ok := b.CheckConvergence(context.Background(), expID)
	if ok {
		t.Errorf("样本不足不应收敛，但 winner=%s", winner)
	}
}

// TestBanditAllocator_CheckConvergence_Converged 已收敛
//
// arm_a: alpha=100, beta=10 → 均值 ~0.91
// arm_b: alpha=10, beta=100 → 均值 ~0.09
// P(arm_a 最优) ≈ 1.0 ≥ 0.95
func TestBanditAllocator_CheckConvergence_Converged(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_convergence_yes"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a", Alpha: 100, Beta: 10, TotalTrials: 110, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_b", Alpha: 10, Beta: 100, TotalTrials: 110, Status: model.BanditArmStatusExploring},
	}
	if err := db.Create(&arms).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	winner, ok := b.CheckConvergence(context.Background(), expID)
	if !ok {
		t.Errorf("应收敛但未收敛")
	}
	if winner != "arm_a" {
		t.Errorf("winner = %q want arm_a", winner)
	}
}

// TestBanditAllocator_PromoteArm 提升胜出臂 + 退役其他
func TestBanditAllocator_PromoteArm(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_promote"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a", Alpha: 100, Beta: 10, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_b", Alpha: 10, Beta: 100, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_c", Alpha: 5, Beta: 50, Status: model.BanditArmStatusExploring},
	}
	if err := db.Create(&arms).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	if err := b.PromoteArm(context.Background(), expID, "arm_a"); err != nil {
		t.Fatalf("PromoteArm: %v", err)
	}
	// 验证 arm_a 状态 promoted
	var winner model.BanditArm
	_ = db.Where("experiment_id = ? AND arm_key = ?", expID, "arm_a").First(&winner).Error
	if winner.Status != model.BanditArmStatusPromoted {
		t.Errorf("arm_a status = %q want promoted", winner.Status)
	}
	if winner.PromotedAt == nil {
		t.Errorf("arm_a promoted_at 应非空")
	}
	// 验证 arm_b / arm_c 状态 retired
	var losers []model.BanditArm
	_ = db.Where("experiment_id = ? AND arm_key IN ?", expID, []string{"arm_b", "arm_c"}).Find(&losers).Error
	if len(losers) != 2 {
		t.Errorf("应找到 2 个 retired 臂, got %d", len(losers))
	}
	for _, l := range losers {
		if l.Status != model.BanditArmStatusRetired {
			t.Errorf("arm %s status = %q want retired", l.ArmKey, l.Status)
		}
	}
}

// TestBanditAllocator_PromoteArm_InvalidInput 空参数错误
func TestBanditAllocator_PromoteArm_InvalidInput(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	if err := b.PromoteArm(context.Background(), "", "arm_a"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 experimentID 应返回 ErrInvalidInput, got %v", err)
	}
	if err := b.PromoteArm(context.Background(), "exp", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 winnerKey 应返回 ErrInvalidInput, got %v", err)
	}
}

// TestBanditAllocator_InvalidateCache 失效缓存
//
// 验证：失效缓存后下次 SelectArm 会重新查 DB
func TestBanditAllocator_InvalidateCache(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_invalidate"
	arm := model.BanditArm{
		ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a",
		Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring,
	}
	if err := db.Create(&arm).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	// 第一次调用加载缓存
	_, _, err := b.SelectArm(ctx, expID)
	if err != nil {
		t.Fatalf("first SelectArm: %v", err)
	}
	// 失效缓存
	b.InvalidateCache(expID)
	// 第二次调用应重新查 DB（不会报错）
	_, _, err = b.SelectArm(ctx, expID)
	if err != nil {
		t.Fatalf("second SelectArm after invalidate: %v", err)
	}
}

// TestBanditAllocator_SelectPrompt_NoRunningTest 无运行中的实验返回 0
func TestBanditAllocator_SelectPrompt_NoRunningTest(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	promptID, armKey, err := b.SelectPrompt(context.Background(), 1, "node_1")
	if err != nil {
		t.Fatalf("SelectPrompt: %v", err)
	}
	if promptID != 0 {
		t.Errorf("无运行中实验时 promptID = %v want 0", promptID)
	}
	if armKey != "" {
		t.Errorf("无运行中实验时 armKey = %q want empty", armKey)
	}
}

// TestBanditAllocator_SelectPrompt_WithRunningTest 有运行中实验返回候选
func TestBanditAllocator_SelectPrompt_WithRunningTest(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	// 创建 prompt candidate
	cand := model.PromptCandidate{
		SOPNodeID: "node_1", SOPID: 1, Scenario: "sop_reply", Version: "v1.0",
		Title: "test", SystemPrompt: "sys", UserPromptTemplate: "user",
		Status: model.PromptCandidateStatusActive,
	}
	if err := db.Create(&cand).Error; err != nil {
		t.Fatalf("seed candidate: %v", err)
	}
	// 创建 A/B test
	now := time.Now()
	expID := fmt.Sprintf("sop1_node1_%d", now.Unix())
	test := model.PromptABTest{
		ExperimentID: expID, ExperimentType: model.BanditExperimentTypePrompt,
		SOPID: 1, SOPNodeID: "node_1", Name: "test",
		ArmKeys:   model.JSONArray([]any{"arm_0"}),
		Config:    model.JSONMap{"min_samples": 100},
		Status:    model.PromptABTestStatusRunning,
		StartedAt: &now,
	}
	if err := db.Create(&test).Error; err != nil {
		t.Fatalf("seed ab test: %v", err)
	}
	// 创建 bandit arm
	arm := model.BanditArm{
		ExperimentID: expID, ExperimentType: model.BanditExperimentTypePrompt,
		ArmKey: "arm_0", SOPID: 1, PromptCandidateID: cand.ID,
		Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring,
	}
	if err := db.Create(&arm).Error; err != nil {
		t.Fatalf("seed arm: %v", err)
	}

	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	promptID, armKey, err := b.SelectPrompt(context.Background(), 1, "node_1")
	if err != nil {
		t.Fatalf("SelectPrompt: %v", err)
	}
	if promptID != cand.ID {
		t.Errorf("promptID = %v want %d", promptID, cand.ID)
	}
	if armKey != "arm_0" {
		t.Errorf("armKey = %q want arm_0", armKey)
	}
}
