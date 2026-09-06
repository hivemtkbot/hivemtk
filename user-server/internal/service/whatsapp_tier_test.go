package service

import (
	"testing"
	"time"
)

// TestClassifyTierOf_CategoryPriority 模板类别优先于意图
func TestClassifyTierOf_CategoryPriority(t *testing.T) {
	cases := []struct {
		cat, intent string
		want        Tier
	}{
		{"MARKETING", "订单查询", TierMarketing},
		{"Utility", "营销唤醒", TierUtility},
		{"authentication", "营销唤醒", TierAuthentication},
		{"AUTH", "", TierAuthentication},
		{"", "", TierUtility},
	}
	for _, c := range cases {
		if got := ClassifyTierOf(c.cat, c.intent); got != c.want {
			t.Errorf("ClassifyTierOf(%q,%q) = %s, want %s", c.cat, c.intent, got, c.want)
		}
	}
}

// TestClassifyTierOf_IntentMapping 无模板类别时按意图映射
func TestClassifyTierOf_IntentMapping(t *testing.T) {
	cases := []struct {
		intent string
		want   Tier
	}{
		{"订单已发货", TierUtility},
		{"logistics update", TierUtility},
		{"物流签收提醒", TierUtility},
		{"您的验证码是1234", TierAuthentication},
		{"otp code", TierAuthentication},
		{"大促唤醒老客户", TierMarketing},
		{"promo campaign", TierMarketing},
		{"随便聊聊天气", TierUtility},
	}
	for _, c := range cases {
		if got := ClassifyTierOf("", c.intent); got != c.want {
			t.Errorf("ClassifyTierOf(\"\",%q) = %s, want %s", c.intent, got, c.want)
		}
	}
}

// TestClassifyTier_SpecSignature 规格单参签名：类别→tier，未知回落 utility
func TestClassifyTier_SpecSignature(t *testing.T) {
	cases := []struct {
		cat  string
		want WATier
	}{
		{"marketing", WATierMarketing},
		{"utility", WATierUtility},
		{"authentication", WATierAuthentication},
		{"unknown", WATierUtility},
		{"", WATierUtility},
	}
	for _, c := range cases {
		if got := ClassifyTier(c.cat); got != c.want {
			t.Errorf("ClassifyTier(%q) = %s, want %s", c.cat, got, c.want)
		}
	}
}

func newTestPacer(dayCap int) *TierPacer {
	p := NewTierPacer()
	p.DayCap = dayCap
	p.TierQuotas = map[WATier]int{}
	return p
}

// TestTierPacer_BucketRefill 令牌桶补币数学：12/min = 0.2/s，耗尽后 5s 恰补 1 枚
func TestTierPacer_BucketRefill(t *testing.T) {
	p := newTestPacer(TierDayCap)
	base := time.Now()
	tier := TierUtility

	for i := 0; i < 12; i++ {
		if allow, _ := p.Enforce("peer1", tier, base); !allow {
			t.Fatalf("第 %d 次发送应允许（冷启动满桶）", i+1)
		}
	}

	allow, retry := p.Enforce("peer1", tier, base)
	if allow {
		t.Fatal("桶耗尽后应拒绝")
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("retryAfter = %v, 应在 (0,1min] 内", retry)
	}

	if allow, _ := p.Enforce("peer1", tier, base.Add(4*time.Second)); allow {
		t.Fatal("4s 仅补 0.8 枚，应拒绝")
	}

	if allow, _ := p.Enforce("peer1", tier, base.Add(5*time.Second)); !allow {
		t.Fatal("5s 补满 1 枚，应允许")
	}
}

// TestTierPacer_QualityDowngrade 评级转黄速率减半：6/min = 0.1/s
func TestTierPacer_QualityDowngrade(t *testing.T) {
	p := newTestPacer(TierDayCap)
	p.QualityDowngrade = true
	base := time.Now()
	tier := TierUtility

	for i := 0; i < 12; i++ {
		p.Enforce("peer2", tier, base)
	}

	if allow, _ := p.Enforce("peer2", tier, base); allow {
		t.Fatal("降级后桶容量减半，第 7 次应拒绝")
	}

	if allow, _ := p.Enforce("peer2", tier, base.Add(5*time.Second)); allow {
		t.Fatal("降级速率减半，5s 补币 0.5 枚应拒绝")
	}

	if allow, _ := p.Enforce("peer2", tier, base.Add(10*time.Second)); !allow {
		t.Fatal("降级后 10s 补满 1 枚应允许")
	}
}

