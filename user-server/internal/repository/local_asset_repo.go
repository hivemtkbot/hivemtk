package repository

import (
	"context"

	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/model"

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
	AdvanceReportedUseCount(ctx context.Context, id int64, delta int64) error

	FindActiveAssetIDByType(ctx context.Context, assetType string) (string, error)

	PurchaseAndSyncTx(ctx context.Context, la *model.LocalAsset, data []byte, syncLog *model.LocalAssetSyncLog) error
	SyncDataTx(ctx context.Context, la *model.LocalAsset, data []byte, syncLog *model.LocalAssetSyncLog) error
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

func (r *localAssetRepo) AdvanceReportedUseCount(ctx context.Context, id int64, delta int64) error {
	if delta <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.LocalAsset{}).
		Where("id = ?", id).
		UpdateColumn("reported_use_count", gorm.Expr("reported_use_count + ?", delta)).Error
}

func (r *localAssetRepo) SetReportedUseCountIfMatch(ctx context.Context, id int64, newVal int64, expectedUseCount int64) error {
	return r.db.WithContext(ctx).Model(&model.LocalAsset{}).
		Where("id = ? AND use_count = ?", id, expectedUseCount).
		UpdateColumn("reported_use_count", newVal).Error
}

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

func (r *localAssetRepo) PurchaseAndSyncTx(ctx context.Context, la *model.LocalAsset, data []byte, syncLog *model.LocalAssetSyncLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "asset_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"asset_type", "industry", "name", "version", "source",
				"is_active", "purchase_id", "purchased_at", "synced_at", "deleted_at", "updated_at",
			}),
		}).Create(la).Error; err != nil {
			return bizerr.Wrap(bizerr.CodeInternal, "保存资产主表失败", err)
		}
		if la.ID == 0 {
			var got model.LocalAsset
			if ferr := tx.Where("asset_id = ?", la.AssetID).First(&got).Error; ferr == nil {
				la.ID = got.ID
			}
		}
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
