package textutil

import (
	"fmt"
	"unicode/utf8"
)

const DefaultMaxBytes = 8192

func TruncateText(s string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if len(s) <= maxBytes {
		return s
	}
	head := s[:maxBytes]
	for len(head) > 0 {
		if r, _ := utf8.DecodeLastRuneInString(head); r != utf8.RuneError {
			break
		}
		head = head[:len(head)-1]
	}
	return head + fmt.Sprintf("\u2026[truncated %d bytes]", len(s)-len(head))
}
