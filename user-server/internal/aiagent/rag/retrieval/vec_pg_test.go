package ragretrieval


import (
	"strings"
	"testing"
)

func TestVecToPGString(t *testing.T) {
	v := []float32{0.1, 0.2, 0.3}
	s := vecToPGString(v)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		t.Errorf("invalid pgvector literal: %q", s)
	}
	parsed, err := parsePGVector(s)
	if err != nil {
		t.Fatalf("parsePGVector failed: %v", err)
	}
	if len(parsed) != 3 {
		t.Errorf("parsed len=%d want=3", len(parsed))
	}
}

func TestVecToPGString_Empty(t *testing.T) {
	s := vecToPGString([]float32{})
	if s != "[]" {
		t.Errorf("empty vec should be [], got %q", s)
	}
}

func TestParsePGVector_Valid(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"[0.1,0.2,0.3]", 3},
		{"[1.0, 2.0, 3.0, 4.0]", 4},
		{"[]", 0},
		{"[1.5e-3,2.5e-4]", 2},
	}
	for _, c := range cases {
		v, err := parsePGVector(c.input)
		if err != nil {
			t.Errorf("parsePGVector(%q) err: %v", c.input, err)
			continue
		}
		if len(v) != c.want {
			t.Errorf("parsePGVector(%q) len=%d want=%d", c.input, len(v), c.want)
		}
	}
}

func TestParsePGVector_Invalid(t *testing.T) {
	cases := []string{
		"",
		"not a vector",
		"[",
		"[]]",
		"[abc]",
	}
	for _, c := range cases {
		_, err := parsePGVector(c)
		if err == nil {
			t.Errorf("parsePGVector(%q) should fail", c)
		}
	}
}

func TestEncodeDecodeVec_RoundTrip(t *testing.T) {
	original := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	encoded := encodeVec(original)
	decoded, err := decodeVec(encoded)
	if err != nil {
		t.Fatalf("decodeVec failed: %v", err)
	}
	if len(decoded) != len(original) {
		t.Fatalf("len mismatch: %d vs %d", len(decoded), len(original))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("idx %d: %.6f vs %.6f", i, decoded[i], original[i])
		}
	}
}

func TestSha256Hex(t *testing.T) {
	h1 := sha256Hex("hello")
	h2 := sha256Hex("hello")
	if h1 != h2 {
		t.Errorf("sha256Hex not deterministic")
	}
	h3 := sha256Hex("world")
	if h1 == h3 {
		t.Errorf("sha256Hex should differ for different inputs")
	}
	if len(h1) != 64 {
		t.Errorf("sha256Hex length=%d want=64", len(h1))
	}
}

func TestNormalizeQuery(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello world"},
		{"  Hello   World  ", "hello world"}, 
		{"HELLO\tWORLD\n", "hello world"},    
		{"", ""},
		{"Hello世界", "hello世界"}, 
	}
	for _, c := range cases {
		got := normalizeQuery(c.input)
		if got != c.want {
			t.Errorf("normalizeQuery(%q)=%q want=%q", c.input, got, c.want)
		}
	}
}

func TestTruncateContent(t *testing.T) {
	short := "短文本"
	if got := truncateContent(short, 100); got != short {
		t.Errorf("short text should not truncate, got=%q", got)
	}
	long := strings.Repeat("中", 200)
	got := truncateContent(long, 100)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated text should end with …, got=%q", got[len(got)-5:])
	}
	runes := []rune(got)
	if len(runes) != 101 {
		t.Errorf("truncated rune len=%d want=101", len(runes))
	}
}

