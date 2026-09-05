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

	_ = math.Abs(0.0)
}

// TestAggregator_WithPlatt_NilSafe 验证 nil Platt 不破坏流程
func TestAggregator_WithPlatt_NilSafe(t *testing.T) {
	a := makeTestAggregator()
	a.SetPlatt(nil)

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

	scores := make([]float64, 100)
	for i := range scores {
		scores[i] = float64(i) / 100.0
	}
	cp := NewConformalPredictor(scores, 0.1)
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

	if dec.DecisionBand == dto.BandHandoff && dec.VetoTriggered == "" {
		t.Errorf("high conf should not escalate to handoff, got band=%s", dec.DecisionBand)
	}
}

// TestAggregator_WithConformal_LowCoverage 验证 Conformal 触发 abstention
//
// 当 1-conf > Conformal quantile 时，band 升级为 handoff
func TestAggregator_WithConformal_LowCoverage(t *testing.T) {
	a := makeTestAggregator()

	cp := NewConformalPredictor([]float64{0.1, 0.1, 0.1, 0.1, 0.1}, 0.1)
	a.SetConformal(cp)

	in := &dto.SignalCollectionInput{
		SessionID:     "s1",
		CustomerID:    "c1",
		MessageID:     "m1",
		Text:          "hello",
		IntentType:    "consult",
		RawIntentConf: 0.4,
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

	platt := NewPlattScaling()
	samples := []PlattSample{
		{DecisionValue: 0, Label: 0},
		{DecisionValue: 2, Label: 1},
		{DecisionValue: -1, Label: 0},
		{DecisionValue: 3, Label: 1},
	}
	platt.Fit(samples)
	a.SetPlatt(platt)

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
