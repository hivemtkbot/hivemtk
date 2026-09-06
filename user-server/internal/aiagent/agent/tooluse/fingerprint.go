package tooluse

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

func canonicalArgs(v any) []byte {
	var buf bytes.Buffer
	writeCanonical(&buf, v)
	return buf.Bytes()
}

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

		b, err := json.Marshal(x)
		if err != nil {
			buf.WriteString("\"<unsupported:")
			buf.WriteString(err.Error())
			buf.WriteString(">\"")
			return
		}

		var reparsed any
		if err := json.Unmarshal(b, &reparsed); err == nil {
			writeCanonical(buf, reparsed)
		} else {
			buf.Write(b)
		}
	}
}

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

func writeJSONString(buf *bytes.Buffer, s string) {
	b, _ := json.Marshal(s)
	buf.Write(b)
}

func structuralFingerprint(args map[string]any) string {
	if len(args) == 0 {
		return "empty"
	}
	canonical := canonicalArgs(args)
	h := sha256.Sum256(canonical)
	return hex.EncodeToString(h[:16])
}
