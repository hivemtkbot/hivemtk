package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// setupAutoTaggerTestDB 设置测试数据库
func setupAutoTaggerTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.CustomerTag{},
		&model.Customer{},
		&model.CustomerEvent{},
	)
	db.SetTestDB(database)
	return database
}

// setupAutoTagger 设置测试 AutoTagger
func setupAutoTagger(t *testing.T) *AutoTagger {
	setupAutoTaggerTestDB(t)
	return NewAutoTagger()
}

// TestNewAutoTagger 测试创建 AutoTagger 实例
func TestNewAutoTagger(t *testing.T) {
	tagger := NewAutoTagger()
	if tagger == nil {
		t.Fatal("Expected AutoTagger instance, got nil")
	}
}

// TestAutoTagger_CreateAutoTag 测试创建自动标签
func TestAutoTagger_CreateAutoTag(t *testing.T) {
	tagger := setupAutoTagger(t)

	rule := map[string]any{
		"type":      "simple",
		"field":     "rfm_score",
		"operator":  "gte",
		"threshold": 50,
	}

	err := tagger.CreateAutoTag("High Value", string(model.TagCategoryBehavioral), rule)
	if err != nil {
		t.Fatalf("CreateAutoTag failed: %v", err)
	}

	// 验证标签已创建
	tags, err := tagger.GetAutoTags()
	if err != nil {
		t.Fatalf("GetAutoTags failed: %v", err)
	}
	if len(tags) != 1 {
		t.Errorf("Expected 1 auto tag, got %d", len(tags))
	}
	if tags[0].Name != "High Value" {
		t.Errorf("Expected tag name 'High Value', got %s", tags[0].Name)
	}
}

