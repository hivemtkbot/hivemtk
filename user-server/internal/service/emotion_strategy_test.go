package service

import (
	"strings"
	"testing"
)

func TestClassifyEmotion_PriorityAngerOverAnxiety(t *testing.T) {
	cases := []struct {
		content string
		want    EmotionType
	}{
		{"我要投诉你们", EmotionAnger},
		{"你们是骗子吧", EmotionAnger},
		{"马上退款，不然315曝光", EmotionAnger},
		{"这个东西真的很紧急，快点帮我处理", EmotionAnxiety},
		{"我着急啊，等了三天了还没发货", EmotionAnxiety},
		{"用了很不错，五星好评", EmotionSatisfied},
		{"太好了，正是我需要的", EmotionSatisfied},
		{"你好，请问价格多少", EmotionNeutral},
	}
	for _, c := range cases {
		if got := ClassifyEmotion(c.content); got != c.want {
			t.Errorf("ClassifyEmotion(%q) = %s, want %s", c.content, got, c.want)
		}
	}
}

func TestClassifyEmotion_NegationWindow(t *testing.T) {

	if got := ClassifyEmotion("算了，不用退款了，就留着吧"); got != EmotionNeutral {
		t.Errorf("否定退款不应触发愤怒层, got %s", got)
	}
	if got := ClassifyEmotion("不用着急慢慢来"); got == EmotionAnxiety {
		t.Errorf("否定语境不应触发焦虑层, got %s", got)
	}
}

func TestStrategyForEmotion_LayeredResponse(t *testing.T) {

	as := StrategyForEmotion(EmotionAnger)
	if !as.TransferToHuman {
		t.Error("愤怒层必须转人工")
	}
	if as.TransferReason == "" || !strings.Contains(as.TransferReason, "补偿") {
		t.Errorf("愤怒层应携带补偿语义, got %q", as.TransferReason)
	}

	ax := StrategyForEmotion(EmotionAnxiety)
	if ax.TransferToHuman {
		t.Error("焦虑层不应盲转人工（进度可视化策略）")
	}
	if !strings.Contains(ax.ReplyHint, "进度") {
		t.Errorf("焦虑层提示应含进度可视化语义, got %q", ax.ReplyHint)
	}

	st := StrategyForEmotion(EmotionSatisfied)
	if st.TransferToHuman {
		t.Error("满意层不转人工")
	}
	if st.ReplyHint == "" {
		t.Error("满意层应有裂变引导提示")
	}

	if s := StrategyForEmotion(EmotionNeutral); s != (EmotionStrategy{}) {
		t.Errorf("中性层应为零值策略, got %+v", s)
	}
}

func TestEmotionKeywords_CoverLegacyUrgentSet(t *testing.T) {

	for _, kw := range NLPKeywords.Urgent {
		got := ClassifyEmotion(kw)
		if got != EmotionAnger && got != EmotionAnxiety {
			t.Errorf("旧紧急词 %q 分类为 %s, 必须为 anger/anxiety 之一", kw, got)
		}
	}
}

func TestBuildPrompt_EmotionHintInjected(t *testing.T) {
	e := &SalesEngine{}
	req := &SalesRequest{
		UserMessage: "我的快递怎么还没到，急死了",
		EmotionHint: StrategyForEmotion(EmotionAnxiety).ReplyHint,
	}
	prompt := e.buildPrompt(req, nil, nil, nil, "default", nil, nil, nil)
	if !strings.Contains(prompt, "【情绪应对策略】") {
		t.Error("prompt 应注入情绪应对策略段")
	}
	if !strings.Contains(prompt, "预计完成时间") {
		t.Error("prompt 应包含焦虑层的进度可视化指令")
	}

	plain := e.buildPrompt(&SalesRequest{UserMessage: "你好"}, nil, nil, nil, "default", nil, nil, nil)
	if strings.Contains(plain, "【情绪应对策略】") {
		t.Error("无情绪干预时不应注入策略段")
	}
}
