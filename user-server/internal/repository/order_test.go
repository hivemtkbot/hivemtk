package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	_type "hivemtk-user/internal/pkg/utils/type"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupOrderTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Order{},
	)
	db.SetTestDB(database)
	return database
}

func TestOrderRepository_New(t *testing.T) {
	setupOrderTestDB(t)

	repo := NewOrderRepository()
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

func TestOrderRepository_Create(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	order := &model.Order{
		Status:    _type.OrderStatusPending,
		Price:     "99.00",
		TgID:      12345,
		AccountID: "account123",
	}

	err := repo.Create(ctx, order)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if order.ID == "" {
		t.Error("Expected non-empty ID after create")
	}
}

func TestOrderRepository_Create_MultipleOrders(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	for i := 0; i < 5; i++ {
		order := &model.Order{
			Status:    _type.OrderStatusPending,
			Price:     "99.00",
			TgID:      12345,
			AccountID: "account123",
		}
		repo.Create(ctx, order)
	}

	orders, total, err := repo.GetOrderList(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetOrderList failed: %v", err)
	}

	if len(orders) != 5 {
		t.Errorf("Expected 5 orders, got %d", len(orders))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestOrderRepository_GetOrderList(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	for i := 0; i < 10; i++ {
		order := &model.Order{
			Status:    _type.OrderStatusPending,
			Price:     "99.00",
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
		}
		repo.Create(ctx, order)
	}

	orders, total, err := repo.GetOrderList(ctx, 1, 5)
	if err != nil {
		t.Fatalf("GetOrderList failed: %v", err)
	}

	if len(orders) != 5 {
		t.Errorf("Expected 5 orders, got %d", len(orders))
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
}

func TestOrderRepository_GetOrderList_Empty(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	orders, total, err := repo.GetOrderList(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetOrderList failed: %v", err)
	}

	if len(orders) != 0 {
		t.Errorf("Expected 0 orders, got %d", len(orders))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

func TestOrderRepository_GetGetLastOrder(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	order1 := &model.Order{
		Status:    _type.OrderStatusPending,
		Price:     "99.00",
		TgID:      12345,
		AccountID: "account123",
	}
	repo.Create(ctx, order1)

	order2 := &model.Order{
		Status:    _type.OrderStatusSuccess,
		Price:     "199.00",
		TgID:      12346, 
		AccountID: "account123",
	}
	repo.Create(ctx, order2)

	lastOrder, err := repo.GetGetLastOrder(context.Background(), "account123", 12345)
	if err != nil {
		t.Fatalf("GetGetLastOrder failed: %v", err)
	}

	if lastOrder.Price != "99.00" {
		t.Errorf("Expected order price '99.00', got %s", lastOrder.Price)
	}
	if lastOrder.Status != _type.OrderStatusPending {
		t.Errorf("Expected order status Pending, got %d", lastOrder.Status)
	}
}

func TestOrderRepository_GetGetLastOrder_NotFound(t *testing.T) {
	setupOrderTestDB(t)

	repo := NewOrderRepository()

	_, err := repo.GetGetLastOrder(context.Background(), "nonexistent", 99999)
	if err == nil {
		t.Error("Expected error for non-existent order")
	}
}

func TestOrderRepository_UpdateOrderStatusById(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	order := &model.Order{
		Status:    _type.OrderStatusPending,
		Price:     "99.00",
		TgID:      12345,
		AccountID: "account123",
	}
	repo.Create(ctx, order)

	err := repo.UpdateOrderStatusById(context.Background(), order.ID, _type.OrderStatusSuccess)
	if err != nil {
		t.Fatalf("UpdateOrderStatusById failed: %v", err)
	}

	updatedOrder, _ := repo.GetGetLastOrder(context.Background(), "account123", 12345)
	if updatedOrder.Status != _type.OrderStatusSuccess {
		t.Errorf("Expected status Success, got %d", updatedOrder.Status)
	}
}

func TestOrderRepository_UpdateOrderStatusById_NotFound(t *testing.T) {
	setupOrderTestDB(t)

	repo := NewOrderRepository()

	err := repo.UpdateOrderStatusById(context.Background(), "non-existent-id", _type.OrderStatusSuccess)
	if err == nil {
		t.Error("Expected error for non-existent order")
	}
}

func TestOrderRepository_GetRecentOrderList(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	for i := 0; i < 3; i++ {
		order := &model.Order{
			Status:    _type.OrderStatusPending,
			Price:     "99.00",
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
		}
		repo.Create(ctx, order)
	}

	orders, err := repo.GetRecentOrderList(context.Background())
	if err != nil {
		t.Fatalf("GetRecentOrderList failed: %v", err)
	}

	if len(orders) > 3 {
		t.Errorf("Expected at most 3 recent orders, got %d", len(orders))
	}
}

func TestOrderRepository_GetOrderList_WithDifferentStatus(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	for i := 0; i < 3; i++ {
		order := &model.Order{
			Status:    _type.OrderStatusPending,
			Price:     "99.00",
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
		}
		repo.Create(ctx, order)
	}

	for i := 0; i < 2; i++ {
		order := &model.Order{
			Status:    _type.OrderStatusSuccess,
			Price:     "199.00",
			TgID:      int64(20000 + i),
			AccountID: "account" + string(rune('a'+i)),
		}
		repo.Create(ctx, order)
	}

	_, total, err := repo.GetOrderList(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetOrderList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5 orders, got %d", total)
	}
}

func TestOrderRepository_Create_DifferentPrices(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	prices := []string{"9.99", "19.99", "99.00", "199.00", "999.00"}
	for i, price := range prices {
		order := &model.Order{
			Status:    _type.OrderStatusPending,
			Price:     price,
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
		}
		repo.Create(ctx, order)
	}

	_, total, err := repo.GetOrderList(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetOrderList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestOrderRepository_Create_LargeBatch(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	for i := 0; i < 100; i++ {
		order := &model.Order{
			Status:    _type.OrderStatusPending,
			Price:     "99.00",
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i%10)),
		}
		repo.Create(ctx, order)
	}

	orders, total, err := repo.GetOrderList(ctx, 1, 50)
	if err != nil {
		t.Fatalf("GetOrderList failed: %v", err)
	}

	if len(orders) != 50 {
		t.Errorf("Expected 50 orders, got %d", len(orders))
	}

	if total != 100 {
		t.Errorf("Expected total 100, got %d", total)
	}
}

func TestOrderRepository_GetGetLastOrder_MultipleUsers(t *testing.T) {
	setupOrderTestDB(t)
	ctx := context.Background()

	repo := NewOrderRepository()

	for i := 0; i < 3; i++ {
		order := &model.Order{
			Status:    _type.OrderStatusPending,
			Price:     "99.00",
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
		}
		repo.Create(ctx, order)
	}

	lastOrder, err := repo.GetGetLastOrder(context.Background(), "account0", 10000)
	if err != nil {
		t.Fatalf("GetGetLastOrder failed: %v", err)
	}

	if lastOrder.AccountID != "account0" {
		t.Errorf("Expected account 'account0', got %s", lastOrder.AccountID)
	}
}

