package tooluse

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

// canonicalArgs 规范化参数为稳定的字节表示，用作结构指纹输入。
//
// 业界依据：OpenAI Agents SDK / Anthropic Tool Use / LangGraph 在循环检测时
// 都采用「结构指纹」（structural fingerprint）而非字符串匹配。理由：
//   - LLM 工具调用的参数对象**字段顺序不稳定**（function call 多次采样顺序不同）
//   - JSON Marshal 对 map 自动排序但对 struct 不排序
//   - float64 精度差异（1.0 vs 1.00 vs 1.0e0）字符串化后不同
//   - nil map 与空 map 在 JSON 表现相同但 Go 内存不同
//
// 规范化策略：
//   1. 嵌套 map[string]any 按 key 升序排列
//   2. slice 保留顺序（业务语义相关，不可重排）
//   3. float64 统一 strconv 格式 'g' 保留精度
//   4. nil 显式序列化为 "null"（与 JSON 标准一致）
//   5. 不可处理类型（chan/func）→ 序列化为 "<unsupported>"
//
// 输出为标准 JSON 字节，喂给 SHA256 → 取前 16 字节 hex（与原 hashArgs 长度一致）。
func canonicalArgs(v any) []byte {
	var buf bytes.Buffer
	writeCanonical(&buf, v)
	return buf.Bytes()
}

// writeCanonical 递归写入规范化表示
func writeCanonical(buf *bytes.Buffer, v any) {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		writeJSONString(buf, x)
	case int:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int8:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int16:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int32:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int64:
		buf.WriteString(strconv.FormatInt(x, 10))
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint8:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint16:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint32:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint64:
		buf.WriteString(strconv.FormatUint(x, 10))
	case float32:
		// 'g' 保留精度且去掉无意义的 0
		buf.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	case float64:
		buf.WriteString(strconv.FormatFloat(x, 'g', -1, 64))
	case json.Number:
		buf.WriteString(string(x))
	case map[string]any:
		writeCanonicalMap(buf, x)
	case []any:
		writeCanonicalSlice(buf, x)
	default:
		// 兜底：用 JSON Marshal（struct 等），失败则标 unsupported
		b, err := json.Marshal(x)
		if err != nil {
			buf.WriteString("\"<unsupported:")
			buf.WriteString(err.Error())
			buf.WriteString(">\"")
			return
		}
		// 二次规范化：marshal 后的 bytes 重新解析再规范化，确保嵌套 struct 字段排序
		var reparsed any
		if err := json.Unmarshal(b, &reparsed); err == nil {
			writeCanonical(buf, reparsed)
		} else {
			buf.Write(b)
		}
	}
}

// writeCanonicalMap 按 key 升序写入 {k1:v1,k2:v2,...}
func writeCanonicalMap(buf *bytes.Buffer, m map[string]any) {
	if len(m) == 0 {
		buf.WriteString("{}")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeJSONString(buf, k)
		buf.WriteByte(':')
		writeCanonical(buf, m[k])
	}
	buf.WriteByte('}')
}

// writeCanonicalSlice 保留顺序写入 [v0,v1,...]
func writeCanonicalSlice(buf *bytes.Buffer, s []any) {
	if len(s) == 0 {
		buf.WriteString("[]")
		return
	}
	buf.WriteByte('[')
	for i, item := range s {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeCanonical(buf, item)
	}
	buf.WriteByte(']')
}

// writeJSONString 写入 JSON 字符串（含必要转义）
func writeJSONString(buf *bytes.Buffer, s string) {
	b, _ := json.Marshal(s)
	buf.Write(b)
}

// structuralFingerprint 计算 args 的结构指纹（顺序无关 + 类型无关）。
//
// 与原 hashArgs 的关键区别：
//   - 原：直接 json.Marshal(map) → 依赖 Go 自身对 map key 的字母排序
//   - 新：自实现规范化器，递归排序所有嵌套 map，对 float 用 'g' 格式，对 nil 显式 null
//
// 返回：16 字节 hex（32 字符），与原 hashArgs 同长度（保留 DB / 日志兼容）。
func structuralFingerprint(args map[string]any) string {
	if len(args) == 0 {
		return "empty"
	}
	canonical := canonicalArgs(args)
	h := sha256.Sum256(canonical)
	return hex.EncodeToString(h[:16])
}
