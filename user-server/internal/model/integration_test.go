package model

import (
	"testing"
	"time"
)

func TestIntegrationAccount_TableName(t *testing.T) {
	account := &IntegrationAccount{}
	tableName := account.TableName()
	if tableName != "integration_accounts" {
		t.Errorf("Expected table name 'integration_accounts', got %s", tableName)
	}
}

func TestIntegrationAccount_BasicFields(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(time.Hour)

	account := &IntegrationAccount{
		ID:           1,
		Platform:     "crm_xiaoshouyi",
		AccountName:  "Test Account",
		APIKey:       "test_api_key",
		APISecret:    "test_api_secret",
		RefreshToken: "refresh_token_123",
		AccessToken:  "access_token_123",
		TokenExpires: &expiresAt,
		WebhookURL:   "https://example.com/webhook",
		Config:       `{"region": "cn-north-1"}`,
		Status:       1,
		LastSyncAt:   &now,
	}

	if account.ID != 1 {
		t.Errorf("Expected ID 1, got %d", account.ID)
	}
	if account.Platform != "crm_xiaoshouyi" {
		t.Errorf("Expected Platform 'crm_xiaoshouyi', got %s", account.Platform)
	}
	if account.AccountName != "Test Account" {
		t.Errorf("Expected AccountName 'Test Account', got %s", account.AccountName)
	}
	if account.Status != 1 {
		t.Errorf("Expected Status 1, got %d", account.Status)
	}
}

func TestIntegrationAccount_PlatformValues(t *testing.T) {
	platforms := []string{"crm_xiaoshouyi", "crm_fenxiangxiao", "ecommerce_taobao", "ecommerce_jd"}

	for _, platform := range platforms {
		account := &IntegrationAccount{
			Platform: platform,
		}
		if account.Platform != platform {
			t.Errorf("Expected Platform %s, got %s", platform, account.Platform)
		}
	}
}

func TestSyncLog_TableName(t *testing.T) {
	log := &SyncLog{}
	tableName := log.TableName()
	if tableName != "sync_logs" {
		t.Errorf("Expected table name 'sync_logs', got %s", tableName)
	}
}

func TestSyncLog_BasicFields(t *testing.T) {
	now := time.Now()
	endTime := now.Add(time.Minute)

	log := &SyncLog{
		ID:           1,
		Platform:     "crm_xiaoshouyi",
		SyncType:     "customer",
		Status:       1,
		RecordCount:  100,
		ErrorMessage: "",
		StartTime:    now,
		EndTime:      &endTime,
	}

	if log.ID != 1 {
		t.Errorf("Expected ID 1, got %d", log.ID)
	}
	if log.SyncType != "customer" {
		t.Errorf("Expected SyncType 'customer', got %s", log.SyncType)
	}
	if log.RecordCount != 100 {
		t.Errorf("Expected RecordCount 100, got %d", log.RecordCount)
	}
}

func TestSyncLog_StatusValues(t *testing.T) {
	statuses := map[int]string{
		0: "进行中",
		1: "成功",
		2: "失败",
	}

	for status, desc := range statuses {
		log := &SyncLog{
			Status: status,
		}
		if log.Status != status {
			t.Errorf("Expected Status %d (%s), got %d", status, desc, log.Status)
		}
	}
}

func TestExternalCustomer_TableName(t *testing.T) {
	customer := &ExternalCustomer{}
	tableName := customer.TableName()
	if tableName != "external_customers" {
		t.Errorf("Expected table name 'external_customers', got %s", tableName)
	}
}

func TestExternalCustomer_BasicFields(t *testing.T) {
	now := time.Now()

	customer := &ExternalCustomer{
		ID:            1,
		Platform:      "crm_xiaoshouyi",
		ExternalID:    "ext-001",
		Name:          "John Doe",
		Phone:         "13800138000",
		Email:         "john@example.com",
		Company:       "Test Company",
		Position:      "Manager",
		Industry:      "Technology",
		Level:         "VIP",
		Source:        "Website",
		OwnerID:       "owner-001",
		OwnerName:     "Sales Rep",
		Status:        "potential",
		Tags:          `["vip", "hot"]`,
		LastContactAt: &now,
	}

	if customer.ID != 1 {
		t.Errorf("Expected ID 1, got %d", customer.ID)
	}
	if customer.Name != "John Doe" {
		t.Errorf("Expected Name 'John Doe', got %s", customer.Name)
	}
	if customer.Phone != "13800138000" {
		t.Errorf("Expected Phone '13800138000', got %s", customer.Phone)
	}
	if customer.Level != "VIP" {
		t.Errorf("Expected Level 'VIP', got %s", customer.Level)
	}
}

