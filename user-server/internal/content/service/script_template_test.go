package service

import (
	"hivemtk-user/internal/content/model"
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
	if score < 0.3 {
		t.Errorf("expected title match boost, got %v", score)
	}
}

func TestCalculateRelevance_ShortKeywordsSkipped(t *testing.T) {
	s := &ScriptTemplateService{}
	tmpl := &model.ScriptTemplate{Title: "ZZZ", Content: "ab cd ef"}
	score := s.calculateRelevance("ab cd ef ZZZ", tmpl)
	if score > 0.3 {
		t.Errorf("expected <= 0.3 for short keywords, got %v", score)
	}
}

