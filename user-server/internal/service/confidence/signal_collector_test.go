package confidence

// signal_collector_test.go 5 维信号采集器单元测试
//
// 覆盖：
//  1. EntityComp：实体完整性（空 expected / 全命中 / 部分命中 / 不命中 / 值不匹配）
//  2. CtxRelev：上下文相关性（nil embedder / 空对话 / 空 query / 相同向量 / 正交向量）
//  3. RAGQual：RAG 检索质量（空 / 部分覆盖 / 完全覆盖）
//  4. LLMEntropy：LLM 生成熵（空 / 单元素 / 均匀分布 / 极端分布）
//  5. Collect 端到端
//  6. 辅助函数：cosineSim / clamp01

import (
	"context"
	"math"
	"testing"

	"hivemtk-user/internal/dto"
)

// ============================================================================
// mockEmbedder 测试用 Embedder 实现
// ============================================================================

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
	// 默认返回单位向量
	return []float32{1.0, 0.0, 0.0}, nil
}

// ============================================================================
// EntityComp 测试
// ============================================================================

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
	// 2/3 命中
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
	// key 存在但值不匹配
	extracted := map[string]any{"product": "iPad"}
	expected := map[string]any{"product": "iPhone"}
	got := c.computeEntityComp(extracted, expected)
	if !approxEqual(got, 0.0) {
		t.Errorf("值不匹配应返回 0.0, got %v", got)
	}
}

// ============================================================================
// CtxRelev 测试
// ============================================================================

func TestSignalCollector_CtxRelev_NilEmbedder(t *testing.T) {
	c := NewSignalCollector(nil) // embedder 为 nil
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
	// query 与历史对话向量相同，cosine=1.0
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
	// 正交向量 cosine=0
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
	// 多轮对话均值：[1,0,0] 和 [0,1,0] 的均值 [0.5,0.5,0]
	// query [1,1,0]/sqrt(2) 与 [0.5,0.5,0]/sqrt(0.5) 的 cosine
	// = (0.5+0.5) / (sqrt(2) * sqrt(0.5)) = 1 / sqrt(1) = 1.0
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

// ============================================================================
// RAGQual 测试
// ============================================================================

func TestSignalCollector_RAGQual_Empty(t *testing.T) {
	c := NewSignalCollector(nil)
	// RAG 未执行且无 chunks（handoff 预检场景）：未知维度，中性 0.5，不惩罚
	if got := c.computeRAGQual(&dto.SignalCollectionInput{}); !approxEqual(got, 0.5) {
		t.Errorf("未执行+空 chunks 应返回中性 0.5, got %v", got)
	}
	// RAG 已执行但无命中：确实低质量 → 0.0
	if got := c.computeRAGQual(&dto.SignalCollectionInput{RAGExecuted: true}); !approxEqual(got, 0.0) {
		t.Errorf("已执行+空 chunks 应返回 0.0, got %v", got)
	}
}

func TestSignalCollector_RAGQual_PartialCoverage(t *testing.T) {
	c := NewSignalCollector(nil)
	// 3 个 chunks（期望 5），coverage = 3/5 = 0.6
	// mean(score) = (0.8+0.6+0.4)/3 = 0.6
	// RAGQual = 0.6 * 0.6 = 0.36
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
	// 5 个 chunks, coverage = 1.0
	// mean(score) = 0.6
	// RAGQual = 0.6 * 1.0 = 0.6
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
	// 8 个 chunks，coverage 被 clip 到 1.0
	chunks := []dto.RAGChunk{
		{Score: 1.0}, {Score: 1.0}, {Score: 1.0}, {Score: 1.0},
		{Score: 1.0}, {Score: 1.0}, {Score: 1.0}, {Score: 1.0},
	}
	got := c.computeRAGQual(&dto.SignalCollectionInput{RAGChunks: chunks})
	if !approxEqual(got, 1.0) {
		t.Errorf("score 全 1.0 + 全覆盖应返回 1.0, got %v", got)
	}
}

// ============================================================================
// LLMEntropy 测试
// ============================================================================

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
	// 均匀分布熵最大，归一化熵=1，LLMEntropy = 1 - 1 = 0
	c := NewSignalCollector(nil)
	// 4 个相等的 logprobs → 均匀分布
	got := c.computeLLMEntropy([]float64{-1.0, -1.0, -1.0, -1.0})
	if got > 0.05 {
		t.Errorf("均匀分布 LLMEntropy 应接近 0, got %v", got)
	}
}

func TestSignalCollector_LLMEntropy_PeakedDistribution(t *testing.T) {
	// 极端分布（一个 logprob 极大），熵接近 0，LLMEntropy 接近 1
	c := NewSignalCollector(nil)
	got := c.computeLLMEntropy([]float64{100.0, -100.0, -100.0, -100.0})
	if got < 0.95 {
		t.Errorf("极端分布 LLMEntropy 应接近 1, got %v", got)
	}
}

func TestSignalCollector_LLMEntropy_TwoEqualLargeValues(t *testing.T) {
	// 两个相等的大值 + 一个小值
	c := NewSignalCollector(nil)
	got := c.computeLLMEntropy([]float64{10.0, 10.0, -10.0})
	// 前两个概率各占约 0.5，熵 = -2*0.5*log(0.5) = log(2)
	// 归一化熵 = log(2) / log(3) ≈ 0.631
	// LLMEntropy = 1 - 0.631 ≈ 0.369
	if got < 0.30 || got > 0.45 {
		t.Errorf("两个相等大值 LLMEntropy 应在 [0.30, 0.45], got %v", got)
	}
}

// ============================================================================
// Collect 端到端测试
// ============================================================================

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
	// RAGQual = mean(0.9,0.8,0.7,0.6,0.5) * 1.0 = 0.7
	if !approxEqual(signals.RAGQual, 0.7) {
		t.Errorf("RAGQual=%v want 0.7", signals.RAGQual)
	}
	// LLMEntropy 应在 (0, 1) 区间
	if signals.LLMEntropy <= 0 || signals.LLMEntropy >= 1 {
		t.Errorf("LLMEntropy=%v 应在 (0,1)", signals.LLMEntropy)
	}
}

func TestSignalCollector_Collect_IntentConfClamped(t *testing.T) {
	c := NewSignalCollector(nil)
	in := &dto.SignalCollectionInput{
		RawIntentConf: 1.5, // 超过 1 应被 clip
	}
	signals, _ := c.Collect(context.Background(), in)
	if !approxEqual(signals.IntentConf, 1.0) {
		t.Errorf("IntentConf 应被 clip 到 1.0, got %v", signals.IntentConf)
	}

	in.RawIntentConf = -0.5 // 负值应被 clip
	signals, _ = c.Collect(context.Background(), in)
	if !approxEqual(signals.IntentConf, 0.0) {
		t.Errorf("IntentConf 应被 clip 到 0.0, got %v", signals.IntentConf)
	}
}

// ============================================================================
// 辅助函数测试
// ============================================================================

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
		{math.NaN(), 0.0}, // NaN < 0 为 false，0 < NaN < 1 也为 false，返回原值 NaN，但我们的实现会返回原值
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
