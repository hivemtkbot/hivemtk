package repository

import (
	"context"

	"time"
)

type SyncGapConv struct {
	Platform       string
	AccountID      string
	ConversationID string
	CustomerID     string
}

func (r *MessageHubRepository) FindSyncGapConversations(ctx context.Context, since time.Time) ([]SyncGapConv, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	const triad = "(CASE" +
		" WHEN m.is_group AND m.conversation_id <> '' THEN m.conversation_id" +
		" WHEN m.conversation_id <> '' AND (m.sender_id LIKE (m.conversation_id || ' %') OR m.receiver_id LIKE (m.conversation_id || ' %')) THEN m.conversation_id" +
		" ELSE (CASE WHEN m.direction = 'inbound' THEN m.sender_id ELSE m.receiver_id END)" +
		" END)"
	sql := `SELECT DISTINCT m.platform, m.account_id, m.conversation_id, ` + triad + ` AS customer_id
		FROM message_hub m
		WHERE m.conversation_id IS NOT NULL AND m.conversation_id <> ''
		  AND m.created_at >= $1
		  AND NOT EXISTS (
			SELECT 1 FROM inbox_conversations ic
			WHERE ic.platform = m.platform AND ic.account_id = m.account_id AND ic.customer_id = ` + triad + `
		  )`
	var rows []SyncGapConv
	if err := r.db.WithContext(ctx).Raw(sql, since).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

