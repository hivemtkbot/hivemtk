// Package strhash 提供字符串哈希工具
package strhash

import "crypto/sha256"

// StringToInt64 将字符串哈希为 int64(用于 product_id 字段)
//
// UUID 字符串 → int64:知识库主键是 UUID 字符串,而 knowledge_chunks.product_id
// 是 INTEGER 字段,存储 StringToInt64(UUID) 映射值以兼容既有 schema。
func StringToInt64(s string) int64 {
	h := sha256.Sum256([]byte(s))
	var n int64
	for i := 0; i < 8; i++ {
		n = (n << 8) | int64(h[i])
	}
	if n < 0 {
		n = -n
	}
	return n
}