func TestExternalOrder_TableName(t *testing.T) {
	order := &ExternalOrder{}
	tableName := order.TableName()
	if tableName != "external_orders" {
		t.Errorf("Expected table name 'external_orders', got %s", tableName)
	}
}

func TestExternalOrder_BasicFields(t *testing.T) {
	now := time.Now()

	order := &ExternalOrder{
		ID:             1,
		Platform:       "ecommerce_taobao",
		OrderID:        "tb-123456",
		OrderNo:        "internal-001",
		UserID:         "user-001",
		UserName:       "Test User",
		UserPhone:      "13800138000",
		TotalAmount:    19999,
		PayAmount:      17999,
		DiscountAmount: 2000,
		Status:         "paid",
		PayTime:        &now,
		Items:          `[{"id": "item1", "qty": 2}]`,
		ShippingAddr:   "Beijing, China",
	}

	if order.ID != 1 {
		t.Errorf("Expected ID 1, got %d", order.ID)
	}
	if order.OrderID != "tb-123456" {
		t.Errorf("Expected OrderID 'tb-123456', got %s", order.OrderID)
	}
	if order.TotalAmount != 19999 {
		t.Errorf("Expected TotalAmount 19999, got %d", order.TotalAmount)
	}
	if order.PayAmount != 17999 {
		t.Errorf("Expected PayAmount 17999, got %d", order.PayAmount)
	}
}

func TestExternalProduct_TableName(t *testing.T) {
	product := &ExternalProduct{}
	tableName := product.TableName()
	if tableName != "external_products" {
		t.Errorf("Expected table name 'external_products', got %s", tableName)
	}
}

func TestExternalProduct_BasicFields(t *testing.T) {
	product := &ExternalProduct{
		ID:            1,
		Platform:      "ecommerce_taobao",
		ProductID:     "prod-001",
		Name:          "Test Product",
		CategoryID:    "cat-001",
		CategoryName:  "Electronics",
		Price:         9999,
		OriginalPrice: 12999,
		Stock:         100,
		Sales:         500,
		Images:        `["img1.jpg", "img2.jpg"]`,
		Status:        1,
	}

	if product.ID != 1 {
		t.Errorf("Expected ID 1, got %d", product.ID)
	}
	if product.Name != "Test Product" {
		t.Errorf("Expected Name 'Test Product', got %s", product.Name)
	}
	if product.Price != 9999 {
		t.Errorf("Expected Price 9999, got %d", product.Price)
	}
	if product.Stock != 100 {
		t.Errorf("Expected Stock 100, got %d", product.Stock)
	}
}

func TestWebhookEvent_TableName(t *testing.T) {
	event := &WebhookEvent{}
	tableName := event.TableName()
	if tableName != "webhook_events" {
		t.Errorf("Expected table name 'webhook_events', got %s", tableName)
	}
}

func TestWebhookEvent_BasicFields(t *testing.T) {
	now := time.Now()
	processedAt := now.Add(time.Second)

	event := &WebhookEvent{
		ID:          1,
		Platform:    "crm_xiaoshouyi",
		EventID:     "evt-001",
		EventType:   "customer.created",
		RawData:     `{"event": "customer.created"}`,
		Processed:   true,
		ProcessedAt: &processedAt,
	}

	if event.ID != 1 {
		t.Errorf("Expected ID 1, got %d", event.ID)
	}
	if event.EventID != "evt-001" {
		t.Errorf("Expected EventID 'evt-001', got %s", event.EventID)
	}
	if event.EventType != "customer.created" {
		t.Errorf("Expected EventType 'customer.created', got %s", event.EventType)
	}
	if !event.Processed {
		t.Error("Expected Processed to be true")
	}
}
