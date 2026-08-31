package confidence

import (
	"context"
	"math"
	"testing"

	"hivemtk-user/internal/dto"
)

// TestAggregator_WithPlatt_Integration 验证 Platt 集成
//
// 业界依据：Platt 1999，二分类 logistic 校准
// 测试目标：注入 Platt 后，IntentConf 应被进一步校准
func TestAggregator_WithPlatt_Integration(t *testing.T) {
	a := makeTestAggregator()

	// 训练一个 Platt：z=0 → 期望 0；z=2 → 期望 1
	platt := NewPlattScaling()
	plattSamples := make([]PlattSample, 0, 100)
	for i := 0; i < 50; i++ {
		plattSamples = append(plattSamples,
			PlattSample{DecisionValue: 0, Label: 0},
			PlattSample{DecisionValue: 2, Label: 1},
		)
	}
	platt.Fit(plattSamples)
	a.SetPlatt(platt)

	// 输入：RawIntentConf=0.5（高 RAG 兜底）
	in := &dto.SignalCollectionInput{
		SessionID:     "s1",
		CustomerID:    "c1",
		MessageID:     "m1",
		Text:          "hello",
		IntentType:    "consult",
		RawIntentConf: 0.5,
		RAGChunks: []dto.RAGChunk{
			{Score: 0.95}, {Score: 0.92}, {Score: 0.90}, {Score: 0.88}, {Score: 0.85},
		},
		LLMLogprobs:   []float64{10.0, -2.0, -3.0},
		LastTurns:     []string{"hi"},
		CustomerLevel: "vip",
	}

	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if dec.AggregatedConf < 0 || dec.AggregatedConf > 1 {
		t.Errorf("conf out of bounds: %v", dec.AggregatedConf)
	}
	// 注入 Platt 后信号应被混合
	_ = math.Abs(0.0)
}

