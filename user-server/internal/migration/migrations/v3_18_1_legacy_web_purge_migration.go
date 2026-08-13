package migrations

// v3_18_1_legacy_web_purge_migration.go 渠道编码统一收尾（彻底清除 *_web / xhs 历史值）
//
// 背景：v3.18.0 把 *_web / xhs 归一化为全名，但 message_hub 的 UPDATE 带了
// `extra::text LIKE '%"bridge":true%'` 这个 gate，导致大量未带 bridge 标记（或非桥接来源）的
// message_hub 历史行（线上实测 1697 条）以及 bridge_accounts（104 条）、channel_agent_bindings（3 条）
// 残留 *_web 值，最终在「消息中台 MQ 平台分布图」和统一收件箱会话标签上显示为「抖音web/闲鱼web/...」。
//
// 本迁移是无 gate 的彻底收尾：把 6 张核心业务表中所有 *_web / xhs 历史值统一归一化为来源平台全名，
// 与现行写入路径（internal/bridge/channel.go ToBridgeChannel 已为 identity、常量值为全名）保持一致。
// 幂等可重入：已无 *_web 行时各语句 0 rows affected，重复执行安全。

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// legacyWebPurgeMap 历史渠道名 -> 来源平台全名（2026-08-13 收尾归一）。
var legacyWebPurgeMap = map[string]string{
	"xhs_web":      "xiaohongshu",
	"douyin_web":   "douyin",
	"kuaishou_web": "kuaishou",
	"xianyu_web":    "xianyu",
	"tiktok_web":   "tiktok",
	"xhs":          "xiaohongshu", // 早期 ChannelXHS 简写
}

type BridgeChannelUnifyV3_18_1Migration struct {
	db *gorm.DB
}

func NewBridgeChannelUnifyV3_18_1Migration(db *gorm.DB) *BridgeChannelUnifyV3_18_1Migration {
	return &BridgeChannelUnifyV3_18_1Migration{db: db}
}

func (m *BridgeChannelUnifyV3_18_1Migration) Version() string { return "v3.18.1" }
func (m *BridgeChannelUnifyV3_18_1Migration) Name() string {
	return "渠道编码统一收尾（彻底清除 *_web / xhs 历史值）"
}
func (m *BridgeChannelUnifyV3_18_1Migration) Description() string {
	return "无 gate 地把 message_hub / customer_sessions / inbox_conversations / bridge_accounts / channel_agent_bindings / ai_suggestions 中的 *_web / xhs 历史值归一化为来源平台全名"
}

func (m *BridgeChannelUnifyV3_18_1Migration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 1) message_hub.platform：无 gate，所有 *_web / xhs 行全部归一化。
	for old, new := range legacyWebPurgeMap {
		result := m.db.WithContext(ctx).
			Table("message_hub").
			Where("platform = ?", old).
			Update("platform", new)
		if result.Error != nil {
			return fmt.Errorf("message_hub 收尾归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.1] message_hub: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 2) customer_sessions.platform
	for old, new := range legacyWebPurgeMap {
		result := m.db.WithContext(ctx).
			Table("customer_sessions").
			Where("platform = ?", old).
			Update("platform", new)
		if result.Error != nil {
			return fmt.Errorf("customer_sessions 收尾归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.1] customer_sessions: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 3) inbox_conversations.platform
	for old, new := range legacyWebPurgeMap {
		result := m.db.WithContext(ctx).
			Table("inbox_conversations").
			Where("platform = ?", old).
			Update("platform", new)
		if result.Error != nil {
			return fmt.Errorf("inbox_conversations 收尾归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.1] inbox_conversations: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 4) bridge_accounts.channel
	//    唯一约束 uk_bridge_ch_acc(account_id, channel) 可能存在新旧值并存的脏数据，
	//    先删除会与目标值冲突的 old 行（保留 new 规范行），再归一化剩余 old 行。
	for old, new := range legacyWebPurgeMap {
		del := m.db.WithContext(ctx).
			Exec(`DELETE FROM bridge_accounts WHERE channel = $1 AND EXISTS (SELECT 1 FROM bridge_accounts b2 WHERE b2.account_id = bridge_accounts.account_id AND b2.channel = $2)`, old, new)
		if del.Error != nil {
			return fmt.Errorf("bridge_accounts 删除冲突 %s 失败: %w", old, del.Error)
		}
		if del.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.1] bridge_accounts: 删除 %d 条冲突 %s（已存在 %s）\n", del.RowsAffected, old, new)
		}

		result := m.db.WithContext(ctx).
			Table("bridge_accounts").
			Where("channel = ?", old).
			Update("channel", new)
		if result.Error != nil {
			return fmt.Errorf("bridge_accounts 收尾归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.1] bridge_accounts: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 5) channel_agent_bindings.channel_type
	for old, new := range legacyWebPurgeMap {
		result := m.db.WithContext(ctx).
			Table("channel_agent_bindings").
			Where("channel_type = ?", old).
			Update("channel_type", new)
		if result.Error != nil {
			return fmt.Errorf("channel_agent_bindings 收尾归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.1] channel_agent_bindings: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 6) ai_suggestions.session_id 形如 "xhs_web:xxx:yyy"，把前缀替换为全名
	for old, new := range legacyWebPurgeMap {
		result := m.db.WithContext(ctx).
			Exec(`UPDATE ai_suggestions SET session_id = ? || SUBSTRING(session_id FROM ?) WHERE session_id LIKE ?`,
				new, fmt.Sprintf("LENGTH('%s') + 1", old)+"", old+":%")
		if result.Error != nil {
			return fmt.Errorf("ai_suggestions session_id 收尾归一化 %s: -> %s: 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.1] ai_suggestions.session_id: %d 条 %s: -> %s:\n", result.RowsAffected, old, new)
		}
	}

	return nil
}

func (m *BridgeChannelUnifyV3_18_1Migration) Down(ctx context.Context) error {
	// 回滚：全名 -> *_web（与 v3.18.0 Down 一致，xhs/xhs_web 统一还原为 *_web）。
	reverse := map[string]string{
		"xiaohongshu": "xhs_web",
		"douyin":      "douyin_web",
		"kuaishou":    "kuaishou_web",
		"xianyu":      "xianyu_web",
		"tiktok":      "tiktok_web",
	}
	for new, old := range reverse {
		m.db.WithContext(ctx).Table("message_hub").Where("platform = ?", new).Update("platform", old)
		m.db.WithContext(ctx).Table("customer_sessions").Where("platform = ?", new).Update("platform", old)
		m.db.WithContext(ctx).Table("inbox_conversations").Where("platform = ?", new).Update("platform", old)
		m.db.WithContext(ctx).Table("bridge_accounts").Where("channel = ?", new).Update("channel", old)
		m.db.WithContext(ctx).Table("channel_agent_bindings").Where("channel_type = ?", new).Update("channel_type", old)
		m.db.WithContext(ctx).Exec(`UPDATE ai_suggestions SET session_id = ? || SUBSTRING(session_id FROM ?) WHERE session_id LIKE ?`,
			old, fmt.Sprintf("LENGTH('%s') + 1", new)+"", new+":%")
	}
	return nil
}

var _ migration.Migration = (*BridgeChannelUnifyV3_18_1Migration)(nil)
