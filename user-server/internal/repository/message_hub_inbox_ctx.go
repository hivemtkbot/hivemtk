package repository

import (
	"context"

	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

func (r *MessageHubRepository) GetLastByPlatformAccount(ctx context.Context, platform, accountID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var msg model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ?", platform, accountID).
		Order("sent_at DESC").First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *MessageHubRepository) HasUnrepliedCustomerMessage(ctx context.Context, conversationID string, replyWindow time.Duration) (unreplied bool, withinWindow bool, err error) {
	if r == nil || r.db == nil {
		return false, false, nil
	}
	if conversationID == "" {
		return false, false, nil
	}
	var last model.MessageHub

	if err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("sent_at DESC").
		Limit(1).
		First(&last).Error; err != nil {
		if err == gorm.ErrRecordNotFound {

			return true, true, nil
		}

		return false, false, err
	}

	if last.Direction != "inbound" {
		return false, false, nil
	}

	cutoff := time.Now().Add(-replyWindow)
	if last.SentAt.Before(cutoff) {

		return true, false, nil
	}
	return true, true, nil
}

func (r *MessageHubRepository) GetLastOutboundByConversation(ctx context.Context, conversationID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if conversationID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	cutoff := time.Now().Add(-5 * time.Minute)
	var msg model.MessageHub
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND direction = ? AND sent_at >= ?", conversationID, "outbound", cutoff).
		Order("sent_at DESC").
		First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *MessageHubRepository) GetLastInboundByConversation(ctx context.Context, conversationID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if conversationID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var msg model.MessageHub
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND direction = ?", conversationID, "inbound").
		Order("sent_at DESC").
		First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *MessageHubRepository) ListByConversationContext(ctx context.Context, platform, accountID, customerID string) ([]*model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var hubs []*model.MessageHub
	if err := r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("platform = ? AND account_id = ? AND (sender_id = ? OR receiver_id = ?)",
			platform, accountID, customerID, customerID).
		Find(&hubs).Error; err != nil {
		return nil, err
	}
	return hubs, nil
}

func (r *MessageHubRepository) FindNullConversationIDRows(ctx context.Context) ([]model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var rows []model.MessageHub
	if err := r.db.WithContext(ctx).
		Where("conversation_id IS NULL OR conversation_id = ?", "").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *MessageHubRepository) UpdateConversationID(ctx context.Context, id uint, conversationID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		Where("id = ?", id).
		Update("conversation_id", conversationID).Error
}

func (r *MessageHubRepository) FindConversationIDsMissingInbox(ctx context.Context) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var convIDs []string
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT m.conversation_id
		FROM message_hub m
		WHERE m.conversation_id IS NOT NULL AND m.conversation_id <> ''
		  AND NOT EXISTS (
			SELECT 1 FROM inbox_conversations i WHERE i.conversation_id = m.conversation_id
		  )
	`).Scan(&convIDs).Error
	if err != nil {
		return nil, err
	}
	return convIDs, nil
}

func (r *MessageHubRepository) FindLatestByConversation(ctx context.Context, conversationID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if conversationID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var msg model.MessageHub
	if err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id DESC").
		First(&msg).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *MessageHubRepository) NormalizePollutedConversationIDs(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	// 末尾时间戳/状态 token 正则（按 $ 锚定，'g' 模式去除所有后缀时间戳）：
	//   昨天/今天/前天/明天 + HH:MM（两 token，须排在单 token 日前词之前，否则会只剥半截）
	//   刚刚/刚才/前天/昨天/今天/明天/周X（单 token 日标识）
	//   N分钟前/小时前/天前（相对时间）
	//   YYYY/MM/DD、MM/DD（日期）   HH:MM（时刻）
	//   有新交易评价/交易成功（交易状态串）
	const tsPat = "( 昨天 \\d{1,2}:\\d{2}| 今天 \\d{1,2}:\\d{2}| 前天 \\d{1,2}:\\d{2}| 明天 \\d{1,2}:\\d{2}" +
		"| 刚刚| 刚才| 前天| 大前天| 昨天| 今天| 明天| 周[一二三四五六日天]" +
		"| \\d+分钟前| \\d+小时前| \\d+天前" +
		"| \\d{4}/\\d{1,2}/\\d{1,2}| \\d{1,2}/\\d{1,2}| \\d{1,2}:\\d{2}" +
		"| 有新交易评价| 交易成功)$"
	res := r.db.WithContext(ctx).Exec(`
		UPDATE message_hub m
		SET conversation_id = regexp_replace(
			CASE
				WHEN m.sender_id   LIKE (m.conversation_id || ' %') THEN m.sender_id
				WHEN m.receiver_id LIKE (m.conversation_id || ' %') THEN m.receiver_id
				ELSE m.conversation_id
			END,
			?, '', 'g'
		)
		WHERE m.conversation_id LIKE 'conv:%'
		  AND (m.sender_id   LIKE (m.conversation_id || ' %')
		    OR m.receiver_id LIKE (m.conversation_id || ' %'))`,
		tsPat)
	return res.RowsAffected, res.Error
}

func (r *MessageHubRepository) NormalizePollutedTraceConversationIDs(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	const tsPat = "( 昨天 \\d{1,2}:\\d{2}| 今天 \\d{1,2}:\\d{2}| 前天 \\d{1,2}:\\d{2}| 明天 \\d{1,2}:\\d{2}" +
		"| 刚刚| 刚才| 前天| 大前天| 昨天| 今天| 明天| 周[一二三四五六日天]" +
		"| \\d+分钟前| \\d+小时前| \\d+天前" +
		"| \\d{4}/\\d{1,2}/\\d{1,2}| \\d{1,2}/\\d{1,2}| \\d{1,2}:\\d{2}" +
		"| 有新交易评价| 交易成功)$"
	res := r.db.WithContext(ctx).Exec(`
		UPDATE message_trace m
		SET conversation_id = regexp_replace(m.conversation_id, ?, '', 'g')
		WHERE m.conversation_id LIKE 'conv:% %'`,
		tsPat)
	return res.RowsAffected, res.Error
}
