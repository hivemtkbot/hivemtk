package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"marketing/internal/domain/asset"
	bizerr "marketing/internal/domain/errors"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// ============================================================================
// 五层架构治理（二轮）：type alias 解决 controller 反向依赖 repository
// ============================================================================
//
// 原架构问题：`LocalAssetService.List` 接受 `repository.LocalAssetFilter`，
// 导致 controller 需 `import repository` 来构造 filter，违反五层架构
// （controller → service → repository，禁止反向引用）。
//
// 修复：定义别名 `LocalAssetFilter = repository.LocalAssetFilter`，
// controller 改为 `service.LocalAssetFilter{...}` 即可，不再 import repository。
// 类型完全等价，无运行时差异。
type LocalAssetFilter = repository.LocalAssetFilter

// LocalAssetService 本地资产业务（同源同构）
type LocalAssetService struct {
	assetRepo      repository.LocalAssetRepository
	dataRepo       repository.LocalAssetDataRepository
	syncLogRepo    repository.LocalAssetSyncLogRepository
	platformClient repository.PlatformAPIClient
	db             *gorm.DB
}

func NewLocalAssetService(
	ar repository.LocalAssetRepository,
	dr repository.LocalAssetDataRepository,
	sr repository.LocalAssetSyncLogRepository,
	pc repository.PlatformAPIClient,
	db *gorm.DB,
) *LocalAssetService {
	return &LocalAssetService{assetRepo: ar, dataRepo: dr, syncLogRepo: sr, platformClient: pc, db: db}
}

// CreateAssetInput 自建输入
type CreateAssetInput struct {
	AssetID   string          `json:"asset_id"`
	AssetType string          `json:"asset_type"`
	Industry  string          `json:"industry"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
}

// UpdateAssetInput 更新输入
type UpdateAssetInput struct {
	AssetType string          `json:"asset_type"`
	Industry  string          `json:"industry"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
}

// PurchaseAndSync 购买并同步
func (s *LocalAssetService) PurchaseAndSync(ctx context.Context, platformAssetID string) error {
	existing, err := s.assetRepo.FindByAssetID(ctx, platformAssetID)
	if err == nil && existing != nil {
		return bizerr.New(bizerr.CodeAssetDup, "资产已存在,请直接点击「同步到最新版本」")
	}

	if err := s.platformClient.Purchase(ctx, platformAssetID); err != nil {
		return bizerr.Wrap(bizerr.CodePlatformUnavail, "平台购买失败", err)
	}

	payload, err := s.platformClient.PullData(ctx, platformAssetID)
	if err != nil {
		return bizerr.Wrap(bizerr.CodeSyncFailed, "拉取平台数据失败", err)
	}

	if err := asset.ValidateAssetData(asset.AssetType(payload.AssetType), payload.Data); err != nil {
		return bizerr.Wrap(bizerr.CodeAssetInvalid, "平台数据校验失败", err)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		la := &model.LocalAsset{
			AssetID:     payload.AssetID,
			AssetType:   payload.AssetType,
			Industry:    payload.Industry,
			Name:        payload.Name,
			Version:     payload.Version,
			Source:      model.AssetSourcePurchased,
			IsActive:    true,
			PurchaseID:  &payload.PurchaseID,
			PurchasedAt: &now,
			SyncedAt:    now,
		}
		if err := tx.Create(la).Error; err != nil {
			return bizerr.Wrap(bizerr.CodeInternal, "保存资产主表失败", err)
		}
		lad := &model.LocalAssetData{LocalAssetID: la.ID, Data: payload.Data, UpdatedAt: now}
		if err := tx.Create(lad).Error; err != nil {
			return bizerr.Wrap(bizerr.CodeInternal, "保存资产数据失败", err)
		}
		return tx.Create(&model.LocalAssetSyncLog{
			AssetID: la.AssetID, Action: "purchase_sync", Status: "success",
		}).Error
	})
}

// SyncFromPlatform 同步最新版本
func (s *LocalAssetService) SyncFromPlatform(ctx context.Context, assetID string) error {
	la, err := s.assetRepo.FindByAssetID(ctx, assetID)
	if err != nil {
		return bizerr.Wrap(bizerr.CodeAssetNotFound, "资产不存在", err)
	}
	if la.Source != model.AssetSourcePurchased && la.Source != model.AssetSourceSynced {
		return bizerr.New(bizerr.CodeForbidden, "仅平台来源资产支持同步")
	}

	payload, err := s.platformClient.PullData(ctx, assetID)
	if err != nil {
		_ = s.syncLogRepo.Create(ctx, &model.LocalAssetSyncLog{
			AssetID: assetID, Action: "sync", Status: "failed", ErrorMsg: err.Error(),
		})
		return bizerr.Wrap(bizerr.CodeSyncFailed, "拉取失败", err)
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		la.Version = payload.Version
		la.SyncedAt = time.Now()
		if err := tx.Save(la).Error; err != nil {
			return err
		}
		return tx.Exec(`
			INSERT INTO local_asset_data (local_asset_id, data, updated_at)
			VALUES (?, ?, NOW())
			ON CONFLICT (local_asset_id) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()
		`, la.ID, payload.Data).Error
	})
	if err != nil {
		return err
	}
	return s.syncLogRepo.Create(ctx, &model.LocalAssetSyncLog{
		AssetID: assetID, Action: "sync", Status: "success",
	})
}

