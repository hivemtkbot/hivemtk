package bridge

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
)

// ErrAccountOwnedByOther 归属冲突：该渠道账号已归属于其他用户，
// Upsert 仅刷新在线状态、不改动归属字段。供 handler 记录审计日志。
var ErrAccountOwnedByOther = errors.New("bridge account owned by another user")

// BridgeAccountRepository 桥接账号持久化（实现 BridgeAccountRepo 接口）。
//
// 放在 bridge 包内以避免 import cycle（repository 已间接依赖 bridge）。
// 注册即落库：维护 bridge_accounts，并联动 channel_agent_bindings 以保证 AI 路由。
type BridgeAccountRepository struct {
	db *gorm.DB
}

func NewBridgeAccountRepository(db *gorm.DB) *BridgeAccountRepository {
	return &BridgeAccountRepository{db: db}
}

// Upsert 注册/上线：写入 bridge_accounts（owner + 在线状态），并绑定智能体。
//
// 安全约束（防水平越权）：
//   - 记录已存在且归属(user_id)与当前请求不同 → 不覆盖归属/昵称/智能体，仅刷新在线状态，
//     并返回 ErrAccountOwnedByOther（handler 记审计日志，但 hub 收发不受影响）。
//   - 并发首建存在竞争：Create 命中唯一冲突时重试一次（此时记录已存在，走"已存在"分支校验归属）。
func (r *BridgeAccountRepository) Upsert(ctx context.Context, u BridgeAccountUpsert) error {
	for attempt := 0; attempt < 2; attempt++ {
		var acc model.BridgeAccount
		err := r.db.WithContext(ctx).Where("channel = ? AND account_id = ?", u.Channel, u.AccountID).First(&acc).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 首建：归属必须是当前请求用户
			acc = model.BridgeAccount{
				Channel:   u.Channel,
				AccountID: u.AccountID,
				UserID:    u.UserID,
				Status:    u.Status,
			}
			if u.AccountName != "" {
				acc.AccountName = u.AccountName
			}
			if u.AgentID != 0 {
				acc.AgentID = u.AgentID
			}
			now := time.Now()
			acc.LastSyncAt = &now
			if cerr := r.db.WithContext(ctx).Create(&acc).Error; cerr != nil {
				if isUniqueViolation(cerr) {
					continue // 并发首建冲突：下一轮按"已存在"分支处理
				}
				return cerr
			}
			if u.AgentID != 0 {
				return r.upsertBinding(ctx, u.Channel, u.AccountID, u.AgentID)
			}
			return nil
		} else if err != nil {
			return err
		}

		// 记录已存在：归属冲突校验
		if acc.UserID != u.UserID {
			now := time.Now()
			_ = r.db.WithContext(ctx).Model(&model.BridgeAccount{}).
				Where("channel = ? AND account_id = ?", u.Channel, u.AccountID).
				Updates(map[string]any{"status": u.Status, "last_sync_at": now}).Error
			return ErrAccountOwnedByOther
		}

		// 归属一致：允许更新昵称/智能体/状态，但绝不改动 user_id
		acc.Status = u.Status
		now := time.Now()
		acc.LastSyncAt = &now
		if u.AccountName != "" {
			acc.AccountName = u.AccountName
		}
		if u.AgentID != 0 {
			acc.AgentID = u.AgentID
		}
		if err := r.db.WithContext(ctx).Save(&acc).Error; err != nil {
			return err
		}
		if u.AgentID != 0 {
			return r.upsertBinding(ctx, u.Channel, u.AccountID, u.AgentID)
		}
		return nil
	}
	return nil
}

// isUniqueViolation 判断是否为唯一索引冲突（Postgres: duplicate key value）。
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key value")
}

// upsertBinding 维护单一主绑定：先取消同渠道同账号其他主选，再写入/更新本条。
func (r *BridgeAccountRepository) upsertBinding(ctx context.Context, channel, accountID string, agentID uint) error {
	r.db.Model(&model.ChannelAgentBinding{}).WithContext(ctx).
		Where("channel_type = ? AND account_id = ? AND agent_id <> ?", channel, accountID, agentID).
		Update("is_primary", false)

	var b model.ChannelAgentBinding
	err := r.db.WithContext(ctx).
		Where("channel_type = ? AND account_id = ? AND agent_id = ?", channel, accountID, agentID).
		First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		b = model.ChannelAgentBinding{ChannelType: channel, AccountID: accountID, AgentID: agentID}
	} else if err != nil {
		return err
	}
	b.IsPrimary = true
	b.Enabled = true
	return r.db.WithContext(ctx).Save(&b).Error
}

// SetOffline 连接断开：置离线 + 记录最后同步时间（G12）。
func (r *BridgeAccountRepository) SetOffline(channel, accountID string) error {
	now := time.Now()
	return r.db.WithContext(context.Background()).Model(&model.BridgeAccount{}).
		Where("channel = ? AND account_id = ?", channel, accountID).
		Updates(map[string]any{"status": "offline", "last_sync_at": now}).Error
}

// ListByUser 列出某用户全部桥接账号（含 hub 实时在线状态）。
func (r *BridgeAccountRepository) ListByUser(ctx context.Context, userID uint) ([]BridgeAccountView, error) {
	var accs []model.BridgeAccount
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id desc").Find(&accs).Error; err != nil {
		return nil, err
	}
	views := make([]BridgeAccountView, 0, len(accs))
	for _, a := range accs {
		views = append(views, BridgeAccountView{
			ID:          a.ID,
			UserID:      a.UserID,
			Channel:     a.Channel,
			AccountID:   a.AccountID,
			AccountName: a.AccountName,
			AgentID:     a.AgentID,
			Status:      a.Status,
			LastSyncAt:  a.LastSyncAt,
			Online:      GetBridgeHub().IsOnline(a.Channel, a.AccountID),
		})
	}
	return views, nil
}

// GetByChannelAccount 按渠道+账号查询（供归属校验使用）。
func (r *BridgeAccountRepository) GetByChannelAccount(ctx context.Context, channel, accountID string) (*model.BridgeAccount, error) {
	var acc model.BridgeAccount
	err := r.db.WithContext(ctx).Where("channel = ? AND account_id = ?", channel, accountID).First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

var _ BridgeAccountRepo = (*BridgeAccountRepository)(nil)
