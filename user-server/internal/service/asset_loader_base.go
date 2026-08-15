package service

import (
	"encoding/json"
	"log/slog"

	"gorm.io/gorm"
)

// LoadAssetFromDB 通用 DB 加载（优先 local_assets）
func LoadAssetFromDB(db *gorm.DB, assetType, assetID string) ([]byte, bool) {
	if db == nil {
		return nil, false
	}
	var row struct {
		Data json.RawMessage
	}
	err := db.Table("local_assets la").
		Joins("JOIN local_asset_data lad ON lad.local_asset_id = la.id").
		Where("la.asset_id = ? AND la.asset_type = ? AND la.is_active = ? AND la.deleted_at IS NULL", assetID, assetType, true).
		Select("lad.data").
		Scan(&row).Error
	if err != nil {
		slog.Warn("Loader DB error, fallback to default",
			"asset_type", assetType, "asset_id", assetID, "error", err.Error())
		return nil, false
	}
	if len(row.Data) == 0 {
		return nil, false
	}
	return row.Data, true
}

// ListAssetsFromDB 按类型列出激活资产
func ListAssetsFromDB(db *gorm.DB, assetType string) ([]struct {
	AssetID string
	Name    string
	Data    json.RawMessage
}, error) {
	var rows []struct {
		AssetID string
		Name    string
		Data    json.RawMessage
	}
	if db == nil {
		return rows, nil
	}
	err := db.Table("local_assets la").
		Joins("JOIN local_asset_data lad ON lad.local_asset_id = la.id").
		Where("la.asset_type = ? AND la.is_active = ? AND la.deleted_at IS NULL", assetType, true).
		Select("la.asset_id, la.name, lad.data").
		Scan(&rows).Error
	return rows, err
}

