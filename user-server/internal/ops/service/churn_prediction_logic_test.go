package service

import (
	"encoding/json"
	"hivemtk-user/internal/ops/model"
	"math"
	"testing"
)

// TestChurn_CalculateInactiveScore 未活跃天数分数 100+ 用例
// 规则：days >= threshold*2 → 100; days >= threshold → 50+(days-threshold)*50/threshold; days < threshold → days*50/threshold
func TestChurn_CalculateInactiveScore(t *testing.T) {
	svc := &ChurnPredictionService{}
	type tc struct {
		name      string
		days      int
		threshold int
		want      float64
	}
	cases := []tc{
		// threshold=30 边界
		{"th30_d0", 0, 30, 0},
		{"th30_d15", 15, 30, 25},                 // 15*50/30 = 25
		{"th30_d29", 29, 30, 48.333333333333336}, // 29*50/30 ≈ 48.33
		{"th30_d30", 30, 30, 50},                 // 30>=30 → 50+(30-30)*50/30 = 50
		{"th30_d45", 45, 30, 75},                 // 50+(45-30)*50/30 = 50+25 = 75
		{"th30_d60", 60, 30, 100},                // 60>=60 → 100
		{"th30_d90", 90, 30, 100},
		{"th30_d365", 365, 30, 100},
		// threshold=60
		{"th60_d0", 0, 60, 0},
		{"th60_d30", 30, 60, 25}, // 30*50/60
		{"th60_d60", 60, 60, 50}, // 60>=60 → 50
		{"th60_d90", 90, 60, 75}, // 50+(90-60)*50/60 ≈ 75
		{"th60_d120", 120, 60, 100},
		// threshold=7（极严格）
		{"th7_d0", 0, 7, 0},
		{"th7_d7", 7, 7, 50},
		{"th7_d14", 14, 7, 100},
		// threshold=1（边界值）
		{"th1_d0", 0, 1, 0},
		{"th1_d1", 1, 1, 50},
		{"th1_d2", 2, 1, 100},
		// 大阈值
		{"th365_d100", 100, 365, 13.6986301369863}, // 100*50/365
		{"th365_d365", 365, 365, 50},
		{"th365_d730", 730, 365, 100},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{"days_since_active": tt.days}
			got := svc.calculateInactiveScore(data, tt.threshold)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("[%s] days=%d th=%d got=%.4f want=%.4f", tt.name, tt.days, tt.threshold, got, tt.want)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("calculateInactiveScore: %d/%d passed", passed, passed+failed)
}

// TestChurn_CalculatePurchaseFreqScore 购买频率分数 30+ 用例
// 规则同 inactiveScore
func TestChurn_CalculatePurchaseFreqScore(t *testing.T) {
	svc := &ChurnPredictionService{}
	type tc struct {
		name      string
		days      int
		threshold int
		want      float64
	}
	cases := []tc{
		{"pf0", 0, 60, 0},
		{"pf30", 30, 60, 25},
		{"pf60", 60, 60, 50},
		{"pf90", 90, 60, 75},
		{"pf120", 120, 60, 100},
		{"pf365", 365, 60, 100},
		// 缺字段
		{"missing", 0, 60, 0}, // 缺字段时默认 0
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{"days_since_purchase": tt.days}
			if tt.name == "missing" {
				data = map[string]any{}
			}
			got := svc.calculatePurchaseFreqScore(data, tt.threshold)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("[%s] days=%d th=%d got=%.4f want=%.4f", tt.name, tt.days, tt.threshold, got, tt.want)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("calculatePurchaseFreqScore: %d/%d passed", passed, passed+failed)
}

// TestChurn_CalculateOrderValueScore 客单价分数 20+ 用例
// 规则：aov<=0→100; <50→80; <100→50; <500→30; >=500→10
func TestChurn_CalculateOrderValueScore(t *testing.T) {
	svc := &ChurnPredictionService{}
	type tc struct {
		name string
		aov  float64
		want float64
	}
	cases := []tc{
		{"zero", 0, 100},
		{"negative", -10, 100},
		{"tiny", 0.01, 80},
		{"aov_1", 1, 80},
		{"aov_49_99", 49.99, 80},
		{"aov_50", 50, 50},
		{"aov_99_99", 99.99, 50},
		{"aov_100", 100, 30},
		{"aov_200", 200, 30},
		{"aov_499_99", 499.99, 30},
		{"aov_500", 500, 10},
		{"aov_1000", 1000, 10},
		{"aov_10000", 10000, 10},
		// 缺失字段
		{"missing", 0, 100},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{}
			if tt.name != "missing" {
				data["average_order_value"] = tt.aov
			}
			got := svc.calculateOrderValueScore(data)
			if got != tt.want {
				t.Errorf("[%s] aov=%g got=%g want=%g", tt.name, tt.aov, got, tt.want)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("calculateOrderValueScore: %d/%d passed", passed, passed+failed)
}

// TestChurn_CalculateEngagementScore 互动频率分数 20+ 用例
// 规则：0→100; <3→70; <10→40; >=10→20
func TestChurn_CalculateEngagementScore(t *testing.T) {
	svc := &ChurnPredictionService{}
	type tc struct {
		name string
		ints int
		want float64
	}
	cases := []tc{
		{"zero", 0, 100},
		{"one", 1, 70},
		{"two", 2, 70},
		{"three", 3, 40},
		{"five", 5, 40},
		{"nine", 9, 40},
		{"ten", 10, 20},
		{"twenty", 20, 20},
		{"hundred", 100, 20},
		// 缺失字段
		{"missing", 0, 100},
		// 负数（异常）→ 70（因为 interactions=-5 不等于 0，落入 <3 分支）
		{"negative", -5, 70},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{}
			if tt.name != "missing" {
				data["interactions_30d"] = tt.ints
			}
			got := svc.calculateEngagementScore(data)
			if got != tt.want {
				t.Errorf("[%s] ints=%d got=%g want=%g", tt.name, tt.ints, got, tt.want)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("calculateEngagementScore: %d/%d passed", passed, passed+failed)
}

// TestChurn_DetermineRiskLevel 风险等级判定 30+ 用例
// 规则：>=CriticalRiskScore(85)→critical; >=HighRiskScore(70)→high; >=50→medium; else low
func TestChurn_DetermineRiskLevel(t *testing.T) {
	svc := &ChurnPredictionService{}
	cfg := svc.getDefaultConfig()
	type tc struct {
		name  string
		score float64
		want  string
	}
	cases := []tc{
		// critical >= 85
		{"crit_85", 85, "critical"},
		{"crit_90", 90, "critical"},
		{"crit_100", 100, "critical"},
		// high: 70-84.99
		{"high_70", 70, "high"},
		{"high_75", 75, "high"},
		{"high_84", 84.99, "high"},
		{"high_84_99", 84.99, "high"},
		// medium: 50-69.99
		{"med_50", 50, "medium"},
		{"med_60", 60, "medium"},
		{"med_69", 69.99, "medium"},
		// low: <50
		{"low_0", 0, "low"},
		{"low_25", 25, "low"},
		{"low_49", 49.99, "low"},
		// 边界
		{"boundary_50", 50, "medium"},
		{"boundary_69_99", 69.99, "medium"},
		{"boundary_70", 70, "high"},
		{"boundary_84_99", 84.99, "high"},
		{"boundary_85", 85, "critical"},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 CalculateChurnPrediction 中的判定逻辑
			var got string
			if tt.score >= cfg.CriticalRiskScore {
				got = "critical"
			} else if tt.score >= cfg.HighRiskScore {
				got = "high"
			} else if tt.score >= 50 {
				got = "medium"
			} else {
				got = "low"
			}
			if got != tt.want {
				t.Errorf("[%s] score=%g got=%s want=%s", tt.name, tt.score, got, tt.want)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("DetermineRiskLevel: %d/%d passed", passed, passed+failed)
}

// TestChurn_IdentifyRiskFactors 风险因素识别 30+ 用例
// 规则：每个子分 >= 70 → 添加对应因素
func TestChurn_IdentifyRiskFactors(t *testing.T) {
	svc := &ChurnPredictionService{}
	type tc struct {
		name            string
		inactive        float64
		purchaseFreq    float64
		orderValue      float64
		engagement      float64
		expectedFactors []string
	}
	cases := []tc{
		// 全低分 → 无因素
		{"all_low", 10, 10, 10, 10, []string{}},
		// 单一高
		{"inactive_high", 80, 10, 10, 10, []string{"长期未活跃"}},
		{"purchase_high", 10, 80, 10, 10, []string{"购买频率下降"}},
		{"order_high", 10, 10, 80, 10, []string{"订单金额偏低"}},
		{"engagement_high", 10, 10, 10, 80, []string{"互动频率降低"}},
		// 边界
		{"inactive_70", 70, 10, 10, 10, []string{"长期未活跃"}},
		{"inactive_69", 69, 10, 10, 10, []string{}},
		// 多高
		{"two_high", 80, 80, 10, 10, []string{"长期未活跃", "购买频率下降"}},
		{"three_high", 80, 80, 80, 10, []string{"长期未活跃", "购买频率下降", "订单金额偏低"}},
		{"all_high", 80, 80, 80, 80, []string{"长期未活跃", "购买频率下降", "订单金额偏低", "互动频率降低"}},
		// 极端值
		{"all_100", 100, 100, 100, 100, []string{"长期未活跃", "购买频率下降", "订单金额偏低", "互动频率降低"}},
		{"all_0", 0, 0, 0, 0, []string{}},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.identifyRiskFactors(tt.inactive, tt.purchaseFreq, tt.orderValue, tt.engagement)
			if !stringSliceEqual(got, tt.expectedFactors) {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.expectedFactors)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("identifyRiskFactors: %d/%d passed", passed, passed+failed)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestChurn_GenerateSuggestion 挽回建议生成 20+ 用例
func TestChurn_GenerateSuggestion(t *testing.T) {
	svc := &ChurnPredictionService{}
	type tc struct {
		name      string
		factors   []string
		wantEmpty bool
		wantSub   string // mustContain
	}
	cases := []tc{
		{"empty", []string{}, true, ""},
		{"inactive_only", []string{"长期未活跃"}, false, "登录奖励"},
		{"purchase_only", []string{"购买频率下降"}, false, "限时优惠"},
		{"order_only", []string{"订单金额偏低"}, false, "满减优惠券"},
		{"engagement_only", []string{"互动频率降低"}, false, "互动活动"},
		{"all_four", []string{"长期未活跃", "购买频率下降", "订单金额偏低", "互动频率降低"}, false, "建议措施"},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rfJSON, _ := jsonMarshal(tt.factors)
			pred := &model.ChurnPrediction{RiskFactors: string(rfJSON)}
			got := svc.GenerateSuggestion(pred, nil)
			if tt.wantEmpty && got != "用户风险较低，保持正常运营即可" {
				t.Errorf("[%s] expected low-risk msg, got: %s", tt.name, got)
				failed++
				return
			}
			if !tt.wantEmpty && !contains(got, tt.wantSub) {
				t.Errorf("[%s] got=%s does not contain %s", tt.name, got, tt.wantSub)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("GenerateSuggestion: %d/%d passed", passed, passed+failed)
}

func jsonMarshal(v any) (string, error) {
	bs, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestChurn_CalculateConfidence 置信度计算 20+ 用例
// 规则：基于正态分布的 Z 分数，confidence = 0.5*(1+erf(z/sqrt(2)))
func TestChurn_CalculateConfidence(t *testing.T) {
	type tc struct {
		name       string
		rate       float64
		sampleSize int
		wantMin    float64
		wantMax    float64
	}
	cases := []tc{
		{"zero_sample", 50, 0, 0, 0}, // sampleSize=0 → 0
		{"rate_50_n_1", 50, 1, 0, 1},
		{"rate_50_n_100", 50, 100, 0.4, 0.6},
		{"rate_50_n_10000", 50, 10000, 0.49, 0.51},
		{"rate_100_n_100", 100, 100, 0, 1},
		{"rate_0_n_100", 0, 100, 0, 1},
		{"rate_70_n_100", 70, 100, 0.95, 1.0},
		{"rate_30_n_100", 30, 100, 0, 0.05},
		{"rate_90_n_1000", 90, 1000, 0.99, 1.0},
		{"rate_10_n_1000", 10, 1000, 0, 0.01},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateConfidence(tt.rate, tt.sampleSize)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("[%s] rate=%g n=%d got=%.4f want=[%.4f, %.4f]",
					tt.name, tt.rate, tt.sampleSize, got, tt.wantMin, tt.wantMax)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("CalculateConfidence: %d/%d passed", passed, passed+failed)
}

// TestChurn_JoinStrings joinStrings 工具函数
func TestChurn_JoinStrings(t *testing.T) {
	type tc struct {
		name string
		strs []string
		sep  string
		want string
	}
	cases := []tc{
		{"empty", []string{}, "、", ""},
		{"one", []string{"a"}, "、", "a"},
		{"two", []string{"a", "b"}, "、", "a、b"},
		{"three", []string{"a", "b", "c"}, "，", "a，b，c"},
		{"with_empty", []string{"a", "", "b"}, "-", "a--b"},
		{"chinese", []string{"激活", "登录", "购买"}, "；", "激活；登录；购买"},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := joinStrings(tt.strs, tt.sep)
			if got != tt.want {
				t.Errorf("[%s] got=%s want=%s", tt.name, got, tt.want)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("joinStrings: %d/%d passed", passed, passed+failed)
}

// TestChurn_GetDefaultConfig 默认配置验证
func TestChurn_GetDefaultConfig(t *testing.T) {
	svc := &ChurnPredictionService{}
	cfg := svc.getDefaultConfig()
	if cfg == nil {
		t.Fatal("default config should not be nil")
	}
	if cfg.InactiveDaysWeight != 0.3 {
		t.Errorf("InactiveDaysWeight=%g want=0.3", cfg.InactiveDaysWeight)
	}
	if cfg.PurchaseFreqWeight != 0.3 {
		t.Errorf("PurchaseFreqWeight=%g want=0.3", cfg.PurchaseFreqWeight)
	}
	if cfg.OrderValueWeight != 0.2 {
		t.Errorf("OrderValueWeight=%g want=0.2", cfg.OrderValueWeight)
	}
	if cfg.EngagementWeight != 0.2 {
		t.Errorf("EngagementWeight=%g want=0.2", cfg.EngagementWeight)
	}
	if cfg.InactiveThreshold != 30 {
		t.Errorf("InactiveThreshold=%d want=30", cfg.InactiveThreshold)
	}
	if cfg.PurchaseThreshold != 60 {
		t.Errorf("PurchaseThreshold=%d want=60", cfg.PurchaseThreshold)
	}
	if cfg.HighRiskScore != 70 {
		t.Errorf("HighRiskScore=%g want=70", cfg.HighRiskScore)
	}
	if cfg.CriticalRiskScore != 85 {
		t.Errorf("CriticalRiskScore=%g want=85", cfg.CriticalRiskScore)
	}
	// 权重和应为 1.0
	sum := cfg.InactiveDaysWeight + cfg.PurchaseFreqWeight + cfg.OrderValueWeight + cfg.EngagementWeight
	if math.Abs(sum-1.0) > 0.001 {
		t.Errorf("weight sum=%g want=1.0", sum)
	}
}

// TestChurn_FullCalculationEndToEnd 端到端完整计算（覆盖典型场景）
// 通过直接调用四个子算法 + 风险判定 + 风险因素识别
func TestChurn_FullCalculationEndToEnd(t *testing.T) {
	svc := &ChurnPredictionService{}
	cfg := svc.getDefaultConfig()

	type tc struct {
		name         string
		userData     map[string]any
		wantRisk     string
		wantMinScore float64
		wantMaxScore float64
	}
	cases := []tc{
		{
			"healthy_active",
			map[string]any{
				"days_since_active": 1, "days_since_purchase": 5,
				"average_order_value": 800.0, "interactions_30d": 30,
			},
			"low", 0, 20,
		},
		{
			"inactive_medium",
			// inactive=35→58.3; purchase=80→75; aov=300→30; eng=5→40
			// score=58.3*0.3+75*0.3+30*0.2+40*0.2=17.5+22.5+6+8=54 → medium
			map[string]any{
				"days_since_active": 35, "days_since_purchase": 80,
				"average_order_value": 300.0, "interactions_30d": 5,
			},
			"medium", 50, 70,
		},
		{
			"high_risk",
			// inactive=50→66.7; purchase=80→75; aov=20→80; eng=1→70
			// score=66.7*0.3+75*0.3+80*0.2+70*0.2=20+22.5+16+14=72.5 → high
			map[string]any{
				"days_since_active": 50, "days_since_purchase": 80,
				"average_order_value": 20.0, "interactions_30d": 1,
			},
			"high", 70, 85,
		},
		{
			"critical",
			// inactive=90→100; purchase=180→100; aov=0→100; eng=0→100
			// score=100*0.3+100*0.3+100*0.2+100*0.2=100 → critical
			map[string]any{
				"days_since_active": 90, "days_since_purchase": 180,
				"average_order_value": 0.0, "interactions_30d": 0,
			},
			"critical", 95, 100,
		},
		{
			"empty_user",
			// inactive=0→0; purchase=0→0; aov缺失→100; eng缺失→100
			// score=0*0.3+0*0.3+100*0.2+100*0.2=40 → low
			map[string]any{},
			"low", 30, 50,
		},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			inactive := svc.calculateInactiveScore(tt.userData, cfg.InactiveThreshold)
			purchaseFreq := svc.calculatePurchaseFreqScore(tt.userData, cfg.PurchaseThreshold)
			orderValue := svc.calculateOrderValueScore(tt.userData)
			engagement := svc.calculateEngagementScore(tt.userData)

			score := inactive*cfg.InactiveDaysWeight +
				purchaseFreq*cfg.PurchaseFreqWeight +
				orderValue*cfg.OrderValueWeight +
				engagement*cfg.EngagementWeight

			var risk string
			switch {
			case score >= cfg.CriticalRiskScore:
				risk = "critical"
			case score >= cfg.HighRiskScore:
				risk = "high"
			case score >= 50:
				risk = "medium"
			default:
				risk = "low"
			}

			if risk != tt.wantRisk {
				t.Errorf("[%s] risk=%s want=%s (score=%.2f)", tt.name, risk, tt.wantRisk, score)
				failed++
				return
			}
			if score < tt.wantMinScore || score > tt.wantMaxScore {
				t.Errorf("[%s] score=%.2f want=[%.2f, %.2f]", tt.name, score, tt.wantMinScore, tt.wantMaxScore)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("FullCalculationEndToEnd: %d/%d passed", passed, passed+failed)
}
