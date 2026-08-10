// 拆分自 customer_360.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"strconv"
	"time"
)

func (s *Customer360Service) buildOrderInfoFromMap(userSessions []*model.CustomerSession, orderMap map[string][]*model.Order) *OrderInfo {
	if len(userSessions) == 0 {
		return &OrderInfo{Orders: make([]*OrderItem, 0)}
	}
	accountID := userSessions[0].AccountID
	if accountID == "" {
		return &OrderInfo{Orders: make([]*OrderItem, 0)}
	}
	userOrders := orderMap[accountID]
	if len(userOrders) == 0 {
		return &OrderInfo{Orders: make([]*OrderItem, 0)}
	}
	var totalAmount float64
	lastOrder := userOrders[0] // ListByAccountIDs 已按 create_time DESC 排序
	orderItems := make([]*OrderItem, 0, len(userOrders))
	for _, order := range userOrders {
		amount, _ := strconv.ParseFloat(order.Price, 64)
		totalAmount += amount
		orderItems = append(orderItems, &OrderItem{
			OrderID:     order.ID,
			Amount:      amount,
			Status:      orderStatusToString(order.Status),
			CreatedAt:   time.Unix(order.CreateTime, 0).Format("2006-01-02 15:04:05"),
			ProductName: "平台商品",
		})
	}
	orderInfo := &OrderInfo{
		TotalOrders: int64(len(userOrders)),
		TotalAmount: totalAmount,
		Orders:      orderItems,
	}
	if lastOrder != nil {
		orderInfo.LastOrderID = lastOrder.ID
		orderInfo.LastOrderAt = time.Unix(lastOrder.CreateTime, 0).Format("2006-01-02 15:04:05")
		amount, _ := strconv.ParseFloat(lastOrder.Price, 64)
		orderInfo.LastOrderAmount = amount
	}
	return orderInfo
}

// Customer360ServiceForTest 用于测试的 Customer360Service（公开字段）
type Customer360ServiceForTest struct {
	SessionRepo      *repository.CustomerSessionRepository
	MessageRepo      *repository.SessionMessageRepository
	ClueRepo         repository.ClueRepository
	OrderRepo        repository.OrderRepository
	UnifiedMsgRepo   repository.UnifiedMessageRepository
	UnifiedReplyRepo repository.UnifiedReplyRepository
}

// GetCustomer360 获取客户 360 视图（测试版本）
func (s *Customer360ServiceForTest) GetCustomer360(ctx context.Context, userID string) (*Customer360DTO, error) {
	realService := &Customer360Service{
		sessionRepo:      s.SessionRepo,
		messageRepo:      s.MessageRepo,
		clueRepo:         s.ClueRepo,
		orderRepo:        s.OrderRepo,
		unifiedMsgRepo:   s.UnifiedMsgRepo,
		unifiedReplyRepo: s.UnifiedReplyRepo,
	}
	return realService.GetCustomer360(ctx, userID)
}

// GetCustomerList 获取客户列表（测试版本）
func (s *Customer360ServiceForTest) GetCustomerList(ctx context.Context, page, pageSize int, filters map[string]string) (map[string]*Customer360DTO, int64, error) {
	realService := &Customer360Service{
		sessionRepo:      s.SessionRepo,
		messageRepo:      s.MessageRepo,
		clueRepo:         s.ClueRepo,
		orderRepo:        s.OrderRepo,
		unifiedMsgRepo:   s.UnifiedMsgRepo,
		unifiedReplyRepo: s.UnifiedReplyRepo,
	}
	return realService.GetCustomerList(ctx, page, pageSize, filters)
}
