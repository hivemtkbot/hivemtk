package repository

import (
	"context"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"

	"gorm.io/gorm"
)

// 2026-07-31 AI 智能体性能优化 - FAQ Repository 测试
// 使用 testutil.NewTestDB 跑 PG 真实库 (项目唯一允许的测试 DB 模式)
// 每个 test 用独立 question 前缀 + tx 隔离,避免进程级共享库的数据污染

// boolPtr 工具函数,把 bool 转成 *bool (GORM v2 零值 false 处理)
func boolPtr(v bool) *bool { return &v }

func setupFAQTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.FAQEntry{})
	// 每次 setup TRUNCATE 表,避免跨 test 数据污染
	// 用 TRUNCATE RESTART IDENTITY 而非 DELETE: 确保自增 ID 也归零,排查更清晰
	if err := db.Exec("TRUNCATE TABLE faq_entries RESTART IDENTITY").Error; err != nil {
		t.Fatalf("truncate faq_entries: %v", err)
	}
	return db
}

// setupFAQRepoWithTX 创建 repo + 一个会自动 rollback 的 tx, 隔离数据
func setupFAQRepoWithTX(t *testing.T) (repo *FAQRepository, tx *gorm.DB, cleanup func()) {
	t.Helper()
	db := setupFAQTestDB(t)
	tx = db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	repo = NewFAQRepository(tx)
	cleanup = func() { tx.Rollback() }
	return
}

func TestFAQRepository_Create(t *testing.T) {
	repo, _, done := setupFAQRepoWithTX(t)
	defer done()
	ctx := context.Background()

	entry := &model.FAQEntry{
		Question: "韵达发货吗",
		Answer:   "韵达不发的哦",
		Category: "logistics",
		Intent:   "logistics",
		Enabled:  boolPtr(true),
	}
	if err := repo.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if entry.ID == 0 {
		t.Error("expected auto-increment ID")
	}
}

func TestFAQRepository_ListEnabled(t *testing.T) {
	repo, _, done := setupFAQRepoWithTX(t)
	defer done()
	ctx := context.Background()

	entries := []*model.FAQEntry{
		{Question: "q1_listenabled", Answer: "a1", Enabled: boolPtr(true)},
		{Question: "q2_listenabled", Answer: "a2", Enabled: boolPtr(true)},
		{Question: "q3_listenabled", Answer: "a3", Enabled: boolPtr(false)},
	}
	for _, e := range entries {
		if err := repo.Create(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.ListEnabled(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 enabled entries, got %d", len(got))
	}
}

func TestFAQRepository_MatchByKeyword_Simple(t *testing.T) {
	repo, _, done := setupFAQRepoWithTX(t)
	defer done()
	ctx := context.Background()

	entries := []*model.FAQEntry{
		{Question: "韵达发货吗", Answer: "韵达不发的哦", Intent: "logistics", Confidence: 0.9, Enabled: boolPtr(true)},
		{Question: "可以优惠价吗", Answer: "200 把起优惠", Intent: "pricing", Confidence: 0.85, Enabled: boolPtr(true)},
		{Question: "纸皮核桃好不好", Answer: "是的哦", Intent: "product", Confidence: 0.8, Enabled: boolPtr(true)},
		{Question: "退换货怎么操作", Answer: "联系客服", Intent: "aftersales", Confidence: 0.9, Enabled: boolPtr(false)}, // 禁用
	}
	for _, e := range entries {
		if err := repo.Create(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	// 测 1: 用户问"韵达能发吗" -> 应匹配 "韵达发货吗"
	matches, err := repo.MatchByKeyword(ctx, "韵达能发吗", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Error("expected at least 1 match for 韵达能发吗")
	}
	if len(matches) > 0 && matches[0].Intent != "logistics" {
		t.Errorf("expected first match intent=logistics, got %s", matches[0].Intent)
	}

	// 测 2: 禁用的"退换货"不应被返回
	matches2, _ := repo.MatchByKeyword(ctx, "退换货", 3)
	for _, m := range matches2 {
		if !m.IsEnabled() {
			t.Errorf("disabled FAQ should not match: %+v", m)
		}
	}

	// 测 3: 关键词匹配"优惠价"
	matches3, _ := repo.MatchByKeyword(ctx, "优惠价可以", 3)
	if len(matches3) == 0 {
		t.Error("expected at least 1 match for 优惠价")
	}
}

func TestFAQRepository_DisabledNotMatched(t *testing.T) {
	repo, _, done := setupFAQRepoWithTX(t)
	defer done()
	ctx := context.Background()

	if err := repo.Create(ctx, &model.FAQEntry{
		Question: "韵达禁词",
		Answer:   "xxx",
		Enabled:  boolPtr(false),
	}); err != nil {
		t.Fatal(err)
	}
	matches, _ := repo.MatchByKeyword(ctx, "韵达", 3)
	for _, m := range matches {
		if !m.IsEnabled() {
			t.Errorf("disabled FAQ leaked into results: %s", m.Question)
		}
	}
}

func TestFAQRepository_IncrementHitCount(t *testing.T) {
	repo, _, done := setupFAQRepoWithTX(t)
	defer done()
	ctx := context.Background()

	entry := &model.FAQEntry{Question: "q_incr", Answer: "a", Enabled: boolPtr(true)}
	if err := repo.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if err := repo.IncrementHitCount(ctx, entry.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.IncrementHitCount(ctx, entry.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.HitCount != 2 {
		t.Errorf("expected hit_count=2, got %d", got.HitCount)
	}
}
