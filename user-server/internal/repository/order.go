package repository

import (
	"context"
	"marketing/internal/model"
	_type "marketing/internal/pkg/utils/type"

	_db "marketing/internal/pkg/utils/db"

	"time"

	"gorm.io/gorm"
)

type OrderRepository interface {
	Create(ctx context.Context, order *model.Order) error
	GetByID(ctx context.Context, id uint) (*model.Order, error)
	GetByStringID(ctx context.Context, id string) (*model.Order, error)
	GetOrderList(ctx context.Context, page int, limit int) ([]*model.Order, int64, error)
	Delete(ctx context.Context, id string) error
	LastOrderIsPay(ctx context.Context, account_ID string, tgID int64) bool
	GetGetLastOrder(ctx context.Context, account_ID string, tgID int64) (*model.Order, error)
	UpdateOrderStatusById(ctx context.Context, id string, status _type.OrderStatusType) error
	GetRecentOrderList(ctx context.Context) ([]*model.Order, error)
	GetByTgID(ctx context.Context, tgID int64) ([]*model.Order, error)
	GetDistinctPaidTgIDs(ctx context.Context) ([]int64, error)
	Update(ctx context.Context, order *model.Order) error
}

type orderRepo struct {
	db *gorm.DB
}

func NewOrderRepository() OrderRepository {
	return &orderRepo{db: _db.GetDB()}
}

// NewOrderRepositoryWithDB 创建指定数据库连接的 OrderRepository 实例（用于测试）
func NewOrderRepositoryWithDB(db *gorm.DB) OrderRepository {
	return &orderRepo{db: db}
}

func (r *orderRepo) Create(ctx context.Context, order *model.Order) error {
	return r.db.Create(order).Error
}

func (r *orderRepo) GetByID(ctx context.Context, id uint) (*model.Order, error) {
	var order model.Order
	err := r.db.First(&order, id).Error
	return &order, err
}

// GetByStringID 根据 UUID 字符串 ID 查询订单
func (r *orderRepo) GetByStringID(ctx context.Context, id string) (*model.Order, error) {
	var order model.Order
	err := r.db.Where("id = ?", id).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *orderRepo) GetOrderList(ctx context.Context, page int, limit int) ([]*model.Order, int64, error) {
	var orders []*model.Order
	var total int64
	err := r.db.Offset((page - 1) * limit).Limit(limit).Find(&orders).Count(&total).Error
	return orders, total, err
}

func (r *orderRepo) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Order{}).Error
}

func (r *orderRepo) LastOrderIsPay(ctx context.Context, account_ID string, tgID int64) bool {
	var order model.Order
	err := r.db.Where("status = ? and account_id = ? and tg_id = ?", _type.OrderStatusSuccess, account_ID, tgID).Order("create_time desc").First(&order).Error
	if err != nil {
		return false
	}
	if order.Status == _type.OrderStatusSuccess {
		return true
	}
	return false
}

func (r *orderRepo) GetGetLastOrder(ctx context.Context, account_ID string, tgID int64) (*model.Order, error) {
	var order model.Order
	err := r.db.Where("account_id = ? and tg_id = ?", account_ID, tgID).Order("create_time desc").First(&order).Error
	return &order, err
}
func (r *orderRepo) UpdateOrderStatusById(ctx context.Context, id string, status _type.OrderStatusType) error {
	var order model.Order
	err := r.db.First(&order, "id = ?", id).Error
	if err != nil {
		return err
	}
	order.Status = _type.OrderStatusType(status)
	err = r.db.Save(&order).Error
	return err
}

// 获取最近订单
func (r *orderRepo) GetRecentOrderList(ctx context.Context) ([]*model.Order, error) {
	var orders []*model.Order
	// 最近一分钟的订单
	var start_time = time.Now().Add(-time.Minute * 5).Unix()
	var end_time = time.Now().Unix()
	err := r.db.Where("status = ? and create_time > ? and create_time < ?", _type.OrderStatusPending, start_time, end_time).Order("create_time desc").Find(&orders).Error
	return orders, err
}

// GetByTgID 根据 TgID 获取用户所有订单
func (r *orderRepo) GetByTgID(ctx context.Context, tgID int64) ([]*model.Order, error) {
	var orders []*model.Order
	err := r.db.Where("tg_id = ?", tgID).Order("create_time desc").Find(&orders).Error
	return orders, err
}

// GetDistinctPaidTgIDs 获取所有已支付订单的不同 TgID 列表
func (r *orderRepo) GetDistinctPaidTgIDs(ctx context.Context) ([]int64, error) {
	var tgIDs []int64
	err := r.db.Model(&model.Order{}).
		Where("status = ?", _type.OrderStatusSuccess).
		Distinct("tg_id").
		Pluck("tg_id", &tgIDs).Error
	return tgIDs, err
}

// Update 更新订单
func (r *orderRepo) Update(ctx context.Context, order *model.Order) error {
	return r.db.Save(order).Error
}
