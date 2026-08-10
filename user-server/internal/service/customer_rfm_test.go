package service

import (
	"testing"

	"hivemtk-user/internal/model"
)

func TestRFMScoreRecency(t *testing.T) {
	buckets := []int{7, 30, 90, 180}
	cases := []struct {
		days int
		want int
	}{
		{1, 5},
		{7, 5},
		{8, 4},
		{30, 4},
		{31, 3},
		{90, 3},
		{91, 2},
		{180, 2},
		{181, 1},
		{9999, 1},
	}
	for _, c := range cases {
		if got := rfmScoreRecency(c.days, buckets); got != c.want {
			t.Errorf("days=%d expected %d, got %d", c.days, c.want, got)
		}
	}
}

func TestRFMScoreFrequency(t *testing.T) {
	buckets := []int{10, 5, 3, 1}
	cases := []struct {
		freq int
		want int
	}{
		{15, 5},
		{10, 5},
		{9, 4},
		{5, 4},
		{4, 3},
		{3, 3},
		{2, 2},
		{1, 2},
		{0, 1},
	}
	for _, c := range cases {
		if got := rfmScoreFrequency(c.freq, buckets); got != c.want {
			t.Errorf("freq=%d expected %d, got %d", c.freq, c.want, got)
		}
	}
}

func TestRFMScoreMonetary(t *testing.T) {
	buckets := []int64{500000, 100000, 30000, 5000}
	cases := []struct {
		m    int64
		want int
	}{
		{600000, 5},
		{500000, 5},
		{499999, 4},
		{100000, 4},
		{99999, 3},
		{30000, 3},
		{29999, 2},
		{5000, 2},
		{4999, 1},
		{0, 1},
	}
	for _, c := range cases {
		if got := rfmScoreMonetary(c.m, buckets); got != c.want {
			t.Errorf("m=%d expected %d, got %d", c.m, c.want, got)
		}
	}
}

func TestDetermineSegment_Churn(t *testing.T) {
	// recency >= churnThreshold
	if got := determineSegment(1, 1, 1, 200, 180); got != model.RFMSegmentChurn {
		t.Errorf("expected churn, got %s", got)
	}
	// R=1 F=1
	if got := determineSegment(1, 1, 5, 100, 180); got != model.RFMSegmentChurn {
		t.Errorf("expected churn (R=1 F=1), got %s", got)
	}
}

func TestDetermineSegment_Champion(t *testing.T) {
	// R=5 F=4
	if got := determineSegment(5, 4, 5, 1, 180); got != model.RFMSegmentChampion {
		t.Errorf("expected champion, got %s", got)
	}
}

func TestDetermineSegment_Loyal(t *testing.T) {
	// R=4 F=3
	if got := determineSegment(4, 3, 3, 30, 180); got != model.RFMSegmentLoyal {
		t.Errorf("expected loyal, got %s", got)
	}
}

func TestDetermineSegment_AtRisk(t *testing.T) {
	// R=2 (会落入 at_risk)
	if got := determineSegment(2, 2, 2, 60, 180); got != model.RFMSegmentAtRisk {
		t.Errorf("expected at_risk, got %s", got)
	}
}

func TestDetermineSegment_Potential(t *testing.T) {
	// R=3 F=2 (potential)
	if got := determineSegment(3, 2, 2, 60, 180); got != model.RFMSegmentPotential {
		t.Errorf("expected potential, got %s", got)
	}
}

func TestCalcChurnRisk(t *testing.T) {
	cfg := DefaultRFMConfig()
	// 1) recency >= threshold → high
	level, score := calcChurnRisk(200, 0, 0, cfg)
	if level != "high" || score < 70 {
		t.Errorf("expected high/score>=70, got %s/%d", level, score)
	}
	// 2) recency 0 + F>=5 + M>=100000 → low
	level, score = calcChurnRisk(0, 10, 200000, cfg)
	if level != "low" {
		t.Errorf("expected low, got %s/%d", level, score)
	}
	// 3) 中等 (90~180 天, 频率低, 无金额) → medium
	level, score = calcChurnRisk(120, 1, 0, cfg)
	if level != "medium" {
		t.Errorf("expected medium, got %s/%d", level, score)
	}
	// 4) 极低风险（高频+高额+最近活跃）→ low
	level, _ = calcChurnRisk(0, 10, 200000, cfg)
	if level != "low" {
		t.Errorf("expected low, got %s", level)
	}
}
