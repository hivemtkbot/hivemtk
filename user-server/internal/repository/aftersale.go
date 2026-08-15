package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// AfterSaleRepository 售后单仓库。
// 售后单是客服侧发起、由电商执行落地的记录；本仓库只负责本地持久化与查询。
type AfterSaleRepository struct {
	db *gorm.DB
}

// NewAfterSaleRepository 创建售后单仓库实例
func NewAfterSaleRepository() *AfterSaleRepository {
	return &AfterSaleRepository{db: _db.GetDB()}
}

// SetDB 注入 db（测试 / 显式装配用）
//
// 五层架构 §三.5 + §七：仓库方法必须首参为 ctx context.Context。
func (r *AfterSaleRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// Create 创建售后单
func (r *AfterSaleRepository) Create(ctx context.Context, as *model.AfterSale) error {
	return r.db.Create(as).Error
}

// Update 更新售后单（电商回写状态时用）
func (r *AfterSaleRepository) Update(ctx context.Context, as *model.AfterSale) error {
	return r.db.Save(as).Error
}

// GetByID 按 ID 获取售后单
func (r *AfterSaleRepository) GetByID(ctx context.Context, id uint) (*model.AfterSale, error) {
	var as model.AfterSale
	if err := r.db.First(&as, id).Error; err != nil {
		return nil, err
	}
	return &as, nil
}

// ListByOrder 按 平台+订单号 列出售后单
func (r *AfterSaleRepository) ListByOrder(ctx context.Context, platform, orderID string) ([]model.AfterSale, error) {
	var list []model.AfterSale
	err := r.db.Where("platform = ? AND order_id = ?", platform, orderID).
		Order("id DESC").Find(&list).Error
	return list, err
}

// ListByCustomer 按客户手机列出售后单
func (r *AfterSaleRepository) ListByCustomer(ctx context.Context, phone string) ([]model.AfterSale, error) {
	var list []model.AfterSale
	q := r.db.Model(&model.AfterSale{})
	if phone != "" {
		q = q.Where("customer_phone = ?", phone)
	}
	err := q.Order("id DESC").Find(&list).Error
	return list, err
}

