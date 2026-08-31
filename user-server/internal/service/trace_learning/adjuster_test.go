package trace_learning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWeightAction(t *testing.T) {
	cfg := DefaultConfig()
	cases := []struct {
		name          string
		score         int
		bad           bool
		safety        float64
		safetyPresent bool
		want          string
	}{
		{"低分差", 30, false, 80, true, "decay"},
		{"LLM标差(即使高分)", 78, true, 90, true, "decay"},
		{"safety违规强制差", 95, false, 65, true, "decay"},
		{"高分好", 90, false, 100, true, "boost"},
		{"中间分不变", 70, false, 100, true, "none"},
		{"缺失safety不误判为差", 90, false, 0, false, "boost"},
		{"安全维度显式0仍判差", 90, false, 0, true, "decay"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, weightAction(c.score, c.bad, c.safety, c.safetyPresent, cfg), c.name)
	}
}

func TestClampWeight(t *testing.T) {
	assert.Equal(t, 0.1, clampWeight(0.01, 0.1, 3.0))
	assert.Equal(t, 3.0, clampWeight(9, 0.1, 3.0))
	assert.Equal(t, 1.0, clampWeight(1.0, 0.1, 3.0))
	assert.Equal(t, 1.5, clampWeight(1.5, 0.1, 3.0))
}

func TestComputeNewWeight_MeanReversion(t *testing.T) {
	cfg := DefaultConfig()
	assert.InDelta(t, 1.108, computeNewWeight(1.0, "boost", cfg), 1e-9)
	assert.InDelta(t, 0.865, computeNewWeight(1.0, "decay", cfg), 1e-9)
	assert.InDelta(t, 2.8, computeNewWeight(3.0, "boost", cfg), 1e-9)
	assert.InDelta(t, 0.19, computeNewWeight(0.1, "decay", cfg), 1e-9)
	assert.InDelta(t, 2.8, computeNewWeight(3.0, "none", cfg), 1e-9)
}

func TestDedupeParseIDs(t *testing.T) {
	got := dedupeParseIDs([]string{"1", "1", "2", "abc", " 3 ", "3"})
	assert.Equal(t, []uint64{1, 2, 3}, got)
}
