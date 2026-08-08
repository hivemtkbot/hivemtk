package trace_learning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWeightAction(t *testing.T) {
	cfg := DefaultConfig()
	cases := []struct {
		name   string
		score  int
		bad    bool
		safety float64
		want   string
	}{
		{"低分差", 30, false, 80, "decay"},
		{"LLM标差(即使高分)", 78, true, 90, "decay"},
		{"safety违规强制差", 95, false, 65, "decay"},
		{"高分好", 90, false, 100, "boost"},
		{"中间分不变", 70, false, 100, "none"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, weightAction(c.score, c.bad, c.safety, cfg), c.name)
	}
}

func TestClampWeight(t *testing.T) {
	assert.Equal(t, 0.1, clampWeight(0.01, 0.1, 3.0))
	assert.Equal(t, 3.0, clampWeight(9, 0.1, 3.0))
	assert.Equal(t, 1.0, clampWeight(1.0, 0.1, 3.0))
	assert.Equal(t, 1.5, clampWeight(1.5, 0.1, 3.0))
}

func TestComputeNewWeight_MeanReversion(t *testing.T) {
	cfg := DefaultConfig() // MeanReversion=0.1
	// 好回复：1.0 -> *1.12 = 1.12，再回归 0.1*1.0+0.9*1.12=1.108
	assert.InDelta(t, 1.108, computeNewWeight(1.0, "boost", cfg), 1e-9)
	// 差回复：1.0 -> *0.85 = 0.85，再回归 0.1*1.0+0.9*0.85=0.865
	assert.InDelta(t, 0.865, computeNewWeight(1.0, "decay", cfg), 1e-9)
	// 锚定在 MaxWeight：连续好的 chunk 不会无限上涨，每次都被拉回 1.0 一侧
	// 3.0 *1.12 = 3.0(clamp) -> 回归 0.1+0.9*3.0=2.8
	assert.InDelta(t, 2.8, computeNewWeight(3.0, "boost", cfg), 1e-9)
	// 锚定在 MinWeight：0.1 *0.85 = 0.085 -> clamp 0.1 -> 回归 0.1+0.9*0.1=0.19
	assert.InDelta(t, 0.19, computeNewWeight(0.1, "decay", cfg), 1e-9)
	// none 动作：不偏离（仅在 1.0 时不变；非 1.0 也只回归不变动作）
	assert.InDelta(t, 2.8, computeNewWeight(3.0, "none", cfg), 1e-9)
}

func TestDedupeParseIDs(t *testing.T) {
	got := dedupeParseIDs([]string{"1", "1", "2", "abc", " 3 ", "3"})
	assert.Equal(t, []uint64{1, 2, 3}, got)
}
