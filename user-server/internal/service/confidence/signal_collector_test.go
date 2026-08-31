package confidence

import (
	"context"
	"math"
	"testing"

	"hivemtk-user/internal/dto"
)

type mockEmbedder struct {
	vectors map[string][]float32
	err     error
}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	if v, ok := m.vectors[text]; ok {
		return v, nil
	}
	return []float32{1.0, 0.0, 0.0}, nil
}

func TestSignalCollector_EntityComp_EmptyExpected(t *testing.T) {
	c := NewSignalCollector(nil)
	got := c.computeEntityComp(map[string]any{"a": 1}, nil)
	if !approxEqual(got, 1.0) {
		t.Errorf("expected 为空应返回 1.0, got %v", got)
	}
	got = c.computeEntityComp(nil, map[string]any{})
	if !approxEqual(got, 1.0) {
		t.Errorf("expected 为空 map 应返回 1.0, got %v", got)
	}
}

func TestSignalCollector_EntityComp_FullMatch(t *testing.T) {
	c := NewSignalCollector(nil)
	extracted := map[string]any{"product": "iPhone", "price": 999, "color": "black"}
	expected := map[string]any{"product": "iPhone", "price": 999, "color": "black"}
	got := c.computeEntityComp(extracted, expected)
	if !approxEqual(got, 1.0) {
		t.Errorf("全部命中应返回 1.0, got %v", got)
	}
}

func TestSignalCollector_EntityComp_PartialMatch(t *testing.T) {
	c := NewSignalCollector(nil)
	extracted := map[string]any{"product": "iPhone", "price": 999}
	expected := map[string]any{"product": "iPhone", "price": 999, "color": "black"}
	got := c.computeEntityComp(extracted, expected)
	if !approxEqual(got, 2.0/3.0) {
		t.Errorf("部分命中应返回 2/3 ≈ 0.667, got %v", got)
	}
}

func TestSignalCollector_EntityComp_NoMatch(t *testing.T) {
	c := NewSignalCollector(nil)
	extracted := map[string]any{"a": 1}
	expected := map[string]any{"b": 2, "c": 3}
	got := c.computeEntityComp(extracted, expected)
	if !approxEqual(got, 0.0) {
		t.Errorf("完全不命中应返回 0.0, got %v", got)
	}
}

func TestSignalCollector_EntityComp_ValueMismatch(t *testing.T) {
	c := NewSignalCollector(nil)
	extracted := map[string]any{"product": "iPad"}
	expected := map[string]any{"product": "iPhone"}
	got := c.computeEntityComp(extracted, expected)
	if !approxEqual(got, 0.0) {
		t.Errorf("值不匹配应返回 0.0, got %v", got)
	}
}

func TestSignalCollector_CtxRelev_NilEmbedder(t *testing.T) {
	c := NewSignalCollector(nil)
	got, err := c.computeCtxRelev(context.Background(), "hello", []string{"hi"})
	if err != nil {
		t.Fatalf("nil embedder 不应返回错误: %v", err)
	}
	if !approxEqual(got, 0.5) {
		t.Errorf("nil embedder 应返回中性值 0.5, got %v", got)
	}
}

func TestSignalCollector_CtxRelev_EmptyTurns(t *testing.T) {
	c := NewSignalCollector(&mockEmbedder{})
	got, err := c.computeCtxRelev(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("空对话不应返回错误: %v", err)
	}
	if !approxEqual(got, 0.5) {
		t.Errorf("空对话应返回 0.5, got %v", got)
	}
}

func TestSignalCollector_CtxRelev_EmptyQuery(t *testing.T) {
	c := NewSignalCollector(&mockEmbedder{})
	got, err := c.computeCtxRelev(context.Background(), "  ", []string{"hi"})
	if err != nil {
		t.Fatalf("空 query 不应返回错误: %v", err)
	}
	if !approxEqual(got, 0.5) {
		t.Errorf("空 query 应返回 0.5, got %v", got)
	}
}

func TestSignalCollector_CtxRelev_IdenticalVectors(t *testing.T) {
	emb := &mockEmbedder{
		vectors: map[string][]float32{
			"hello": {1.0, 0.0, 0.0},
			"hi":    {1.0, 0.0, 0.0},
		},
	}
	c := NewSignalCollector(emb)
	got, err := c.computeCtxRelev(context.Background(), "hello", []string{"hi"})
	if err != nil {
		t.Fatalf("相同向量不应返回错误: %v", err)
	}
	if !approxEqual(got, 1.0) {
		t.Errorf("相同向量 cosine 应为 1.0, got %v", got)
	}
}

