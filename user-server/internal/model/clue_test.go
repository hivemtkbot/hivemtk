package model

import (
	"testing"
)

func TestClue_TableName(t *testing.T) {
	clue := &Clue{}
	tableName := clue.TableName()
	if tableName != "clues" {
		t.Errorf("Expected table name 'clues', got %s", tableName)
	}
}

func TestClue_BasicFields(t *testing.T) {
	clue := &Clue{
		SourceID: "source-123",
		Account:  "test@example.com",
		Type:     1,
		IsVerify: 1,
		Name:     "Test Lead",
		City:     "Beijing",
		Address:  "123 Test Street",
		Desc:     "Test description",
	}

	if clue.SourceID != "source-123" {
		t.Errorf("Expected SourceID 'source-123', got %s", clue.SourceID)
	}
	if clue.Account != "test@example.com" {
		t.Errorf("Expected Account 'test@example.com', got %s", clue.Account)
	}
	if clue.Type != 1 {
		t.Errorf("Expected Type 1, got %d", clue.Type)
	}
	if clue.IsVerify != 1 {
		t.Errorf("Expected IsVerify 1, got %d", clue.IsVerify)
	}
	if clue.Name != "Test Lead" {
		t.Errorf("Expected Name 'Test Lead', got %s", clue.Name)
	}
	if clue.City != "Beijing" {
		t.Errorf("Expected City 'Beijing', got %s", clue.City)
	}
	if clue.Address != "123 Test Street" {
		t.Errorf("Expected Address '123 Test Street', got %s", clue.Address)
	}
	if clue.Desc != "Test description" {
		t.Errorf("Expected Desc 'Test description', got %s", clue.Desc)
	}
}

func TestClue_WithEmptyID(t *testing.T) {
	clue := &Clue{
		Name: "Test Lead",
		ID:   "",
	}

	if clue.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", clue.ID)
	}
}

func TestClue_WithEmptyFields(t *testing.T) {
	clue := &Clue{}

	if clue.SourceID != "" {
		t.Errorf("Expected empty SourceID, got %s", clue.SourceID)
	}
	if clue.Account != "" {
		t.Errorf("Expected empty Account, got %s", clue.Account)
	}
	if clue.Type != 0 {
		t.Errorf("Expected Type 0, got %d", clue.Type)
	}
	if clue.IsVerify != 0 {
		t.Errorf("Expected IsVerify 0, got %d", clue.IsVerify)
	}
}

func TestClue_WithTypeValues(t *testing.T) {
	types := []int64{0, 1, 2}

	for _, clueType := range types {
		clue := &Clue{
			Type: clueType,
		}
		if clue.Type != clueType {
			t.Errorf("Expected Type %d, got %d", clueType, clue.Type)
		}
	}
}

func TestClue_WithIsVerifyValues(t *testing.T) {
	isVerifyValues := []int64{0, 1}

	for _, isVerify := range isVerifyValues {
		clue := &Clue{
			IsVerify: isVerify,
		}
		if clue.IsVerify != isVerify {
			t.Errorf("Expected IsVerify %d, got %d", isVerify, clue.IsVerify)
		}
	}
}

func TestClue_WithLongDesc(t *testing.T) {
	longDesc := "This is a very long description for the clue. It contains detailed information about the potential lead including their requirements, preferences, and other relevant details that might be useful for the sales team."
	clue := &Clue{
		Desc: longDesc,
	}

	if clue.Desc != longDesc {
		t.Error("Expected long description to be stored")
	}
}

func TestClue_BeforeCreate(t *testing.T) {
	clue := &Clue{
		Name: "Test Lead",
	}

	err := clue.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if clue.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(clue.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(clue.ID))
	}
}
