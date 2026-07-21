package repository

import (
	"errors"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

type ClueRepository interface {
	Create(user *model.Clue) error
	GetByID(id uint) (*model.Clue, error)
	GetClueList(page int, limit int) ([]*model.Clue, int64, error)
	Delete(id string) error
	GetRecentClueList() ([]*model.Clue, error)
	GetClueStatistics() ([]map[string]any, error)
	GetClueAllList(clueType int64) ([]*model.Clue, int64, error)
	ExistsByTypeAndAccount(clueType int64, account string) (bool, error) // 添加此方法声明
	GetDistinctTypes() ([]int64, error)
	// UpdateByID 按主键更新指定字段，用于营销流程 update_lead 动作
	UpdateByID(id string, updates map[string]any) error
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

func (r *clueRepo) Create(clue *model.Clue) error {
	// 基于 type  account 去重
	var count int64
	err := r.db.Model(&model.Clue{}).Where("type = ? and account = ?", clue.Type, clue.Account).Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("重复数据")
	}
	return r.db.Create(clue).Error
}

func (r *clueRepo) GetByID(id uint) (*model.Clue, error) {
	var smlist model.Clue
	err := r.db.First(&smlist, id).Error
	return &smlist, err
}

func (r *clueRepo) GetClueList(page int, limit int) ([]*model.Clue, int64, error) {
	var cluelists []*model.Clue
	var total int64
	// 分别查询list 和 total
	err := r.db.Model(&model.Clue{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = r.db.Offset((page - 1) * limit).Limit(limit).Find(&cluelists).Error
	if err != nil {
		return nil, 0, err
	}
	return cluelists, total, err
}
func (r *clueRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Clue{}).Error
}

func (r *clueRepo) GetRecentClueList() ([]*model.Clue, error) {
	var cluelists []*model.Clue
	// 最近一分钟的订单
	var start_time = time.Now().Add(-time.Hour * 48).Unix()
	var end_time = time.Now().Unix()
	err := r.db.Where("create_time > ? and create_time < ?", start_time, end_time).Order("create_time desc").Find(&cluelists).Error
	return cluelists, err
}

func (r *clueRepo) GetClueStatistics() ([]map[string]any, error) {
	var statistics []map[string]any
	// 注意：模型 TableName() = "clues" (复数)，必须使用复数表名
	// 兼容两种列名：type (新) 和 clue_type (旧)
	err := r.db.Raw("SELECT type AS type, COUNT(*) AS total, SUM(is_verify) AS verify_total FROM clues GROUP BY type ORDER BY type").Scan(&statistics).Error
	if err != nil {
		// 兼容旧的 clue_type 列名（迁移未完成时的回退）
		err = r.db.Raw("SELECT clue_type AS type, COUNT(*) AS total, SUM(is_verify) AS verify_total FROM clues GROUP BY clue_type ORDER BY clue_type").Scan(&statistics).Error
	}
	return statistics, err
}

func (r *clueRepo) GetClueAllList(clueType int64) ([]*model.Clue, int64, error) {
	var cluelists []*model.Clue
	var total int64
	// 分别查询list 和 total
	err := r.db.Where("type = ?", clueType).Model(&model.Clue{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = r.db.Where("type = ?", clueType).Find(&cluelists).Error
	if err != nil {
		return nil, 0, err
	}
	return cluelists, total, nil
}

// ExistsByTypeAndAccount 检查相同类型和账号的线索是否已存在
func (r *clueRepo) ExistsByTypeAndAccount(clueType int64, account string) (bool, error) {
	var count int64
	err := r.db.Model(&model.Clue{}).Where("type = ? and account = ?", clueType, account).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetDistinctTypes 查询数据库中所有不同的线索类型
func (r *clueRepo) GetDistinctTypes() ([]int64, error) {
	var types []int64
	err := r.db.Model(&model.Clue{}).Distinct("type").Pluck("type", &types).Error
	return types, err
}

// UpdateByID 按主键更新指定字段（用于营销流程 update_lead 动作）
// updates 为字段名到新值的映射，例如 {"is_verify": 1}
func (r *clueRepo) UpdateByID(id string, updates map[string]any) error {
	if id == "" {
		return errors.New("线索 ID 不能为空")
	}
	if len(updates) == 0 {
		return nil
	}
	result := r.db.Model(&model.Clue{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("线索不存在或未更新")
	}
	return nil
}
