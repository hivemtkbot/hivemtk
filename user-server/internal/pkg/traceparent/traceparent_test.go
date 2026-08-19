package traceparent

import (
	"strings"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	header := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	c, err := Parse(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Version != "00" {
		t.Errorf("version: got %q want 00", c.Version)
	}
	if c.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id mismatch: %q", c.TraceID)
	}
	if c.SpanID != "00f067aa0ba902b7" {
		t.Errorf("span_id mismatch: %q", c.SpanID)
	}
	if !c.Sampled {
		t.Error("sampled should be true (flags=01)")
	}
}

func TestParse_NotSampled(t *testing.T) {
	c, err := Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Sampled {
		t.Error("sampled should be false (flags=00)")
	}
}

func TestParse_Empty(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Error("expected error for empty header")
	}
}

func TestParse_InvalidParts(t *testing.T) {
	cases := []string{
		"00-abc-def",                       // only 3 parts
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7", // 3 parts
		"00-4bf92f3577b34da6a3ce929d0e0e47-00f067aa0ba902b7-01", // trace_id too short
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902-01", // span_id too short
		"00-ZZf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // non-hex
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01", // all-zero trace_id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", // all-zero span_id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-1",  // flags too short
	}
	for _, h := range cases {
		_, err := Parse(h)
		if err == nil {
			t.Errorf("expected error for invalid header %q", h)
		}
	}
}

func TestParse_WhitespaceTolerated(t *testing.T) {
	c, err := Parse("  00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id mismatch: %q", c.TraceID)
	}
}

func TestBuild_RoundTrip(t *testing.T) {
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	spanID := "00f067aa0ba902b7"
	h, err := Build(traceID, spanID, true)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	c, err := Parse(h)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if c.TraceID != traceID {
		t.Errorf("round trip trace_id mismatch: %q", c.TraceID)
	}
	if c.SpanID != spanID {
		t.Errorf("round trip span_id mismatch: %q", c.SpanID)
	}
}

func TestBuild_Invalid(t *testing.T) {
	_, err := Build("tooshort", "00f067aa0ba902b7", true)
	if err == nil {
		t.Error("expected error for short trace_id")
	}
	_, err = Build("00000000000000000000000000000000", "00f067aa0ba902b7", true)
	if err == nil {
		t.Error("expected error for all-zero trace_id")
	}
	_, err = Build("4bf92f3577b34da6a3ce929d0e0e4736", "bad", true)
	if err == nil {
		t.Error("expected error for short span_id")
	}
}

func TestBuild_NotSampled(t *testing.T) {
	h, err := Build("4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7", false)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if !strings.HasSuffix(h, "-00") {
		t.Errorf("expected flags=00, got %q", h)
	}
}

func TestIsZeroContext(t *testing.T) {
	var c Context
	if !c.IsZeroContext() {
		t.Error("zero Context should be zero")
	}
	c.TraceID = "abc"
	if c.IsZeroContext() {
		t.Error("non-zero Context reported as zero")
	}
}

func TestFormatTraceIDForLegacy(t *testing.T) {
	got := FormatTraceIDForLegacy("  4BF92F3577B34DA6A3CE929D0E0E4736  ")
	if got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected lower+trim, got %q", got)
	}
}

func TestHexEncode(t *testing.T) {
	b := []byte{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	s, err := HexEncode(b, 16)
	if err != nil {
		t.Fatal(err)
	}
	if s != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("hex encode mismatch: %q", s)
	}
	_, err = HexEncode(b, 8)
	if err == nil {
		t.Error("expected length mismatch error")
	}
}
