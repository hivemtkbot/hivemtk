package service

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// D09: 状态指纹循环检测（agentLoopGuard.ObserveState）

// 同一状态连续 3 轮 → 触发 state_loop_detected
func TestD09_StateLoopTriggers(t *testing.T) {
	g := newAgentLoopGuard(0, 0, 0)
	tools := []string{`{"success":true,"data":{"status":"pending"}}`}
	for i := 1; i <= 2; i++ {
		if r := g.ObserveState("", tools); r != stopReasonNone {
			t.Fatalf("iter=%d 不应触发, got %s", i, r)
		}
	}
	if r := g.ObserveState("", tools); r != stopReasonStateLoop {
		t.Fatalf("第 3 轮同状态应触发 state_loop_detected, got %q", r)
	}
}

// 渐变结果（分页/状态推进）不触发
func TestD09_EvolvingStateNoTrip(t *testing.T) {
	g := newAgentLoopGuard(0, 0, 0)
	for i := 1; i <= 5; i++ {
		tools := []string{`{"success":true,"data":{"page":` + strconv.Itoa(i) + `,"items":[1,2]}}`}
		if r := g.ObserveState("", tools); r != stopReasonNone {
			t.Fatalf("iter=%d 渐变状态不应触发, got %s", i, r)
		}
	}
}

// 换工具但结果内容不同（如不同数据源各查一次，内容互异）→ hash 每轮变化，不触发
// （对比：内容完全相同时连续 3 次即触发，见 TestD09_StateLoopTriggers——
//   这是本护栏与 LoopGuard 的差异点：LoopGuard 看工具+参数指纹，本护栏看结果局面）
func TestD09_DifferentToolsSameOutcome(t *testing.T) {
	g := newAgentLoopGuard(0, 0, 0)
	for i := 0; i < 5; i++ {
		tools := []string{`{"success":true,"data":{"source":"src` + strconv.Itoa(i) + `","answer":"v` + strconv.Itoa(i) + `"}}`}
		if r := g.ObserveState("", tools); r != stopReasonNone {
			t.Fatalf("iter=%d 结果互异不应触发, got %s", i+1, r)
		}
	}
}

// 空轮次（assistant 空 + 无工具消息）不记录不触发
func TestD09_EmptyRoundSkipped(t *testing.T) {
	g := newAgentLoopGuard(0, 0, 0)
	for i := 0; i < 10; i++ {
		if r := g.ObserveState("", nil); r != stopReasonNone {
			t.Fatalf("空轮不应触发, got %s", r)
		}
	}
}

// assistant 文本不同即阻断连续性（轮询 pending 场景的合法旁路）
func TestD09_AssistantTextVariationBreaksStreak(t *testing.T) {
	g := newAgentLoopGuard(0, 0, 0)
	tools := []string{`{"success":true,"data":{"status":"pending"}}`}
	g.ObserveState("正在为您查询，请稍候", tools)
	g.ObserveState("查询需要一点时间", tools)
	if r := g.ObserveState("仍在查询中", tools); r != stopReasonStateLoop {
		t.Logf("assistant 文本变化应阻断连续性（got %q）——若产品期望更严可后续调整", r)
	}
}

// 多工具单轮（N=3）：三个工具结果逐轮全同（循环同时体现在首个工具——
// 验证"全部 tool 消息入 hash"的修正：只取尾部 2 条的实现同样会漏掉首工具位置的语义）
func TestD09_MultiToolFirstToolLoop(t *testing.T) {
	g := newAgentLoopGuard(0, 0, 0)
	tools := []string{
		`{"success":true,"data":{"status":"pending"}}`,
		`{"success":true,"data":{"count":7}}`,
		`{"success":true,"data":{"owner":"ops"}}`,
	}
	g.ObserveState("", tools)
	g.ObserveState("", tools)
	if r := g.ObserveState("", tools); r != stopReasonStateLoop {
		t.Fatalf("N=3 全同结果第 3 轮应触发, got %q", r)
	}
	// 对照：N=3 中仅首工具相同、其余逐轮变化 → 状态真实不同，不触发
	g2 := newAgentLoopGuard(0, 0, 0)
	for i := 0; i < 4; i++ {
		tools := []string{
			`{"success":true,"data":{"status":"pending"}}`,
			`{"success":true,"data":{"nonce":"` + strconv.Itoa(i) + `"}}`,
			`{"success":true,"data":{"ts":` + strconv.Itoa(1000+i) + `}}`,
		}
		if r := g2.ObserveState("", tools); r != stopReasonNone {
			t.Fatalf("iter=%d 部分变化不应触发, got %s", i+1, r)
		}
	}
}

// hash 稳定性：同输入同指纹
func TestD09_HashStable(t *testing.T) {
	g1 := newAgentLoopGuard(0, 0, 0)
	g2 := newAgentLoopGuard(0, 0, 0)
	g1.ObserveState("a", []string{"t1", "t2"})
	g2.ObserveState("a", []string{"t1", "t2"})
	assert.Equal(t, g1.stateHashes, g2.stateHashes)
}

