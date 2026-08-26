package service

import (
	"context"
	"testing"
)

// --- T-3 status_quo 第五类异议 ---

func TestClassify_StatusQuo(t *testing.T) {
	s := &ObjectionHandlerService{}
	ctx := context.Background()

	tests := []struct {
		name         string
		text         string
		wantCategory ObjectionCategory
	}{
		{"再想想", "我再想想吧", ObjectionStatusQuo},
		{"暂时不用", "暂时不用，谢谢", ObjectionStatusQuo},
		{"目前挺好", "目前挺好，不想换", ObjectionStatusQuo},
		{"挺好的", "现在用的挺好的", ObjectionStatusQuo},
		{"不需要了", "不需要了", ObjectionStatusQuo},
		{"维持现状", "就想维持现状", ObjectionStatusQuo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, _, conf := s.classifyWithConfidence(ctx, tt.text)
			if cat != tt.wantCategory {
				t.Errorf("category = %s, want %s", cat, tt.wantCategory)
			}
			if conf != confidenceSingleHit {
				t.Errorf("confidence = %v, want %v", conf, confidenceSingleHit)
			}
		})
	}
}

func TestClassify_StatusQuoNotShadowing(t *testing.T) {
	s := &ObjectionHandlerService{}
	ctx := context.Background()

	// 反例：status_quo 不应吞掉其他类别
	tests := []struct {
		name         string
		text         string
		wantCategory ObjectionCategory
	}{
		{"考虑一下仍是时机类", "我考虑一下再答复你", ObjectionTiming},
		{"太贵仍是价格类", "太贵了，便宜点", ObjectionPrice},
		{"用不上不带『了』仍是需求类", "这个我用不上", ObjectionNeed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, _ := s.Classify(ctx, tt.text)
			if cat != tt.wantCategory {
				t.Errorf("category = %s, want %s", cat, tt.wantCategory)
			}
		})
	}
}

func TestDefaultSuggestion_StatusQuo(t *testing.T) {
	s := &ObjectionHandlerService{}
	got := s.defaultSuggestion(context.Background(), ObjectionStatusQuo, "暂时不用")
	if got == "" {
		t.Error("status_quo 兜底文案不应为空")
	}
}

func TestListCategories_IncludesStatusQuo(t *testing.T) {
	s := &ObjectionHandlerService{}
	cats := s.ListCategories(context.Background())
	found := false
	for _, c := range cats {
		if c["category"] == string(ObjectionStatusQuo) {
			found = true
		}
	}
	if !found {
		t.Error("ListCategories 应包含 status_quo")
	}
}

// --- T-4 LAER Acknowledge + Explore ---

func TestPickAcknowledge_StableByTemplateID(t *testing.T) {
	// 同一模板 ID → 同一选择（稳定伪随机）
	a := pickAcknowledge(ObjectionPrice, 42, "太贵了")
	b := pickAcknowledge(ObjectionPrice, 42, "完全不同的文本")
	if a != b || a == "" {
		t.Errorf("same template id should yield stable ack: %q vs %q", a, b)
	}

	// 无模板 ID 时按文本哈希稳定
	c1 := pickAcknowledge(ObjectionTrust, 0, "怕是骗子")
	c2 := pickAcknowledge(ObjectionTrust, 0, "怕是骗子")
	if c1 != c2 || c1 == "" {
		t.Errorf("text-seeded ack should be stable: %q vs %q", c1, c2)
	}

	// 选择必须落在模板集合内，且不同种子应能覆盖多个模板（伪随机分布）
	valid := map[string]bool{}
	for _, tpl := range acknowledgeTemplates[ObjectionTiming] {
		valid[tpl] = true
	}
	seen := map[string]bool{}
	for id := uint(1); id <= 60; id++ {
		got := pickAcknowledge(ObjectionTiming, id, "下次吧")
		if !valid[got] {
			t.Fatalf("picked ack not in template set: %q", got)
		}
		seen[got] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected pseudo-random spread across templates, got %d distinct", len(seen))
	}
}

func TestPickAcknowledge_OtherCategoryEmpty(t *testing.T) {
	if got := pickAcknowledge(ObjectionOther, 7, "随便说说"); got != "" {
		t.Errorf("other 兜底类别不应有 Acknowledge，got %q", got)
	}
}

func TestHandle_AcknowledgeAndClarify(t *testing.T) {
	s := &ObjectionHandlerService{scriptRepo: nil}

	// price：有 Acknowledge + Explore 澄清问题
	resp, err := s.Handle(context.Background(), HandleRequest{Text: "太贵了，能便宜点吗"})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if resp.Acknowledge == "" {
		t.Error("price 异议应有 Acknowledge 前缀")
	}
	if resp.Clarify == "" {
		t.Error("price 高价值异议应有澄清问题（Explore）")
	}

	// trust：同样双开
	resp2, err := s.Handle(context.Background(), HandleRequest{Text: "你们不会是骗子吧"})
	if err != nil {
		t.Fatalf("handle(trust): %v", err)
	}
	if resp2.Acknowledge == "" || resp2.Clarify == "" {
		t.Error("trust 异议应有 Acknowledge 与澄清问题")
	}

	// timing：有 Acknowledge、无澄清
	resp3, err := s.Handle(context.Background(), HandleRequest{Text: "我考虑一下"})
	if err != nil {
		t.Fatalf("handle(timing): %v", err)
	}
	if resp3.Acknowledge == "" {
		t.Error("timing 异议应有 Acknowledge")
	}
	if resp3.Clarify != "" {
		t.Errorf("timing 非高价值类别不应有澄清问题，got %q", resp3.Clarify)
	}
}
