package model

import (
	"testing"
	"time"
)

func TestABExperiment_TableName(t *testing.T) {
	experiment := &ABExperiment{}
	tableName := experiment.TableName()
	if tableName != "ab_experiments" {
		t.Errorf("Expected table name 'ab_experiments', got %s", tableName)
	}
}

func TestABExperiment_BasicFields(t *testing.T) {
	now := time.Now()
	endDate := now.AddDate(0, 1, 0)

	experiment := &ABExperiment{
		ID:           1,
		Name:         "Test Experiment",
		Description:  "Test A/B experiment",
		Status:       "running",
		SourceType:   "page",
		SourceID:     "page-001",
		TrafficSplit: 50,
		StartDate:    &now,
		EndDate:      &endDate,
		CreatedBy:    100,
	}

	if experiment.ID != 1 {
		t.Errorf("Expected ID 1, got %d", experiment.ID)
	}

	// 私域部署：不校验 MerchantID（单租户无此字段）
	_ = experiment
	if experiment.Name != "Test Experiment" {
		t.Errorf("Expected Name 'Test Experiment', got %s", experiment.Name)
	}
	if experiment.Status != "running" {
		t.Errorf("Expected Status 'running', got %s", experiment.Status)
	}
	if experiment.TrafficSplit != 50 {
		t.Errorf("Expected TrafficSplit 50, got %d", experiment.TrafficSplit)
	}
}

func TestABExperiment_StatusValues(t *testing.T) {
	statuses := []string{"draft", "running", "paused", "completed"}

	for _, status := range statuses {
		experiment := &ABExperiment{
			Status: status,
		}
		if experiment.Status != status {
			t.Errorf("Expected Status %s, got %s", status, experiment.Status)
		}
	}
}

func TestABVariant_TableName(t *testing.T) {
	variant := &ABVariant{}
	tableName := variant.TableName()
	if tableName != "ab_variants" {
		t.Errorf("Expected table name 'ab_variants', got %s", tableName)
	}
}

func TestABVariant_BasicFields(t *testing.T) {
	variant := &ABVariant{
		ID:              1,
		ExperimentID:    100,
		Name:            "A",
		IsControl:       true,
		Config:          `{"color": "blue"}`,
		Weight:          50,
		TrafficCount:    1000,
		ConversionCount: 50,
	}

	if variant.ID != 1 {
		t.Errorf("Expected ID 1, got %d", variant.ID)
	}
	if variant.ExperimentID != 100 {
		t.Errorf("Expected ExperimentID 100, got %d", variant.ExperimentID)
	}
	if variant.Name != "A" {
		t.Errorf("Expected Name 'A', got %s", variant.Name)
	}
	if !variant.IsControl {
		t.Error("Expected IsControl to be true")
	}
	if variant.Weight != 50 {
		t.Errorf("Expected Weight 50, got %d", variant.Weight)
	}
}

func TestABConversionEvent_TableName(t *testing.T) {
	event := &ABConversionEvent{}
	tableName := event.TableName()
	if tableName != "ab_conversion_events" {
		t.Errorf("Expected table name 'ab_conversion_events', got %s", tableName)
	}
}

func TestABConversionEvent_BasicFields(t *testing.T) {
	event := &ABConversionEvent{
		ID:           1,
		ExperimentID: 100,
		EventName:    "purchase",
		EventType:    "click",
		EventValue:   9999, // 99.99 元 = 9999 分
		UserID:       "user-001",
		VariantID:    1,
		Metadata:     `{"product_id": "p123"}`,
	}

	if event.ID != 1 {
		t.Errorf("Expected ID 1, got %d", event.ID)
	}
	if event.EventName != "purchase" {
		t.Errorf("Expected EventName 'purchase', got %s", event.EventName)
	}
	if event.EventType != "click" {
		t.Errorf("Expected EventType 'click', got %s", event.EventType)
	}
	if event.EventValue != 9999 {
		t.Errorf("Expected EventValue 9999, got %d", event.EventValue)
	}
}

func TestABExperimentResult_TableName(t *testing.T) {
	result := &ABExperimentResult{}
	tableName := result.TableName()
	if tableName != "ab_experiment_results" {
		t.Errorf("Expected table name 'ab_experiment_results', got %s", tableName)
	}
}

func TestABExperimentResult_BasicFields(t *testing.T) {
	result := &ABExperimentResult{
		ID:              1,
		ExperimentID:    100,
		VariantID:       1,
		VariantName:     "A",
		IsControl:       true,
		TrafficCount:    1000,
		ConversionCount: 50,
		ConversionRate:  0.05,
		Revenue:         500000, // 5000.00 元 = 500000 分
		AverageValue:    10000,  // 100.00 元 = 10000 分
		ConfidenceLevel: 0.95,
		IsWinner:        true,
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}
	if result.VariantName != "A" {
		t.Errorf("Expected VariantName 'A', got %s", result.VariantName)
	}
	if result.ConversionRate != 0.05 {
		t.Errorf("Expected ConversionRate 0.05, got %f", result.ConversionRate)
	}
	if !result.IsWinner {
		t.Error("Expected IsWinner to be true")
	}
}
