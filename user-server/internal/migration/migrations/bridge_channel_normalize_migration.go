package migrations

// bridge_channel_normalize_migration.go 归一化旧 bridge channel 数据
//
// 背景：早期 bridge 扩展上报的 channel 值可能使用基础渠道名（douyin/xhs/tiktok/xianyu）
// 而非桥接渠道名（douyin_web/xhs_web/tiktok_web/xianyu_web），导致：
//   - 前端 getChannelLabel 无法正确映射（返回原始值如 "douyin" 而非 "抖音"）
//   - webhook 出站 switch 匹配不到桥接渠道，AI 回复被静默丢弃
//   - channel_agent_bindings 查询不到对应绑定
//
// 本迁移将以下表中的旧基础渠道值归一化为桥接渠道值：
//   - bridge_accounts.channel
//   - channel_agent_bindings.channel_type
//   - message_hub.channel（仅当 Extra 包含 bridge:true 标记时）

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// baseToBridge 旧基础渠道 -> 桥接渠道映射
// 注意：tiktok 不再需要转换（bridge 渠道直接使用 tiktok 而非 tiktok_web）
var baseToBridge = map[string]string{
	"douyin":      "douyin_web",
	"xhs":         "xhs_web",
	"xianyu":      "xianyu_web",
	"kuaishou":    "kuaishou_web",
	"xiaohongshu": "xhs_web",
}

type BridgeChannelNormalizeMigration struct {
	db *gorm.DB
}

func NewBridgeChannelNormalizeMigration(db *gorm.DB) *BridgeChannelNormalizeMigration {
	return &BridgeChannelNormalizeMigration{db: db}
}

// 注：v3.17.0 已被 v3_17_drop_rag_alerts_migration 占用，本迁移排在之后（且在 unify v3.18.0 之前），
// 故用 v3.17.1 避免注册表 map 覆盖导致本迁移被静默丢弃。
func (m *BridgeChannelNormalizeMigration) Version() string { return "v3.17.1" }
func (m *BridgeChannelNormalizeMigration) Name() string    { return "归一化旧 bridge channel 数据" }
func (m *BridgeChannelNormalizeMigration) Description() string {
	return "将 bridge_accounts / channel_agent_bindings / message_hub 中的旧基础渠道值（douyin/xhs/xianyu）归一化为桥接渠道值（douyin_web/xhs_web/xianyu_web）"
}

func (m *BridgeChannelNormalizeMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 1) bridge_accounts.channel: douyin -> douyin_web 等
	for base, bridge := range baseToBridge {
		result := m.db.WithContext(ctx).
			Table("bridge_accounts").
			Where("channel = ?", base).
			Update("channel", bridge)
		if result.Error != nil {
			return fmt.Errorf("bridge_accounts 归一化 %s -> %s 失败: %w", base, bridge, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration] bridge_accounts: %d 条记录 %s -> %s\n", result.RowsAffected, base, bridge)
		}
	}

	// 2) channel_agent_bindings.channel_type: 同理
	for base, bridge := range baseToBridge {
		result := m.db.WithContext(ctx).
			Table("channel_agent_bindings").
			Where("channel_type = ?", base).
			Update("channel_type", bridge)
		if result.Error != nil {
			return fmt.Errorf("channel_agent_bindings 归一化 %s -> %s 失败: %w", base, bridge, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration] channel_agent_bindings: %d 条记录 %s -> %s\n", result.RowsAffected, base, bridge)
		}
	}

	// 3) message_hub: 仅处理 bridge 来源的消息（Extra 包含 bridge:true）
	//    message_hub 使用 JSON 列，GORM 的 JSON 查询需用 raw SQL
	for base, bridge := range baseToBridge {
		result := m.db.WithContext(ctx).
			Exec(`UPDATE message_hub SET channel = ? WHERE channel = ? AND extra::text LIKE '%"bridge":true%'`, bridge, base)
		if result.Error != nil {
			return fmt.Errorf("message_hub 归一化 %s -> %s 失败: %w", base, bridge, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration] message_hub: %d 条 bridge 消息 %s -> %s\n", result.RowsAffected, base, bridge)
		}
	}

	return nil
}

func (m *BridgeChannelNormalizeMigration) Down(ctx context.Context) error {
	// 回滚：bridge -> base（反向映射）
	// 注意：tiktok 不再需要回滚（bridge 渠道直接使用 tiktok）
	bridgeToBase := map[string]string{
		"douyin_web":   "douyin",
		"xhs_web":      "xhs",
		"xianyu_web":   "xianyu",
		"kuaishou_web": "kuaishou",
	}

	for bridge, base := range bridgeToBase {
		m.db.WithContext(ctx).Table("bridge_accounts").Where("channel = ?", bridge).Update("channel", base)
		m.db.WithContext(ctx).Table("channel_agent_bindings").Where("channel_type = ?", bridge).Update("channel_type", base)
		m.db.WithContext(ctx).Exec(`UPDATE message_hub SET channel = ? WHERE channel = ? AND extra::text LIKE '%"bridge":true%'`, base, bridge)
	}
	return nil
}

var _ migration.Migration = (*BridgeChannelNormalizeMigration)(nil)
