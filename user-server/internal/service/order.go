package service

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_type "hivemtk-user/internal/pkg/utils/type"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

type OrderService struct {
	repo repository.OrderRepository
}

func NewOrderService() *OrderService {
	return &OrderService{repo: repository.NewOrderRepository()}
}

// NewOrderServiceWithDB 创建带 DB 的订单服务（用于测试 / 多 DB 场景）
func NewOrderServiceWithDB(db *gorm.DB) *OrderService {
	return &OrderService{repo: repository.NewOrderRepositoryWithDB(db)}
}

func (s *OrderService) CreateOrder(ctx context.Context, order model.Order) (*model.Order, error) {
	if err := s.repo.Create(ctx, &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// CreateOrderFromRequest 接收 *model.Order,内部补全默认值后入库
func (s *OrderService) CreateOrderFromRequest(ctx context.Context, order *model.Order) (*model.Order, error) {
	if order == nil {
		return nil, errors.New("订单不能为空")
	}
	if order.Status == 0 {
		order.Status = _type.OrderStatusPending
	}
	if err := s.repo.Create(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// GetOrderByID 按字符串 UUID 查询订单
func (s *OrderService) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	return s.repo.GetByStringID(ctx, id)
}

func (s *OrderService) GetOrder(ctx context.Context, id uint) (*model.Order, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *OrderService) GetOrderList(ctx context.Context, page int, limit int) ([]*model.Order, int64, error) {
	return s.repo.GetOrderList(ctx, page, limit)
}

// CancelOrder 取消订单(仅 pending 状态可取消,其他返回错误)
func (s *OrderService) CancelOrder(ctx context.Context, id string, reason string) error {
	order, err := s.repo.GetByStringID(ctx, id)
	if err != nil {
		return err
	}
	if order.Status == _type.OrderStatusSuccess {
		return errors.New("已支付订单不可取消")
	}
	if order.Status == _type.OrderStatusForceClose {
		return errors.New("订单已关闭,无需取消")
	}
	return s.repo.UpdateOrderStatusById(ctx, id, _type.OrderStatusForceClose)
}

func (s *OrderService) DeleteOrder(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *OrderService) GetRecentOrderList(ctx context.Context) ([]*model.Order, error) {
	return s.repo.GetRecentOrderList(ctx)
}

func (s *OrderService) UpdateOrderStatusById(ctx context.Context, id string, status _type.OrderStatusType) error {
	return s.repo.UpdateOrderStatusById(ctx, id, status)
}

// UpdateOrder 更新订单信息
func (s *OrderService) UpdateOrder(ctx context.Context, order *model.Order) error {
	return s.repo.Update(ctx, order)
}
