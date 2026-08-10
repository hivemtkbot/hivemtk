package service

import (
	"context"
	"encoding/json"
	"hivemtk-user/internal/platform"
)

// SubmitToPlatform 将本地调试好的资产包提交到平台端审核上架（开发者上架链路）。
// 数据流：user-server -> platform-server(contributor-api)，符合资产包三端职责（开发者产出上交平台）。
func (s *AssetBundleService) SubmitToPlatform(ctx context.Context, assetID string) (int64, error) {
	bundle, err := s.GetBundleByAssetID(ctx, assetID)
	if err != nil {
		return 0, err
	}
	clean := make([]map[string]string, 0, len(bundle.Messages))
	for _, m := range bundle.Messages {
		clean = append(clean, map[string]string{"role": m.Role, "content": m.Content})
	}
	data, err := json.Marshal(clean)
	if err != nil {
		return 0, err
	}
	cc := platform.NewContributorClient()
	platformAssetID, err := cc.CreateAsset(platform.CreateAssetPayload{
		AssetType:   "bundle",
		Industry:    bundle.Industry,
		Name:        bundle.Title,
		Description: bundle.Description,
		Version:     bundle.Version,
		Changelog:   "from merchant bundle " + assetID,
		Data:        data,
	})
	if err != nil {
		return 0, err
	}
	if err := cc.SubmitAudit(platformAssetID); err != nil {
		return 0, err
	}
	return platformAssetID, nil
}
