package service

import "testing"

// IntentMetricsRegistry（I-5b 监督口径）手算验证
//
// 录入序列 RecordPrediction(gold, predicted, conf)：
//  1. price_inquiry   ← price_inquiry   TP[pi]++
//  2. objection_price ← price_inquiry   FP[pi]++ / FN[op]++
//  3. price_inquiry   ← after_sale      FP[as]++ / FN[pi]++
//  4. unknown         ← unknown         TP[unknown]++（兜底类独立统计）
//  5. ""              ← x               忽略（gold 缺失）
//  6. price_inquiry   ← ""              漏检 FN[pi]++
//  7. churn           ← fallback        fallback 独立计数，不入矩阵
//
// 手工推算：
//
//	price_inquiry  : TP=1 FP=1 FN=2 → P=0.5 R=0.3333 F1=0.4
//	objection_price: TP=0 FP=0 FN=1 → P=R=F1=0
//	after_sale     : TP=0 FP=1 FN=0 → P=R=F1=0
//	unknown        : TP=1 FP=0 FN=0 → P=R=F1=1（fallbackClassSet 成员，不参与宏平均）
//	MacroAvg = (0.4+0+0)/3 = 0.1333；Total=5（fallback 不计）；Fallback=1
func TestIntentMetricsRegistryHandComputed(t *testing.T) {
	reg := NewIntentMetricsRegistry()
	reg.RecordPrediction("price_inquiry", "price_inquiry", 0.95)
	reg.RecordPrediction("objection_price", "price_inquiry", 0.60)
	reg.RecordPrediction("price_inquiry", "after_sale", 0.70)
	reg.RecordPrediction("unknown", "unknown", 0.95)
	reg.RecordPrediction("", "x", 0.99)
	reg.RecordPrediction("price_inquiry", "", 0.50)
	reg.RecordPrediction("churn", fallbackPredictedClass, 0.40)

	snap := reg.Snapshot()
	if snap.Total != 5 {
		t.Fatalf("Total=%d, want 5（gold 空/fallback 不计入）", snap.Total)
	}
	if snap.Fallback != 1 {
		t.Fatalf("Fallback=%d, want 1（独立计数不入矩阵）", snap.Fallback)
	}
	if _, ok := snap.PerIntent["churn"]; ok {
		t.Fatal("fallback 样本不应在 PerIntent 产生 churn 类")
	}

	pi := snap.PerIntent["price_inquiry"]
	if pi.Precision != 0.5 || pi.Recall != 0.3333 || pi.F1 != 0.4 {
		t.Fatalf("price_inquiry = %+v, want P=0.5 R=0.3333 F1=0.4", pi)
	}
	if c := reg.counters["price_inquiry"]; c == nil || c.TP != 1 || c.FP != 1 || c.FN != 2 {
		t.Fatalf("price_inquiry counters = %+v, want TP=1 FP=1 FN=2", c)
	}
	op := snap.PerIntent["objection_price"]
	if op.Precision != 0 || op.Recall != 0 || op.F1 != 0 {
		t.Fatalf("objection_price = %+v, want P=R=F1=0（FN=1）", op)
	}
	if c := reg.counters["objection_price"]; c == nil || c.TP != 0 || c.FP != 0 || c.FN != 1 {
		t.Fatalf("objection_price counters = %+v, want FN=1 其余 0", c)
	}
	as := snap.PerIntent["after_sale"]
	if as.Precision != 0 || as.Recall != 0 || as.F1 != 0 {
		t.Fatalf("after_sale = %+v, want P=R=F1=0（FP=1）", as)
	}
	if c := reg.counters["after_sale"]; c == nil || c.TP != 0 || c.FP != 1 || c.FN != 0 {
		t.Fatalf("after_sale counters = %+v, want FP=1 其余 0", c)
	}
	if snap.PerIntent[IntentUnknown].F1 != 1 {
		t.Fatalf("unknown F1 应独立统计为 1: %+v", snap.PerIntent[IntentUnknown])
	}
	assertMetricValue(t, "MacroAvg", snap.MacroAvg, 0.1333)
}

// Reset 清零：重复统计互不残留
func TestIntentMetricsRegistryReset(t *testing.T) {
	reg := NewIntentMetricsRegistry()
	reg.RecordPrediction("a", "a", 0.9)
	reg.RecordPrediction("b", "fallback", 0.1)
	if reg.Snapshot().Total != 1 || reg.Snapshot().Fallback != 1 {
		t.Fatal("前置计数不符")
	}
	reg.Reset()
	snap := reg.Snapshot()
	if snap.Total != 0 || snap.Fallback != 0 || len(snap.PerIntent) != 0 || snap.MacroAvg != 0 {
		t.Fatalf("Reset 后应全零: %+v", snap)
	}
}

// 默认注册表可达且 Reset 幂等（进程级单例守卫）
func TestIntentMetricsRegistryDefaultSingleton(t *testing.T) {
	reg := DefaultIntentMetricsRegistry()
	if reg == nil {
		t.Fatal("DefaultIntentMetricsRegistry 不应为 nil")
	}
	ResetIntentMetrics()
	if got := DefaultIntentMetricsRegistry().Snapshot().Total; got != 0 {
		t.Fatalf("ResetIntentMetrics 后 Total=%d, want 0", got)
	}
}
