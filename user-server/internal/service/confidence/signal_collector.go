package confidence

// signal_collector.go 5 维置信度信号采集器
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.2
//
// 5 维信号：
//   - IntentConf:  意图分类器置信度（外部传入，本采集器透传）
//   - EntityComp:  实体完整性 = |extracted ∩ expected| / |expected|
//   - CtxRelev:    上下文相关性 = cosine(query_embed, last_3_turns_mean_embed)
//   - RAGQual:     RAG 检索质量 = mean(top-k chunk score) * coverage_ratio
//   - LLMEntropy:  LLM 生成熵 = 1 - normalize(ShannonEntropy(top-20 logprobs))
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"math"
	"strings"

	"marketing/internal/dto"
)

// Embedder 向量化接口（解耦对 llm.EmbeddingServiceInterface 的依赖）
//
// SignalCollector 仅需要 Embed(text) → []float32 能力
// 生产环境注入 llm.EmbeddingService；测试注入 mock
type Embedder interface {
	// Embed 返回文本的 1024 维向量
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

	// 2. EntityComp
	signals.EntityComp = c.computeEntityComp(in.ExtractedEntities, in.ExpectedEntities)

	// 3. CtxRelev
	ctxRelev, err := c.computeCtxRelev(ctx, in.Text, in.LastTurns)
	if err == nil {
		signals.CtxRelev = ctxRelev
	} else {
		signals.CtxRelev = 0.5 // 降级
	}

	// 4. RAGQual
	signals.RAGQual = c.computeRAGQual(in.RAGChunks)

	// 5. LLMEntropy
	signals.LLMEntropy = c.computeLLMEntropy(in.LLMLogprobs)

	return signals, nil
}

// computeEntityComp 实体完整性
//
// 公式：|extracted ∩ expected| / |expected|
// expected 为空时返回 1.0（无期望实体则视为完整）
// 值为比较时：expected[k] 与 extracted[k] 相等才算命中
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

// computeCtxRelev 上下文相关性
//
// 公式：cosine(embed(query), mean(embed(last_3_turns)))
// 无上下文或 query 为空时返回 0.5（中性值）
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

	// 取最近 3 轮的均值向量
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

// computeRAGQual RAG 检索质量
//
// 公式：mean(top-k chunk score) * coverage_ratio
// coverage_ratio = min(1, len(chunks) / expected_k)，expected_k=5
// 无 chunks 时返回 0
func (c *SignalCollector) computeRAGQual(chunks []dto.RAGChunk) float64 {
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

// computeLLMEntropy LLM 生成熵
//
// 公式：1 - normalize(ShannonEntropy(top-k logprobs))
// 归一化：除以 log(k)，使熵 ∈ [0, 1]
// LLMEntropy 信号 = 1 - normalized_entropy（高熵→低置信）
// 无 logprobs 时返回 0.5（中性值，对应 API 未返回 logprobs 场景）
func (c *SignalCollector) computeLLMEntropy(logprobs []float64) float64 {
	if len(logprobs) == 0 {
		return 0.5
	}
	// softmax 归一化 logprobs
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
	// Shannon 熵
	entropy := 0.0
	for _, e := range expLp {
		p := e / expSum
		if p > 0 {
			entropy -= p * math.Log(p)
		}
	}
	// 归一化到 [0, 1]
	k := float64(len(logprobs))
	if k <= 1 {
		return 1.0
	}
	normalizedEntropy := entropy / math.Log(k)
	return clamp01(1.0 - normalizedEntropy)
}

// cosineSim 余弦相似度
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

// clamp01 把 v 限制到 [0, 1]
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
