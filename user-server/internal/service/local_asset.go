package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"marketing/internal/domain/asset"
	bizerr "marketing/internal/domain/errors"
	"marketing/internal/model"
	"marketing/internal/repository"
)

type LocalAssetFilter = repository.LocalAssetFilter

// LocalAssetService 本地资产业务（同源同构）
type LocalAssetService struct {
	assetRepo      repository.LocalAssetRepository
	dataRepo       repository.LocalAssetDataRepository
	syncLogRepo    repository.LocalAssetSyncLogRepository
	platformClient repository.PlatformAPIClient
}

func NewLocalAssetService(
	ar repository.LocalAssetRepository,
	dr repository.LocalAssetDataRepository,
	sr repository.LocalAssetSyncLogRepository,
	pc repository.PlatformAPIClient,
	db *gorm.DB,
) *LocalAssetService {
	// 事务由 repository 层管理，service 不持有 *gorm.DB；db 参数仅用于签名兼容。
	_ = db
	return &LocalAssetService{assetRepo: ar, dataRepo: dr, syncLogRepo: sr, platformClient: pc}
}

// CreateAssetInput 自建输入
type CreateAssetInput struct {
	AssetID   string          `json:"asset_id"`
	AssetType string          `json:"asset_type"`
	Industry  string          `json:"industry"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
}

// platformNameIntoData 把平台 payload 顶层的 name 合并进 data 对象。
// 平台侧约定 name 位于 payload 顶层、data 仅含类型内容；而本地校验与前端自建
// 资产均要求 data 含 name。这里做一次归一化，避免购买/同步时校验失败。
func platformNameIntoData(raw json.RawMessage, name string) json.RawMessage {
	if name == "" {
		return raw
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	if _, ok := m["name"]; ok {
		return raw // 已含 name，不覆盖
	}
	nameJSON, err := json.Marshal(name)
	if err != nil {
		return raw
	}
	m["name"] = nameJSON
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
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
	// 平台返回的 data 不含 name（name 位于 payload 顶层），而本地校验约定 data 含
	// name（与前端自建资产一致）。把顶层 name 合并进 data，避免校验失败并保持数据
	// 结构与自建资产一致。
	payload.Data = platformNameIntoData(payload.Data, payload.Name)

	if err := asset.ValidateAssetData(asset.AssetType(payload.AssetType), payload.Data); err != nil {
		return bizerr.Wrap(bizerr.CodeAssetInvalid, "平台数据校验失败", err)
	}

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
	syncLog := &model.LocalAssetSyncLog{
		AssetID: la.AssetID, Action: "purchase_sync", Status: "success",
	}
	return s.assetRepo.PurchaseAndSyncTx(ctx, la, payload.Data, syncLog)
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
	// 与购买同步保持一致：顶层 name 合并进 data。
	payload.Data = platformNameIntoData(payload.Data, payload.Name)

	la.Version = payload.Version
	la.SyncedAt = time.Now()
	syncLog := &model.LocalAssetSyncLog{
		AssetID: assetID, Action: "sync", Status: "success",
	}
	if err := s.assetRepo.SyncDataTx(ctx, la, payload.Data, syncLog); err != nil {
		return err
	}
	return nil
}

// CreateManual 自建资产
func (s *LocalAssetService) CreateManual(ctx context.Context, in *CreateAssetInput) (*model.LocalAsset, error) {
	if !asset.AssetType(in.AssetType).Valid() {
		return nil, bizerr.New(bizerr.CodeParamInvalid, "asset_type 非法")
	}
	if !asset.Industry(in.Industry).Valid() {
		return nil, bizerr.New(bizerr.CodeParamInvalid, "industry 不在支持列表内")
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
	la.Name = in.Name
	la.AssetType = in.AssetType
	la.Industry = in.Industry
	la.UpdatedAt = time.Now()
	return s.assetRepo.UpdateWithDataTx(ctx, la, in.Data)
}

// List 列表
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
		// 闭环：每次运行时被实际加载使用，累加本地使用次数，并 best-effort 回传平台。
		if la.PurchaseID != nil {
			_ = s.assetRepo.IncrementUseCount(ctx, la.ID, 1)
			go s.reportUsageAsync(la.AssetID)
		}
	}
	return list, datas, nil
}

// LoadOne 按 AssetID 加载单个已同步本地资产及其 ChatML 数据（闭环：运行时消费）。
//
// 实现 asset_bundle.LocalAssetLoader 接口，供 AssetBundleService.ResolveSystemPrompt
// 在 asset_bundles 未命中时回退使用，闭合「平台下发资产包 → 商户同步 → 运行时被
// 智能体消费」全链路。同时累加使用次数并 best-effort 异步上报用量到平台（telemetry）。
// 资产不存在时返回 (nil, nil, nil)。
func (s *LocalAssetService) LoadOne(ctx context.Context, assetID string) (*model.LocalAsset, []byte, error) {
	la, err := s.assetRepo.FindByAssetID(ctx, assetID)
	if err != nil || la == nil {
		return nil, nil, nil
	}
	lad, err := s.dataRepo.FindByLocalAssetID(ctx, la.ID)
	if err != nil || lad == nil {
		return la, nil, nil
	}
	// telemetry：仅平台来源资产回传用量到平台
	if la.PurchaseID != nil {
		_ = s.assetRepo.IncrementUseCount(ctx, la.ID, 1)
		go s.reportUsageAsync(la.AssetID)
	}
	return la, lad.Data, nil
}

// reportingInFlight 防止并发重复上报同一资产（避免平台重复计数）。
var reportingInFlight sync.Map

// ReportUsage 将本地累计的使用次数回传平台（闭环：本地使用 → 平台统计）。
func (s *LocalAssetService) ReportUsage(ctx context.Context, assetID string) error {
	la, err := s.assetRepo.FindByAssetID(ctx, assetID)
	if err != nil {
		return bizerr.New(bizerr.CodeNotFound, "资产不存在")
	}
	if la.PurchaseID == nil {
		return bizerr.New(bizerr.CodeForbidden, "非平台购买资产无需上报")
	}
	delta := la.UseCount - la.ReportedUseCount
	if delta <= 0 {
		return nil
	}
	if err := s.platformClient.ReportUsage(ctx, assetID, delta); err != nil {
		return bizerr.Wrap(bizerr.CodePlatformUnavail, "平台上报失败", err)
	}
	return s.assetRepo.SetReportedUseCount(ctx, la.ID, la.UseCount)
}

// reportUsageAsync 后台 best-effort 上报，避免阻塞运行时主流程。
func (s *LocalAssetService) reportUsageAsync(assetID string) {
	if _, loaded := reportingInFlight.LoadOrStore(assetID, true); loaded {
		return
	}
	defer reportingInFlight.Delete(assetID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.ReportUsage(ctx, assetID)
}
