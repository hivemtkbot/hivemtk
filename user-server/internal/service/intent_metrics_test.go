package service

import (
	"math"
	"testing"

	"hivemtk-user/internal/dto"
)

const epsFloat = 1e-9

func assertMetricValue(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > epsFloat {
		t.Fatalf("%s = %.10f, want %.10f", name, got, want)
	}
}

// 手工数值验证 P/R/F1/宏平均 + 兜底隔离 + 低置信隔离
//
// 录入序列（阈值 N=0.9）：
//  1. price_inquiry  ← price_inquiry(0.95)   入矩阵
//  2. objection_price← objection_price(0.90) 入矩阵（边界：等于阈值即入）
//  3. objection_price← price_inquiry(0.85)   只进低置信桶
//  4. after_sale     ← price_inquiry(0.92)   入矩阵
//  5. unknown        ← unknown(0.95)         入矩阵（兜底类隔离验证）
//  6. price_inquiry  ← after_sale(0.99)      入矩阵
//
// 手工推算：
//
//	price_inquiry  : TP=1 FP=1(第6条) FN=1(第4条) → P=R=F1=0.5
//	objection_price: TP=1 FP=0(第3条低置信不得入矩阵!) FN=0 → P=R=F1=1
//	after_sale     : TP=0 FP=1(第4条) FN=1(第6条) → P=R=F1=0
//	unknown        : TP=1 FP=0 FN=0 → 各=1，但不参与宏平均
//	MacroF1 = (0.5+1+0)/3 = 0.5
func TestIntentWeakLabelHandComputed(t *testing.T) {
	store := NewConfusionStore()
	type rec struct {
		pred, truth string
		conf        float64
	}
	seq := []rec{
		{"price_inquiry", "price_inquiry", 0.95},
		{"objection_price", "objection_price", 0.90},
		{"objection_price", "price_inquiry", 0.85},
		{"after_sale", "price_inquiry", 0.92},
		{IntentUnknown, IntentUnknown, 0.95},
		{"price_inquiry", "after_sale", 0.99},
	}
	for _, r := range seq {
		store.RecordPrediction(r.pred, r.truth, r.conf)
	}

	snap := store.Snapshot()
	if snap.Total != 5 {
		t.Fatalf("Total=%d, want 5（第3条低置信不入矩阵）", snap.Total)
	}
	if snap.LowConfTotal != 1 || snap.LowConf["objection_price|price_inquiry"] != 1 {
		t.Fatalf("LowConf=%v, want 仅 objection_price|price_inquiry=1", snap.LowConf)
	}

	pi := snap.PerClass["price_inquiry"]
	if pi.TP != 1 || pi.FP != 1 || pi.FN != 1 {
		t.Fatalf("price_inquiry TP/FP/FN=%d/%d/%d, want 1/1/1", pi.TP, pi.FP, pi.FN)
	}
	assertMetricValue(t, "price_inquiry.P", pi.Precision, 0.5)
	assertMetricValue(t, "price_inquiry.R", pi.Recall, 0.5)
	assertMetricValue(t, "price_inquiry.F1", pi.F1, 0.5)

	op := snap.PerClass["objection_price"]
	if op.TP != 1 || op.FP != 0 || op.FN != 0 {
		t.Fatalf("objection_price TP/FP/FN=%d/%d/%d, want 1/0/0（低置信隔离）", op.TP, op.FP, op.FN)
	}
	assertMetricValue(t, "objection_price.F1", op.F1, 1)

	as := snap.PerClass["after_sale"]
	if as.TP != 0 || as.FP != 1 || as.FN != 1 {
		t.Fatalf("after_sale TP/FP/FN=%d/%d/%d, want 0/1/1", as.TP, as.FP, as.FN)
	}
	assertMetricValue(t, "after_sale.F1", as.F1, 0)

	if snap.PerClass[IntentUnknown].F1 != 1 {
		t.Fatalf("unknown F1 应独立统计为 1")
	}
	assertMetricValue(t, "MacroF1", snap.MacroF1, 0.5)
}

