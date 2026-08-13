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
	//    唯一约束 uk_bridge_ch_acc(channel, account_id) 可能已存在新旧值并存的脏数据
	//    （如同一账号同时有 douyin 与 douyin_web），直接 UPDATE 会 duplicate key 致整批迁移中止。
	//    故先删除会与目标值冲突的 base 行（保留 bridge 行即规范值），再归一化剩余 base 行。
	for base, bridge := range baseToBridge {
		del := m.db.WithContext(ctx).
			Exec(`DELETE FROM bridge_accounts WHERE channel = $1 AND EXISTS (SELECT 1 FROM bridge_accounts b2 WHERE b2.account_id = bridge_accounts.account_id AND b2.channel = $2)`, base, bridge)
		if del.Error != nil {
			return fmt.Errorf("bridge_accounts 删除冲突 %s 失败: %w", base, del.Error)
		}
		if del.RowsAffected > 0 {
			fmt.Printf("[migration] bridge_accounts: 删除 %d 条冲突 %s（已存在 %s）\n", del.RowsAffected, base, bridge)
		}
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

	// 2) channel_agent_bindings.channel_type: 同理（唯一约束可能为 (channel_type, account_id)）
	for base, bridge := range baseToBridge {
		del := m.db.WithContext(ctx).
			Exec(`DELETE FROM channel_agent_bindings WHERE channel_type = $1 AND EXISTS (SELECT 1 FROM channel_agent_bindings b2 WHERE b2.account_id = channel_agent_bindings.account_id AND b2.channel_type = $2)`, base, bridge)
		if del.Error != nil {
			return fmt.Errorf("channel_agent_bindings 删除冲突 %s 失败: %w", base, del.Error)
		}
		if del.RowsAffected > 0 {
			fmt.Printf("[migration] channel_agent_bindings: 删除 %d 条冲突 %s（已存在 %s）\n", del.RowsAffected, base, bridge)
		}
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
	//    注意：message_hub 的渠道字段为 platform（无 channel 列），此处用 platform 归一化。
	for base, bridge := range baseToBridge {
		result := m.db.WithContext(ctx).
			Exec(`UPDATE message_hub SET platform = ? WHERE platform = ? AND extra::text LIKE '%"bridge":true%'`, bridge, base)
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
