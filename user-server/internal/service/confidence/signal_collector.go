package confidence

import (
	"context"
	"math"
	"strings"

	"hivemtk-user/internal/dto"
)

// Embedder 向量化接口（解耦对 llm.EmbeddingServiceInterface 的依赖）
//
// SignalCollector 仅需要 Embed(text) → []float32 能力
// 生产环境注入 llm.EmbeddingService；测试注入 mock
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// SignalCollector 5 维信号采集器
type SignalCollector struct {
	embedder Embedder
}

// NewSignalCollector 构造采集器
//
// embedder 可为 nil（CtxRelev 信号将降级为中性值 0.5）
func NewSignalCollector(embedder Embedder) *SignalCollector {
	return &SignalCollector{embedder: embedder}
}

// Collect 采集 5 维信号
//
// 步骤：
//  1. IntentConf：从意图识别器输出读取（已在 input.RawIntentConf 中）
//  2. EntityComp：实体完整性 = |extracted ∩ expected| / |expected|
//  3. CtxRelev：query 与最近 3 轮对话均值的余弦相似度
//  4. RAGQual：top-k chunk 余弦均值 * 覆盖率
//  5. LLMEntropy：1 - normalize(entropy(top-20 logprobs))
func (c *SignalCollector) Collect(ctx context.Context, in *dto.SignalCollectionInput) (*dto.FiveSignals, error) {
	signals := &dto.FiveSignals{
		IntentConf: clamp01(in.RawIntentConf),
	}

	signals.EntityComp = c.computeEntityComp(in.ExtractedEntities, in.ExpectedEntities)

	ctxRelev, err := c.computeCtxRelev(ctx, in.Text, in.LastTurns)
	if err == nil {
		signals.CtxRelev = ctxRelev
	} else {
		signals.CtxRelev = 0.5
	}

	signals.RAGQual = c.computeRAGQual(in)

	signals.LLMEntropy = c.computeLLMEntropy(in.LLMLogprobs)

	return signals, nil
}

func (c *SignalCollector) computeEntityComp(extracted, expected map[string]any) float64 {
	if len(expected) == 0 {
		return 1.0
	}
	hit := 0
	for k, v := range expected {
		if ev, ok := extracted[k]; ok && ev == v {
			hit++
		}
	}
	return clamp01(float64(hit) / float64(len(expected)))
}

func (c *SignalCollector) computeCtxRelev(ctx context.Context, query string, lastTurns []string) (float64, error) {
	if c.embedder == nil {
		return 0.5, nil
	}
	if len(lastTurns) == 0 || strings.TrimSpace(query) == "" {
		return 0.5, nil
	}
	queryVec, err := c.embedder.Embed(ctx, query)
	if err != nil {
		return 0.5, err
	}
	if len(queryVec) == 0 {
		return 0.5, nil
	}

	meanVec := make([]float32, len(queryVec))
	validCount := 0
	for _, turn := range lastTurns {
		if strings.TrimSpace(turn) == "" {
			continue
		}
		v, err := c.embedder.Embed(ctx, turn)
		if err != nil || len(v) != len(meanVec) {
			continue
		}
		for i := range v {
			meanVec[i] += v[i]
		}
		validCount++
	}
	if validCount == 0 {
		return 0.5, nil
	}
	for i := range meanVec {
		meanVec[i] /= float32(validCount)
	}
	return clamp01(cosineSim(queryVec, meanVec)), nil
}

func (c *SignalCollector) computeRAGQual(in *dto.SignalCollectionInput) float64 {
	if !in.RAGExecuted && len(in.RAGChunks) == 0 {
		return 0.5
	}
	chunks := in.RAGChunks
	if len(chunks) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, ch := range chunks {
		sum += ch.Score
	}
	meanScore := sum / float64(len(chunks))
	expectedK := 5.0
	coverage := math.Min(1.0, float64(len(chunks))/expectedK)
	return clamp01(meanScore * coverage)
}

func (c *SignalCollector) computeLLMEntropy(logprobs []float64) float64 {
	if len(logprobs) == 0 {
		return 0.5
	}
	maxLp := logprobs[0]
	for _, lp := range logprobs {
		if lp > maxLp {
			maxLp = lp
		}
	}
	expSum := 0.0
	expLp := make([]float64, len(logprobs))
	for i, lp := range logprobs {
		expLp[i] = math.Exp(lp - maxLp)
		expSum += expLp[i]
	}
	if expSum == 0 {
		return 0.5
	}
	entropy := 0.0
	for _, e := range expLp {
		p := e / expSum
		if p > 0 {
			entropy -= p * math.Log(p)
		}
	}
	k := float64(len(logprobs))
	if k <= 1 {
		return 1.0
	}
	normalizedEntropy := entropy / math.Log(k)
	return clamp01(1.0 - normalizedEntropy)
}

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	dot := float64(0)
	na := float64(0)
	nb := float64(0)
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0.0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
