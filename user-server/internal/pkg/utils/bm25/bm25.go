// Package bm25 提供 BM25-lite 文本匹配工具(私域基线 fallback)
package bm25

import "strings"

// Tokenize 分词(中文按字符 + 英文按词)
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	terms := make([]string, 0)
	word := strings.Builder{}
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			if word.Len() > 0 {
				terms = append(terms, word.String())
				word.Reset()
			}
			terms = append(terms, string(r))
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			word.WriteRune(r)
		} else {
			if word.Len() > 0 {
				terms = append(terms, word.String())
				word.Reset()
			}
		}
	}
	if word.Len() > 0 {
		terms = append(terms, word.String())
	}
	return terms
}

// ScoreText BM25-lite 文本打分(基于子串命中比例)
func ScoreText(text string, terms []string) float64 {
	lower := strings.ToLower(text)
	hits := 0
	for _, term := range terms {
		if strings.Contains(lower, term) {
			hits++
		}
	}
	if len(terms) == 0 {
		return 0
	}
	return float64(hits) / float64(len(terms))
}

