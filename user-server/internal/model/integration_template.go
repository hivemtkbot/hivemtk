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
	Code       string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"code"`       // 模板唯一编码
	Platform   string    `gorm:"type:varchar(32);not null;index" json:"platform"`         // dingtalk/wecom/feishu/yonyou/kingdee/grasp/sap
	Category   string    `gorm:"type:varchar(32);not null;default:'erp'" json:"category"` // erp / crm / hr / finance
	Name       string    `gorm:"type:varchar(128);not null" json:"name"`
	Version    string    `gorm:"type:varchar(16);not null;default:'1.0.0'" json:"version"`
	APIBase    string    `gorm:"type:varchar(255);not null;default:''" json:"api_base"`     // API 基础地址
	AuthType   string    `gorm:"type:varchar(32);not null;default:'none'" json:"auth_type"` // none / api_key / oauth2 / hmac
	AuthConfig string    `gorm:"type:text;default:'{}'" json:"auth_config"`                 // 认证配置 JSON
	DocURL     string    `gorm:"type:varchar(255);default:''" json:"doc_url"`               // 官方文档链接
	FieldMaps  string    `gorm:"type:text;not null;default:'[]'" json:"field_maps"`         // 字段映射 JSON
	Endpoints  string    `gorm:"type:text;not null;default:'[]'" json:"endpoints"`          // 端点配置 JSON
	BuiltIn    bool      `gorm:"default:false;index" json:"is_built_in"`                    // 是否系统预置
	Enabled    bool      `gorm:"default:true;index" json:"enabled"`
	Remark     string    `gorm:"type:varchar(500);default:''" json:"remark"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (IntegrationTemplate) TableName() string { return "integration_templates" }

// FieldMapping 字段映射
type FieldMapping struct {
	Source    string            `json:"source"`   // 本地字段名（如 customer.name）
	Target    string            `json:"target"`   // 外部字段名（如 Name）
	Type      string            `json:"type"`     // string/number/bool/date
	Required  bool              `json:"required"` // 是否必填
	Default   any               `json:"default,omitempty"`
	Transform string            `json:"transform,omitempty"` // transform 表达式或函数名
	Options   map[string]string `json:"options,omitempty"`   // 枚举映射
}

// EndpointConfig 端点配置
type EndpointConfig struct {
	Name        string            `json:"name"`   // 端点名称
	Method      string            `json:"method"` // GET/POST/PUT/DELETE
	Path        string            `json:"path"`   // 路径
	Description string            `json:"description"`
	Headers     map[string]string `json:"headers,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
}

// 预置平台常量
const (
	PlatformDingTalk = "dingtalk" // 钉钉
	PlatformWeCom    = "wecom"    // 企业微信
	PlatformFeishu   = "feishu"   // 飞书
	PlatformYonyou   = "yonyou"   // 用友 (U8/YonBIP)
	PlatformKingdee  = "kingdee"  // 金蝶 (K3Cloud/EAS)
	PlatformGrasp    = "grasp"    // 管家婆
	PlatformSAP      = "sap"      // SAP (S/4HANA)

	CategoryERP     = "erp"
	CategoryCRM     = "crm"
	CategoryHR      = "hr"
	CategoryFinance = "finance"

	AuthTypeNone   = "none"
	AuthTypeAPIKey = "api_key"
	AuthTypeOAuth2 = "oauth2"
	AuthTypeHMAC   = "hmac"
)
