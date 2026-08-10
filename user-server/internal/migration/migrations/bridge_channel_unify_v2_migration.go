package migrations

// bridge_channel_unify_v2.go 渠道编码统一 v2：把 *_web / xhs 全部归一化为全名
//
// 背景：v3.17.0 把基础渠道名（douyin/xhs/tiktok/xianyu）归一化为 *_web 桥接名，
// 但前端/后端/DB 三方 *_web 与 xiaohongshu / xhs 命名长期不一致，
// 导致小红书 2268 现场出现「message_hub 有 inbound、0 个 customer_sessions、AI 不回复」。
//
// 本迁移执行反向归一化（*_web / xhs -> 全名），覆盖 6 张核心业务表：
//   - message_hub.platform            （仅 bridge:true 标记的消息）
//   - customer_sessions.platform
//   - inbox_conversations.platform
//   - bridge_accounts.channel
//   - channel_agent_bindings.channel_type
//   - ai_suggestions.session_id 内的 platform 前缀（历史 session_id 形如 "xhs_web:..."）
//
// 单一源：hivemtk-user/internal/model/message_event.go ChannelXHS/ChannelDouyin/ChannelKuaishou/ChannelXianyu/ChannelTikTok。

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// bridgeUnifyV2Map 历史渠道名 -> 全名（2026-08-05 统一编码）
//
// 终态：所有"桥接渠道"= 平台全名（无 _web 后缀）。
// tiktok 历史上 bridge 直接用 tiktok（不带 _web），无需迁移但保留 entry 以便 Down() 反向兼容。
var bridgeUnifyV2Map = map[string]string{
	"xhs_web":      "xiaohongshu",
	"douyin_web":   "douyin",
	"kuaishou_web": "kuaishou",
	"xianyu_web":   "xianyu",
	"tiktok_web":   "tiktok",
	"xhs":          "xiaohongshu", // 早期 ChannelXHS 简写
}

type BridgeChannelUnifyV2Migration struct {
	db *gorm.DB
}

func NewBridgeChannelUnifyV2Migration(db *gorm.DB) *BridgeChannelUnifyV2Migration {
	return &BridgeChannelUnifyV2Migration{db: db}
}

func (m *BridgeChannelUnifyV2Migration) Version() string { return "v3.18.0" }
func (m *BridgeChannelUnifyV2Migration) Name() string {
	return "渠道编码统一 v2（*_web/xhs -> 全名）"
}
func (m *BridgeChannelUnifyV2Migration) Description() string {
	return "把 message_hub / customer_sessions / inbox_conversations / bridge_accounts / channel_agent_bindings / ai_suggestions 中的 *_web / xhs 历史值归一化为全名（xiaohongshu/douyin/kuaishou/xianyu/tiktok）"
}

func (m *BridgeChannelUnifyV2Migration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 1) message_hub.platform：仅迁移 bridge 来源（extra::text LIKE '%"bridge":true%'）
	//    非 bridge 渠道（如 web_embed/douyin 官方 API）保留原值。
	for old, new := range bridgeUnifyV2Map {
		result := m.db.WithContext(ctx).
			Exec(`UPDATE message_hub SET platform = ? WHERE platform = ? AND extra::text LIKE '%"bridge":true%'`, new, old)
		if result.Error != nil {
			return fmt.Errorf("message_hub 归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.0] message_hub(bridge): %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 2) customer_sessions.platform：会话级
	for old, new := range bridgeUnifyV2Map {
		result := m.db.WithContext(ctx).
			Table("customer_sessions").
			Where("platform = ?", old).
			Update("platform", new)
		if result.Error != nil {
			return fmt.Errorf("customer_sessions 归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.0] customer_sessions: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 3) inbox_conversations.platform：统一收件箱
	for old, new := range bridgeUnifyV2Map {
		result := m.db.WithContext(ctx).
			Table("inbox_conversations").
			Where("platform = ?", old).
			Update("platform", new)
		if result.Error != nil {
			return fmt.Errorf("inbox_conversations 归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.0] inbox_conversations: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 4) bridge_accounts.channel
	for old, new := range bridgeUnifyV2Map {
		result := m.db.WithContext(ctx).
			Table("bridge_accounts").
			Where("channel = ?", old).
			Update("channel", new)
		if result.Error != nil {
			return fmt.Errorf("bridge_accounts 归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.0] bridge_accounts: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 5) channel_agent_bindings.channel_type
	for old, new := range bridgeUnifyV2Map {
		result := m.db.WithContext(ctx).
			Table("channel_agent_bindings").
			Where("channel_type = ?", old).
			Update("channel_type", new)
		if result.Error != nil {
			return fmt.Errorf("channel_agent_bindings 归一化 %s -> %s 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.0] channel_agent_bindings: %d 条 %s -> %s\n", result.RowsAffected, old, new)
		}
	}

	// 6) ai_suggestions.session_id 形如 "xhs_web:xxx:yyy"，把前缀替换为全名
	//    session_id 字符串前缀用 LIKE 'old:%' 精确匹配，避免误改其他字段
	for old, new := range bridgeUnifyV2Map {
		result := m.db.WithContext(ctx).
			Exec(`UPDATE ai_suggestions SET session_id = ? || SUBSTRING(session_id FROM ?) WHERE session_id LIKE ?`,
				new, fmt.Sprintf("LENGTH('%s') + 1", old)+"", old+":%")
		if result.Error != nil {
			return fmt.Errorf("ai_suggestions session_id 归一化 %s: -> %s: 失败: %w", old, new, result.Error)
		}
		if result.RowsAffected > 0 {
			fmt.Printf("[migration v3.18.0] ai_suggestions.session_id: %d 条 %s: -> %s:\n", result.RowsAffected, old, new)
		}
	}

	return nil
}

func (m *BridgeChannelUnifyV2Migration) Down(ctx context.Context) error {
	// 回滚：全名 -> *_web
	// 注意：xhs/xhs_web 都映射到 xiaohongshu，回滚时无法 100% 还原原始 xhs 简写，统一还原为 *_web 形式。
	reverse := map[string]string{
		"xiaohongshu": "xhs_web",
		"douyin":      "douyin_web",
		"kuaishou":    "kuaishou_web",
		"xianyu":      "xianyu_web",
		"tiktok":      "tiktok_web",
	}
	for new, old := range reverse {
		m.db.WithContext(ctx).
			Exec(`UPDATE message_hub SET platform = ? WHERE platform = ? AND extra::text LIKE '%"bridge":true%'`, old, new)
		m.db.WithContext(ctx).Table("customer_sessions").Where("platform = ?", new).Update("platform", old)
		m.db.WithContext(ctx).Table("inbox_conversations").Where("platform = ?", new).Update("platform", old)
		m.db.WithContext(ctx).Table("bridge_accounts").Where("channel = ?", new).Update("channel", old)
		m.db.WithContext(ctx).Table("channel_agent_bindings").Where("channel_type = ?", new).Update("channel_type", old)
		m.db.WithContext(ctx).Exec(`UPDATE ai_suggestions SET session_id = ? || SUBSTRING(session_id FROM ?) WHERE session_id LIKE ?`,
			old, fmt.Sprintf("LENGTH('%s') + 1", new)+"", new+":%")
	}
	return nil
}

var _ migration.Migration = (*BridgeChannelUnifyV2Migration)(nil)
