package model

// SystemConfig 系统配置模型
type SystemConfig struct {
	ID                   uint   `gorm:"primaryKey" json:"id"`
	Name                 string `gorm:"size:255;not null" json:"site_name"`         
	WebsiteURL           string `gorm:"size:255" json:"website_url"`                
	LogoURL              string `gorm:"size:500" json:"logo_url"`                   
	ThemeColor           string `gorm:"size:20" json:"theme_color"`                 
	SEOKeywords          string `gorm:"size:500" json:"seo_keywords"`               
	SEODescription       string `gorm:"type:text" json:"seo_description"`           
	ServicePhone         string `gorm:"size:50" json:"service_phone"`               
	ServiceEmail         string `gorm:"size:100" json:"service_email"`              
	ICPRecord            string `gorm:"size:100" json:"icp_record"`                 
	PoliceRecord         string `gorm:"size:100" json:"police_record"`              
	EnableRegister       bool   `gorm:"default:true" json:"enable_register"`        
	EnableEmailMarketing bool   `gorm:"default:true" json:"enable_email_marketing"` 
	EnableRAG            bool   `gorm:"default:true" json:"enable_rag"`             
	MaintenanceMode      bool   `gorm:"default:false" json:"maintenance_mode"`      
	MaxUsers          int  `gorm:"default:0" json:"max_users"`
	MaxUploadSizeMB   int  `gorm:"default:50" json:"max_upload_size_mb"`    
}

func (*SystemConfig) TableName() string {
	return "system_config"
}

