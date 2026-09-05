package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

var legacyWebPurgeMap = map[string]string{
	"xhs_web":      "xiaohongshu",
	"douyin_web":   "douyin",
	"kuaishou_web": "kuaishou",
	"xianyu_web":   "xianyu",
	"tiktok_web":   "tiktok",
	"xhs":          "xiaohongshu",
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
