package service

import (
	"marketing/internal/content/model"
	"testing"
)

func TestCalculateRelevance_Empty(t *testing.T) {
	s := &ScriptTemplateService{}
	tmpl := &model.ScriptTemplate{Title: "", Content: ""}
	if score := s.calculateRelevance("hello", tmpl); score != 0 {
		t.Errorf("expected 0 for empty template, got %v", score)
	}
}

func TestCalculateRelevance_NoMatch(t *testing.T) {
	s := &ScriptTemplateService{}
	tmpl := &model.ScriptTemplate{Title: "产品介绍", Content: "我们的产品很好"}
	score := s.calculateRelevance("hello world", tmpl)
	if score < 0 || score > 0.5 {
		t.Errorf("expected low score, got %v", score)
	}
}

func TestCalculateRelevance_Match(t *testing.T) {
	s := &ScriptTemplateService{}
	// 关键词 "价格" 在消息中出现，应该至少 0.3
	tmpl := &model.ScriptTemplate{Title: "t", Content: "价格 优惠"}
	score := s.calculateRelevance("价格优惠", tmpl)
	if score <= 0.3 {
		t.Errorf("expected score > 0.3, got %v", score)
	}
}

func TestCalculateRelevance_TitleMatch(t *testing.T) {
	s := &ScriptTemplateService{}
	tmpl := &model.ScriptTemplate{Title: "退款", Content: "其他其他其他其他"}
	score := s.calculateRelevance("我想了解退款政策", tmpl)
	// 标题匹配应该加分
	if score < 0.3 {
		t.Errorf("expected title match boost, got %v", score)
	}
}

func TestCalculateRelevance_ShortKeywordsSkipped(t *testing.T) {
	s := &ScriptTemplateService{}
	// 全部短于等于 2 个字符的关键词
	tmpl := &model.ScriptTemplate{Title: "ZZZ", Content: "ab cd ef"}
	score := s.calculateRelevance("ab cd ef ZZZ", tmpl)
	// 短于等于 2 个字符的关键词被忽略，但标题匹配可能加分
	if score > 0.3 {
		t.Errorf("expected <= 0.3 for short keywords, got %v", score)
	}
}