// TestAutoTagger_GetAutoTags 测试获取自动标签列表
func TestAutoTagger_GetAutoTags(t *testing.T) {
	tagger := setupAutoTagger(t)

	// 创建多个标签
	rule := map[string]any{"type": "simple", "field": "rfm_score", "operator": "gte", "value": 50}
	tagger.CreateAutoTag("Tag1", string(model.TagCategoryBehavioral), rule)
	tagger.CreateAutoTag("Tag2", string(model.TagCategoryDemographic), rule)

	tags, err := tagger.GetAutoTags()
	if err != nil {
		t.Fatalf("GetAutoTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("Expected 2 auto tags, got %d", len(tags))
	}
}

// TestAutoTagger_evaluateSimpleRule 测试简单规则评估
func TestAutoTagger_evaluateSimpleRule(t *testing.T) {
	tagger := setupAutoTagger(t)

	customerData := map[string]any{
		"rfm_score": 75,
	}

	rule := map[string]any{
		"field":    "rfm_score",
		"operator": "gte",
		"value":    50,
	}

	result := tagger.evaluateSimpleRule(customerData, rule)
	if !result {
		t.Error("Expected rule to match (75 >= 50)")
	}

	// 测试不匹配的情况
	ruleNotMatch := map[string]any{
		"field":    "rfm_score",
		"operator": "gte",
		"value":    100,
	}
	result2 := tagger.evaluateSimpleRule(customerData, ruleNotMatch)
	if result2 {
		t.Error("Expected rule not to match (75 < 100)")
	}
}

// TestAutoTagger_evaluateEventCountRule 测试事件数量规则评估
func TestAutoTagger_evaluateEventCountRule(t *testing.T) {
	tagger := setupAutoTagger(t)

	events := []*model.CustomerEvent{
		{EventType: model.EventTypePageView},
		{EventType: model.EventTypePageView},
		{EventType: model.EventTypePurchase},
	}

	customerData := map[string]any{
		"events": events,
	}

	// 测试页面浏览数量规则
	rule := map[string]any{
		"type":       "event_count",
		"event_type": "page_view",
		"operator":   "gte",
		"threshold":  2,
	}

	result := tagger.evaluateEventCountRule(customerData, rule)
	if !result {
		t.Error("Expected rule to match (2 page_views >= 2)")
	}

	// 测试购买数量规则（不匹配）
	ruleNotMatch := map[string]any{
		"type":       "event_count",
		"event_type": "purchase",
		"operator":   "gte",
		"threshold":  2,
	}
	result2 := tagger.evaluateEventCountRule(customerData, ruleNotMatch)
	if result2 {
		t.Error("Expected rule not to match (1 purchase < 2)")
	}
}

// TestAutoTagger_evaluatePurchaseAmountRule 测试购买金额规则评估
func TestAutoTagger_evaluatePurchaseAmountRule(t *testing.T) {
	tagger := setupAutoTagger(t)

	customerData := map[string]any{
		"total_purchase_amount": 500.00,
	}

	// 测试匹配规则
	rule := map[string]any{
		"type":      "purchase_amount",
		"operator":  "gte",
		"threshold": 300.00,
	}

	result := tagger.evaluatePurchaseAmountRule(customerData, rule)
	if !result {
		t.Error("Expected rule to match (500 >= 300)")
	}

	// 测试不匹配规则
	ruleNotMatch := map[string]any{
		"type":      "purchase_amount",
		"operator":  "gte",
		"threshold": 1000.00,
	}
	result2 := tagger.evaluatePurchaseAmountRule(customerData, ruleNotMatch)
	if result2 {
		t.Error("Expected rule not to match (500 < 1000)")
	}
}

// TestAutoTagger_evaluateDaysSinceRule 测试距离天数规则评估
func TestAutoTagger_evaluateDaysSinceRule(t *testing.T) {
	tagger := setupAutoTagger(t)

	customerData := map[string]any{
		"days_since_last_purchase": 45,
	}

	// 测试匹配规则
	rule := map[string]any{
		"type":      "days_since",
		"field":     "days_since_last_purchase",
		"operator":  "gte",
		"threshold": 30,
	}

	result := tagger.evaluateDaysSinceRule(customerData, rule)
	if !result {
		t.Error("Expected rule to match (45 >= 30)")
	}
}

// TestAutoTagger_evaluateCustomRule_AND 测试自定义规则（AND 逻辑）
func TestAutoTagger_evaluateCustomRule_AND(t *testing.T) {
	tagger := setupAutoTagger(t)

	customerData := map[string]any{
		"rfm_score":             80,
		"total_purchase_amount": 1000.00,
	}

	rule := map[string]any{
		"type":  "custom",
		"logic": "AND",
		"conditions": []any{
			map[string]any{
				"field": "rfm_score", "operator": "gte", "value": 50.0,
			},
			map[string]any{
				"field": "total_purchase_amount", "operator": "gte", "value": 500.0,
			},
		},
	}

	result := tagger.evaluateCustomRule(customerData, rule)
	if !result {
		t.Error("Expected AND rule to match (all conditions met)")
	}
}

// TestAutoTagger_evaluateCustomRule_OR 测试自定义规则（OR 逻辑）
func TestAutoTagger_evaluateCustomRule_OR(t *testing.T) {
	tagger := setupAutoTagger(t)

	customerData := map[string]any{
		"rfm_score": 30,
	}

	rule := map[string]any{
		"type":  "custom",
		"logic": "OR",
		"conditions": []any{
			map[string]any{
				"field": "rfm_score", "operator": "gte", "value": 50.0,
			},
			map[string]any{
				"field": "rfm_score", "operator": "lte", "value": 40.0,
			},
		},
	}

	result := tagger.evaluateCustomRule(customerData, rule)
	if !result {
		t.Error("Expected OR rule to match (one condition met)")
	}
}

// TestAutoTagger_buildCustomerDataSnapshot 测试构建客户数据快照
func TestAutoTagger_buildCustomerDataSnapshot(t *testing.T) {
	tagger := setupAutoTagger(t)

	customer := &model.Customer{
		ID:        "test-customer",
		RFMScore:  75,
		ChurnRisk: "low",
		Tags:      "[]",
	}

	events := []*model.CustomerEvent{
		{
			EventType:  model.EventTypePurchase,
			OccurredAt: tagger.getTimeDaysAgo(5),
		},
	}
	// 设置事件数据
	eventData := map[string]any{"amount": 100.00}
	events[0].SetEventData(eventData)

	snapshot := tagger.buildCustomerDataSnapshot(customer, events)

	if snapshot["rfm_score"] != 75 {
		t.Errorf("Expected rfm_score 75, got %v", snapshot["rfm_score"])
	}

	totalAmount, _ := snapshot["total_purchase_amount"].(float64)
	if totalAmount != 100.00 {
		t.Errorf("Expected total_purchase_amount 100.00, got %v", totalAmount)
	}

	if snapshot["purchase_count"] != 1 {
		t.Errorf("Expected purchase_count 1, got %v", snapshot["purchase_count"])
	}
}

// getTimeDaysAgo 获取 N 天前的时间
func (a *AutoTagger) getTimeDaysAgo(ctx context.Context, days int) time.Time {
	return time.Now().AddDate(0, 0, -days)
}

// TestAutoTagger_compareValues 测试值比较函数
func TestAutoTagger_compareValues(t *testing.T) {
	tests := []struct {
		name       string
		fieldValue any
		operator   string
		value      any
		want       bool
	}{
		{"等于", 50.0, "eq", 50.0, true},
		{"不等于", 50.0, "ne", 40.0, true},
		{"大于", 60.0, "gt", 50.0, true},
		{"小于", 40.0, "lt", 50.0, true},
		{"大于等于", 50.0, "gte", 50.0, true},
		{"小于等于", 50.0, "lte", 50.0, true},
		{"整数比较", 100, "gte", 50.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareValues(tt.fieldValue, tt.operator, tt.value)
			if got != tt.want {
				t.Errorf("compareValues(%v, %s, %v) = %v, want %v",
					tt.fieldValue, tt.operator, tt.value, got, tt.want)
			}
		})
	}
}

