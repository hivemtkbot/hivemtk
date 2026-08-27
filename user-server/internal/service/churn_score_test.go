package service

import (
	"math"
	"testing"
)

// TestChurnScore_Formula 公式逐项验证：40*RFM + 25*clamp(slope) + 20*Neg + 15*Delay
func TestChurnScore_Formula(t *testing.T) {
	cases := []struct {
		name string
		in   ChurnInput
		want float64
	}{
		{"全零", ChurnInput{}, 0},
		{"满分", ChurnInput{RFMScore: 1, MsgSlope30d: 1, NegSentRatio: 1, ReplyDelayProlong: 1}, 100},
		{"仅RFM", ChurnInput{RFMScore: 0.5}, 20},
		{"slope超1截断", ChurnInput{MsgSlope30d: 2.5}, 25},
		{"slope负值截断", ChurnInput{MsgSlope30d: -3}, 0},
		{"仅负面", ChurnInput{NegSentRatio: 0.5}, 10},
		{"仅延迟", ChurnInput{ReplyDelayProlong: 0.5}, 7.5},
		{"混合", ChurnInput{RFMScore: 0.8, MsgSlope30d: 0.4, NegSentRatio: 0.3, ReplyDelayProlong: 0.2}, 32 + 10 + 6 + 3},
	}
	for _, c := range cases {
		if got := ChurnScore(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: ChurnScore = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestChurnScore_Deterministic 纯函数：同输入恒同输出
func TestChurnScore_Deterministic(t *testing.T) {
	in := ChurnInput{RFMScore: 0.6, MsgSlope30d: 1.2, NegSentRatio: 0.4, ReplyDelayProlong: 0.3}
	want := ChurnScore(in)
	for i := 0; i < 10; i++ {
		if got := ChurnScore(in); got != want {
			t.Fatalf("评分不稳定: %v vs %v", got, want)
		}
	}
}

// TestChurnBand_ThreeBands 三档边界：<40 healthy / 40-70 watch / >70 high_risk
func TestChurnBand_ThreeBands(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, ChurnBandHealthy},
		{39.9, ChurnBandHealthy},
		{40, ChurnBandWatch},
		{55, ChurnBandWatch},
		{70, ChurnBandWatch},
		{70.1, ChurnBandHighRisk},
		{100, ChurnBandHighRisk},
	}
	for _, c := range cases {
		if got := ChurnBand(c.score); got != c.want {
			t.Errorf("ChurnBand(%v) = %s, want %s", c.score, got, c.want)
		}
	}
}

// TestChurnScore_BandIntegration 评分与分档联动：典型画像落入预期档位
func TestChurnScore_BandIntegration(t *testing.T) {
	healthy := ChurnScore(ChurnInput{RFMScore: 0.1, MsgSlope30d: 0.2, NegSentRatio: 0.1, ReplyDelayProlong: 0.1})
	if ChurnBand(healthy) != ChurnBandHealthy {
		t.Fatalf("低风险画像应为 healthy, score=%v", healthy)
	}
	risky := ChurnScore(ChurnInput{RFMScore: 1, MsgSlope30d: 1, NegSentRatio: 0.8, ReplyDelayProlong: 0.9})
	if ChurnBand(risky) != ChurnBandHighRisk {
		t.Fatalf("高风险画像应为 high_risk, score=%v", risky)
	}
}
