// Package retrieval 查询权重自适应（D17）。
//
// 作用范围声明：仅 HybridSearcher.SearchIndex 路径生效；
// RagRetrievalServiceImpl（rag_retrieval.go）不经 HybridSearcher，自适应对其不生效——预期内。
package ragretrieval

import (
	"regexp"
	"strings"
)

// QueryWeightProfile 单次查询的融合权重档位
type QueryWeightProfile struct {
	VectorWeight  float64
	KeywordWeight float64
}

// identifierPatterns 标识符强模式（D17）：
// 字母前缀 + 数字 ≥2 位（可带 -_ 连接），如 JGJ80-2003 / TS-999 / ABC123 / GB 50168（归一空格后命中）。
// 刻意排除纯数字（电话/价格/流水号缺判别性字母前缀，ts_rank 分词难命中，更偏语义路——审核修正 2）。
var identifierPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[A-Za-z]{1,4}[-_ ]?\d{2,}`),
}

// ResolveQueryWeightProfile 按查询特征返回融合权重档位：
// 标识符查询（型号/编号/错误码）→ keyword 主导（0.7/0.3 对调）；
// 常规语义查询 → base 档（config 默认 0.7/0.3）。权重和恒为 1。
func ResolveQueryWeightProfile(query string, baseVecW, baseKwW float64) QueryWeightProfile {
	q := strings.ToUpper(strings.TrimSpace(query))
	for _, re := range identifierPatterns {
		if re.MatchString(q) {
			return normalizeWeights(0.3, 0.7)
		}
	}
	return normalizeWeights(baseVecW, baseKwW)
}

func normalizeWeights(vecW, kwW float64) QueryWeightProfile {
	sum := vecW + kwW
	if sum <= 0 {
		return QueryWeightProfile{VectorWeight: 0.7, KeywordWeight: 0.3}
	}
	return QueryWeightProfile{VectorWeight: vecW / sum, KeywordWeight: kwW / sum}
}