// TestAutoTagger_compareStringValues 测试字符串比较
func TestAutoTagger_compareStringValues(t *testing.T) {
	tests := []struct {
		name       string
		fieldValue string
		operator   string
		value      string
		want       bool
	}{
		{"等于", "VIP", "eq", "VIP", true},
		{"包含", "VIP Customer", "contains", "vip", true},
		{"大于", "zebra", "gt", "apple", true},
		{"小于", "apple", "lt", "zebra", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareStringValues(tt.fieldValue, tt.operator, tt.value)
			if got != tt.want {
				t.Errorf("compareStringValues(%s, %s, %s) = %v, want %v",
					tt.fieldValue, tt.operator, tt.value, got, tt.want)
			}
		})
	}
}

// TestAutoTagger_evaluateRuleWithEvents 测试规则评估
func TestAutoTagger_evaluateRuleWithEvents(t *testing.T) {
	tagger := setupAutoTagger(t)

	rule := map[string]any{
		"type":     "simple",
		"field":    "rfm_score",
		"operator": "gte",
		"value":    50,
	}
	ruleJSON, _ := json.Marshal(rule)

	customerData := map[string]any{
		"rfm_score": 75,
	}

	result := tagger.evaluateRuleWithEvents(customerData, string(ruleJSON))
	if !result {
		t.Error("Expected rule to evaluate as true")
	}
}

// TestAutoTagger_ProcessEvent 测试事件处理
func TestAutoTagger_ProcessEvent(t *testing.T) {
	tagger := setupAutoTagger(t)

	// 创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138030",
	})

	// 创建自动标签规则
	rule := map[string]any{
		"type":     "simple",
		"field":    "rfm_score",
		"operator": "gte",
		"value":    0,
	}
	tagger.CreateAutoTag("Active", string(model.TagCategoryBehavioral), rule)

	// 创建事件
	event := &model.CustomerEvent{
		CustomerID:  customer.ID,
		EventType:   model.EventTypePageView,
		EventSource: model.EventSourceWebsite,
	}

	// 处理事件
	err := tagger.ProcessEvent(event)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
}

// TestAutoTagger_EvaluateAndTag 测试评估和打标签
func TestAutoTagger_EvaluateAndTag(t *testing.T) {
	tagger := setupAutoTagger(t)

	// 创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138031",
	})

	// 创建自动标签规则（始终匹配）
	rule := map[string]any{
		"type":     "simple",
		"field":    "rfm_score",
		"operator": "gte",
		"value":    0,
	}
	tagger.CreateAutoTag("TestTag", string(model.TagCategoryBehavioral), rule)

	// 评估和打标签
	err := tagger.EvaluateAndTag(customer.ID)
	if err != nil {
		t.Fatalf("EvaluateAndTag failed: %v", err)
	}

	// 验证标签已添加
	updated, _ := tagger.custRepo.GetByID(customer.ID)
	tags := updated.GetTags()
	found := false
	for _, tag := range tags {
		if tag == "TestTag" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected TestTag to be added")
	}
}
