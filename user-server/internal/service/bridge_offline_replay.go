package service

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// BridgeOfflineReplayService Bridge 离线消息回扫服务
//
// G10: 竞品标配功能 - 每 5 分钟扫描 bridge_metrics 发现离线渠道，
// 重新发送 reach_delayed_outbound 中累积的消息。
//
// 依赖的表（由 v3_26_0_reach_tables_migration 已创建）：
//   - reach_delayed_outbound：延迟出站消息（累积队列）
//   - bridge_metrics：桥接渠道指标（判断渠道离线）
type BridgeOfflineReplayService struct {
	db *gorm.DB
}

// NewBridgeOfflineReplayService 创建离线回扫服务
func NewBridgeOfflineReplayService() *BridgeOfflineReplayService {
	return &BridgeOfflineReplayService{db: db.GetDB()}
}

// NewBridgeOfflineReplayServiceWithDB 注入 DB（测试用）
func (s *BridgeOfflineReplayService) WithDB(d *gorm.DB) *BridgeOfflineReplayService {
	s.db = d
	return s
}

// OfflineChannel 离线渠道快照
type OfflineChannel struct {
	Platform  string `json:"platform"`
	AccountID string `json:"account_id"`
	OfflineSince time.Time `json:"offline_since"`
}

// ReplayStats 回扫统计
type ReplayStats struct {
	ScannedChannels    int                 `json:"scanned_channels"`
	OfflineChannels    int                 `json:"offline_channels"`
	ReplayedMessages   int64               `json:"replayed_messages"`
	FailedMessages     int64               `json:"failed_messages"`
	OfflineSnapshots   []OfflineChannel    `json:"offline_snapshots,omitempty"`
	StartedAt          time.Time           `json:"started_at"`
	FinishedAt         time.Time           `json:"finished_at"`
}

// DetectOfflineChannels 检测离线渠道
//
// 判定规则：bridge_metrics 中最近 10 分钟内无新消息到达 → 视为离线
// （同时 fallback 到 bridge_accounts 中 status != "online" 的渠道）
func (s *BridgeOfflineReplayService) DetectOfflineChannels(ctx context.Context) ([]OfflineChannel, error) {
	if s.db == nil {
		return nil, nil
	}
	// 先尝试 bridge_metrics（如果表不存在则 fallback 到 bridge_accounts）
	type channelStat struct {
		Platform    string    `gorm:"column:platform"`
		AccountID   string    `gorm:"column:account_id"`
		LastSeen    time.Time `gorm:"column:last_seen"`
	}
	var stats []channelStat
	err := s.db.WithContext(ctx).
		Table("bridge_metrics").
		Select("platform, account_id, MAX(updated_at) as last_seen").
		Group("platform, account_id").
		Scan(&stats).Error
	if err != nil {
		// 表不存在或无数据，尝试 bridge_accounts
		logger.Warnf("[BridgeReplay] bridge_metrics 查询失败，fallback bridge_accounts: %v", err)
		type accOffline struct {
			Platform  string    `gorm:"column:platform"`
			AccountID string    `gorm:"column:account_id"`
			UpdatedAt time.Time `gorm:"column:updated_at"`
		}
		var accs []accOffline
		if err2 := s.db.WithContext(ctx).
			Table("bridge_accounts").
			Select("platform, account_id, updated_at").
			Where("status != ?", "online").
			Scan(&accs).Error; err2 != nil {
			return nil, fmt.Errorf("detect offline channels: %w", err2)
		}
		now := time.Now()
		out := make([]OfflineChannel, 0, len(accs))
		for _, a := range accs {
			if now.Sub(a.UpdatedAt) > 10*time.Minute {
				out = append(out, OfflineChannel{
					Platform:     a.Platform,
					AccountID:    a.AccountID,
					OfflineSince: a.UpdatedAt,
				})
			}
		}
		return out, nil
	}

	threshold := time.Now().Add(-10 * time.Minute)
	out := make([]OfflineChannel, 0)
	for _, st := range stats {
		if st.LastSeen.Before(threshold) {
			out = append(out, OfflineChannel{
				Platform:     st.Platform,
				AccountID:    st.AccountID,
				OfflineSince: st.LastSeen,
			})
		}
	}
	return out, nil
}

