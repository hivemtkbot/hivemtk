package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"marketing/internal/model"
	db "marketing/internal/pkg/utils/db"
)

// ErrNotFound 未找到记录的错误
var ErrNotFound = errors.New("record not found")

// PaymentConfigRepository 支付配置仓库
type PaymentConfigRepository struct{}

// NewPaymentConfigRepository 创建支付配置仓库实例
func NewPaymentConfigRepository() *PaymentConfigRepository {
	return &PaymentConfigRepository{}
}

// GetConfig 获取支付配置(单租户模式)
func (r *PaymentConfigRepository) GetConfig(ctx context.Context) (*model.PaymentConfig, error) {
	var config model.PaymentConfig
	err := db.GetDB().First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &config, nil
}

// Create 创建支付配置
func (r *PaymentConfigRepository) Create(ctx context.Context, config *model.PaymentConfig) error {
	return db.GetDB().Create(config).Error
}

// Update 更新支付配置
func (r *PaymentConfigRepository) Update(ctx context.Context, config *model.PaymentConfig) error {
	return db.GetDB().Save(config).Error
}

// Upsert 存在则更新，不存在则创建
func (r *PaymentConfigRepository) Upsert(ctx context.Context, config *model.PaymentConfig) error {
	var existing model.PaymentConfig
	err := db.GetDB().Where("id > ?", 0).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return db.GetDB().Create(config).Error
		}
		return err
	}
	config.ID = existing.ID
	return db.GetDB().Save(config).Error
}
