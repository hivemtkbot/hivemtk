package service

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"
	"time"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupEventTrackerTestDB 设置测试数据库
func setupEventTrackerTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.CustomerEvent{},
		&model.Customer{},
		&model.CustomerTag{},
	)
	db.SetTestDB(database)
	return database
}

// setupEventTracker 设置测试 EventTracker
func setupEventTracker(t *testing.T) *EventTracker {
	setupEventTrackerTestDB(t)
	customerService := NewCustomerService()
	tracker := NewEventTracker(customerService)
	tracker.DisableAsync() // 禁用异步处理避免测试冲突
	return tracker
}

// TestNewEventTracker 测试创建 EventTracker 实例
func TestNewEventTracker(t *testing.T) {
	customerService := NewCustomerService()
	tracker := NewEventTracker(customerService)
	if tracker == nil {
		t.Fatal("Expected EventTracker instance, got nil")
	}
}

// TestEventTracker_Track 测试追踪通用事件
func TestEventTracker_Track(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138010",
	})

	dto := &EventDTO{
		CustomerID:  customer.ID,
		EventType:   model.EventTypePageView,
		EventSource: model.EventSourceWebsite,
		EventData:   map[string]any{"url": "/home"},
	}

	ctx := context.Background()
	err := tracker.Track(ctx, dto)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}

	// 验证事件已记录
	events, err := tracker.repo.GetByCustomerID(customer.ID, 10)
	if err != nil {
		t.Fatalf("GetByCustomerID failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}

// TestEventTracker_Track_InvalidDTO 测试无效 DTO
func TestEventTracker_Track_InvalidDTO(t *testing.T) {
	tracker := setupEventTracker(t)

	ctx := context.Background()
	err := tracker.Track(ctx, nil)
	if err != ErrInvalidDTO {
		t.Errorf("Expected ErrInvalidDTO, got %v", err)
	}

	err = tracker.Track(ctx, &EventDTO{})
	if err != ErrInvalidDTO {
		t.Errorf("Expected ErrInvalidDTO for empty customer ID, got %v", err)
	}
}

// TestEventTracker_TrackPageView 测试追踪页面浏览
func TestEventTracker_TrackPageView(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138011",
	})

	ctx := context.Background()
	err := tracker.TrackPageView(ctx, customer.ID, "/products", "Product Page")
	if err != nil {
		t.Fatalf("TrackPageView failed: %v", err)
	}

	// 验证事件
	events, _ := tracker.repo.GetByCustomerID(customer.ID, 10)
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if events[0].EventType != model.EventTypePageView {
		t.Errorf("Expected event type page_view, got %s", events[0].EventType)
	}

	eventData := events[0].GetEventData()
	if url, ok := eventData["url"].(string); !ok || url != "/products" {
		t.Errorf("Expected url /products, got %s", url)
	}
}

// TestEventTracker_TrackClick 测试追踪点击事件
func TestEventTracker_TrackClick(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138012",
	})

	ctx := context.Background()
	err := tracker.TrackClick(ctx, customer.ID, "buy-button", "product-123")
	if err != nil {
		t.Fatalf("TrackClick failed: %v", err)
	}

	events, _ := tracker.repo.GetByCustomerID(customer.ID, 10)
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if events[0].EventType != model.EventTypeClick {
		t.Errorf("Expected event type click, got %s", events[0].EventType)
	}
}

// TestEventTracker_TrackPurchase 测试追踪购买事件
func TestEventTracker_TrackPurchase(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138013",
	})

	items := []string{"product-1", "product-2"}
	ctx := context.Background()
	err := tracker.TrackPurchase(ctx, customer.ID, 199.99, items)
	if err != nil {
		t.Fatalf("TrackPurchase failed: %v", err)
	}

	events, _ := tracker.repo.GetByCustomerID(customer.ID, 10)
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if events[0].EventType != model.EventTypePurchase {
		t.Errorf("Expected event type purchase, got %s", events[0].EventType)
	}

	eventData := events[0].GetEventData()
	amount, ok := eventData["amount"].(float64)
	if !ok || amount != 199.99 {
		t.Errorf("Expected amount 199.99, got %v", eventData["amount"])
	}
}

// TestEventTracker_GetEventHistory 测试获取事件历史
func TestEventTracker_GetEventHistory(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138014",
	})

	// 创建多个事件
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		tracker.TrackPageView(ctx, customer.ID, "/page"+string(rune('0'+i)), "Page "+string(rune('0'+i)))
	}

	// 获取历史
	events, err := tracker.GetEventHistory(customer.ID, 10)
	if err != nil {
		t.Fatalf("GetEventHistory failed: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("Expected 5 events, got %d", len(events))
	}
}

