package repository

import (
	"context"

	"hivemtk-user/internal/model"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

func (r *InboxConversationRepository) DeletePollutedInboxRows(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("conversation_id LIKE ? OR customer_id LIKE ?", "conv:% %", "conv:% %").
		Delete(&model.InboxConversation{})
	return res.RowsAffected, res.Error
}

func (r *InboxConversationRepository) DeleteOrphanConvInboxRows(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("conversation_id LIKE ? AND NOT EXISTS (SELECT 1 FROM message_hub m WHERE m.conversation_id = inbox_conversations.conversation_id)",
			"conv:%").
		Delete(&model.InboxConversation{})
	return res.RowsAffected, res.Error
}

type InboxConversationRepository struct {
	db *gorm.DB
}

func NewInboxConversationRepository() *InboxConversationRepository {
	return &InboxConversationRepository{db: _db.GetDB()}
}

// ===== merged from message_hub_inbox_part2.go (was a mechanical _partN split) =====
func NewInboxConversationRepositoryWithDB(db *gorm.DB) *InboxConversationRepository {
	return &InboxConversationRepository{db: db}
}

// SetInboxConversationRepoDB 工具函数（service 层使用）
func SetInboxConversationRepoDB(r *InboxConversationRepository, db *gorm.DB) {
	r.SetDB(context.Background(), db)
}

// InboxConversationQuery 会话列表查询条件（与 service.InboxQuery 字段对齐）
type InboxConversationQuery struct {
	Platform    string
	AccountID   string
	CustomerID  string
	Keyword     string
	Status      string
	AssignedTo  string
	AssignedSOP uint
	Pinned      *bool
	Starred     *bool
	Muted       *bool
	Page        int
	PageSize    int
	OrderBy     string
}
