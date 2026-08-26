package text

import "testing"

func TestEstimateTokens_Empty(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("empty = %d, want 0", got)
	}
}

func TestEstimateTokens_ASCII(t *testing.T) {
	// 4 ASCII 字符 ≈ 1 token
	cases := []struct {
		in   string
		want int
	}{
		{"a", 1},      // ceil(1/4)
		{"abcd", 1},   // 4/4
		{"abcde", 2},  // ceil(5/4)
		{"abcdefgh", 2},
	}
	for _, c := range cases {
		if got := EstimateTokens(c.in); got != c.want {
			t.Fatalf("EstimateTokens(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestEstimateTokens_CJK(t *testing.T) {
	// 中文口径：字符数/2（向上取整）
	cases := []struct {
		in   string
		want int
	}{
		{"你好", 1},     // 2/2
		{"你好世界", 2},   // 4/2
		{"你好世", 2},    // ceil(3/2)
	}
	for _, c := range cases {
		if got := EstimateTokens(c.in); got != c.want {
			t.Fatalf("EstimateTokens(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestEstimateTokens_Mixed(t *testing.T) {
	// "hello 你好" = 6 ascii(ceil=2) + 2 wide(1) + 空格算 ascii → ascii=7? 
	// 精确："hello 你好" runes: h e l l o ' ' 你 好 → ascii=7, wide=2 → ceil(7/4)=2 + 1 = 3
	if got := EstimateTokens("hello 你好"); got != 3 {
		t.Fatalf("mixed = %d, want 3", got)
	}
}

func TestEstimateTokensOf(t *testing.T) {
	if got := EstimateTokensOf("你好", "abcd"); got != 2 {
		t.Fatalf("batch = %d, want 2", got)
	}
}
