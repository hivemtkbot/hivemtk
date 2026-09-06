package ragretrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

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

func encodeVec(v []float32) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeVec(s string) ([]float32, error) {
	var v []float32
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func normalizeQuery(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

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

func truncateContent(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
