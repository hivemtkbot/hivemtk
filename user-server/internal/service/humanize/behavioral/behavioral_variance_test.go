package behavioral

import (
	"math/rand"
	"testing"
)

// A11：匀速=机器人特征；打字速度方差使总延迟在不同 seed 下产生差异
func TestPlanSend_TypingSpeedVariance(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	text := "这款产品现在有活动，价格非常实惠，而且支持七天无理由退货，您看什么时候方便详细了解一下呢？"

	var totals []float64
	for _, seed := range []int64{1, 2, 3, 4, 5, 6} {
		plan := PlanSend(text, cfg, false, rand.New(rand.NewSource(seed)))
		totals = append(totals, plan.TotalDelay)
	}
	allSame := true
	for i := 1; i < len(totals); i++ {
		if totals[i] != totals[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatalf("变速后不同 seed 的总延迟应存在差异: %v", totals)
	}
}

// 犹豫停顿：概率触发时片段间隔显著大于基础间隔
func TestPlanSend_HesitationPauseInjected(t *testing.T) {
	base := DefaultBehaviorConfig()
	base.HesitationProb = 0

	text := "好的呢。这个确实值得考虑。我给您详细介绍下产品细节。另外还有配套的售后服务。我们这款产品的核心优势在于品质稳定。很多老客户用了之后都反馈非常好。现在下单还有优惠活动。您看什么时候方便我给您详细说明一下呢。"

	noHesit := PlanSend(text, base, false, rand.New(rand.NewSource(7)))

	hesi := base
	hesi.HesitationProb = 1.0
	withHesit := PlanSend(text, hesi, false, rand.New(rand.NewSource(7)))

	if len(withHesit.Intervals) == 0 || len(noHesit.Intervals) == 0 {
		t.Fatalf("长文本应有分段间隔: hesi=%v no=%v", len(withHesit.Intervals), len(noHesit.Intervals))
	}
	sumNo, sumWith := 0.0, 0.0
	for _, v := range noHesit.Intervals {
		sumNo += v
	}
	for _, v := range withHesit.Intervals {
		sumWith += v
	}
	if sumWith <= sumNo {
		t.Fatalf("犹豫停顿应增大间隔总和: with=%.2f no=%.2f", sumWith, sumNo)
	}
}

// 配置兼容：零值 jitter/hesitation 退化为原固定行为
func TestPlanSend_ZeroVarianceBackwardCompatible(t *testing.T) {
	cfg := BehaviorConfig{
		EnableTypingDelay:   true,
		TypingSpeedCPS:      25,
		ThinkingPauseSec:    3,
		EnableMessageSplit:  false,
		SplitMinIntervalSec: 1.5,
		TypingSpeedJitter:   0,
		HesitationProb:      0,
	}
	p1 := PlanSend("短消息", cfg, true, rand.New(rand.NewSource(42)))
	p2 := PlanSend("短消息", cfg, true, rand.New(rand.NewSource(99)))
	if p1.TotalDelay != p2.TotalDelay {
		t.Fatalf("零方差下延迟应确定: %.4f vs %.4f", p1.TotalDelay, p2.TotalDelay)
	}
	want := float64(len([]rune("短消息"))) / 25.0
	if p1.TotalDelay != want {
		t.Fatalf("零方差延迟应等于 原公式: got=%.4f want=%.4f", p1.TotalDelay, want)
	}
}

// 默认配置字段完整性守护
func TestDefaultBehaviorConfig_VarianceFields(t *testing.T) {
	c := DefaultBehaviorConfig()
	if c.TypingSpeedJitter != 0.2 || c.HesitationProb != 0.15 {
		t.Fatalf("默认方差参数漂移: %+v", c)
	}
	if c.HesitationMaxSec < c.HesitationMinSec {
		t.Fatalf("犹豫停顿上下限倒置")
	}
}
