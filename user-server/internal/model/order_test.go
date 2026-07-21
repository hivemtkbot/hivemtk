package model

import (
	_type "marketing/internal/pkg/utils/type"
	"testing"
)

func TestOrder_TableName(t *testing.T) {
	order := &Order{}
	tableName := order.TableName()
	if tableName != "order" {
		t.Errorf("Expected table name 'order', got %s", tableName)
	}
}

func TestOrder_BasicFields(t *testing.T) {
	order := &Order{
		Status:     _type.OrderStatusType(100),
		CreateTime: 1234567890,
		Price:      "99.00",
		TgID:       123456789,
		AccountID:  "account-123",
	}

	if order.Status != 100 {
		t.Errorf("Expected Status 100, got %d", order.Status)
	}
	if order.CreateTime != 1234567890 {
		t.Errorf("Expected CreateTime 1234567890, got %d", order.CreateTime)
	}
	if order.Price != "99.00" {
		t.Errorf("Expected Price '99.00', got %s", order.Price)
	}
	if order.TgID != 123456789 {
		t.Errorf("Expected TgID 123456789, got %d", order.TgID)
	}
	if order.AccountID != "account-123" {
		t.Errorf("Expected AccountID 'account-123', got %s", order.AccountID)
	}
}

func TestOrder_WithEmptyID(t *testing.T) {
	order := &Order{
		Price: "99.00",
		ID:    "",
	}

	// ID should be empty before BeforeCreate is called
	if order.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", order.ID)
	}
}

func TestOrder_WithStatusValues(t *testing.T) {
	statuses := []_type.OrderStatusType{0, 100, -1, -2}
	statusNames := map[_type.OrderStatusType]string{
		0:   "待支付",
		100: "已支付",
		-1:  "超时",
		-2:  "强行关闭",
	}

	for _, status := range statuses {
		order := &Order{
			Status: status,
		}
		if order.Status != status {
			t.Errorf("Expected Status %d (%s), got %d", status, statusNames[status], order.Status)
		}
	}
}

func TestOrder_WithPrice(t *testing.T) {
	prices := []string{"99.00", "100", "0.01", "999.99"}

	for _, price := range prices {
		order := &Order{
			Price: price,
		}
		if order.Price != price {
			t.Errorf("Expected Price %s, got %s", price, order.Price)
		}
	}
}

func TestOrder_WithTgID(t *testing.T) {
	order := &Order{
		TgID: 987654321,
	}

	if order.TgID != 987654321 {
		t.Errorf("Expected TgID 987654321, got %d", order.TgID)
	}
}

func TestOrder_MultipleOrdersSameTgID(t *testing.T) {
	tgID := int64(123456789)

	order1 := &Order{
		TgID:   tgID,
		Price:  "99.00",
		Status: 0,
	}

	order2 := &Order{
		TgID:   tgID,
		Price:  "199.00",
		Status: 100,
	}

	if order1.TgID != order2.TgID {
		t.Error("Expected same TgID for both orders")
	}
	if order1.Price != order2.Price {
		t.Logf("Order prices differ: %s vs %s", order1.Price, order2.Price)
	}
}

func TestOrder_WithAccountID(t *testing.T) {
	order := &Order{
		AccountID: "merchant-456",
	}

	if order.AccountID != "merchant-456" {
		t.Errorf("Expected AccountID 'merchant-456', got %s", order.AccountID)
	}
}

func TestOrder_BeforeCreate(t *testing.T) {
	order := &Order{
		Price: "99.00",
	}

	// BeforeCreate should generate an ID
	err := order.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if order.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	// Verify it's a valid UUID format
	if len(order.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(order.ID))
	}
}
