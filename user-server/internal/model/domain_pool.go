package model

import (
	"time"
)

// DomainPool 域名池模型
// G 域 扩展：健康度评分、自动切换、平台黑名单检测
//
// 健康度评分体系（0-100）：
//   - 100  健康
//   - 80-99 警告（最近一次 HEAD 探测耗时偏高或部分指标轻微下降）
//   - 60-79  风险（连续一次探测失败但未到阈值）
//   - 1-59   严重（连续多次失败、HTTP 非 2xx/3xx、被标记为黑名单）
//   - 0      不可用（连续失败次数超阈值或被风控）
type DomainPool struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Domain    string    `json:"domain" gorm:"size:255;not null;uniqueIndex"` 
	Port      int       `json:"port" gorm:"default:80"`                      
	Purpose   string    `json:"purpose" gorm:"size:500"`                     
	Status    int       `json:"status" gorm:"default:1"`                     
	LastCheck time.Time `json:"last_check"`                                  

	HealthScore int `json:"health_score" gorm:"default:100"` 

	ConsecutiveFailures int `json:"consecutive_failures" gorm:"default:0"`

	DNSResolved bool   `json:"dns_resolved" gorm:"default:false"`
	DNSError    string `json:"dns_error" gorm:"size:500;default:''"`

	LastHTTPStatus int `json:"last_http_status" gorm:"default:0"`
	LastLatencyMs int `json:"last_latency_ms" gorm:"default:0"`

	OnBlacklist   bool      `json:"on_blacklist" gorm:"default:false;index"`   
	BlacklistAt   time.Time `json:"blacklist_at"`                              
	BlacklistNote string    `json:"blacklist_note" gorm:"size:500;default:''"` 

	AutoSwitchEnabled bool       `json:"auto_switch_enabled" gorm:"default:true"` 
	SwitchedAt        *time.Time `json:"switched_at"`                             
	SwitchedFromID    int        `json:"switched_from_id" gorm:"default:0"`       
	IsActive          bool       `json:"is_active" gorm:"default:false;index"`    

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 设置表名
func (DomainPool) TableName() string {
	return "domain_pool"
}

// DomainHealthLog 域名健康度探测日志
// G 域 ：每次探测生成一条日志，便于事后审计与回溯
type DomainHealthLog struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	DomainID  int       `json:"domain_id" gorm:"index;not null"`
	Domain    string    `json:"domain" gorm:"size:255;index"`
	CheckedAt time.Time `json:"checked_at" gorm:"index"`

	DNSOK    bool   `json:"dns_ok"`
	DNSError string `json:"dns_error" gorm:"size:500;default:''"`

	HTTPOk       bool   `json:"http_ok"`
	HTTPStatus   int    `json:"http_status"`
	HTTPLatency  int    `json:"http_latency_ms"`
	HTTPErrorMsg string `json:"http_error_msg" gorm:"size:500;default:''"`

	OnBlacklist  bool   `json:"on_blacklist"`
	BlacklistSrc string `json:"blacklist_source" gorm:"size:200;default:''"`

	HealthScore int    `json:"health_score"`
	ActionTaken string `json:"action_taken" gorm:"size:64;default:''"` 

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 设置表名
func (DomainHealthLog) TableName() string {
	return "domain_health_log"
}

// DomainBlacklist 平台域名黑名单（内置 + 外部维护）
// G 域 ：用于域名健康度探测时快速判定当前域名是否被微信/抖音/快手等封禁
type DomainBlacklist struct {
	ID        int        `json:"id" gorm:"primaryKey;autoIncrement"`
	Domain    string     `json:"domain" gorm:"size:255;not null;uniqueIndex"` 
	Platform  string     `json:"platform" gorm:"size:32;index;default:'all'"` 
	Reason    string     `json:"reason" gorm:"size:500;default:''"`           
	Source    string     `json:"source" gorm:"size:64;default:'manual'"`      
	ExpiresAt *time.Time `json:"expires_at"`                                  
	Active    bool       `json:"active" gorm:"default:true;index"`
	CreatedAt time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 设置表名
func (DomainBlacklist) TableName() string {
	return "domain_blacklist"
}