// CreateManual 自建资产
func (s *LocalAssetService) CreateManual(ctx context.Context, in *CreateAssetInput) (*model.LocalAsset, error) {
	if !asset.AssetType(in.AssetType).Valid() {
		return nil, bizerr.New(bizerr.CodeParamInvalid, "asset_type 非法")
	}
	if !asset.Industry(in.Industry).Valid() {
		return nil, bizerr.New(bizerr.CodeParamInvalid, "industry 必须是 5 行业之一")
	}
	if err := asset.ValidateAssetData(asset.AssetType(in.AssetType), in.Data); err != nil {
		return nil, bizerr.Wrap(bizerr.CodeAssetInvalid, "资产 JSON 校验失败", err)
	}
	if in.AssetID == "" {
		in.AssetID = "manual-" + uuid.NewString()
	}
	if existing, _ := s.assetRepo.FindByAssetID(ctx, in.AssetID); existing != nil && existing.ID > 0 {
		return nil, bizerr.New(bizerr.CodeAssetDup, "asset_id 已存在")
	}

	la := &model.LocalAsset{
		AssetID:   in.AssetID,
		AssetType: in.AssetType,
		Industry:  in.Industry,
		Name:      in.Name,
		Version:   "1.0.0",
		Source:    model.AssetSourceManual,
		IsActive:  true,
		SyncedAt:  time.Now(),
	}
	if err := s.assetRepo.Create(ctx, la); err != nil {
		return nil, bizerr.Wrap(bizerr.CodeInternal, "创建资产失败", err)
	}
	lad := &model.LocalAssetData{LocalAssetID: la.ID, Data: in.Data, UpdatedAt: time.Now()}
	if err := s.dataRepo.Create(ctx, lad); err != nil {
		return nil, bizerr.Wrap(bizerr.CodeInternal, "保存资产数据失败", err)
	}
	return la, nil
}

// Update 编辑
func (s *LocalAssetService) Update(ctx context.Context, id int64, in *UpdateAssetInput) error {
	la, err := s.assetRepo.FindByID(ctx, id)
	if err != nil {
		return bizerr.Wrap(bizerr.CodeAssetNotFound, "资产不存在", err)
	}
	if !asset.AssetType(in.AssetType).Valid() {
		return bizerr.New(bizerr.CodeParamInvalid, "asset_type 非法")
	}
	if err := asset.ValidateAssetData(asset.AssetType(in.AssetType), in.Data); err != nil {
		return bizerr.Wrap(bizerr.CodeAssetInvalid, "资产 JSON 校验失败", err)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		la.Name = in.Name
		la.AssetType = in.AssetType
		la.Industry = in.Industry
		la.UpdatedAt = time.Now()
		if err := tx.Save(la).Error; err != nil {
			return err
		}
		return tx.Exec(`
			INSERT INTO local_asset_data (local_asset_id, data, updated_at)
			VALUES (?, ?, NOW())
			ON CONFLICT (local_asset_id) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()
		`, la.ID, in.Data).Error
	})
}

// List 列表
//
// 五层架构治理（二轮）：原签名 `f repository.LocalAssetFilter` 强制 controller
// 反向依赖 repository，违反五层架构。改为 service 自有别名（与 repository 类型
// 完全等价），controller 不再 import repository 即可调用。
func (s *LocalAssetService) List(ctx context.Context, f LocalAssetFilter) ([]*model.LocalAsset, int64, error) {
	return s.assetRepo.List(ctx, f)
}

// Get 详情
func (s *LocalAssetService) Get(ctx context.Context, id int64) (*model.LocalAsset, []byte, error) {
	la, err := s.assetRepo.FindByID(ctx, id)
	if err != nil {
		return nil, nil, bizerr.Wrap(bizerr.CodeAssetNotFound, "资产不存在", err)
	}
	lad, err := s.dataRepo.FindByLocalAssetID(ctx, la.ID)
	if err != nil {
		return la, nil, nil
	}
	return la, lad.Data, nil
}

// SoftDelete 软删
func (s *LocalAssetService) SoftDelete(ctx context.Context, id int64) error {
	return s.assetRepo.SoftDelete(ctx, id)
}

// ToggleActive 启停
func (s *LocalAssetService) ToggleActive(ctx context.Context, id int64, active bool) error {
	return s.assetRepo.ToggleActive(ctx, id, active)
}

// GetSyncLog 同步日志
func (s *LocalAssetService) GetSyncLog(ctx context.Context, assetID string, limit int) ([]*model.LocalAssetSyncLog, error) {
	return s.syncLogRepo.List(ctx, assetID, limit)
}

// LoadByType Loader 使用
func (s *LocalAssetService) LoadByType(ctx context.Context, assetType string) ([]*model.LocalAsset, [][]byte, error) {
	list, err := s.assetRepo.ListByTypeAndActive(ctx, assetType)
	if err != nil {
		return nil, nil, err
	}
	var datas [][]byte
	for _, la := range list {
		lad, err := s.dataRepo.FindByLocalAssetID(ctx, la.ID)
		if err == nil {
			datas = append(datas, lad.Data)
		}
	}
	return list, datas, nil
}
