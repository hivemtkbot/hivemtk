package dto

// PurchaseRequest 资产市场购买请求
type PurchaseRequest struct {
	AssetID string `json:"asset_id" binding:"required"`
}

// SyncRequest 资产市场同步请求
type SyncRequest struct {
	AssetID string `json:"asset_id" binding:"required"`
}

// ReportUsageRequest 使用次数上报请求
type ReportUsageRequest struct {
	AssetID string `json:"asset_id" binding:"required"`
}

// ToggleActiveRequest 启停请求
type ToggleActiveRequest struct {
	Active bool `json:"active"`
}

