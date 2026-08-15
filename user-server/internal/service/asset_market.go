package service

import (
	"context"

	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/repository"
)

// AssetMarketService 资产市场代理服务
type AssetMarketService struct {
	platformClient repository.PlatformAPIClient
}

func NewAssetMarketService(pc repository.PlatformAPIClient) *AssetMarketService {
	return &AssetMarketService{platformClient: pc}
}

func (s *AssetMarketService) ListMarketAssets(ctx context.Context, assetType, industry string, page, size int) ([]map[string]any, int64, error) {
	list, total, err := s.platformClient.ListAssets(ctx, assetType, industry, page, size)
	if err != nil {
		return nil, 0, bizerr.Wrap(bizerr.CodePlatformUnavail, "获取市场列表失败", err)
	}
	return list, total, nil
}

func (s *AssetMarketService) GetMarketAssetDetail(ctx context.Context, assetID string) (map[string]any, error) {
	detail, err := s.platformClient.GetAssetDetail(ctx, assetID)
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodePlatformUnavail, "获取资产详情失败", err)
	}
	return detail, nil
}

func (s *AssetMarketService) MyPurchases(ctx context.Context) ([]map[string]any, error) {
	list, err := s.platformClient.MyPurchases(ctx)
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodePlatformUnavail, "获取已购列表失败", err)
	}
	return list, nil
}

