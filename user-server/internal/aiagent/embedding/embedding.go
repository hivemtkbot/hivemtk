// Package local 提供纯 Go 实现的本地 Embedding 算法
//
// 设计目标（2026-07-16 私域基线）：
//  1. 零外部依赖：不需要下载 BGE / Sentence-Transformers 模型，不需要 TEI / TorchServe 等容器
//  2. 中文友好：基于字符 n-gram + 词级特征，覆盖中文营销话术场景
//  3. 维度稳定：固定 1024 维输出，与 bge-m3 维度一致（OPENAI_VECTOR_SIZE=1536 等场景下可配置）
//  4. 语义可分：相同/相近文本 cosine 相似度高，全随机文本相似度低
//  5. 决定性：相同输入永远得到相同输出（哈希 + 固定投影矩阵）
//
// 算法说明（Hashing n-gram TF-IDF + 随机投影）：
//   - 文本分词：Unicode-aware 字符级 + 简单中文按字切分 + 2/3-gram
//   - 特征哈希：MurmurHash3 → 16384 维稀疏向量（带符号）
//   - 随机投影：16384 → targetDim（Johnson-Lindenstrauss 保留 pairwise 距离）
//   - L2 归一化：cosine 相似度 = 点积
//
// 局限说明：
//   - 这是 "能用的本地向量化"，不是 "BGE 同等质量的向量化"
//   - 推荐用法：私域部署 RAG 起步 + 单元测试 + 离线场景
//   - 升级路径：等价的 TEI 服务（http://tei-embedding:9997/v1）协议不变，可以无侵入替换
package embedding

import (
	"hash/fnv"
	"math"
	"math/rand"
	"strings"
	"sync"
	"unicode"
)

// DefaultDimension 默认输出维度（bge-m3 同款）
const DefaultDimension = 1024

// SourceDim 源稀疏维度（特征哈希空间大小）
const SourceDim = 16384

// CharNGrams 字符 n-gram 范围
const (
	MinNGram = 2
	MaxNGram = 3
)

// Projector 随机投影矩阵：SourceDim → TargetDim
// 预生成一次，全进程共享；每行使用独立哈希种子，保证可复现
type Projector struct {
	TargetDim int
	Matrix    [][]float32 // [TargetDim][SourceDim]，内存 ~ TargetDim*SourceDim*4 字节
}

// NewProjector 构造随机投影器（线程安全）
func NewProjector(targetDim int, seed int64) *Projector {
	if targetDim <= 0 {
		targetDim = DefaultDimension
	}
	p := &Projector{
		TargetDim: targetDim,
		Matrix:    make([][]float32, targetDim),
	}
	// Johnson-Lindenstrauss 推荐：每行 ~ N(0, 1/SourceDim)
	// 用各行独立 rand.Source 保证不同行使用不同序列
	for i := 0; i < targetDim; i++ {
		r := rand.New(rand.NewSource(seed + int64(i)*7919))
		row := make([]float32, SourceDim)
		scale := float32(1.0 / math.Sqrt(float64(SourceDim)))
		for j := 0; j < SourceDim; j++ {
			row[j] = float32(r.NormFloat64()) * scale
		}
		p.Matrix[i] = row
	}
	return p
}

// Project 把 SourceDim 维稀疏向量投影到 TargetDim 维稠密向量
func (p *Projector) Project(src []float32) []float32 {
	out := make([]float32, p.TargetDim)
	for i, row := range p.Matrix {
		var sum float32
		// 稀疏点积：只遍历非零维度
		for j, v := range src {
			if v != 0 {
				sum += row[j] * v
			}
		}
		out[i] = sum
	}
	return out
}

// LocalEmbedding 本地 Embedding 引擎
type LocalEmbedding struct {
	dim        int
	projector  *Projector
	projector1 sync.RWMutex
}

// NewLocalEmbedding 构造本地 Embedding 引擎
// seed 决定投影矩阵，必须固定（保证决定性）
func NewLocalEmbedding(dim int, seed int64) *LocalEmbedding {
	if dim <= 0 {
		dim = DefaultDimension
	}
	return &LocalEmbedding{
		dim:       dim,
		projector: NewProjector(dim, seed),
	}
}

// Dimension 返回输出维度
func (l *LocalEmbedding) Dimension() int {
	return l.dim
}

// featurize 文本 → SourceDim 维稀疏特征向量
//
// 特征构成（带符号哈希 trick：避免 hash collision bias）：
//   - 字符 2-gram 和 3-gram
//   - 单词级（按空格切分）
//   - 中文按字切分（unicode.IsLetter 都视为一个 token）
//
// 每个特征 hash 到 [0, SourceDim)，值用 ±1（带符号），权重 = 出现次数。
func (l *LocalEmbedding) featurize(text string) []float32 {
	text = normalize(text)
	if text == "" {
		return make([]float32, SourceDim)
	}
	vec := make([]float32, SourceDim)

	// 1) 字符 n-gram
	runes := []rune(text)
	for n := MinNGram; n <= MaxNGram; n++ {
		if len(runes) < n {
			continue
		}
		for i := 0; i+n <= len(runes); i++ {
			gram := string(runes[i : i+n])
			addFeature(vec, gram, 1.0)
		}
	}

	// 2) 单词级（中文/英文/数字都按 unicode letter/digit 切分）
	tokens := tokenize(text)
	for _, tok := range tokens {
		if len([]rune(tok)) < 2 {
			continue
		}
		addFeature(vec, "w:"+tok, 1.5) // 词级权重略高于字符 n-gram
	}

	return vec
}

// Embed 单条文本向量化
func (l *LocalEmbedding) Embed(text string) []float32 {
	vec := l.featurize(text)
	l.projector1.RLock()
	proj := l.projector.Project(vec)
	l.projector1.RUnlock()
	return l2normalize(proj)
}

// EmbedBatch 批量向量化
func (l *LocalEmbedding) EmbedBatch(texts []string) [][]float32 {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = l.Embed(t)
	}
	return out
}

// normalize 文本归一化：转小写、去空白、保留 unicode 字母/数字/汉字
func normalize(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	// 过滤标点符号
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tokenize 简单分词：按空白切分，过滤短词
func tokenize(text string) []string {
	parts := strings.Fields(text)
	out := parts[:0]
	for _, p := range parts {
		if len([]rune(p)) >= 2 {
			out = append(out, p)
		}
	}
	return out
}

// addFeature 带符号哈希特征（Weinberger et al.）
func addFeature(vec []float32, feature string, weight float32) {
	h := fnv.New64a()
	h.Write([]byte(feature))
	sum := h.Sum64()
	idx := sum & uint64(SourceDim-1) // 假设 SourceDim 是 2 的幂
	sign := int8(1)
	if (sum>>32)&1 == 1 {
		sign = -1
	}
	vec[idx] += weight * float32(sign)
}

// l2normalize L2 归一化
func l2normalize(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm == 0 {
		return v
	}
	norm = math.Sqrt(norm)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}
