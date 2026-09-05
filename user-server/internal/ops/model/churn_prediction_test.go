package model

import (
	"testing"
	"time"
)

func TestChurnPrediction_TableName(t *testing.T) {
	prediction := &ChurnPrediction{}
	tableName := prediction.TableName()
	if tableName != "churn_predictions" {
		t.Errorf("Expected table name 'churn_predictions', got %s", tableName)
	}
}

func TestChurnPrediction_BasicFields(t *testing.T) {
	now := time.Now()
	lastActivity := now.AddDate(0, 0, -45)
	lastPurchase := now.AddDate(0, 0, -60)

	prediction := &ChurnPrediction{
		ID:                1,
		UserID:            "user-001",
		ChurnScore:        75.5,
		ChurnRisk:         "high",
		RiskFactors:       `["inactive_days", "purchase_drop"]`,
		LastActivityAt:    &lastActivity,
		LastPurchaseAt:    &lastPurchase,
		DaysSinceActive:   45,
		PurchaseFreq:      2.5,
		AverageOrderValue: 150.00,
	}

	if prediction.ID != 1 {
		t.Errorf("Expected ID 1, got %d", prediction.ID)
	}
	if prediction.UserID != "user-001" {
		t.Errorf("Expected UserID 'user-001', got %s", prediction.UserID)
	}
	if prediction.ChurnScore != 75.5 {
		t.Errorf("Expected ChurnScore 75.5, got %f", prediction.ChurnScore)
	}
	if prediction.ChurnRisk != "high" {
		t.Errorf("Expected ChurnRisk 'high', got %s", prediction.ChurnRisk)
	}
	if prediction.DaysSinceActive != 45 {
		t.Errorf("Expected DaysSinceActive 45, got %d", prediction.DaysSinceActive)
	}
}

func TestChurnPrediction_RiskLevels(t *testing.T) {
	risks := []string{"low", "medium", "high", "critical"}

	for _, risk := range risks {
		prediction := &ChurnPrediction{
			ChurnRisk: risk,
		}
		if prediction.ChurnRisk != risk {
			t.Errorf("Expected ChurnRisk %s, got %s", risk, prediction.ChurnRisk)
		}
	}
}

func TestChurnWarning_TableName(t *testing.T) {
	warning := &ChurnWarning{}
	tableName := warning.TableName()
	if tableName != "churn_warnings" {
		t.Errorf("Expected table name 'churn_warnings', got %s", tableName)
	}
}

func TestChurnWarning_BasicFields(t *testing.T) {
	now := time.Now()
	handledAt := now.Add(-time.Hour)

	warning := &ChurnWarning{
		ID:           1,
		UserID:       "user-001",
		WarningLevel: "high",
		WarningType:  "inactive_days",
		Description:  "User has been inactive for 45 days",
		Suggestion:   "Send re-engagement email",
		IsHandled:    true,
		HandledAt:    &handledAt,
		HandledBy:    100,
		HandledNote:  "Sent email campaign",
	}

	if warning.ID != 1 {
		t.Errorf("Expected ID 1, got %d", warning.ID)
	}
	if warning.WarningLevel != "high" {
		t.Errorf("Expected WarningLevel 'high', got %s", warning.WarningLevel)
	}
	if !warning.IsHandled {
		t.Error("Expected IsHandled to be true")
	}
}

func TestChurnModelConfig_TableName(t *testing.T) {
	config := &ChurnModelConfig{}
	tableName := config.TableName()
	if tableName != "churn_model_configs" {
		t.Errorf("Expected table name 'churn_model_configs', got %s", tableName)
	}
}

func TestChurnModelConfig_BasicFields(t *testing.T) {
	config := &ChurnModelConfig{
		ID:                 1,
		InactiveDaysWeight: 0.3,
		PurchaseFreqWeight: 0.3,
		OrderValueWeight:   0.2,
		EngagementWeight:   0.2,
		InactiveThreshold:  30,
		PurchaseThreshold:  60,
		HighRiskScore:      70,
		CriticalRiskScore:  85,
	}

	if config.ID != 1 {
		t.Errorf("Expected ID 1, got %d", config.ID)
	}
	if config.InactiveDaysWeight != 0.3 {
		t.Errorf("Expected InactiveDaysWeight 0.3, got %f", config.InactiveDaysWeight)
	}
	if config.InactiveThreshold != 30 {
		t.Errorf("Expected InactiveThreshold 30, got %d", config.InactiveThreshold)
	}
	if config.HighRiskScore != 70 {
		t.Errorf("Expected HighRiskScore 70, got %f", config.HighRiskScore)
	}
}

func TestChurnStatistics_TableName(t *testing.T) {
	stats := &ChurnStatistics{}
	tableName := stats.TableName()
	if tableName != "churn_statistics" {
		t.Errorf("Expected table name 'churn_statistics', got %s", tableName)
	}
}

func TestChurnStatistics_BasicFields(t *testing.T) {
	stats := &ChurnStatistics{
		ID:             1,
		StatDate:       "2024-01-15",
		TotalUsers:     1000,
		ChurnUsers:     50,
		ChurnRate:      5.0,
		HighRiskUsers:  100,
		CriticalUsers:  25,
		RecoveredUsers: 10,
	}

	if stats.ID != 1 {
		t.Errorf("Expected ID 1, got %d", stats.ID)
	}
	if stats.StatDate != "2024-01-15" {
		t.Errorf("Expected StatDate '2024-01-15', got %s", stats.StatDate)
	}
	if stats.ChurnRate != 5.0 {
		t.Errorf("Expected ChurnRate 5.0, got %f", stats.ChurnRate)
	}
}
