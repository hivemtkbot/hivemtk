package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// ConfigParamRepository 动态参数仓储
type ConfigParamRepository struct {
	db *gorm.DB
}

func NewConfigParamRepository(db *gorm.DB) *ConfigParamRepository {
	return &ConfigParamRepository{db: db}
}

// List 返回全部参数（管理端用）
func (r *ConfigParamRepository) List(ctx context.Context) ([]model.ConfigParam, error) {
	var params []model.ConfigParam
	if err := r.db.WithContext(ctx).Order("param_group").Order("key").Find(&params).Error; err != nil {
		return nil, err
	}
	return params, nil
}

// ListByGroup 按分组返回参数（管理端分组展示）
func (r *ConfigParamRepository) ListByGroup(ctx context.Context, group string) ([]model.ConfigParam, error) {
	var params []model.ConfigParam
	if err := r.db.WithContext(ctx).Where(&model.ConfigParam{Group: group}).Order("key").Find(&params).Error; err != nil {
		return nil, err
	}
	return params, nil
}

// GetByGroupKey 单条查询（Service 层类型化读取的底层）
func (r *ConfigParamRepository) GetByGroupKey(ctx context.Context, group, key string) (*model.ConfigParam, error) {
	var p model.ConfigParam
	if err := r.db.WithContext(ctx).Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateValue 更新参数值（含 actor 审计）
func (r *ConfigParamRepository) UpdateValue(ctx context.Context, group, key, newValue string, actorID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.ConfigParam
		if err := tx.Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
			return err
		}
		if p.ReadOnly {
			return gorm.ErrRecordNotFound
		}
		oldValue := p.Value
		updates := map[string]any{
			"Value":      newValue,
			"updated_by": actorID,
		}
		if err := tx.Model(&p).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Create(&model.ConfigParamAuditLog{
			ParamKey: key,
			OldValue: oldValue,
			NewValue: newValue,
			Action:   "update",
			ActorID:  actorID,
		}).Error
	})
}

// ResetToDefault 重置单条为默认值
func (r *ConfigParamRepository) ResetToDefault(ctx context.Context, group, key string, actorID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.ConfigParam
		if err := tx.Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
			return err
		}
		oldValue := p.Value
		if err := tx.Model(&p).Updates(map[string]any{
			"Value":      p.DefaultValue,
			"updated_by": actorID,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.ConfigParamAuditLog{
			ParamKey: key,
			OldValue: oldValue,
			NewValue: p.DefaultValue,
			Action:   "reset",
			ActorID:  actorID,
		}).Error
	})
}

// BulkResetGroup 整组重置默认值
func (r *ConfigParamRepository) BulkResetGroup(ctx context.Context, group string, actorID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var params []model.ConfigParam
		if err := tx.Where(&model.ConfigParam{Group: group}).Find(&params).Error; err != nil {
			return err
		}
		for _, p := range params {
			if p.ReadOnly || p.Value == p.DefaultValue {
				continue
			}
			oldValue := p.Value
			if err := tx.Model(&p).Update("Value", p.DefaultValue).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.ConfigParamAuditLog{
				ParamKey: p.Key,
				OldValue: oldValue,
				NewValue: p.DefaultValue,
				Action:   "bulk_reset",
				ActorID:  actorID,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AuditLogs 变更日志查询（管理端只读）
func (r *ConfigParamRepository) AuditLogs(ctx context.Context, limit int) ([]model.ConfigParamAuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	var logs []model.ConfigParamAuditLog
	if err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}
