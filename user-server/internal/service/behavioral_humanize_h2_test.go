package service

import (
	"testing"

	"hivemtk-user/internal/service/humanize/behavioral"
)

// --- H-2 短回复分条豁免 ---

func TestBuildWithScene_ShortReplyNoSplit(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	b.SetEnabled(true)
	short := "好的，这个问题我马上帮您核实处理，请稍等一下。"
	if len([]rune(short)) > noSplitMaxChars {
		t.Fatalf("fixture 应 ≤ %d 字符", noSplitMaxChars)
	}
	plan := b.BuildWithScene(short, true, SceneSupport)
	if len(plan.Messages) != 1 {
		t.Errorf("≤%d 字符不应分条，got %d messages", noSplitMaxChars, len(plan.Messages))
	}
}

func TestBuildWithScene_LongReplySplits(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	b.SetEnabled(true)
	long := "您好！我是智能助手，可以帮您处理订单查询、物流跟踪、退款申请、售后服务等常见问题。请您简要描述下您当前遇到的具体情况。我会立即为您查询相关数据并提供详细的解决方案，祝您生活愉快！"
	if len([]rune(long)) <= noSplitMaxChars {
		t.Fatal("fixture 应超过豁免阈值")
	}
	plan := b.BuildWithScene(long, true, SceneSupport)
	if len(plan.Messages) < 2 {
		t.Error("超过 40 字符的长文本应分条")
	}
}

// --- H-2 动态延迟公式 ---

func TestDynamicTotalDelay_Formula(t *testing.T) {
	cfg := behavioral.DefaultBehaviorConfig()

	tests := []struct {
		name      string
		chars     int
		scene     HumanizeScene
		isFirst   bool
		wantDelay float64
	}{
		{"客服 20 字符首条", 20, SceneSupport, true, 2.0 + 1*0.8},
		{"客服 40 字符首条", 40, SceneSupport, true, 2.0 + 2*0.8},
		{"客服 41 字符进位", 41, SceneSupport, true, 2.0 + 3*0.8},
		{"销售 40 字符首条 ×1.5", 40, SceneSales, true, (2.0 + 2*0.8) * 1.5},
		{"客服 40 字符非首条加思考停顿", 40, SceneSupport, false, 2.0 + 2*0.8 + cfg.ThinkingPauseSec},
		{"销售 100 字符非首条", 100, SceneSales, false, (2.0 + 5*0.8) * 1.5 + cfg.ThinkingPauseSec},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dynamicTotalDelay(tt.chars, tt.scene, tt.isFirst, cfg)
			if diff := got - tt.wantDelay; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("delay = %v, want %v", got, tt.wantDelay)
			}
		})
	}
}

func TestBuildWithScene_TotalDelayOverridden(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	b.SetEnabled(true)
	long := "您好！我是智能助手，可以帮您处理订单查询、物流跟踪、退款申请、售后服务等常见问题。请您简要描述下您当前遇到的具体情况。我会立即为您查询相关数据并提供详细的解决方案，祝您生活愉快！"

	plan := b.BuildWithScene(long, true, SceneSupport)
	n := len([]rune(long))
	want := dynamicTotalDelay(n, SceneSupport, true, behavioral.DefaultBehaviorConfig())
	if plan.TotalDelay != want {
		t.Errorf("TotalDelay = %v, want dynamic formula value %v", plan.TotalDelay, want)
	}
}

func TestSetScene_DefaultSupportAndEmptyFallback(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	if b.sceneOrDefault() != SceneSupport {
		t.Error("默认场景应为 support")
	}
	b.SetScene(SceneSales)
	if b.sceneOrDefault() != SceneSales {
		t.Error("SetScene(sales) 未生效")
	}
	b.SetScene("")
	if b.sceneOrDefault() != SceneSupport {
		t.Error("空场景应回退 support")
	}
	var nb *BehavioralPlanBuilder
	if nb.sceneOrDefault() != SceneSupport {
		t.Error("nil builder 场景应为 support")
	}
	nb.SetScene(SceneSales) // 不应 panic
}

func TestBuild_DisabledUnchangedByH2(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	plan := b.Build("短文本", true)
	if len(plan.Messages) != 1 || plan.TotalDelay != 0 {
		t.Errorf("禁用时应返回 trivial plan，got %+v", plan)
	}
}
