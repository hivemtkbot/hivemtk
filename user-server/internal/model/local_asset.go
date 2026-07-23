package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// AssetSource 资产来源
type AssetSource string

const (
	AssetSourcePurchased AssetSource = "purchased"
	AssetSourceManual    AssetSource = "manual"
	AssetSourceSynced    AssetSource = "synced"
	AssetSourceImported  AssetSource = "imported"
)

// LocalAsset 本地资产主表
type LocalAsset struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	AssetID     string         `gorm:"size:64;uniqueIndex;not null" json:"asset_id"`
	AssetType   string         `gorm:"size:32;index;not null" json:"asset_type"`
	Industry    string         `gorm:"size:32;index;not null" json:"industry"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Version     string         `gorm:"size:16;not null" json:"version"`
	Source      AssetSource    `gorm:"size:16;index;default:purchased" json:"source"`
	IsActive    bool           `gorm:"index;default:true" json:"is_active"`
	PurchaseID  *int64         `json:"purchase_id,omitempty"`
	PurchasedAt *time.Time     `json:"purchased_at,omitempty"`
	SyncedAt         time.Time      `json:"synced_at"`
	UseCount         int64          `gorm:"default:0" json:"use_count"`
	ReportedUseCount int64          `gorm:"default:0" json:"reported_use_count"`
	UpdatedAt        time.Time      `json:"updated_at"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (LocalAsset) TableName() string { return "local_assets" }

// LocalAssetData 资产 JSON 数据
type LocalAssetData struct {
	ID           int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	LocalAssetID int64           `gorm:"uniqueIndex;not null" json:"local_asset_id"`
	Data         json.RawMessage `gorm:"type:jsonb;not null" json:"data"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func (LocalAssetData) TableName() string { return "local_asset_data" }

// LocalAssetSyncLog 同步日志
type LocalAssetSyncLog struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AssetID   string    `gorm:"size:64;index;not null" json:"asset_id"`
	Action    string    `gorm:"size:32;not null" json:"action"`
	Status    string    `gorm:"size:16;not null" json:"status"`
	ErrorMsg  string    `gorm:"type:text" json:"error_msg"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (LocalAssetSyncLog) TableName() string { return "local_asset_sync_log" }
