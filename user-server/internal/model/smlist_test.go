package model

import (
	"testing"
)

func TestSmlist_TableName(t *testing.T) {
	smlist := &Smlist{}
	tableName := smlist.TableName()
	if tableName != "smlist" {
		t.Errorf("Expected table name 'smlist', got %s", tableName)
	}
}

func TestSmlist_BasicFields(t *testing.T) {
	smlist := &Smlist{
		QQ:      "12345678",
		Tg:      "testuser",
		WX:      "wechat123",
		X:       "twitter_handle",
		Name:    "Test User",
		Phone:   "+1234567890",
		City:    "Beijing",
		Address: "123 Test Street",
		Desc:    "Test description",
		Age:     "25",
		Score:   "95",
		Price:   "100.00",
		Service: "VIP",
		Images:  "image1.jpg,image2.jpg",
	}

	if smlist.QQ != "12345678" {
		t.Errorf("Expected QQ '12345678', got %s", smlist.QQ)
	}
	if smlist.Tg != "testuser" {
		t.Errorf("Expected Tg 'testuser', got %s", smlist.Tg)
	}
	if smlist.WX != "wechat123" {
		t.Errorf("Expected WX 'wechat123', got %s", smlist.WX)
	}
	if smlist.X != "twitter_handle" {
		t.Errorf("Expected X 'twitter_handle', got %s", smlist.X)
	}
	if smlist.Name != "Test User" {
		t.Errorf("Expected Name 'Test User', got %s", smlist.Name)
	}
	if smlist.Phone != "+1234567890" {
		t.Errorf("Expected Phone '+1234567890', got %s", smlist.Phone)
	}
	if smlist.City != "Beijing" {
		t.Errorf("Expected City 'Beijing', got %s", smlist.City)
	}
	if smlist.Address != "123 Test Street" {
		t.Errorf("Expected Address '123 Test Street', got %s", smlist.Address)
	}
	if smlist.Desc != "Test description" {
		t.Errorf("Expected Desc 'Test description', got %s", smlist.Desc)
	}
	if smlist.Age != "25" {
		t.Errorf("Expected Age '25', got %s", smlist.Age)
	}
	if smlist.Score != "95" {
		t.Errorf("Expected Score '95', got %s", smlist.Score)
	}
	if smlist.Price != "100.00" {
		t.Errorf("Expected Price '100.00', got %s", smlist.Price)
	}
	if smlist.Service != "VIP" {
		t.Errorf("Expected Service 'VIP', got %s", smlist.Service)
	}
	if smlist.Images != "image1.jpg,image2.jpg" {
		t.Errorf("Expected Images 'image1.jpg,image2.jpg', got %s", smlist.Images)
	}
}

func TestSmlist_WithEmptyID(t *testing.T) {
	smlist := &Smlist{
		Name: "Test User",
		ID:   "",
	}

	// ID should be empty before BeforeCreate is called
	if smlist.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", smlist.ID)
	}
}

func TestSmlist_WithEmptyFields(t *testing.T) {
	smlist := &Smlist{}

	if smlist.QQ != "" {
		t.Errorf("Expected empty QQ, got %s", smlist.QQ)
	}
	if smlist.Tg != "" {
		t.Errorf("Expected empty Tg, got %s", smlist.Tg)
	}
	if smlist.Name != "" {
		t.Errorf("Expected empty Name, got %s", smlist.Name)
	}
}

func TestSmlist_WithPartialFields(t *testing.T) {
	smlist := &Smlist{
		QQ:    "12345678",
		Name:  "Partial User",
		Phone: "+1234567890",
	}

	if smlist.QQ != "12345678" {
		t.Errorf("Expected QQ '12345678', got %s", smlist.QQ)
	}
	if smlist.Name != "Partial User" {
		t.Errorf("Expected Name 'Partial User', got %s", smlist.Name)
	}
	if smlist.Phone != "+1234567890" {
		t.Errorf("Expected Phone '+1234567890', got %s", smlist.Phone)
	}
	// Other fields should be empty
	if smlist.Tg != "" {
		t.Errorf("Expected empty Tg, got %s", smlist.Tg)
	}
}

func TestSmlist_WithAllSocialAccounts(t *testing.T) {
	smlist := &Smlist{
		QQ: "qq_user",
		Tg: "tg_user",
		WX: "wx_user",
		X:  "x_user",
	}

	if smlist.QQ != "qq_user" {
		t.Errorf("Expected QQ 'qq_user', got %s", smlist.QQ)
	}
	if smlist.Tg != "tg_user" {
		t.Errorf("Expected Tg 'tg_user', got %s", smlist.Tg)
	}
	if smlist.WX != "wx_user" {
		t.Errorf("Expected WX 'wx_user', got %s", smlist.WX)
	}
	if smlist.X != "x_user" {
		t.Errorf("Expected X 'x_user', got %s", smlist.X)
	}
}

func TestSmlist_BeforeCreate_GeneratesID(t *testing.T) {
	smlist := &Smlist{
		Name: "Test User",
		ID:   "",
	}

	err := smlist.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if smlist.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(smlist.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(smlist.ID))
	}
}

func TestSmlist_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := "existing-smlist-id-123"
	smlist := &Smlist{
		ID:   existingID,
		Name: "Test User",
	}

	err := smlist.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if smlist.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, smlist.ID)
	}
}
