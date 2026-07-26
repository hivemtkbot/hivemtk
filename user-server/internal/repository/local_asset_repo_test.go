package repository

import (
	"context"
	"sync"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// setupLocalAssetTestDB 初始化 LocalAsset 测试库（PostgreSQL 进程级隔离）
func setupLocalAssetTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t, &model.LocalAsset{})
	db.SetTestDB(database)
	return database
}

// setupLocalAssetRepo 构造注入测试 DB 的 repo
func setupLocalAssetRepo(t *testing.T) *localAssetRepo {
	setupLocalAssetTestDB(t)
	return &localAssetRepo{db: db.GetDB()}
}

// seedLocalAsset 写入一条购买资产，返回其主键
func seedLocalAsset(t *testing.T, r *localAssetRepo, assetID string, useCount, reportedCount int64) int64 {
	t.Helper()
	pid := int64(1001)
	la := &model.LocalAsset{
		AssetID:          assetID,
		AssetType:        "agent_persona",
		Industry:         "美妆",
		Name:             "test-asset",
		Version:          "1.0.0",
		Source:           model.AssetSourcePurchased,
		IsActive:         true,
		PurchaseID:       &pid,
		UseCount:         useCount,
		ReportedUseCount: reportedCount,
	}
	if err := r.db.Create(la).Error; err != nil {
		t.Fatalf("seed local asset: %v", err)
	}
	return la.ID
}

