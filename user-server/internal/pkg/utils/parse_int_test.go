package utils

import "testing"

func TestParseInt64OrZero(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int64
	}{
		{"empty", "", 0},
		{"zero", "0", 0},
		{"positive", "12345", 12345},
		{"negative", "-7", -7},
		{"overflow_to_zero", "999999999999999999999999", 0},
		{"non_numeric", "abc", 0},
		{"mixed", "12abc", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseInt64OrZero("test."+c.name, c.raw)
			if got != c.want {
				t.Errorf("ParseInt64OrZero(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}

func TestParseInt64Strict(t *testing.T) {
	if v, err := ParseInt64Strict("ok", "42"); err != nil || v != 42 {
		t.Errorf("expected (42, nil), got (%d, %v)", v, err)
	}
	if v, err := ParseInt64Strict("empty", ""); err == nil || v != 0 {
		t.Errorf("empty should error, got (%d, %v)", v, err)
	}
	if v, err := ParseInt64Strict("bad", "abc"); err == nil || v != 0 {
		t.Errorf("bad should error, got (%d, %v)", v, err)
	}
	if _, err := ParseInt64Strict("neg", "-1"); err != nil {
		t.Errorf("neg should accept, got %v", err)
	}
}

func TestParamErrorMessage(t *testing.T) {
	e := &ParamError{Scope: "x", Raw: "abc", Reason: "syntax"}
	got := e.Error()
	if got == "" {
		t.Fatal("non-empty message expected")
	}
}
