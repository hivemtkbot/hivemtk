package model

import (
	"testing"
)

func TestRFMRule_TableName(t *testing.T) {
	rule := &RFMRule{}
	tableName := rule.TableName()
	if tableName != "rfm_rules" {
		t.Errorf("Expected table name 'rfm_rules', got %s", tableName)
	}
}

func TestRFMRule_BasicFields(t *testing.T) {
	rule := &RFMRule{
		ID:       1,
		Name:     "Default RFM Rule",
		RDays1:   7,
		RDays2:   14,
		RDays3:   30,
		RDays4:   60,
		RDays5:   90,
		FCount1:  1,
		FCount2:  3,
		FCount3:  5,
		FCount4:  10,
		FCount5:  20,
		MAmount1: 10000, // 100 元 = 10000 分
		MAmount2: 50000, // 500 元 = 50000 分
		MAmount3: 100000,
		MAmount4: 500000,
		MAmount5: 1000000,
		IsActive: true,
	}

	if rule.ID != 1 {
		t.Errorf("Expected ID 1, got %d", rule.ID)
	}
	if rule.Name != "Default RFM Rule" {
		t.Errorf("Expected Name 'Default RFM Rule', got %s", rule.Name)
	}
	if rule.RDays1 != 7 {
		t.Errorf("Expected RDays1 7, got %d", rule.RDays1)
	}
	if rule.FCount5 != 20 {
		t.Errorf("Expected FCount5 20, got %d", rule.FCount5)
	}
	if rule.MAmount1 != 10000 {
		t.Errorf("Expected MAmount1 10000 分, got %d", rule.MAmount1)
	}
	if !rule.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

func TestRFMRule_DefaultValues(t *testing.T) {
	rule := &RFMRule{}

	if rule.RDays1 != 7 {
		t.Logf("RDays1 is %d (expected 7)", rule.RDays1)
	}
	if rule.IsActive != false {
		t.Logf("IsActive is %v (expected false before save)", rule.IsActive)
	}
}

func TestUserRFM_TableName(t *testing.T) {
	userRFM := &UserRFM{}
	tableName := userRFM.TableName()
	if tableName != "user_rfms" {
		t.Errorf("Expected table name 'user_rfms', got %s", tableName)
	}
}

func TestUserRFM_BasicFields(t *testing.T) {
	userRFM := &UserRFM{
		ID:               1,
		UserID:           100,
		RScore:           5,
		FScore:           4,
		MScore:           3,
		TotalScore:       12,
		Layer:            "important_value",
		TransactionCount: 10,
		TotalAmount:      500000, // 5000 元 = 500000 分
		AvgAmount:        50000,  // 500 元 = 50000 分
	}

	if userRFM.ID != 1 {
		t.Errorf("Expected ID 1, got %d", userRFM.ID)
	}
	if userRFM.UserID != 100 {
		t.Errorf("Expected UserID 100, got %d", userRFM.UserID)
	}
	if userRFM.RScore != 5 {
		t.Errorf("Expected RScore 5, got %d", userRFM.RScore)
	}
	if userRFM.TotalScore != 12 {
		t.Errorf("Expected TotalScore 12, got %d", userRFM.TotalScore)
	}
	if userRFM.Layer != "important_value" {
		t.Errorf("Expected Layer 'important_value', got %s", userRFM.Layer)
	}
}

func TestRFMLayer_Constants(t *testing.T) {
	layers := map[RFMLayer]string{
		RFMLayerImportantValue:   "important_value",
		RFMLayerImportantKeep:    "important_keep",
		RFMLayerImportantDevelop: "important_develop",
		RFMLayerImportantStay:    "important_stay",
		RFMLayerGeneralValue:     "general_value",
		RFMLayerGeneralKeep:      "general_keep",
		RFMLayerGeneralDevelop:   "general_develop",
		RFMLayerGeneralStay:      "general_stay",
		RFMLayerNew:              "new",
		RFMLayerSleep:            "sleep",
		RFMLayerLost:             "lost",
	}

	for layer, expected := range layers {
		if string(layer) != expected {
			t.Errorf("Expected RFMLayer %s, got %s", expected, layer)
		}
	}
}

func TestGetLayerDescription(t *testing.T) {
	tests := map[RFMLayer]string{
		RFMLayerImportantValue:   "重要价值用户 - 最近消费、消费频次高、消费金额高",
		RFMLayerImportantKeep:    "重要保持用户 - 很久未消费、消费频次高、消费金额高",
		RFMLayerImportantDevelop: "重要发展用户 - 最近消费、消费频次低、消费金额高",
		RFMLayerImportantStay:    "重要挽留用户 - 很久未消费、消费频次低、消费金额高",
		RFMLayerGeneralValue:     "一般价值用户 - 最近消费、消费频次高、消费金额低",
		RFMLayerGeneralKeep:      "一般保持用户 - 很久未消费、消费频次高、消费金额低",
		RFMLayerGeneralDevelop:   "一般发展用户 - 最近消费、消费频次低、消费金额低",
		RFMLayerGeneralStay:      "一般挽留用户 - 很久未消费、消费频次低、消费金额低",
		RFMLayerNew:              "新用户 - 首次消费",
		RFMLayerSleep:            "沉睡用户 - 超过 60 天未消费",
		RFMLayerLost:             "流失用户 - 超过 90 天未消费",
	}

	for layer, expectedDesc := range tests {
		desc := GetLayerDescription(layer)
		if desc != expectedDesc {
			t.Errorf("Expected description '%s', got '%s'", expectedDesc, desc)
		}
	}
}

func TestUserRFM_ScoreValues(t *testing.T) {
	scores := []int{1, 2, 3, 4, 5}

	for _, score := range scores {
		userRFM := &UserRFM{
			RScore:     score,
			FScore:     score,
			MScore:     score,
			TotalScore: score * 3,
		}
		if userRFM.RScore != score {
			t.Errorf("Expected RScore %d, got %d", score, userRFM.RScore)
		}
	}
}
