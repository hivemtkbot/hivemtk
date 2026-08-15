package model

import (
	"testing"
	"time"
)


func TestFAQEntry_TableName(t *testing.T) {
	entry := &FAQEntry{}
	if got := entry.TableName(); got != "faq_entries" {
		t.Errorf("TableName() = %q, want %q", got, "faq_entries")
	}
}

func TestFAQEntry_BasicFields(t *testing.T) {
	now := time.Now()
	trueVal := true
	entry := &FAQEntry{
		ID:         1,
		Question:   "韵达发货吗",
		Answer:     "韵达不发的哦",
		Category:   "logistics",
		Intent:     "logistics",
		Confidence: 0.85,
		HitCount:   10,
		Enabled:    &trueVal,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if entry.Question != "韵达发货吗" {
		t.Errorf("Question = %q, want %q", entry.Question, "韵达发货吗")
	}
	if entry.Confidence != 0.85 {
		t.Errorf("Confidence = %f, want 0.85", entry.Confidence)
	}
	if entry.Enabled == nil || !*entry.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestSOPTemplate_TableName(t *testing.T) {
	tpl := &SOPTemplate{}
	if got := tpl.TableName(); got != "sop_templates" {
		t.Errorf("TableName() = %q, want %q", got, "sop_templates")
	}
}

func TestSOPTemplate_BasicFields(t *testing.T) {
	now := time.Now()
	trueVal := true
	tpl := &SOPTemplate{
		ID:         1,
		Name:       "韵达不发标准回复",
		Intent:     "logistics",
		Stage:      "initial",
		Template:   "亲，{{.ProductName}} 发 {{.ExpressCompany}} 哦，{{.Note}}",
		Vars:       `{"ProductName":{"desc":"商品名称","example":"核桃"},"ExpressCompany":{"desc":"快递公司","example":"邮政"},"Note":{"desc":"备注","example":"包邮"}}`,
		Priority:   10,
		Confidence: 0.9,
		Enabled:    &trueVal,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if tpl.Intent != "logistics" {
		t.Errorf("Intent = %q, want %q", tpl.Intent, "logistics")
	}
	if tpl.Priority != 10 {
		t.Errorf("Priority = %d, want 10", tpl.Priority)
	}
}

func TestLayerDecisionLog_TableName(t *testing.T) {
	log := &LayerDecisionLog{}
	if got := log.TableName(); got != "layer_decision_logs" {
		t.Errorf("TableName() = %q, want %q", got, "layer_decision_logs")
	}
}

func TestLayerDecisionLog_BasicFields(t *testing.T) {
	now := time.Now()
	trueVal := true
	log := &LayerDecisionLog{
		ID:         1,
		TraceID:    "trace_2026_07_31_abc123",
		SessionID:  "sess_xyz",
		CustomerID: "cust_001",
		Layer:      "layer1",
		Reason:     "faq_match",
		Intent:     "logistics",
		ConfIn:     0.6,
		ConfOut:    0.9,
		WallMs:     15,
		LLMSkipped: &trueVal,
		Extra:      `{"faq_id":42,"matched_keyword":"韵达"}`,
		CreatedAt:  now,
	}
	if log.Layer != "layer1" {
		t.Errorf("Layer = %q, want %q", log.Layer, "layer1")
	}
	if log.LLMSkipped == nil || !*log.LLMSkipped {
		t.Error("LLMSkipped should be true (Layer1 FAQ hit)")
	}
	if log.WallMs != 15 {
		t.Errorf("WallMs = %d, want 15", log.WallMs)
	}
}