// TestEventTracker_GetEventHistory_Limit 测试事件历史限制
func TestEventTracker_GetEventHistory_Limit(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138015",
	})

	// 创建多个事件
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		tracker.TrackPageView(ctx, customer.ID, "/page"+string(rune('0'+i)), "Page "+string(rune('0'+i)))
	}

	// 获取前 10 条
	events, err := tracker.GetEventHistory(customer.ID, 10)
	if err != nil {
		t.Fatalf("GetEventHistory failed: %v", err)
	}
	if len(events) != 10 {
		t.Errorf("Expected 10 events with limit, got %d", len(events))
	}
}

// TestEventTracker_GetStats 测试获取事件统计
func TestEventTracker_GetStats(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138016",
	})

	// 创建事件
	ctx := context.Background()
	tracker.TrackPageView(ctx, customer.ID, "/home", "Home")
	tracker.TrackPurchase(ctx, customer.ID, 99.99, []string{"item-1"})

	// 获取统计
	start := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	end := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	stats, err := tracker.GetStats(start, end)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalEvents < 2 {
		t.Errorf("Expected at least 2 events, got %d", stats.TotalEvents)
	}
}

// TestEventTracker_TrackSignup 测试追踪注册事件
func TestEventTracker_TrackSignup(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138017",
	})

	ctx := context.Background()
	err := tracker.TrackSignup(ctx, customer.ID, "email")
	if err != nil {
		t.Fatalf("TrackSignup failed: %v", err)
	}

	events, _ := tracker.repo.GetByCustomerID(customer.ID, 10)
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if events[0].EventType != model.EventTypeSignup {
		t.Errorf("Expected event type signup, got %s", events[0].EventType)
	}
}

// TestEventTracker_TrackLogin 测试追踪登录事件
func TestEventTracker_TrackLogin(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138018",
	})

	ctx := context.Background()
	err := tracker.TrackLogin(ctx, customer.ID, "wechat")
	if err != nil {
		t.Fatalf("TrackLogin failed: %v", err)
	}

	events, _ := tracker.repo.GetByCustomerID(customer.ID, 10)
	if events[0].EventType != model.EventTypeLogin {
		t.Errorf("Expected event type login, got %s", events[0].EventType)
	}
}

// TestEventTracker_TrackAddToCart 测试追踪加购事件
func TestEventTracker_TrackAddToCart(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138019",
	})

	ctx := context.Background()
	err := tracker.TrackAddToCart(ctx, customer.ID, "prod-1", "Test Product", 59.99, 2)
	if err != nil {
		t.Fatalf("TrackAddToCart failed: %v", err)
	}

	events, _ := tracker.repo.GetByCustomerID(customer.ID, 10)
	if events[0].EventType != model.EventTypeAddToCart {
		t.Errorf("Expected event type add_to_cart, got %s", events[0].EventType)
	}

	eventData := events[0].GetEventData()
	price, _ := eventData["price"].(float64)
	if price != 59.99 {
		t.Errorf("Expected price 59.99, got %v", eventData["price"])
	}
}

// TestEventTracker_GetEventCount 测试获取事件数量
func TestEventTracker_GetEventCount(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138020",
	})

	// 创建 3 个事件
	ctx := context.Background()
	tracker.TrackPageView(ctx, customer.ID, "/page1", "Page 1")
	tracker.TrackPageView(ctx, customer.ID, "/page2", "Page 2")
	tracker.TrackPurchase(ctx, customer.ID, 50.00, []string{"item"})

	count, err := tracker.GetEventCount(customer.ID)
	if err != nil {
		t.Fatalf("GetEventCount failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 events, got %d", count)
	}
}

// TestEventTracker_TrackWithEventData 测试追踪自定义事件
func TestEventTracker_TrackWithEventData(t *testing.T) {
	tracker := setupEventTracker(t)

	// 先创建客户
	customerService := NewCustomerService()
	customer, _ := customerService.CreateOrUpdate(&CustomerDTO{
		Phone: "13800138021",
	})

	ctx := context.Background()
	customData := map[string]any{
		"custom_field": "custom_value",
		"score":        100,
	}
	err := tracker.TrackWithEventData(ctx, customer.ID, "custom_event", "app", customData)
	if err != nil {
		t.Fatalf("TrackWithEventData failed: %v", err)
	}

	events, _ := tracker.repo.GetByCustomerID(customer.ID, 10)
	eventData := events[0].GetEventData()
	if val, ok := eventData["custom_field"].(string); !ok || val != "custom_value" {
		t.Errorf("Expected custom_field to be preserved")
	}
}

// TestSerializeEventData 测试序列化事件数据
func TestSerializeEventData(t *testing.T) {
	data := map[string]any{
		"key":   "value",
		"count": 42,
	}

	serialized, err := SerializeEventData(data)
	if err != nil {
		t.Fatalf("SerializeEventData failed: %v", err)
	}
	if serialized == "" {
		t.Error("Expected non-empty serialized data")
	}

	// 测试 nil 数据
	empty, err := SerializeEventData(nil)
	if err != nil {
		t.Fatalf("SerializeEventData with nil failed: %v", err)
	}
	if empty != "{}" {
		t.Errorf("Expected {} for nil data, got %s", empty)
	}
}
