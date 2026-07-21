package ragretrieval

// rrf_fusion.go Reciprocal Rank Fusion 融合器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.4
// 论文: Cormack, Clarke, Büttcher 2009 SIGIR "Reciprocal Rank Fusion outperforms Condorcet
//       and individual Rank Learning Methods"（k=60 推导）
//
// 公式: score(d) = Σ_r 1 / (k + rank_r(d))
// 参数: k=60（Cormack 2009 SIGIR 推荐，OpenSearch/Elasticsearch 默认）
//
// 特性:
//   - 只用 rank 不用 score，避免 BM25 与 cosine 量纲不可比问题
//   - 文档在两路都出现则双倍加分（鼓励一致性）
//   - 文档只出现在一路也不丢弃（保留单路命中）
//   - 私域独立部署: 无 merchant_id 字段

import (
	"sort"
)

// RRFFusion Reciprocal Rank Fusion 融合器
type RRFFusion struct {
	k int
}

// NewRRFFusion 创建 RRF 融合器
//
// k <= 0 时使用默认值 60（Cormack 2009 SIGIR 推荐值）
func NewRRFFusion(k int) *RRFFusion {
	if k <= 0 {
		k = 60
	}
	return &RRFFusion{k: k}
}

// K 返回平滑常数 k
func (f *RRFFusion) K() int { return f.k }

// rrfEntry 内部聚合条目
type rrfEntry struct {
	chunk Chunk
	score float64
}

// Fuse 融合多路召回结果
//
// 输入: vecResults, bm25Results（按各自 score 降序）
// 输出: 融合后 topN（按 RRF score 降序）
//
// 注意:
//   - 融合后 Chunk.Score 被覆盖为 RRF score（0 ~ 2/k，单路命中 1/k，双路命中 2/k）
//   - topN <= 0 时使用默认值 20
//   - 输入切片可以为空（一路无结果时也能正常融合）
//   - 同一 chunk ID 在两路都出现时聚合分数；不同 chunk ID 各自独立计分
func (f *RRFFusion) Fuse(vecResults, bm25Results []Chunk, topN int) []Chunk {
	if topN <= 0 {
		topN = 20
	}
	merged := make(map[string]*rrfEntry, len(vecResults)+len(bm25Results))

	// 向量路：rank 从 1 开始（i+1）
	for i, c := range vecResults {
		id := c.ID
		rrfScore := 1.0 / float64(f.k+i+1)
		if e, ok := merged[id]; ok {
			e.score += rrfScore
		} else {
			merged[id] = &rrfEntry{chunk: c, score: rrfScore}
		}
	}
	// BM25 路
	for i, c := range bm25Results {
		id := c.ID
		rrfScore := 1.0 / float64(f.k+i+1)
		if e, ok := merged[id]; ok {
			e.score += rrfScore
		} else {
			merged[id] = &rrfEntry{chunk: c, score: rrfScore}
		}
	}

	// 排序取 topN（按 RRF score 降序；同分时按 chunk ID 稳定排序）
	list := make([]*rrfEntry, 0, len(merged))
	for _, e := range merged {
		list = append(list, e)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].chunk.ID < list[j].chunk.ID
	})
	if len(list) > topN {
		list = list[:topN]
	}

	out := make([]Chunk, 0, len(list))
	for _, e := range list {
		// 用 RRF score 覆盖原始 score
		e.chunk.Score = e.score
		out = append(out, e.chunk)
	}
	return out
}

// trimChunks 截断切片到 topK
func trimChunks(chunks []Chunk, topK int) []Chunk {
	if topK <= 0 {
		return chunks
	}
	if len(chunks) > topK {
		return chunks[:topK]
	}
	return chunks
}