func TestSignalCollector_CtxRelev_OrthogonalVectors(t *testing.T) {
	emb := &mockEmbedder{
		vectors: map[string][]float32{
			"hello": {1.0, 0.0, 0.0},
			"world": {0.0, 1.0, 0.0},
		},
	}
	c := NewSignalCollector(emb)
	got, _ := c.computeCtxRelev(context.Background(), "hello", []string{"world"})
	if !approxEqual(got, 0.0) {
		t.Errorf("正交向量 cosine 应为 0.0, got %v", got)
	}
}

func TestSignalCollector_CtxRelev_MeanOfMultipleTurns(t *testing.T) {
	emb := &mockEmbedder{
		vectors: map[string][]float32{
			"q":  {1.0, 1.0, 0.0},
			"t1": {1.0, 0.0, 0.0},
			"t2": {0.0, 1.0, 0.0},
		},
	}
	c := NewSignalCollector(emb)
	got, _ := c.computeCtxRelev(context.Background(), "q", []string{"t1", "t2"})
	if got < 0.99 {
		t.Errorf("均值向量相似度应接近 1.0, got %v", got)
	}
}

func TestSignalCollector_RAGQual_Empty(t *testing.T) {
	c := NewSignalCollector(nil)
	if got := c.computeRAGQual(&dto.SignalCollectionInput{}); !approxEqual(got, 0.5) {
		t.Errorf("未执行+空 chunks 应返回中性 0.5, got %v", got)
	}
	if got := c.computeRAGQual(&dto.SignalCollectionInput{RAGExecuted: true}); !approxEqual(got, 0.0) {
		t.Errorf("已执行+空 chunks 应返回 0.0, got %v", got)
	}
}

func TestSignalCollector_RAGQual_PartialCoverage(t *testing.T) {
	c := NewSignalCollector(nil)
	chunks := []dto.RAGChunk{
		{Score: 0.8},
		{Score: 0.6},
		{Score: 0.4},
	}
	got := c.computeRAGQual(&dto.SignalCollectionInput{RAGChunks: chunks})
	if !approxEqual(got, 0.36) {
		t.Errorf("部分覆盖 RAGQual 应为 0.36, got %v", got)
	}
}

func TestSignalCollector_RAGQual_FullCoverage(t *testing.T) {
	c := NewSignalCollector(nil)
	chunks := []dto.RAGChunk{
		{Score: 0.8},
		{Score: 0.7},
		{Score: 0.6},
		{Score: 0.5},
		{Score: 0.4},
	}
	got := c.computeRAGQual(&dto.SignalCollectionInput{RAGChunks: chunks})
	if !approxEqual(got, 0.6) {
		t.Errorf("全覆盖 RAGQual 应为 0.6, got %v", got)
	}
}

func TestSignalCollector_RAGQual_MoreThanExpected(t *testing.T) {
	c := NewSignalCollector(nil)
	chunks := []dto.RAGChunk{
		{Score: 1.0}, {Score: 1.0}, {Score: 1.0}, {Score: 1.0},
		{Score: 1.0}, {Score: 1.0}, {Score: 1.0}, {Score: 1.0},
	}
	got := c.computeRAGQual(&dto.SignalCollectionInput{RAGChunks: chunks})
	if !approxEqual(got, 1.0) {
		t.Errorf("score 全 1.0 + 全覆盖应返回 1.0, got %v", got)
	}
}

func TestSignalCollector_LLMEntropy_EmptyLogprobs(t *testing.T) {
	c := NewSignalCollector(nil)
	got := c.computeLLMEntropy(nil)
	if !approxEqual(got, 0.5) {
		t.Errorf("空 logprobs 应返回 0.5, got %v", got)
	}
}

func TestSignalCollector_LLMEntropy_SingleLogprob(t *testing.T) {
	c := NewSignalCollector(nil)
	got := c.computeLLMEntropy([]float64{0.5})
	if !approxEqual(got, 1.0) {
		t.Errorf("单元素 logprobs 应返回 1.0, got %v", got)
	}
}

func TestSignalCollector_LLMEntropy_UniformDistribution(t *testing.T) {
	c := NewSignalCollector(nil)
	got := c.computeLLMEntropy([]float64{-1.0, -1.0, -1.0, -1.0})
	if got > 0.05 {
		t.Errorf("均匀分布 LLMEntropy 应接近 0, got %v", got)
	}
}

