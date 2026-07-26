package repository

import (
	"context"

	bizerr "marketing/internal/domain/errors"
	"marketing/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LocalAssetFilter 列表过滤
type LocalAssetFilter struct {
	AssetType string
	Industry  string
	Source    string
	IsActive  *bool
	Keyword   string
	Page      int
	Size      int
}

// LocalAssetRepository 本地资产仓储接口
type LocalAssetRepository interface {
	Create(ctx context.Context, m *model.LocalAsset) error
	Update(ctx context.Context, m *model.LocalAsset) error
	SoftDelete(ctx context.Context, id int64) error
	FindByID(ctx context.Context, id int64) (*model.LocalAsset, error)
	FindByAssetID(ctx context.Context, assetID string) (*model.LocalAsset, error)
	List(ctx context.Context, filter LocalAssetFilter) ([]*model.LocalAsset, int64, error)
	ToggleActive(ctx context.Context, id int64, active bool) error
	ListByTypeAndActive(ctx context.Context, assetType string) ([]*model.LocalAsset, error)
	IncrementUseCount(ctx context.Context, id int64, delta int64) error
	SetReportedUseCount(ctx context.Context, id int64, val int64) error
	// AdvanceReportedUseCount 按 delta 累加 reported_use_count
	// delta<=0 时为 no-op（幂等保护）
	AdvanceReportedUseCount(ctx context.Context, id int64, delta int64) error

	// FindActiveAssetIDByType 返回某类型下「生效中」(is_active=true 且未软删) 资产的 asset_id，
	// 按最近同步时间取第一条。不存在时返回 ("", nil)。
	FindActiveAssetIDByType(ctx context.Context, assetType string) (string, error)

	// PurchaseAndSyncTx 封装「购买并同步」事务：upsert 资产主表 + upsert 数据 + 写同步日志。
	// la 由 service 构造（含时间戳），repo 负责持久化；la.ID 在 upsert 命中冲突时由 repo 回填。
	PurchaseAndSyncTx(ctx context.Context, la *model.LocalAsset, data []byte, syncLog *model.LocalAssetSyncLog) error
	// SyncDataTx 封装「同步最新版本」事务：保存资产主表 + upsert 数据 + 写成功日志。
	// la 由 service 预先置入新 Version/SyncedAt；syncLog 由 service 构造，repo 在事务内创建。
	SyncDataTx(ctx context.Context, la *model.LocalAsset, data []byte, syncLog *model.LocalAssetSyncLog) error
	// UpdateWithDataTx 封装「编辑资产」事务：保存资产主表 + upsert 数据。
	// la 由 service 预先置入新 Name/AssetType/Industry/UpdatedAt；data 为新的资产 JSON。
	UpdateWithDataTx(ctx context.Context, la *model.LocalAsset, data []byte) error
}

type localAssetRepo struct {
	db *gorm.DB
}

func NewLocalAssetRepository(db *gorm.DB) LocalAssetRepository {
	return &localAssetRepo{db: db}
}

func (r *localAssetRepo) Create(ctx context.Context, m *model.LocalAsset) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *localAssetRepo) Update(ctx context.Context, m *model.LocalAsset) error {
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *localAssetRepo) SoftDelete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.LocalAsset{}, id).Error
}

func (r *localAssetRepo) FindByID(ctx context.Context, id int64) (*model.LocalAsset, error) {
	var m model.LocalAsset
	err := r.db.WithContext(ctx).First(&m, id).Error
	return &m, err
}

func (r *localAssetRepo) FindByAssetID(ctx context.Context, assetID string) (*model.LocalAsset, error) {
	var m model.LocalAsset
	err := r.db.WithContext(ctx).Where("asset_id = ?", assetID).First(&m).Error
	return &m, err
}

