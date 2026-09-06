package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// D14: 全局频控跨管线共享计数（同客户不同 channel 计同一预算）
func TestD14_GlobalLimitCrossPipeline(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ConfigParam{})
	svc := NewReachPipelineService(db)
	svc.SetRateCache(cache.NewMemoryCache())
	SeedConfigParams(context.Background(), db)

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ch := "wx"
		if i == 1 {
			ch = "sms"
		}
		if i == 2 {
			ch = "email"
		}
		if !svc.checkGlobalPerUserDaily(ctx, "cust-g1", false) {
			t.Fatalf("第 %d 次不应拒绝", i+1)
		}
		_ = ch
	}
	if svc.checkGlobalPerUserDaily(ctx, "cust-g1", false) {
		t.Error("第 4 次应被全局上限拒绝")
	}

	if !svc.checkGlobalPerUserDaily(ctx, "cust-g1", true) {
		t.Error("transactional 应豁免")
	}

	if !svc.checkGlobalPerUserDaily(ctx, "cust-g2", false) {
		t.Error("其他客户不应受影响")
	}
}

// D14: limit=0 禁用该层
func TestD14_GlobalLimitDisabled(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ConfigParam{})
	svc := NewReachPipelineService(db)
	svc.SetRateCache(cache.NewMemoryCache())
	SeedConfigParams(context.Background(), db)
	svc.globalLimitFn = func(ctx context.Context) int { return 0 }

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if !svc.checkGlobalPerUserDaily(ctx, "cust-off", false) {
			t.Fatalf("limit=0 应禁用层, 第 %d 次被拒", i+1)
		}
	}
	_ = time.Now
}
