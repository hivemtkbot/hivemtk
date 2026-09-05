package dto

import "time"

// MaterialCategoryRequest 素材分类请求
type CreateMaterialCategoryRequest struct {
	Name        string `json:"name" binding:"required,max=100"`
	Type        string `json:"type" binding:"required,oneof=image video audio file"`
	ParentID    string `json:"parent_id"`
	Icon        string `json:"icon" binding:"omitempty,max=100"`
	Color       string `json:"color" binding:"omitempty,max=20"`
	Sort        int    `json:"sort"`
	Description string `json:"description" binding:"omitempty,max=500"`
	LicenseID   string `json:"license_id"`
	UserID      string `json:"user_id"`
}

type UpdateMaterialCategoryRequest struct {
	Name        string `json:"name" binding:"omitempty,max=100"`
	Type        string `json:"type" binding:"omitempty,oneof=image video audio file"`
	ParentID    string `json:"parent_id"`
	Icon        string `json:"icon" binding:"omitempty,max=100"`
	Color       string `json:"color" binding:"omitempty,max=20"`
	Sort        int    `json:"sort"`
	Description string `json:"description" binding:"omitempty,max=500"`
	Status      string `json:"status"`
}

// MaterialCategoryResponse 素材分类响应
type MaterialCategoryResponse struct {
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	Type          string                     `json:"type"`
	ParentID      string                     `json:"parent_id"`
	Parent        *MaterialCategoryResponse  `json:"parent,omitempty"`
	Icon          string                     `json:"icon"`
	Color         string                     `json:"color"`
	Sort          int                        `json:"sort"`
	Description   string                     `json:"description"`
	LicenseID     string                     `json:"license_id"`
	UserID        string                     `json:"user_id"`
	MaterialCount int                        `json:"material_count"`
	Status        string                     `json:"status"`
	Children      []MaterialCategoryResponse `json:"children,omitempty"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}

// MaterialRequest 素材请求
type CreateMaterialRequest struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type" binding:"required,oneof=image video audio file"`
	CategoryID  string `json:"category_id"`
	URL         string `json:"url" binding:"required"`
	Size        int64  `json:"size" binding:"required"`
	MimeType    string `json:"mime_type" binding:"required"`
	Hash        string `json:"hash"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Duration    int    `json:"duration"`
	Provider    string `json:"provider"`
	StoragePath string `json:"storage_path"`
	LicenseID   string `json:"license_id"`
	UserID      string `json:"user_id"`
	Tags        string `json:"tags"`
	Description string `json:"description"`
}

type UpdateMaterialRequest struct {
	Name        string `json:"name"`
	CategoryID  string `json:"category_id"`
	Tags        string `json:"tags"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// MaterialResponse 素材响应
type MaterialResponse struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Type        string                    `json:"type"`
	CategoryID  string                    `json:"category_id"`
	Category    *MaterialCategoryResponse `json:"category,omitempty"`
	URL         string                    `json:"url"`
	Size        int64                     `json:"size"`
	MimeType    string                    `json:"mime_type"`
	Hash        string                    `json:"hash"`
	Width       int                       `json:"width"`
	Height      int                       `json:"height"`
	Duration    int                       `json:"duration"`
	Provider    string                    `json:"provider"`
	StoragePath string                    `json:"storage_path"`
	LicenseID   string                    `json:"license_id"`
	UserID      string                    `json:"user_id"`
	UsageCount  int                       `json:"usage_count"`
	LastUsedAt  *time.Time                `json:"last_used_at"`
	Status      string                    `json:"status"`
	Tags        string                    `json:"tags"`
	Description string                    `json:"description"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
}

// GetMaterialListResponse 素材列表响应
type GetMaterialListResponse struct {
	List  []MaterialResponse `json:"list"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

// UploadMaterialRequest 上传素材请求
type UploadMaterialRequest struct {
	CategoryID  string `json:"category_id"`
	Name        string `json:"name"`
	Tags        string `json:"tags"`
	Description string `json:"description"`
	LicenseID   string `json:"license_id"`
	UserID      string `json:"user_id"`
}

// MaterialSelectorResponse 素材选择器响应
type MaterialSelectorResponse struct {
	Categories []MaterialCategoryResponse `json:"categories"`
	Materials  []MaterialResponse         `json:"materials"`
	Total      int64                      `json:"total"`
}

// GetMaterialCategoryListResponse 素材分类列表响应
type GetMaterialCategoryListResponse struct {
	List  []MaterialCategoryResponse `json:"list"`
	Total int64                      `json:"total"`
	Page  int                        `json:"page"`
	Limit int                        `json:"limit"`
}

// MaterialStatsResponse 素材统计响应
type MaterialStatsResponse struct {
	TotalMaterials  int64 `json:"total_materials"`
	ImageCount      int64 `json:"image_count"`
	VideoCount      int64 `json:"video_count"`
	AudioCount      int64 `json:"audio_count"`
	FileCount       int64 `json:"file_count"`
	TotalSize       int64 `json:"total_size"`
	TotalUsageCount int64 `json:"total_usage_count"`
	TodayAddedCount int64 `json:"today_added_count"`
	TodayUsageCount int64 `json:"today_usage_count"`
}
