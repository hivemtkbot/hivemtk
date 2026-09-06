// Package model 数据层模型 - 第三方对接模板
//
// 五层架构归属: L3 数据模型层
// 设计依据: L 域 缺口修复 - ERP/CRM 预置对接模板
// 私域独立部署: 无 merchant_id 字段
package model

import (
	"time"
)

// IntegrationTemplate 第三方对接模板
// 字段映射关系：source_field (本地系统) → target_field (外部系统)
// 通过 type + code 唯一标识一份模板
type IntegrationTemplate struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Code       string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"code"`
	Platform   string    `gorm:"type:varchar(32);not null;index" json:"platform"`
	Category   string    `gorm:"type:varchar(32);not null;default:'erp'" json:"category"`
	Name       string    `gorm:"type:varchar(128);not null" json:"name"`
	Version    string    `gorm:"type:varchar(16);not null;default:'1.0.0'" json:"version"`
	APIBase    string    `gorm:"type:varchar(255);not null;default:''" json:"api_base"`
	AuthType   string    `gorm:"type:varchar(32);not null;default:'none'" json:"auth_type"`
	AuthConfig string    `gorm:"type:text;default:'{}'" json:"auth_config"`
	DocURL     string    `gorm:"type:varchar(255);default:''" json:"doc_url"`
	FieldMaps  string    `gorm:"type:text;not null;default:'[]'" json:"field_maps"`
	Endpoints  string    `gorm:"type:text;not null;default:'[]'" json:"endpoints"`
	BuiltIn    bool      `gorm:"default:false;index" json:"is_built_in"`
	Enabled    bool      `gorm:"default:true;index" json:"enabled"`
	Remark     string    `gorm:"type:varchar(500);default:''" json:"remark"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (IntegrationTemplate) TableName() string { return "integration_templates" }

// FieldMapping 字段映射
type FieldMapping struct {
	Source    string            `json:"source"`
	Target    string            `json:"target"`
	Type      string            `json:"type"`
	Required  bool              `json:"required"`
	Default   any               `json:"default,omitempty"`
	Transform string            `json:"transform,omitempty"`
	Options   map[string]string `json:"options,omitempty"`
}

// EndpointConfig 端点配置
type EndpointConfig struct {
	Name        string            `json:"name"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Description string            `json:"description"`
	Headers     map[string]string `json:"headers,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
}

// 预置平台常量
const (
	PlatformDingTalk = "dingtalk"
	PlatformWeCom    = "wecom"
	PlatformFeishu   = "feishu"
	PlatformYonyou   = "yonyou"
	PlatformKingdee  = "kingdee"
	PlatformGrasp    = "grasp"
	PlatformSAP      = "sap"

	CategoryERP     = "erp"
	CategoryCRM     = "crm"
	CategoryHR      = "hr"
	CategoryFinance = "finance"

	AuthTypeNone   = "none"
	AuthTypeAPIKey = "api_key"
	AuthTypeOAuth2 = "oauth2"
	AuthTypeHMAC   = "hmac"
)
