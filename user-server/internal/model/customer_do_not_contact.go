package model

import (
	"time"
)

// DoNotContact 渠道常量：空字符串表示"全渠道"退订（全局标志位）
const (
	// DoNotContactChannelAll 全渠道退订标志位
	DoNotContactChannelAll = ""
)

// DoNotContact 退订来源常量
const (
	// DNCSourceSMSKeyword 用户回复短信退订关键词（TD/退订 等）
	DNCSourceSMSKeyword = "sms_keyword"
	// DNCSourceWebhook 渠道方 webhook 上报的退订事件
	DNCSourceWebhook = "webhook"
	// DNCSourceImport 名单导入（含存量回填）
	DNCSourceImport = "import"
	// DNCSourceManual 人工操作（客服/管理后台）
	DNCSourceManual = "manual"
)

// CustomerDoNotContact 客户全局跨渠道退订标志位
//
// 合规语义（竞品合规调研结论）：
//   - 退订状态必须是【全局跨渠道】的：客户在任何渠道表达退订意愿后，
//     所有渠道的主动触达都必须停止，IsBlocked 先查渠道精确行再查全局行。
//   - OneID 是聚合键：同一自然人的所有渠道身份共享一个 UnifiedID/OneID，
//     因此本表以 one_id 为准，而非 phone/email 等单渠道标识。
//   - 每一次 Block 动作必须留痕（source 记录退订来源 + 服务层审计日志）。
//
// 唯一索引 (one_id, channel)：channel 为空串表示全渠道；
// 同一 (one_id, channel) 重复 Block 幂等（唯一索引冲突跳过）。
type CustomerDoNotContact struct {
	ID uint `gorm:"primarykey" json:"id"`
	// OneID 客户聚合键（customer.unified_id）
	OneID string `gorm:"type:varchar(128);uniqueIndex:idx_dnc_one_channel,priority:1;not null" json:"one_id"`
	// Channel 渠道；空串表示全渠道退订（全局标志位）
	Channel string `gorm:"type:varchar(32);uniqueIndex:idx_dnc_one_channel,priority:2;not null;default:''" json:"channel"`
	// Source 退订来源：sms_keyword / webhook / import / manual
	Source string `gorm:"type:varchar(32);not null;default:'manual'" json:"source"`
	// CreatedAt 标志位写入时间（审计留痕）
	CreatedAt time.Time `json:"created_at"`
}

// TableName 显式指定表名
func (*CustomerDoNotContact) TableName() string {
	return "customer_do_not_contact"
}
