package model

import "time"

// OTAVersion OTA 版本发布
type OTAVersion struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Version        string     `gorm:"type:varchar(50);not null;index" json:"version"`
	Name           string     `gorm:"type:varchar(200);not null" json:"name"`
	BuildNumber    int        `gorm:"default:0" json:"build_number"`
	Description    string     `gorm:"type:text" json:"description"`
	ReleaseNotes   string     `gorm:"type:text" json:"release_notes"`
	IsForceUpdate  bool       `gorm:"default:false" json:"is_force_update"`
	MinVersion     string     `gorm:"type:varchar(50)" json:"min_version"`
	DownloadURL    string     `gorm:"type:varchar(500)" json:"download_url"`
	FrontendZipURL string     `gorm:"type:varchar(500)" json:"frontend_zip_url"`
	FileSize       int64      `gorm:"default:0" json:"file_size"`
	FileHash       string     `gorm:"type:varchar(128)" json:"file_hash"`
	Status         string     `gorm:"type:varchar(20);default:'draft';index" json:"status"` // draft/published/rolled_back/archived
	RollbackFromID uint       `json:"rollback_from_id"`
	PublishedAt    *time.Time `json:"published_at"`
	CreatedBy      uint       `json:"created_by"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (OTAVersion) TableName() string { return "ota_versions" }
