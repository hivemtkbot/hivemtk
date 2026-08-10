package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

type ClueRepository interface {
	Create(ctx context.Context, user *model.Clue) error
	GetByID(ctx context.Context, id uint) (*model.Clue, error)
	GetClueList(ctx context.Context, page int, limit int) ([]*model.Clue, int64, error)
	Delete(ctx context.Context, id string) error
	GetRecentClueList(ctx context.Context) ([]*model.Clue, error)
	GetClueStatistics(ctx context.Context) ([]map[string]any, error)
	GetClueAllList(ctx context.Context, clueType int64) ([]*model.Clue, int64, error)
	ExistsByTypeAndAccount(ctx context.Context, clueType int64, account string) (bool, error) // 添加此方法声明
	// FindByTypeAndAccount 按类型+账号返回已存在线索（不存在返回 nil, nil），用于线索去重后的增量更新
	FindByTypeAndAccount(ctx context.Context, clueType int64, account string) (*model.Clue, error)
	GetDistinctTypes(ctx context.Context) ([]int64, error)
	// UpdateByID 按主键更新指定字段，用于营销流程 update_lead 动作
	UpdateByID(ctx context.Context, id string, updates map[string]any) error
	// ListByAccounts 批量按 account / Name 查询线索（CC- N+1 优化）
	ListByAccounts(ctx context.Context, accounts []string) ([]*model.Clue, error)
	// BatchUpdateInTx 事务内批量按 ID 更新线索字段
	// 单条失败不中断事务（仅跳过该条），返回成功更新的条数与事务提交错误。
	BatchUpdateInTx(ctx context.Context, ids []string, updates map[string]any) (int, error)
}

type clueRepo struct {
	db *gorm.DB
}

func NewClueRepository() ClueRepository {
	return &clueRepo{db: _db.GetDB()}
}

// NewClueRepositoryWithDB 创建指定数据库连接的 ClueRepository 实例（用于测试）
func NewClueRepositoryWithDB(db *gorm.DB) ClueRepository {
	return &clueRepo{db: db}
}

func (r *clueRepo) Create(ctx context.Context, clue *model.Clue) error {
	// 基于 type  account 去重
	var count int64
	db := r.db.WithContext(ctx)
	err := db.Model(&model.Clue{}).Where("type = ? and account = ?", clue.Type, clue.Account).Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("重复数据")
	}
	return db.Create(clue).Error
}

func (r *clueRepo) GetByID(ctx context.Context, id uint) (*model.Clue, error) {
	var smlist model.Clue
	err := r.db.WithContext(ctx).First(&smlist, id).Error
	return &smlist, err
}