// ReplayDelayedOutbound 重放某个渠道累积的离线消息
//
// 从 reach_delayed_outbound 取 status="pending" 的消息，
// 重新投送到 DeliverBridgeOutbound 出站管道，然后标记为 replayed。
func (s *BridgeOfflineReplayService) ReplayDelayedOutbound(ctx context.Context, platform, accountID string, limit int) (replayed, failed int64) {
	if s.db == nil {
		return 0, 0
	}
	type delayedMsg struct {
		ID             uint64    `gorm:"column:id"`
		ConversationID string    `gorm:"column:conversation_id"`
		SenderID       string    `gorm:"column:sender_id"`
		ReceiverID     string    `gorm:"column:receiver_id"`
		MsgType        string    `gorm:"column:msg_type"`
		Content        string    `gorm:"column:content"`
		EventID        string    `gorm:"column:event_id"`
		RetryCount     int       `gorm:"column:retry_count"`
	}
	var msgs []delayedMsg
	q := s.db.WithContext(ctx).
		Table("reach_delayed_outbound").
		Where("platform = ? AND account_id = ? AND status = ?", platform, accountID, "pending").
		Order("send_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(&msgs).Error; err != nil {
		logger.Warnf("[BridgeReplay] 查询 reach_delayed_outbound 失败: %v", err)
		return 0, 0
	}
	for _, m := range msgs {
		// 调用统一出站入口（已注入 outbox SSE）
		err := DeliverBridgeOutbound(ctx, platform, accountID, m.ConversationID, m.MsgType, m.Content, m.EventID)
		if err != nil {
			failed++
			logger.Warnf("[BridgeReplay] 重放失败 id=%d err=%v", m.ID, err)
			_ = s.db.WithContext(ctx).Exec(
				"UPDATE reach_delayed_outbound SET retry_count = retry_count + 1, last_error = ?, status = ? WHERE id = ?",
				err.Error(), "replay_failed", m.ID,
			).Error
			continue
		}
		replayed++
		_ = s.db.WithContext(ctx).Exec(
			"UPDATE reach_delayed_outbound SET status = ?, replayed_at = NOW(), retry_count = retry_count + 1 WHERE id = ?",
			"replayed", m.ID,
		).Error
	}
	return replayed, failed
}

// RunOnce 执行一次完整的离线回扫
// 供 cron 调用（每 5 分钟）
func (s *BridgeOfflineReplayService) RunOnce(ctx context.Context) ReplayStats {
	startedAt := time.Now()
	stats := ReplayStats{StartedAt: startedAt}

	channels, err := s.DetectOfflineChannels(ctx)
	if err != nil {
		logger.Warnf("[BridgeReplay] DetectOfflineChannels 出错: %v", err)
	}
	stats.ScannedChannels = len(channels)
	stats.OfflineChannels = len(channels)
	stats.OfflineSnapshots = channels

	// 每个渠道最多重放 50 条，避免雪崩
	perChannelLimit := 50
	for _, ch := range channels {
		r, f := s.ReplayDelayedOutbound(ctx, ch.Platform, ch.AccountID, perChannelLimit)
		stats.ReplayedMessages += r
		stats.FailedMessages += f
	}

	stats.FinishedAt = time.Now()
	logger.Infof("[BridgeReplay] 离线回扫完成: offline=%d replayed=%d failed=%d duration=%s",
		stats.OfflineChannels, stats.ReplayedMessages, stats.FailedMessages,
		stats.FinishedAt.Sub(startedAt).Round(time.Millisecond))
	return stats
}
