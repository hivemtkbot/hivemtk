package repository

import (
	"errors"
	"fmt"
	_db "marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"sync"

	"gorm.io/gorm"
)

// GetDB 返回全局 DB,供非 Repository 上下文(例如 platform 适配器)使用
var (
	dbOnce sync.Once
	dbInst *gorm.DB
)

func GetDB() *gorm.DB {
	if _db.GetDB() != nil {
		return _db.GetDB()
	}
	return dbInst
}

// BaseRepository 泛型基础 Repository
type BaseRepository[T any] struct {
	db *gorm.DB
}

// NewBaseRepository 创建基础 Repository 实例
func NewBaseRepository[T any](db *gorm.DB) *BaseRepository[T] {
	return &BaseRepository[T]{db: db}
}

// Create 创建记录
func (r *BaseRepository[T]) Create(entity *T) error {
	return r.db.Create(entity).Error
}

// Update 更新记录
func (r *BaseRepository[T]) Update(entity *T) error {
	return r.db.Save(entity).Error
}

// Delete 删除记录
func (r *BaseRepository[T]) Delete(id uint) error {
	var entity T
	return r.db.Delete(&entity, id).Error
}

// GetByID 根据 ID 获取记录
func (r *BaseRepository[T]) GetByID(id uint) (*T, error) {
	var entity T
	err := r.db.First(&entity, id).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// GetByIDs 根据 ID 列表获取记录
func (r *BaseRepository[T]) GetByIDs(ids []uint) ([]*T, error) {
	var entities []*T
	err := r.db.Where("id IN ?", ids).Find(&entities).Error
	if err != nil {
		return nil, err
	}
	return entities, nil
}

// GetList 获取列表（支持分页和排序）
func (r *BaseRepository[T]) GetList(page, pageSize int, orderBy string) ([]*T, int64, error) {
	var entities []*T
	var total int64

	query := r.db.Model(new(T))

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	if orderBy == "" {
		orderBy = "created_at DESC"
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order(orderBy).Find(&entities).Error
	if err != nil {
		return nil, 0, err
	}

	return entities, total, nil
}

// GetListByCondition 根据条件获取列表（支持自定义查询条件）
func (r *BaseRepository[T]) GetListByCondition(page, pageSize int, condition map[string]any, orderBy string) ([]*T, int64, error) {
	var entities []*T
	var total int64

	query := r.db.Model(new(T))

	// 添加查询条件
	for key, value := range condition {
		query = query.Where(key+" = ?", value)
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	if orderBy == "" {
		orderBy = "created_at DESC"
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order(orderBy).Find(&entities).Error
	if err != nil {
		return nil, 0, err
	}

	return entities, total, nil
}

// GetListByQuery 根据自定义查询条件获取列表
func (r *BaseRepository[T]) GetListByQuery(page, pageSize int, queryFunc func(*gorm.DB) *gorm.DB, orderBy string) ([]*T, int64, error) {
	var entities []*T
	var total int64

	query := queryFunc(r.db.Model(new(T)))

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	if orderBy == "" {
		orderBy = "created_at DESC"
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order(orderBy).Find(&entities).Error
	if err != nil {
		return nil, 0, err
	}

	return entities, total, nil
}

// Exists 检查记录是否存在
func (r *BaseRepository[T]) Exists(id uint) (bool, error) {
	var entity T
	err := r.db.Select("id").First(&entity, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Count 获取总记录数
func (r *BaseRepository[T]) Count() (int64, error) {
	var count int64
	err := r.db.Model(new(T)).Count(&count).Error
	return count, err
}

// CountByCondition 根据条件获取记录数
func (r *BaseRepository[T]) CountByCondition(condition map[string]any) (int64, error) {
	var count int64
	query := r.db.Model(new(T))
	for key, value := range condition {
		query = query.Where(key+" = ?", value)
	}
	err := query.Count(&count).Error
	return count, err
}

// FindByField 根据字段值查找记录
func (r *BaseRepository[T]) FindByField(field string, value any) ([]*T, error) {
	var entities []*T
	err := r.db.Where(field+" = ?", value).Find(&entities).Error
	if err != nil {
		return nil, err
	}
	return entities, nil
}

// FindOneByField 根据字段值查找单条记录
func (r *BaseRepository[T]) FindOneByField(field string, value any) (*T, error) {
	var entity T
	err := r.db.Where(field+" = ?", value).First(&entity).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// UpdateField 更新单个字段
func (r *BaseRepository[T]) UpdateField(id uint, field string, value any) error {
	var entity T
	return r.db.Model(&entity).Where("id = ?", id).Update(field, value).Error
}

// UpdateFields 更新多个字段
func (r *BaseRepository[T]) UpdateFields(id uint, fields map[string]any) error {
	var entity T
	return r.db.Model(&entity).Where("id = ?", id).Updates(fields).Error
}

// DeleteByField 根据字段值删除记录
func (r *BaseRepository[T]) DeleteByField(field string, value any) error {
	var entity T
	return r.db.Where(field+" = ?", value).Delete(&entity).Error
}

// BatchCreate 批量创建记录
func (r *BaseRepository[T]) BatchCreate(entities []*T) error {
	return r.db.Create(&entities).Error
}

// Transaction 执行事务
func (r *BaseRepository[T]) Transaction(fn func(*gorm.DB) error) error {
	return r.db.Transaction(fn)
}

// DB 获取底层 DB 对象
func (r *BaseRepository[T]) DB() *gorm.DB {
	return r.db
}

// Log 打印日志
func (r *BaseRepository[T]) Log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	logger.Info(msg)
}
