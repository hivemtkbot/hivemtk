package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// TestD22b_ChurnWeeklyJob 真实 PG：ComputeAll 全链路（mock 订单统计 → 拟合 → upsert → 回读）
func TestD22b_ChurnWeeklyJob(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ChurnScore{})
	svc := NewChurnScoreService(db)

	// 注入合成订单统计：3 个高频客户 + 2 个沉寂客户（确定性）
	base := time.Now().Add(-40 * 24 * time.Hour)
	svc.statsFn = func(ctx context.Context) ([]ChurnCustomerStats, error) {
		return []ChurnCustomerStats{
			{CustomerKey: "freq_1", PurchaseAts: []time.Time{base, base.Add(10 * 24 * time.Hour), base.Add(20 * 24 * time.Hour), time.Now().Add(-2 * 24 * time.Hour)}},
			{CustomerKey: "freq_2", PurchaseAts: []time.Time{base, base.Add(5 * 24 * time.Hour), base.Add(12 * 24 * time.Hour), base.Add(30 * 24 * time.Hour)}},
			{CustomerKey: "freq_3", PurchaseAts: []time.Time{base, base.Add(3 * 24 * time.Hour), time.Now().Add(-1 * 24 * time.Hour)}},
			{CustomerKey: "gone_1", PurchaseAts: []time.Time{base, base.Add(24 * time.Hour)}},
			{CustomerKey: "gone_2", PurchaseAts: []time.Time{base.Add(24 * time.Hour), base.Add(2 * 24 * time.Hour)}},
		}, nil
	}

	n, err := svc.ComputeAll(context.Background())
	if err != nil {
		t.Fatalf("ComputeAll 失败: %v", err)
	}
	if n != 5 {
		t.Fatalf("应写入 5 行, got %d", n)
	}

	// 高流失（沉寂）客户应排前
	rows, err := svc.repo.ListByPAliveBelow(context.Background(), 1.0, 10)
	if err != nil {
		t.Fatalf("ListByPAliveBelow 失败: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("至少 2 行可查, got %d", len(rows))
	}
	if rows[0].PAlive > rows[len(rows)-1].PAlive {
		t.Fatalf("ListByPAliveBelow 应按 p_alive ASC 排序")
	}
	// gone_* 两个沉寂客户（x=1, tx≈1 天, T≈40 天）应比 freq_*（近期有购买）p_alive 更低
	paByKey := map[string]float64{}
	for _, r := range rows {
		paByKey[r.CustomerKey] = r.PAlive
	}
	if paByKey["gone_1"] >= paByKey["freq_1"] {
		t.Fatalf("沉寂客户 gone_1 P(alive)=%.3f 应低于近期活跃 freq_1 %.3f", paByKey["gone_1"], paByKey["freq_1"])
	}

	// 幂等：重跑仍 5 行（upsert 不重复）
	n2, err := svc.ComputeAll(context.Background())
	if err != nil {
		t.Fatalf("二次 ComputeAll 失败: %v", err)
	}
	if n2 != 5 {
		t.Fatalf("二次应仍 5 行（upsert 幂等）, got %d", n2)
	}
	if cnt, _ := svc.repo.Count(context.Background()); cnt != 5 {
		t.Fatalf("表行数应恒为 5, got %d", cnt)
	}
}

// TestD22b_ChurnEmptyRun 无订单数据时空跑幂等（生产当前状态：订单表为空）
func TestD22b_ChurnEmptyRun(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ChurnScore{})
	svc := NewChurnScoreService(db)
	svc.statsFn = func(ctx context.Context) ([]ChurnCustomerStats, error) { return nil, nil }
	n, err := svc.ComputeAll(context.Background())
	if err != nil {
		t.Fatalf("空跑不应报错: %v", err)
	}
	if n != 0 {
		t.Fatalf("空数据应 0 行, got %d", n)
	}
}
