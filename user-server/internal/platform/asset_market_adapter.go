package platform

import (
	"context"

	"hivemtk-user/internal/repository"
)

// AssetMarketClientAdapter 适配 repository.PlatformAPIClient
type AssetMarketClientAdapter struct {
	inner *AssetMarketClient
}

func NewPlatformAPIClient() repository.PlatformAPIClient {
	return &AssetMarketClientAdapter{inner: NewAssetMarketClient()}
}

func (a *AssetMarketClientAdapter) ListAssets(ctx context.Context, assetType, industry string, page, size int) ([]map[string]any, int64, error) {
	return a.inner.ListAssets(ctx, assetType, industry, page, size)
}

func (a *AssetMarketClientAdapter) GetAssetDetail(ctx context.Context, assetID string) (map[string]any, error) {
	return a.inner.GetAssetDetail(ctx, assetID)
}

func (a *AssetMarketClientAdapter) Purchase(ctx context.Context, assetID string) error {
	return a.inner.Purchase(ctx, assetID)
}

func (a *AssetMarketClientAdapter) PullData(ctx context.Context, assetID string) (*repository.PlatformAssetPayload, error) {
	p, err := a.inner.PullData(ctx, assetID)
	if err != nil {
		return nil, err
	}
	return &repository.PlatformAssetPayload{
		AssetID:     p.AssetID,
		AssetType:   p.AssetType,
		Industry:    p.Industry,
		Name:        p.Name,
		Version:     p.Version,
		PurchaseID:  p.PurchaseID,
		SHA256:      p.SHA256,
		Data:        p.Data,
		PurchasedAt: p.PurchasedAt,
	}, nil
}

func (a *AssetMarketClientAdapter) MyPurchases(ctx context.Context) ([]map[string]any, error) {
	return a.inner.MyPurchases(ctx)
}

// ReportUsage 上报资产使用次数到平台（闭环遥测：本地使用 → 平台统计）。
func (a *AssetMarketClientAdapter) ReportUsage(ctx context.Context, assetID string, delta int64) error {
	return a.inner.ReportUsage(ctx, assetID, delta)
}