func (r *localAssetRepo) List(ctx context.Context, f LocalAssetFilter) ([]*model.LocalAsset, int64, error) {
	var list []*model.LocalAsset
	var total int64
	q := r.db.WithContext(ctx).Model(&model.LocalAsset{})
	if f.AssetType != "" {
		q = q.Where("asset_type = ?", f.AssetType)
	}
	if f.Industry != "" {
		q = q.Where("industry = ?", f.Industry)
	}
	if f.Source != "" {
		q = q.Where("source = ?", f.Source)
	}
	if f.IsActive != nil {
		q = q.Where("is_active = ?", *f.IsActive)
	}
	if f.Keyword != "" {
		q = q.Where("name ILIKE ?", "%"+f.Keyword+"%")
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Size <= 0 {
		f.Size = 20
	}
	q.Count(&total)
	err := q.Order("synced_at DESC").Offset((f.Page - 1) * f.Size).Limit(f.Size).Find(&list).Error
	return list, total, err
}

func (r *localAssetRepo) ToggleActive(ctx context.Context, id int64, active bool) error {
	return r.db.WithContext(ctx).Model(&model.LocalAsset{}).
		Where("id = ?", id).Update("is_active", active).Error
}

func (r *localAssetRepo) ListByTypeAndActive(ctx context.Context, assetType string) ([]*model.LocalAsset, error) {
	var list []*model.LocalAsset
	err := r.db.WithContext(ctx).Where("asset_type = ? AND is_active = ?", assetType, true).Find(&list).Error
	return list, err
}

func (r *localAssetRepo) IncrementUseCount(ctx context.Context, id int64, delta int64) error {
	return r.db.WithContext(ctx).Model(&model.LocalAsset{}).
		Where("id = ?", id).UpdateColumn("use_count", gorm.Expr("use_count + ?", delta)).Error
}

func (r *localAssetRepo) SetReportedUseCount(ctx context.Context, id int64, val int64) error {
	return r.db.WithContext(ctx).Model(&model.LocalAsset{}).
		Where("id = ?", id).UpdateColumn("reported_use_count", val).Error
}

// AdvanceReportedUseCount 按 delta 累加 reported_use_count
//
// CAS（WHERE reported_use_count = ?）在并发上报场景下会因 use_count 已被
// IncrementUseCount 推进而失效，导致 reported_use_count 永远不前进。
// 这里改用原子累加（UPDATE ... SET reported_use_count = reported_use_count + ?），
// 不依赖 CAS 比较值，并发安全。delta<=0 时为 no-op（幂等保护，避免重复累加）。
//
// 行为：
//   - delta > 0: reported_use_count += delta
//   - delta <= 0: no-op（返回 nil）
func (r *localAssetRepo) AdvanceReportedUseCount(ctx context.Context, id int64, delta int64) error {
	if delta <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.LocalAsset{}).
		Where("id = ?", id).
		UpdateColumn("reported_use_count", gorm.Expr("reported_use_count + ?", delta)).Error
}

// SetReportedUseCountIfMatch 旧 CAS 接口（已废弃）
//
// 警告：当 use_count 被并发 IncrementUseCount 推进后，
// CAS WHERE 条件不命中，reported_use_count 不前进，上报闭环破裂。
// 生产路径已切到 AdvanceReportedUseCount，请勿在新代码中使用此方法。
//
// 行为：UPDATE ... SET reported_use_count = newVal WHERE id = ? AND use_count = expectedUseCount
// 无行命中时不报错（返回 nil），但 reported_use_count 不变
func (r *localAssetRepo) SetReportedUseCountIfMatch(ctx context.Context, id int64, newVal int64, expectedUseCount int64) error {
	return r.db.WithContext(ctx).Model(&model.LocalAsset{}).
		Where("id = ? AND use_count = ?", id, expectedUseCount).
		UpdateColumn("reported_use_count", newVal).Error
}

// FindActiveAssetIDByType 取某类型下「生效中」(is_active=true 且未软删) 资产的 asset_id，
// 按最近同步时间（synced_at DESC）取第一条。不存在时返回 ("", nil)。
func (r *localAssetRepo) FindActiveAssetIDByType(ctx context.Context, assetType string) (string, error) {
	var aid string
	err := r.db.WithContext(ctx).
		Table("local_assets").
		Select("asset_id").
		Where("asset_type = ? AND is_active = ? AND deleted_at IS NULL", assetType, true).
		Order("synced_at DESC").
		Limit(1).
		Scan(&aid).Error
	if err != nil {
		return "", err
	}
	return aid, nil
}

// PurchaseAndSyncTx 封装「购买并同步」事务。
//
// 行为：
//  1. upsert local_assets（含 deleted_at 恢复），避免并发购买触发 UNIQUE(asset_id) 重复键；
//  2. upsert 命中冲突时 GORM 不回填主键，按 asset_id 取回 ID，确保子表关联正确；
//  3. upsert local_asset_data（local_asset_id 唯一）；
//  4. 写入同步日志（purchase_sync / success）。
//
// 错误处理：主表/数据保存失败返回 bizerr.CodeInternal，日志写入返回原始错误。
// la.SyncedAt 同时用作 local_asset_data.UpdatedAt。
func (r *localAssetRepo) PurchaseAndSyncTx(ctx context.Context, la *model.LocalAsset, data []byte, syncLog *model.LocalAssetSyncLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 使用 upsert：已存在的资产（含被软删除的）重新购买时更新并恢复（清空 deleted_at），
		// 同时避免并发购买触发 UNIQUE(asset_id) 重复键导致 500。
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "asset_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"asset_type", "industry", "name", "version", "source",
				"is_active", "purchase_id", "purchased_at", "synced_at", "deleted_at", "updated_at",
			}),
		}).Create(la).Error; err != nil {
			return bizerr.Wrap(bizerr.CodeInternal, "保存资产主表失败", err)
		}
		// upsert 命中冲突时 GORM 不会回填主键，需按 asset_id 取回，确保子表关联正确。
		if la.ID == 0 {
			var got model.LocalAsset
			if ferr := tx.Where("asset_id = ?", la.AssetID).First(&got).Error; ferr == nil {
				la.ID = got.ID
			}
		}
		// local_asset_data.local_asset_id 唯一：重新购买（或恢复软删除）时更新而非重复插入。
		lad := &model.LocalAssetData{LocalAssetID: la.ID, Data: data, UpdatedAt: la.SyncedAt}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "local_asset_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"data", "updated_at"}),
		}).Create(lad).Error; err != nil {
			return bizerr.Wrap(bizerr.CodeInternal, "保存资产数据失败", err)
		}
		return tx.Create(syncLog).Error
	})
}