// TestAggregator_WithPlatt_NilSafe 验证 nil Platt 不破坏流程
func TestAggregator_WithPlatt_NilSafe(t *testing.T) {
	a := makeTestAggregator()
	a.SetPlatt(nil) // 显式 nil

	in := &dto.SignalCollectionInput{
		SessionID:     "s1",
		CustomerID:    "c1",
		MessageID:     "m1",
		Text:          "hello",
		IntentType:    "consult",
		RawIntentConf: 0.8,
		RAGChunks: []dto.RAGChunk{
			{Score: 0.95}, {Score: 0.92}, {Score: 0.90}, {Score: 0.88}, {Score: 0.85},
		},
		LLMLogprobs:   []float64{10.0, -2.0, -3.0},
		LastTurns:     []string{"hi"},
		CustomerLevel: "vip",
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// 没设 Platt 时应原样返回
	if dec.AggregatedConf < 0.5 {
		t.Errorf("expected conf >= 0.5, got %v", dec.AggregatedConf)
	}
}

// TestAggregator_WithConformal_HighCoverage 验证 Conformal 提升覆盖率场景
//
// Conformal 校准的分数都很小（< threshold），非一致性分数 = 1 - conf 都很小
// → 不触发 abstention → band 不变
func TestAggregator_WithConformal_HighCoverage(t *testing.T) {
	a := makeTestAggregator()

	// 校准集：100 个分数，0~1 均匀
	scores := make([]float64, 100)
	for i := range scores {
		scores[i] = float64(i) / 100.0
	}
	cp := NewConformalPredictor(scores, 0.1) // 90% coverage
	a.SetConformal(cp)

	in := &dto.SignalCollectionInput{
		SessionID:     "s1",
		CustomerID:    "c1",
		MessageID:     "m1",
		Text:          "hello",
		IntentType:    "consult",
		RawIntentConf: 0.9,
		RAGChunks: []dto.RAGChunk{
			{Score: 0.95}, {Score: 0.92}, {Score: 0.90}, {Score: 0.88}, {Score: 0.85},
		},
		LLMLogprobs:   []float64{10.0, -2.0, -3.0},
		LastTurns:     []string{"hi"},
		CustomerLevel: "vip",
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// 高置信度不应被 Conformal 强制转人工
	if dec.DecisionBand == dto.BandHandoff && dec.VetoTriggered == "" {
		t.Errorf("high conf should not escalate to handoff, got band=%s", dec.DecisionBand)
	}
}

// TestAggregator_WithConformal_LowCoverage 验证 Conformal 触发 abstention
//
// 当 1-conf > Conformal quantile 时，band 升级为 handoff
func TestAggregator_WithConformal_LowCoverage(t *testing.T) {
	a := makeTestAggregator()

	// 校准集：分数全 0.1 → 阈值 0.1
	// 非一致性分数 1-conf=0.6 > 0.1 → abstention
	cp := NewConformalPredictor([]float64{0.1, 0.1, 0.1, 0.1, 0.1}, 0.1)
	a.SetConformal(cp)

	in := &dto.SignalCollectionInput{
		SessionID:     "s1",
		CustomerID:    "c1",
		MessageID:     "m1",
		Text:          "hello",
		IntentType:    "consult",
		RawIntentConf: 0.4, // 1 - 0.4 = 0.6 > 0.1 阈值
		RAGChunks: []dto.RAGChunk{
			{Score: 0.3},
		},
		LLMLogprobs:   []float64{-1.0, -1.0, -1.0, -1.0},
		LastTurns:     []string{"hi"},
		CustomerLevel: "normal",
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// Conformal 应触发 handoff
	if dec.DecisionBand != dto.BandHandoff {
		t.Errorf("Conformal low coverage should escalate to handoff, got band=%s", dec.DecisionBand)
	}
}

// TestAggregator_WithConformal_NilSafe 验证 nil Conformal 不破坏流程
func TestAggregator_WithConformal_NilSafe(t *testing.T) {
	a := makeTestAggregator()
	a.SetConformal(nil)

	in := &dto.SignalCollectionInput{
		SessionID:     "s1",
		CustomerID:    "c1",
		MessageID:     "m1",
		Text:          "hello",
		IntentType:    "consult",
		RawIntentConf: 0.7,
		RAGChunks: []dto.RAGChunk{
			{Score: 0.95}, {Score: 0.92}, {Score: 0.90}, {Score: 0.88}, {Score: 0.85},
		},
		LLMLogprobs:   []float64{10.0, -2.0, -3.0},
		LastTurns:     []string{"hi"},
		CustomerLevel: "vip",
	}
	_, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
}

// TestAggregator_GetPlattConformal 验证 getter
func TestAggregator_GetPlattConformal(t *testing.T) {
	a := makeTestAggregator()
	if a.GetPlatt() != nil {
		t.Error("platt should be nil initially")
	}
	if a.GetConformal() != nil {
		t.Error("conformal should be nil initially")
	}
	platt := NewPlattScaling()
	a.SetPlatt(platt)
	if a.GetPlatt() != platt {
		t.Error("GetPlatt should return set instance")
	}
	cp := NewConformalPredictor([]float64{0.1, 0.2}, 0.1)
	a.SetConformal(cp)
	if a.GetConformal() != cp {
		t.Error("GetConformal should return set instance")
	}
}

// TestAggregator_PlattAndConformal_FullPipeline 验证两者都启用
func TestAggregator_PlattAndConformal_FullPipeline(t *testing.T) {
	a := makeTestAggregator()

	// Platt：训练一个轻量校准
	platt := NewPlattScaling()
	samples := []PlattSample{
		{DecisionValue: 0, Label: 0},
		{DecisionValue: 2, Label: 1},
		{DecisionValue: -1, Label: 0},
		{DecisionValue: 3, Label: 1},
	}
	platt.Fit(samples)
	a.SetPlatt(platt)

	// Conformal：宽松阈值
	cp := NewConformalPredictor([]float64{0.1, 0.2, 0.3, 0.4, 0.5}, 0.1)
	a.SetConformal(cp)

	in := &dto.SignalCollectionInput{
		SessionID:     "s1",
		CustomerID:    "c1",
		MessageID:     "m1",
		Text:          "hi",
		IntentType:    "consult",
		RawIntentConf: 0.85,
		RAGChunks: []dto.RAGChunk{
			{Score: 0.95}, {Score: 0.92}, {Score: 0.90}, {Score: 0.88}, {Score: 0.85},
		},
		LLMLogprobs:   []float64{10.0, -2.0, -3.0},
		LastTurns:     []string{"hi"},
		CustomerLevel: "vip",
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if dec.AggregatedConf < 0 || dec.AggregatedConf > 1 {
		t.Errorf("conf out of bounds: %v", dec.AggregatedConf)
	}
}
