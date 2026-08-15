package ragretrieval


import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// vecToPGString 把 []float32 序列化为 pgvector 字面量字符串 '[0.1,0.2,...]'
//
// pgvector 支持的格式: '[1.0,2.0,3.0,...]'
// 必须用科学计数或保留小数位，否则 PG 会报 dimension mismatch
func vecToPGString(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 12)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// parsePGVector 把 pgvector 文本表示 '[0.1,0.2,...]' 解析为 []float32
//
// 用于从 embedding_cache 表读取 embedding::text 字段后反序列化
func parsePGVector(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("invalid pgvector literal: %q", s)
	}
	inner := s[1 : len(s)-1]
	if strings.TrimSpace(inner) == "" {
		return []float32{}, nil
	}
	parts := strings.Split(inner, ",")
	out := make([]float32, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		f, err := strconv.ParseFloat(p, 32)
		if err != nil {
			return nil, fmt.Errorf("parse pgvector element %q: %w", p, err)
		}
		out = append(out, float32(f))
	}
	return out, nil
}

// encodeVec 把 []float32 编码为 JSON 字符串（用于 Redis 缓存存储）
func encodeVec(v []float32) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// decodeVec 把 JSON 字符串解码为 []float32（用于从 Redis 读取）
func decodeVec(s string) ([]float32, error) {
	var v []float32
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	return v, nil
}

// sha256Hex 计算 SHA256 哈希并返回 hex 字符串
//
// 用于 query_rewrite_cache.query_hash 与 embedding_cache.text_hash
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// normalizeQuery 查询文本归一化（小写 + 去多余空白）
//
// 用于缓存 key 一致性：相同语义的 query 即使大小写/空白不同也能命中同一缓存
func normalizeQuery(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// 合并连续空白为单个空格
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return b.String()
}

// truncateContent 截断文本到最大长度（按 rune），尾部加省略号
func truncateContent(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

