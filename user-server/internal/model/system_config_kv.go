package model

// system_config_kv.go 通用键值对系统配置
//
// 设计动机：
//   业务上有少量"单条 JSON 存整体配置"的需求（如密码策略、UI 偏好等），
//   强行套用 SystemConfig 业务字段会导致频繁加列、影响其它模块。
//   这里提供一个 key → value 形式的 KV 表，单独建表隔离：
//     - key   VARCHAR(100) PRIMARY KEY
//     - value TEXT NOT NULL
//   value 由调用方自行序列化（JSON / TOML / 简单字符串）。
//
// 私域独立部署：无 merchant_id 字段。
//
// 与 model.SystemConfig（站点基础配置）的关系：
//   - SystemConfig：固定字段、强类型，由 SystemConfigRepository 管理
//   - SystemConfigKV：任意 key 的轻量 KV 存储，由 SystemConfigKVRepository 管理
//
// 五层架构归属：L5 数据层。
import "time"

// SystemConfigKV 单条 KV 配置
type SystemConfigKV struct {
	Key       string    `gorm:"column:key;primaryKey;size:100" json:"key"`
	Value     string    `gorm:"column:value;type:text;not null" json:"value"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName 表名
func (SystemConfigKV) TableName() string {
	return "system_config_kv"
}