// TestTierPacer_DayCap 24h 窗上限：超限拒绝且 retryAfter 指向窗口重置
func TestTierPacer_DayCap(t *testing.T) {
	p := newTestPacer(3)
	base := time.Now()
	tier := TierMarketing

	for i := 0; i < 3; i++ {
		if allow, _ := p.Enforce("peer3", tier, base); !allow {
			t.Fatalf("前 %d 次应允许", i+1)
		}
	}
	allow, retry := p.Enforce("peer3", tier, base)
	if allow {
		t.Fatal("超日上限应拒绝")
	}
	if retry != TierDayWindow {
		t.Fatalf("retryAfter = %v, want %v（窗口重置剩余时间）", retry, TierDayWindow)
	}

	if allow, _ := p.Enforce("peer3", TierUtility, base); !allow {
		t.Fatal("utility 与 marketing 桶独立，应允许")
	}

	if allow, _ := p.Enforce("peer3", tier, base.Add(TierDayWindow)); !allow {
		t.Fatal("窗口重置后应允许")
	}
}

// TestTierPacer_TierQuota 规格默认 per-tier 24h/peer 配额：marketing 1、utility 4、authentication 不限
func TestTierPacer_TierQuota(t *testing.T) {
	p := NewTierPacer()
	p.MinRatePerMin = 1000
	base := time.Now()

	if allow, _ := p.Enforce("p_m", TierMarketing, base); !allow {
		t.Fatal("marketing 第 1 条应允许")
	}
	if allow, retry := p.Enforce("p_m", TierMarketing, base); allow {
		t.Fatal("marketing 第 2 条应拒绝（24h/peer 配额 1）")
	} else if retry <= 0 || retry > TierDayWindow {
		t.Fatalf("marketing retryAfter = %v, 应在 (0,24h] 内", retry)
	}

	if allow, _ := p.Enforce("p_m", TierMarketing, base.Add(TierDayWindow)); !allow {
		t.Fatal("窗口重置后 marketing 应允许")
	}

	for i := 0; i < 4; i++ {
		if allow, _ := p.Enforce("p_u", TierUtility, base); !allow {
			t.Fatalf("utility 第 %d 条应允许", i+1)
		}
	}
	if allow, _ := p.Enforce("p_u", TierUtility, base); allow {
		t.Fatal("utility 第 5 条应拒绝（24h/peer 配额 4）")
	}

	for i := 0; i < 20; i++ {
		if allow, _ := p.Enforce("p_a", TierAuthentication, base); !allow {
			t.Fatalf("authentication 第 %d 条应允许（不限）", i+1)
		}
	}

	p2 := NewTierPacer()
	p2.TierQuotas = map[WATier]int{WATierMarketing: 2}
	for i := 0; i < 2; i++ {
		if allow, _ := p2.Enforce("p_o", TierMarketing, base); !allow {
			t.Fatalf("覆盖表下 marketing 第 %d 条应允许", i+1)
		}
	}
	if allow, _ := p2.Enforce("p_o", TierMarketing, base); allow {
		t.Fatal("覆盖表下 marketing 第 3 条应拒绝")
	}
}

// TestTierPacer_Jitter 抖动因子注入生效（速率上浮 → 更快补币）
func TestTierPacer_Jitter(t *testing.T) {
	p := NewTierPacer()
	p.MinRatePerMin = 12
	p.TierQuotas = map[WATier]int{}
	p.SetJitterFn(func() float64 { return 2.0 })
	base := time.Now()

	for i := 0; i < 12; i++ {
		p.Enforce("peer4", TierUtility, base)
	}

	if allow, _ := p.Enforce("peer4", TierUtility, base.Add(2500*time.Millisecond)); !allow {
		t.Fatal("抖动因子 2.0 下 2.5s 应补满 1 枚")
	}
}
