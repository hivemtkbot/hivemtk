package repository

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

type ClueRepository interface {
	Create(ctx context.Context, user *model.Clue) error
	BatchCreateWithDedup(ctx context.Context, clues []*model.Clue) (successCount, skipCount int64, err error)
	GetByID(ctx context.Context, id uint) (*model.Clue, error)
	GetClueList(ctx context.Context, page int, limit int) ([]*model.Clue, int64, error)
	Delete(ctx context.Context, id string) error
	GetRecentClueList(ctx context.Context) ([]*model.Clue, error)
	GetClueStatistics(ctx context.Context) ([]map[string]any, error)
	GetClueAllList(ctx context.Context, clueType int64) ([]*model.Clue, int64, error)
	ExistsByTypeAndAccount(ctx context.Context, clueType int64, account string) (bool, error)
	FindByTypeAndAccount(ctx context.Context, clueType int64, account string) (*model.Clue, error)
	GetDistinctTypes(ctx context.Context) ([]int64, error)
	UpdateByID(ctx context.Context, id string, updates map[string]any) error
	ListByAccounts(ctx context.Context, accounts []string) ([]*model.Clue, error)
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

// BatchCreateWithDedup 批量创建线索并去重（解决 OPT-ARC-07 N+1）
// 旧实现：每条线索 1 次 SELECT + 1 次 INSERT，N 条 = 2N 次 DB 往返
// 新实现：
//   1. 1 次 SELECT 查询已存在的 (type, account) 对
//   2. 在内存去重 input 中的重复项
//   3. 1 次 Bulk INSERT 新数据
// 总 DB 往返：3 次（与 N 无关）
func (r *clueRepo) BatchCreateWithDedup(ctx context.Context, clues []*model.Clue) (successCount, skipCount int64, err error) {
	if len(clues) == 0 {
		return 0, 0, nil
	}

	// 1) 内存去重 input（保留首次出现）
	seen := make(map[string]struct{}, len(clues))
	deduped := make([]*model.Clue, 0, len(clues))
	for _, c := range clues {
		key := fmt.Sprintf("%d|%s", c.Type, c.Account)
		if _, ok := seen[key]; ok {
			skipCount++
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, c)
	}

	// 2) 1 次 SELECT 查出 DB 已存在的 (type, account)
	keys := make([]string, 0, len(deduped))
	typeKey := make(map[string]struct{}, len(deduped))
	for _, c := range deduped {
		k := fmt.Sprintf("%d|%s", c.Type, c.Account)
		if _, ok := typeKey[k]; !ok {
			keys = append(keys, k)
			typeKey[k] = struct{}{}
		}
	}

	// 提取 (type, account) 对用于批量 EXISTS 查询
	type pair struct {
		Type    int64
		Account string
	}
	pairs := make([]pair, 0, len(deduped))
	for _, c := range deduped {
		pairs = append(pairs, pair{Type: c.Type, Account: c.Account})
	}

	// 一次性查出所有已存在的 (type, account)
	existsSet := make(map[string]struct{})
	if len(pairs) > 0 {
		db := r.db.WithContext(ctx)
		// 使用 GORM 链式 OR 构造 IN 查询
		conds := db.Model(&model.Clue{})
		for i, p := range pairs {
			if i == 0 {
				conds = conds.Where("(type = ? AND account = ?)", p.Type, p.Account)
			} else {
				conds = conds.Or("(type = ? AND account = ?)", p.Type, p.Account)
			}
		}
		type resultRow struct {
			Type    int64
			Account string
		}
		var rows []resultRow
		if err := conds.Distinct("type, account").Find(&rows).Error; err != nil {
			return 0, 0, err
		}
		for _, r := range rows {
			existsSet[fmt.Sprintf("%d|%s", r.Type, r.Account)] = struct{}{}
		}
	}

	// 3) 过滤出真正需要插入的
	toInsert := make([]*model.Clue, 0, len(deduped))
	for _, c := range deduped {
		k := fmt.Sprintf("%d|%s", c.Type, c.Account)
		if _, ok := existsSet[k]; ok {
			skipCount++
			continue
		}
		toInsert = append(toInsert, c)
	}

	if len(toInsert) == 0 {
		return 0, skipCount, nil
	}

	// 4) 1 次 Bulk INSERT（按 100 条/批）
	if err := r.db.WithContext(ctx).CreateInBatches(toInsert, 100).Error; err != nil {
		return 0, skipCount, err
	}
	successCount = int64(len(toInsert))
	return successCount, skipCount, nil
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
	err := db.Raw("SELECT type AS type, COUNT(*) AS total, SUM(is_verify) AS verify_total FROM clues GROUP BY type ORDER BY type").Scan(&statistics).Error
	if err != nil {
		err = db.Raw("SELECT clue_type AS type, COUNT(*) AS total, SUM(is_verify) AS verify_total FROM clues GROUP BY clue_type ORDER BY clue_type").Scan(&statistics).Error
	}
	return statistics, err
}

func (r *clueRepo) GetClueAllList(ctx context.Context, clueType int64) ([]*model.Clue, int64, error) {
	var cluelists []*model.Clue
	var total int64
	db := r.db.WithContext(ctx)
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