// SyncDataTx 封装「同步最新版本」事务。
//
// 行为：保存资产主表（la 已由 service 置入新 Version/SyncedAt）
// + upsert local_asset_data（raw SQL ON CONFLICT）。
//
// syncLog 纳入事务，保证资产与日志原子一致（事务失败日志回滚，事务成功日志提交）。
func (r *localAssetRepo) SyncDataTx(ctx context.Context, la *model.LocalAsset, data []byte, syncLog *model.LocalAssetSyncLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(la).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO local_asset_data (local_asset_id, data, updated_at)
			VALUES (?, ?, NOW())
			ON CONFLICT (local_asset_id) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()
		`, la.ID, data).Error; err != nil {
			return err
		}
		return tx.Create(syncLog).Error
	})
}

// UpdateWithDataTx 封装「编辑资产」事务。
//
// 行为：保存资产主表（la 已由 service 置入新
// Name/AssetType/Industry/UpdatedAt）+ upsert local_asset_data（raw SQL ON CONFLICT）。
func (r *localAssetRepo) UpdateWithDataTx(ctx context.Context, la *model.LocalAsset, data []byte) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(la).Error; err != nil {
			return err
		}
		return tx.Exec(`
			INSERT INTO local_asset_data (local_asset_id, data, updated_at)
			VALUES (?, ?, NOW())
			ON CONFLICT (local_asset_id) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()
		`, la.ID, data).Error
	})
}

// LocalAssetDataRepository 资产数据仓储
type LocalAssetDataRepository interface {
	Create(ctx context.Context, m *model.LocalAssetData) error
	FindByLocalAssetID(ctx context.Context, localAssetID int64) (*model.LocalAssetData, error)
	Upsert(ctx context.Context, localAssetID int64, data []byte) error
}

type localAssetDataRepo struct {
	db *gorm.DB
}

func NewLocalAssetDataRepository(db *gorm.DB) LocalAssetDataRepository {
	return &localAssetDataRepo{db: db}
}

func (r *localAssetDataRepo) Create(ctx context.Context, m *model.LocalAssetData) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *localAssetDataRepo) FindByLocalAssetID(ctx context.Context, localAssetID int64) (*model.LocalAssetData, error) {
	var m model.LocalAssetData
	err := r.db.WithContext(ctx).Where("local_asset_id = ?", localAssetID).First(&m).Error
	return &m, err
}

func (r *localAssetDataRepo) Upsert(ctx context.Context, localAssetID int64, data []byte) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO local_asset_data (local_asset_id, data, updated_at)
		VALUES (?, ?, NOW())
		ON CONFLICT (local_asset_id) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()
	`, localAssetID, data).Error
}

// LocalAssetSyncLogRepository 本地资产同步日志仓储（与 integration 的 SyncLogRepository 区分，避免重名冲突）
type LocalAssetSyncLogRepository interface {
	Create(ctx context.Context, m *model.LocalAssetSyncLog) error
	List(ctx context.Context, assetID string, limit int) ([]*model.LocalAssetSyncLog, error)
}

type syncLogRepo struct {
	db *gorm.DB
}

func NewLocalAssetSyncLogRepository(db *gorm.DB) LocalAssetSyncLogRepository {
	return &syncLogRepo{db: db}
}

func (r *syncLogRepo) Create(ctx context.Context, m *model.LocalAssetSyncLog) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *syncLogRepo) List(ctx context.Context, assetID string, limit int) ([]*model.LocalAssetSyncLog, error) {
	var list []*model.LocalAssetSyncLog
	q := r.db.WithContext(ctx).Model(&model.LocalAssetSyncLog{})
	if assetID != "" {
		q = q.Where("asset_id = ?", assetID)
	}
	if limit <= 0 {
		limit = 50
	}
	err := q.Order("created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}