// TestAdvanceReportedUseCount_BasicIncrement 验证按 delta 累加 reported_use_count 基础行为
func TestAdvanceReportedUseCount_BasicIncrement(t *testing.T) {
	r := setupLocalAssetRepo(t)
	ctx := context.Background()
	id := seedLocalAsset(t, r, "asset-1", useCount(10), reportedCount(5))

	if err := r.AdvanceReportedUseCount(ctx, id, 5); err != nil {
		t.Fatalf("AdvanceReportedUseCount: %v", err)
	}

	var got model.LocalAsset
	if err := r.db.First(&got, id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.ReportedUseCount != 10 {
		t.Fatalf("expected reported=10, got %d", got.ReportedUseCount)
	}
	if got.UseCount != 10 {
		t.Fatalf("use_count should not change, got %d", got.UseCount)
	}
}

// TestAdvanceReportedUseCount_NonPositiveDelta 验证 delta<=0 时为 no-op
func TestAdvanceReportedUseCount_NonPositiveDelta(t *testing.T) {
	r := setupLocalAssetRepo(t)
	ctx := context.Background()
	id := seedLocalAsset(t, r, "asset-2", useCount(10), reportedCount(3))

	for _, d := range []int64{0, -1, -100} {
		if err := r.AdvanceReportedUseCount(ctx, id, d); err != nil {
			t.Fatalf("delta=%d should be no-op not error: %v", d, err)
		}
	}

	var got model.LocalAsset
	_ = r.db.First(&got, id).Error
	if got.ReportedUseCount != 3 {
		t.Fatalf("non-positive delta should not change reported, got %d", got.ReportedUseCount)
	}
}

// TestAdvanceReportedUseCount_ConcurrentSafe 二轮 BUG-1 核心回归测试：
// 模拟"读取 use_count=10 → 上报 delta=5 → 期间 use_count 被其他 goroutine 累加"场景，
// 验证 AdvanceReportedUseCount 仍能正确推进 reported_use_count（不像 CAS 那样失效）。
func TestAdvanceReportedUseCount_ConcurrentSafe(t *testing.T) {
	r := setupLocalAssetRepo(t)
	ctx := context.Background()
	id := seedLocalAsset(t, r, "asset-3", useCount(10), reportedCount(5))

	// 步骤 1: 上报 delta=5（基于读取快照 use_count=10）
	if err := r.AdvanceReportedUseCount(ctx, id, 5); err != nil {
		t.Fatalf("advance: %v", err)
	}
	// 步骤 2: 期间另一 goroutine 又累加了 use_count += 1
	if err := r.IncrementUseCount(ctx, id, 1); err != nil {
		t.Fatalf("increment use_count: %v", err)
	}

	var got model.LocalAsset
	if err := r.db.First(&got, id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	// reported_use_count 应该是 5+5=10（CAS 失效时仍为 5）
	if got.ReportedUseCount != 10 {
		t.Fatalf("BUG-1 regression: expected reported=10, got %d (CAS may have failed)", got.ReportedUseCount)
	}
	// use_count 应该是 10+1=11
	if got.UseCount != 11 {
		t.Fatalf("expected use_count=11, got %d", got.UseCount)
	}
	// 下次应只上报 delta = 11 - 10 = 1
	nextDelta := got.UseCount - got.ReportedUseCount
	if nextDelta != 1 {
		t.Fatalf("next delta should be 1, got %d (would cause duplicate count)", nextDelta)
	}
}

// TestAdvanceReportedUseCount_Parallel 并发场景：N 个 goroutine 同时推进 reported_use_count，
// 验证最终 reported_use_count = sum(deltas)，不会丢更新
func TestAdvanceReportedUseCount_Parallel(t *testing.T) {
	r := setupLocalAssetRepo(t)
	ctx := context.Background()
	id := seedLocalAsset(t, r, "asset-4", useCount(0), reportedCount(0))

	const N = 20
	const perDelta int64 = 1
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := r.AdvanceReportedUseCount(ctx, id, perDelta); err != nil {
				t.Errorf("parallel advance: %v", err)
			}
		}()
	}
	wg.Wait()

	var got model.LocalAsset
	if err := r.db.First(&got, id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.ReportedUseCount != N*perDelta {
		t.Fatalf("expected reported=%d, got %d (race condition detected)", N*perDelta, got.ReportedUseCount)
	}
}

// TestSetReportedUseCountIfMatch_LegacyCAS_BrokenBehavior 文档化旧 CAS 行为：
// 当 use_count 已被并发累加时，CAS WHERE 条件不命中，reported_use_count 不推进。
// 此测试用于确认 BUG-1 的根因，并防止有人误以为旧接口仍然可用。
// 生产路径已切到 AdvanceReportedUseCount，本测试仅做行为基线。
func TestSetReportedUseCountIfMatch_LegacyCAS_BrokenBehavior(t *testing.T) {
	r := setupLocalAssetRepo(t)
	ctx := context.Background()
	id := seedLocalAsset(t, r, "asset-5", useCount(10), reportedCount(5))

	// 期间并发累加 use_count
	if err := r.IncrementUseCount(ctx, id, 1); err != nil {
		t.Fatalf("increment: %v", err)
	}

	// 旧 CAS 调用方式：newVal=10, expected=10（即调用方读取时的快照值）
	err := r.SetReportedUseCountIfMatch(ctx, id, 10, 10)
	if err != nil {
		t.Fatalf("CAS should not error even if no row matched: %v", err)
	}

	var got model.LocalAsset
	_ = r.db.First(&got, id).Error
	// 因为 use_count 已经变成 11，WHERE use_count=10 不命中，reported_use_count 仍为 5
	if got.ReportedUseCount != 5 {
		t.Fatalf("legacy CAS baseline changed: expected reported=5 (broken), got %d", got.ReportedUseCount)
	}
	t.Logf("确认旧 CAS 失效行为：use_count=%d reported_use_count=%d（BUG-1 根因）", got.UseCount, got.ReportedUseCount)
}

// TestIncrementUseCount_Concurrent 并发累加 use_count 不丢更新
func TestIncrementUseCount_Concurrent(t *testing.T) {
	r := setupLocalAssetRepo(t)
	ctx := context.Background()
	id := seedLocalAsset(t, r, "asset-6", useCount(0), reportedCount(0))

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := r.IncrementUseCount(ctx, id, 1); err != nil {
				t.Errorf("increment: %v", err)
			}
		}()
	}
	wg.Wait()

	var got model.LocalAsset
	if err := r.db.First(&got, id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.UseCount != N {
		t.Fatalf("expected use_count=%d, got %d", N, got.UseCount)
	}
}

// 辅助函数，提升测试可读性
func useCount(v int64) int64      { return v }
func reportedCount(v int64) int64 { return v }