func (r *clueRepo) GetClueList(ctx context.Context, page int, limit int) ([]*model.Clue, int64, error) {
	var cluelists []*model.Clue
	var total int64
	db := r.db.WithContext(ctx)
	// 分别查询list 和 total
	err := db.Model(&model.Clue{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = db.Offset((page - 1) * limit).Limit(limit).Find(&cluelists).Error
	if err != nil {
		return nil, 0, err
	}
	return cluelists, total, err
}
func (r *clueRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Clue{}).Error
}

func (r *clueRepo) GetRecentClueList(ctx context.Context) ([]*model.Clue, error) {
	var cluelists []*model.Clue
	// 最近一分钟的订单
	var start_time = time.Now().Add(-time.Hour * 48).Unix()
	var end_time = time.Now().Unix()
	err := r.db.WithContext(ctx).Where("create_time > ? and create_time < ?", start_time, end_time).Order("create_time desc").Find(&cluelists).Error
	return cluelists, err
}

func (r *clueRepo) GetClueStatistics(ctx context.Context) ([]map[string]any, error) {
	var statistics []map[string]any
	db := r.db.WithContext(ctx)
	// 注意：模型 TableName() = "clues" (复数)，必须使用复数表名
	// 兼容两种列名：type (新) 和 clue_type (旧)
	err := db.Raw("SELECT type AS type, COUNT(*) AS total, SUM(is_verify) AS verify_total FROM clues GROUP BY type ORDER BY type").Scan(&statistics).Error
	if err != nil {
		// 兼容旧的 clue_type 列名（迁移未完成时的回退）
		err = db.Raw("SELECT clue_type AS type, COUNT(*) AS total, SUM(is_verify) AS verify_total FROM clues GROUP BY clue_type ORDER BY clue_type").Scan(&statistics).Error
	}
	return statistics, err
}

func (r *clueRepo) GetClueAllList(ctx context.Context, clueType int64) ([]*model.Clue, int64, error) {
	var cluelists []*model.Clue
	var total int64
	db := r.db.WithContext(ctx)
	// 分别查询list 和 total
	err := db.Where("type = ?", clueType).Model(&model.Clue{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = db.Where("type = ?", clueType).Find(&cluelists).Error
	if err != nil {
		return nil, 0, err
	}
	return cluelists, total, nil
}

// ExistsByTypeAndAccount 检查相同类型和账号的线索是否已存在
func (r *clueRepo) ExistsByTypeAndAccount(ctx context.Context, clueType int64, account string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Clue{}).Where("type = ? and account = ?", clueType, account).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// FindByTypeAndAccount 按类型+账号返回已存在线索；不存在返回 (nil, nil)
func (r *clueRepo) FindByTypeAndAccount(ctx context.Context, clueType int64, account string) (*model.Clue, error) {
	var clue model.Clue
	err := r.db.WithContext(ctx).Model(&model.Clue{}).Where("type = ? and account = ?", clueType, account).First(&clue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &clue, nil
}

// GetDistinctTypes 查询数据库中所有不同的线索类型
func (r *clueRepo) GetDistinctTypes(ctx context.Context) ([]int64, error) {
	var types []int64
	err := r.db.WithContext(ctx).Model(&model.Clue{}).Distinct("type").Pluck("type", &types).Error
	return types, err
}

// UpdateByID 按主键更新指定字段（用于营销流程 update_lead 动作）
// updates 为字段名到新值的映射，例如 {"is_verify": 1}
func (r *clueRepo) UpdateByID(ctx context.Context, id string, updates map[string]any) error {
	if id == "" {
		return errors.New("线索 ID 不能为空")
	}
	if len(updates) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).Model(&model.Clue{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("线索不存在或未更新")
	}
	return nil
}

// ListByAccounts 批量按 account / Name（手机号）查询线索（CC- N+1 优化）
//
// 单次 SQL 取代「遍历 6 种 type → GetClueAllList → 内存过滤」模式：
//  1. 主条件：account IN (去重后的手机号 / 邮箱 / accountID 列表)
//  2. 兜底条件：name IN (...) 兼容历史数据中 name 字段实际存储手机号的情形
//
// 命中任一条件即返回，结果按 create_time DESC 排序。
// 入参 accounts 全部为空时返回 (nil, nil)，不查库。
func (r *clueRepo) ListByAccounts(ctx context.Context, accounts []string) ([]*model.Clue, error) {
	if len(accounts) == 0 {
		return nil, nil
	}
	// 去重 + 跳过空串
	unique := make([]string, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	for _, a := range accounts {
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		unique = append(unique, a)
	}
	if len(unique) == 0 {
		return nil, nil
	}
	var clues []*model.Clue
	err := r.db.WithContext(ctx).
		Where("account IN ? OR name IN ?", unique, unique).
		Order("create_time DESC").
		Find(&clues).Error
	if err != nil {
		return nil, err
	}
	return clues, nil
}

// BatchUpdateInTx 事务内批量按 ID 更新线索字段
// 单条失败不中断事务（仅跳过该条），返回成功更新的条数与事务提交错误。
func (r *clueRepo) BatchUpdateInTx(ctx context.Context, ids []string, updates map[string]any) (int, error) {
	count := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			if id == "" {
				continue
			}
			if err := tx.Model(&model.Clue{}).Where("id = ?", id).Updates(updates).Error; err != nil {
				continue
			}
			count++
		}
		return nil
	})
	return count, err
}
