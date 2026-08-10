package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupIntentSOPSBTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.SOPAgent{},
		&model.SOPExecution{},
		&model.IntentRecord{},
	)
}

func TestIntentSOP_SetSOPService(t *testing.T) {
	rec := NewIntentRecognizer(nil, nil, nil)
	if rec.sopService != nil {
		t.Error("expected nil initially")
	}
	rec.SetSOPService(context.Background(), &SOPService{})
	if rec.sopService == nil {
		t.Error("expected non-nil after SetSOPService")
	}
}

func TestIntentSOP_TriggerSOPByIntent_NoService(t *testing.T) {
	rec := NewIntentRecognizer(nil, nil, nil)
	// 不应 panic
	rec.triggerSOPByIntent(context.Background(), "c-1", "s-1", "objection_price", 0.9)
}

func TestIntentSOP_TriggerSOPByIntent_LowConfidence(t *testing.T) {
	db := setupIntentSOPSBTestDB(t)
	svc := NewSOPService(db, nil)
	rec := NewIntentRecognizer(db, nil, nil)
	rec.SetSOPService(context.Background(), svc)

	// 创建匹配的 SOP
	agent := &model.SOPAgent{
		Name:          "price-objection",
		Scenario:      "objection",
		TriggerType:   SOPTriggerIntent,
		TriggerConfig: model.JSONMap{"intents": []any{"objection_price"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 置信度 < 0.7 不触发
	rec.triggerSOPByIntent(context.Background(), "c-1", "s-1", "objection_price", 0.5)
	var count int64
	db.Model(&model.SOPExecution{}).Where("sop_id = ?", agent.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 executions due to low confidence, got %d", count)
	}
}

func TestIntentSOP_TriggerSOPByIntent_MatchAndExecute(t *testing.T) {
	db := setupIntentSOPSBTestDB(t)
	svc := NewSOPService(db, nil)
	rec := NewIntentRecognizer(db, nil, nil)
	rec.SetSOPService(context.Background(), svc)

	agent := &model.SOPAgent{
		Name:          "price-objection",
		Scenario:      "objection",
		TriggerType:   SOPTriggerIntent,
		TriggerConfig: model.JSONMap{"intents": []any{"objection_price"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	rec.triggerSOPByIntent(context.Background(), "c-1", "s-1", "objection_price", 0.95)
	var execs []model.SOPExecution
	db.Where("sop_id = ?", agent.ID).Find(&execs)
	if len(execs) != 1 {
		t.Errorf("expected 1 execution, got %d", len(execs))
	}
	if execs[0].CustomerID != "c-1" {
		t.Errorf("expected customer c-1, got %s", execs[0].CustomerID)
	}
}

func TestIntentSOP_TriggerSOPByIntent_InactiveAgent(t *testing.T) {
	db := setupIntentSOPSBTestDB(t)
	svc := NewSOPService(db, nil)
	rec := NewIntentRecognizer(db, nil, nil)
	rec.SetSOPService(context.Background(), svc)

	agent := &model.SOPAgent{
		Name:          "price-objection",
		Scenario:      "objection",
		TriggerType:   SOPTriggerIntent,
		TriggerConfig: model.JSONMap{"intents": []any{"objection_price"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	// 由于 model IsActive 默认值是 true，强制更新为 false 模拟停用
	if err := db.Model(agent).Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	rec.triggerSOPByIntent(context.Background(), "c-1", "s-1", "objection_price", 0.95)
	var count int64
	db.Model(&model.SOPExecution{}).Where("sop_id = ?", agent.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 executions for inactive agent, got %d", count)
	}
}

func TestIntentSOP_TriggerSOPByIntent_DuplicateGuard(t *testing.T) {
	db := setupIntentSOPSBTestDB(t)
	svc := NewSOPService(db, nil)
	rec := NewIntentRecognizer(db, nil, nil)
	rec.SetSOPService(context.Background(), svc)

	agent := &model.SOPAgent{
		Name:          "price-objection",
		Scenario:      "objection",
		TriggerType:   SOPTriggerIntent,
		TriggerConfig: model.JSONMap{"intents": []any{"objection_price"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	rec.triggerSOPByIntent(context.Background(), "c-1", "s-1", "objection_price", 0.95)
	rec.triggerSOPByIntent(context.Background(), "c-1", "s-1", "objection_price", 0.95)

	var count int64
	db.Model(&model.SOPExecution{}).
		Where("sop_id = ? AND customer_id = ? AND status = ?", agent.ID, "c-1", SOPStatusRunning).
		Count(&count)
	if count != 1 {
		t.Errorf("expected 1 running execution due to dedup, got %d", count)
	}
}

func TestIntentSOP_TriggerSOPByIntent_NoMatch(t *testing.T) {
	db := setupIntentSOPSBTestDB(t)
	svc := NewSOPService(db, nil)
	rec := NewIntentRecognizer(db, nil, nil)
	rec.SetSOPService(context.Background(), svc)

	agent := &model.SOPAgent{
		Name:          "price-objection",
		Scenario:      "objection",
		TriggerType:   SOPTriggerIntent,
		TriggerConfig: model.JSONMap{"intents": []any{"objection_price"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	rec.triggerSOPByIntent(context.Background(), "c-1", "s-1", "unknown_intent", 0.95)
	var count int64
	db.Model(&model.SOPExecution{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 executions for no match, got %d", count)
	}
}

func TestIntentSOP_Recognize_EmptyCustomer(t *testing.T) {
	db := setupIntentSOPSBTestDB(t)
	svc := NewSOPService(db, nil)
	rec := NewIntentRecognizer(db, nil, nil)
	rec.SetSOPService(context.Background(), svc)

	agent := &model.SOPAgent{
		Name:          "test",
		Scenario:      "test",
		TriggerType:   SOPTriggerIntent,
		TriggerConfig: model.JSONMap{"intents": []any{"price_inquiry"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 空 customerID 不应触发 SOP
	rec.Recognize(context.Background(), "s-1", "", "这个多少钱？")

	var count int64
	db.Model(&model.SOPExecution{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 executions for empty customer, got %d", count)
	}
}

func TestIntentSOP_Recognize_TriggersSOP(t *testing.T) {
	db := setupIntentSOPSBTestDB(t)
	svc := NewSOPService(db, nil)
	rec := NewIntentRecognizer(db, nil, nil)
	rec.SetSOPService(context.Background(), svc)

	agent := &model.SOPAgent{
		Name:          "price-inquiry-sop",
		Scenario:      "inquiry",
		TriggerType:   SOPTriggerIntent,
		TriggerConfig: model.JSONMap{"intents": []any{"price_inquiry"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	r, err := rec.Recognize(context.Background(), "s-1", "c-1", "这个多少钱？")
	if err != nil {
		t.Fatalf("recognize: %v", err)
	}
	if r.IntentType != IntentPriceInquiry {
		t.Errorf("expected price_inquiry, got %s", r.IntentType)
	}

	var count int64
	db.Model(&model.SOPExecution{}).Where("sop_id = ? AND customer_id = ?", agent.ID, "c-1").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 execution triggered by Recognize, got %d", count)
	}
}
