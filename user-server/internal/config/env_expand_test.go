package config

import (
	"os"
	"testing"
)

func TestExpandEnvWithDefault(t *testing.T) {
	oldVars := map[string]string{
		"LLM_BASE_URL":        os.Getenv("LLM_BASE_URL"),
		"EMBEDDING_BASE_URL":  os.Getenv("EMBEDDING_BASE_URL"),
		"TEST_PLAIN_VAR":      os.Getenv("TEST_PLAIN_VAR"),
		"TEST_VAR_WITH_COLON": os.Getenv("TEST_VAR_WITH_COLON"),
	}
	_ = os.Unsetenv("LLM_BASE_URL")
	_ = os.Unsetenv("EMBEDDING_BASE_URL")
	_ = os.Unsetenv("TEST_PLAIN_VAR")
	_ = os.Unsetenv("TEST_VAR_WITH_COLON")
	defer func() {
		for k, v := range oldVars {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	cases := []struct {
		name string
		in   string
		want string
		pre  func()
	}{
		{
			name: "no var placeholder, pass through",
			in:   "https://api.example.com/v1",
			want: "https://api.example.com/v1",
		},
		{
			name: "env var unset → use default (bash style)",
			in:   "${LLM_BASE_URL:http://127.0.0.1:8207/v1}",
			want: "http://127.0.0.1:8207/v1",
		},
		{
			name: "env var set → override default",
			in:   "${LLM_BASE_URL:http://127.0.0.1:8207/v1}",
			want: "https://my-llm.example.com/v1",
			pre:  func() { _ = os.Setenv("LLM_BASE_URL", "https://my-llm.example.com/v1") },
		},
		{
			name: "plain ${VAR} (no default) - unset returns empty",
			in:   "${TEST_PLAIN_VAR}",
			want: "",
		},
		{
			name: "plain ${VAR} - set returns value",
			in:   "${TEST_PLAIN_VAR}",
			want: "hello",
			pre:  func() { _ = os.Setenv("TEST_PLAIN_VAR", "hello") },
		},
		{
			name: "default contains port+colon, must not break regex",
			in:   "${EMBEDDING_BASE_URL:http://127.0.0.1:8208/v1}",
			want: "http://127.0.0.1:8208/v1",
		},
		{
			name: "default contains path with slash",
			in:   "${API_BASE:http://x:8000/api/v1}",
			want: "http://x:8000/api/v1",
		},
		{
			name: "yaml multi-line: multiple vars",
			in: `base_url: "${A:http://a:1}"
host: "${B:localhost}"`,
			want: `base_url: "http://a:1"
host: "localhost"`,
		},
		{
			name: "default is empty",
			in:   "${MAYBE:}",
			want: "",
		},
		{
			name: "literal $ not in placeholder",
			in:   "price is $10",
			want: "price is $10",
		},
		{
			name: "literal $ followed by space (not a var)",
			in:   "amount: $ 100",
			want: "amount: $ 100",
		},
		{
			name: "non env var sequence ${1abc} ignored (must start with letter/underscore)",
			in:   "${1invalid}",
			want: "${1invalid}",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.pre != nil {
				tc.pre()
			}
			got := expandEnvWithDefault(tc.in)
			if got != tc.want {
				t.Errorf("expandEnvWithDefault(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

