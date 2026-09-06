package text

import "testing"

func TestEstimateTokens_Empty(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("empty = %d, want 0", got)
	}
}

func TestEstimateTokens_ASCII(t *testing.T) {

	cases := []struct {
		in   string
		want int
	}{
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"abcdefgh", 2},
	}
	for _, c := range cases {
		if got := EstimateTokens(c.in); got != c.want {
			t.Fatalf("EstimateTokens(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestEstimateTokens_CJK(t *testing.T) {

	cases := []struct {
		in   string
		want int
	}{
		{"你好", 1},
		{"你好世界", 2},
		{"你好世", 2},
	}
	for _, c := range cases {
		if got := EstimateTokens(c.in); got != c.want {
			t.Fatalf("EstimateTokens(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestEstimateTokens_Mixed(t *testing.T) {

	if got := EstimateTokens("hello 你好"); got != 3 {
		t.Fatalf("mixed = %d, want 3", got)
	}
}

func TestEstimateTokensOf(t *testing.T) {
	if got := EstimateTokensOf("你好", "abcd"); got != 2 {
		t.Fatalf("batch = %d, want 2", got)
	}
}
