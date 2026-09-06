package service

import (
	"context"
	"testing"
)

// TestClassify_ConfidenceGrading 置信度按规则命中证据分级：
// 命中该类别关键词数 >= 2 → 0.90；恰好 1 个 → 0.70；兜底 other → 0.40
func TestClassify_ConfidenceGrading(t *testing.T) {
	s := &ObjectionHandlerService{}
	ctx := context.Background()

	tests := []struct {
		name           string
		text           string
		wantCategory   ObjectionCategory
		wantConfidence float64
	}{
		{"多关键词命中价格类", "太贵了，能不能便宜点给个折扣？", ObjectionPrice, 0.90},
		{"单关键词命中价格类", "这个有点贵", ObjectionPrice, 0.70},
		{"单关键词命中时机类", "我考虑一下", ObjectionTiming, 0.70},
		{"单关键词命中比较类", "别家好像更便宜哦", ObjectionCompare, 0.70},
		{"无命中兜底 other", "今天天气怎么样", ObjectionOther, 0.40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, _, conf := s.classifyWithConfidence(ctx, tt.text)
			if cat != tt.wantCategory {
				t.Errorf("category = %s, want %s", cat, tt.wantCategory)
			}
			if conf != tt.wantConfidence {
				t.Errorf("confidence = %v, want %v", conf, tt.wantConfidence)
			}
		})
	}
}

// TestClassify_SignatureUnchanged 对外 Classify 签名保持二元返回（controller 依赖）
func TestClassify_SignatureUnchanged(t *testing.T) {
	s := &ObjectionHandlerService{}
	cat, name := s.Classify(context.Background(), "太贵了，能便宜点吗")
	if cat != ObjectionPrice {
		t.Errorf("category = %s, want price", cat)
	}
	if name != "价格异议" {
		t.Errorf("name = %s, want 价格异议", name)
	}
}

// TestHandle_ConfidenceInResponse Handle 响应中 Confidence 不再硬编码 0.85
func TestHandle_ConfidenceInResponse(t *testing.T) {
	s := &ObjectionHandlerService{scriptRepo: nil}
	resp, err := s.Handle(context.Background(), HandleRequest{Text: "太贵了，能不能便宜点、打个折扣"})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if resp.Confidence == 0.85 {
		t.Error("confidence should not be hardcoded 0.85 anymore")
	}
	if resp.Category != ObjectionPrice {
		t.Errorf("category = %s, want price", resp.Category)
	}
	if resp.Confidence != 0.90 {
		t.Errorf("confidence = %v, want 0.90 (multi keyword hit)", resp.Confidence)
	}
}