// 边界：confidence == 阈值入矩阵；恰好低于阈值进低置信桶
func TestIntentWeakLabelThresholdBoundary(t *testing.T) {
	store := NewConfusionStore()
	store.RecordPrediction("a", "a", WeakTruthMinConfidence)
	snap := store.Snapshot()
	if snap.Total != 1 || snap.LowConfTotal != 0 {
		t.Fatalf("阈值等于时应入矩阵: Total=%d LowTotal=%d", snap.Total, snap.LowConfTotal)
	}
	store.Reset()
	store.RecordPrediction("a", "a", WeakTruthMinConfidence-0.0000001)
	snap = store.Snapshot()
	if snap.Total != 0 || snap.LowConfTotal != 1 {
		t.Fatalf("阈值之下应只进低置信桶: Total=%d LowTotal=%d", snap.Total, snap.LowConfTotal)
	}
}

// 防御：空参数忽略，不影响任何计数
func TestIntentWeakLabelIgnoreEmpty(t *testing.T) {
	store := NewConfusionStore()
	store.RecordPrediction("", "b", 0.99)
	store.RecordPrediction("a", "", 0.99)
	store.RecordPrediction("", "", 0.99)
	snap := store.Snapshot()
	if snap.Total != 0 || snap.LowConfTotal != 0 || len(snap.PerClass) != 0 {
		t.Fatalf("空输入应全部忽略: %+v", snap)
	}
}

// 分母为零时指标回退为 0（如全错分类：x 无 TP 只有 FP，y 只有 FN）
func TestIntentWeakLabelZeroDenominator(t *testing.T) {
	store := NewConfusionStore()
	store.RecordPrediction("x", "y", 0.99)
	snap := store.Snapshot()
	x := snap.PerClass["x"]
	y := snap.PerClass["y"]
	if x.TP != 0 || x.FP != 1 || x.FN != 0 || y.TP != 0 || y.FP != 0 || y.FN != 1 {
		t.Fatalf("混淆计数错误: x=%+v y=%+v", x, y)
	}
	if x.Precision != 0 || x.Recall != 0 || x.F1 != 0 || y.Precision != 0 || y.Recall != 0 || y.F1 != 0 {
		t.Fatalf("零分母应回退 0: x=%+v y=%+v", x, y)
	}
	assertMetricValue(t, "MacroF1", snap.MacroF1, 0)
}

// 接入点守卫逻辑：只有规则真实命中才记账，nil/兜底/embedding/llm 全部跳过
func TestRecordIntentWeakLabelGuard(t *testing.T) {
	before := globalIntentMetrics.Snapshot().Total

	RecordIntentWeakLabel(nil)
	RecordIntentWeakLabel(&dto.RecognizeResult{Method: "embedding", IntentType: "price_inquiry", Confidence: 0.95})
	RecordIntentWeakLabel(&dto.RecognizeResult{Method: "llm", IntentType: "price_inquiry", Confidence: 0.95})
	RecordIntentWeakLabel(&dto.RecognizeResult{Method: "rule", IntentType: IntentUnknown, Confidence: 0.3})
	RecordIntentWeakLabel(&dto.RecognizeResult{Method: "rule", IntentType: "", Confidence: 0.95})

	if got := globalIntentMetrics.Snapshot().Total; got != before {
		t.Fatalf("以上情形均不应入账, Total %d -> %d", before, got)
	}

	RecordIntentWeakLabel(&dto.RecognizeResult{Method: "rule", IntentType: "price_inquiry", Confidence: 0.95})
	snap := globalIntentMetrics.Snapshot()
	if snap.Total != before+1 {
		t.Fatalf("规则命中应入账, Total=%d want %d", snap.Total, before+1)
	}
	pr := snap.PerClass["price_inquiry"]
	if pr.TP < 1 {
		t.Fatalf("price_inquiry 应有 TP: %+v", pr)
	}
}
