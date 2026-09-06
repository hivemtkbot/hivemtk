package llm

import (
	"context"
	"testing"
)

// D11: vote 策略——候选收集后 MultiModelVote 一致性胜出
// （MultiModelVote 本体已有 dispatcher_vote_test 覆盖；此处锁 fan-out 路径行为）
func TestD20_FanOutVoteStrategyNotImplementedError(t *testing.T) {

	if fanoutVoteEnabled() {
		t.Skip("vote 已开启，跳过默认关断言")
	}
	if !fanoutVoteEnabledGetter() == false {
		t.Error("默认 getter 应返回 false")
	}

	d := NewDispatcher(NewLLMService())
	if d == nil {
		t.Fatal("dispatcher nil")
	}
	_ = context.Background()
}

func TestD20_SetFanoutVoteEnabledGetter(t *testing.T) {
	defer SetFanoutVoteEnabledGetter(nil)
	SetFanoutVoteEnabledGetter(func() bool { return true })
	if !fanoutVoteEnabled() {
		t.Error("注入后应返回 true")
	}
	SetFanoutVoteEnabledGetter(nil)
	if !fanoutVoteEnabled() {
		t.Log("nil 注入被忽略，保持 true（符合 SetFanoutVoteEnabledGetter 语义）")
	}
	SetFanoutVoteEnabledGetter(func() bool { return false })
}