func TestSignalCollector_LLMEntropy_PeakedDistribution(t *testing.T) {
	c := NewSignalCollector(nil)
	got := c.computeLLMEntropy([]float64{100.0, -100.0, -100.0, -100.0})
	if got < 0.95 {
		t.Errorf("极端分布 LLMEntropy 应接近 1, got %v", got)
	}
}

func TestSignalCollector_LLMEntropy_TwoEqualLargeValues(t *testing.T) {
	c := NewSignalCollector(nil)
	got := c.computeLLMEntropy([]float64{10.0, 10.0, -10.0})
	if got < 0.30 || got > 0.45 {
		t.Errorf("两个相等大值 LLMEntropy 应在 [0.30, 0.45], got %v", got)
	}
}

func TestSignalCollector_Collect_Full(t *testing.T) {
	emb := &mockEmbedder{
		vectors: map[string][]float32{
			"hello": {1.0, 0.0, 0.0},
			"hi":    {1.0, 0.0, 0.0},
		},
	}
	c := NewSignalCollector(emb)
	in := &dto.SignalCollectionInput{
		Text:              "hello",
		IntentType:        "ask_product",
		RawIntentConf:     0.85,
		ExpectedEntities:  map[string]any{"product": "iPhone"},
		ExtractedEntities: map[string]any{"product": "iPhone"},
		RAGChunks: []dto.RAGChunk{
			{Score: 0.9}, {Score: 0.8}, {Score: 0.7}, {Score: 0.6}, {Score: 0.5},
		},
		LLMLogprobs: []float64{10.0, -1.0, -2.0},
		LastTurns:   []string{"hi"},
	}
	signals, err := c.Collect(context.Background(), in)
	if err != nil {
		t.Fatalf("Collect 失败: %v", err)
	}
	if !approxEqual(signals.IntentConf, 0.85) {
		t.Errorf("IntentConf=%v want 0.85", signals.IntentConf)
	}
	if !approxEqual(signals.EntityComp, 1.0) {
		t.Errorf("EntityComp=%v want 1.0", signals.EntityComp)
	}
	if !approxEqual(signals.CtxRelev, 1.0) {
		t.Errorf("CtxRelev=%v want 1.0", signals.CtxRelev)
	}
	if !approxEqual(signals.RAGQual, 0.7) {
		t.Errorf("RAGQual=%v want 0.7", signals.RAGQual)
	}
	if signals.LLMEntropy <= 0 || signals.LLMEntropy >= 1 {
		t.Errorf("LLMEntropy=%v 应在 (0,1)", signals.LLMEntropy)
	}
}

func TestSignalCollector_Collect_IntentConfClamped(t *testing.T) {
	c := NewSignalCollector(nil)
	in := &dto.SignalCollectionInput{
		RawIntentConf: 1.5,
	}
	signals, _ := c.Collect(context.Background(), in)
	if !approxEqual(signals.IntentConf, 1.0) {
		t.Errorf("IntentConf 应被 clip 到 1.0, got %v", signals.IntentConf)
	}

	in.RawIntentConf = -0.5
	signals, _ = c.Collect(context.Background(), in)
	if !approxEqual(signals.IntentConf, 0.0) {
		t.Errorf("IntentConf 应被 clip 到 0.0, got %v", signals.IntentConf)
	}
}

func TestCosineSim_SameVector(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	got := cosineSim(a, a)
	if !approxEqual(got, 1.0) {
		t.Errorf("相同向量 cosine 应为 1.0, got %v", got)
	}
}

func TestCosineSim_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	got := cosineSim(a, b)
	if !approxEqual(got, 0.0) {
		t.Errorf("正交向量 cosine 应为 0.0, got %v", got)
	}
}

func TestCosineSim_DifferentLength(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{1.0}
	got := cosineSim(a, b)
	if !approxEqual(got, 0.0) {
		t.Errorf("不同长度应返回 0.0, got %v", got)
	}
}

func TestCosineSim_ZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{1.0, 2.0}
	got := cosineSim(a, b)
	if !approxEqual(got, 0.0) {
		t.Errorf("零向量应返回 0.0, got %v", got)
	}
}

func TestClamp01(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{-1.0, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
		{math.NaN(), 0.0},
	}
	c := NewSignalCollector(nil)
	_ = c
	for _, tc := range cases {
		got := clamp01(tc.in)
		if !math.IsNaN(tc.in) && !approxEqual(got, tc.want) {
			t.Errorf("clamp01(%v)=%v want %v", tc.in, got, tc.want)
		}
	}
}
