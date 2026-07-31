package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// ============== MessageHubRepository ==============

// MessageHubRepository 消息中台仓库
type MessageHubRepository struct {
	db *gorm.DB
}

// NewMessageHubRepository 创建消息中台仓库实例
func NewMessageHubRepository() *MessageHubRepository {
	return &MessageHubRepository{db: _db.GetDB()}
}

// NewMessageHubRepositoryWithDB 创建指定数据库连接的 MessageHubRepository 实例
// 用于 service 层依赖注入与单元测试；db 为 nil 时所有方法做无操作短路（保持与 service 层 nil 检查一致）。
func NewMessageHubRepositoryWithDB(db *gorm.DB) *MessageHubRepository {
	return &MessageHubRepository{db: db}
}

// SetDB 注入 db（用于测试）
//
// 五层架构 §三.5 + §七：仓库方法必须首参为 ctx context.Context，
// 避免服务层 / 测试层拼接时丢失链路追踪 / 超时控制。
func (r *MessageHubRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// SetMessageHubRepoDB 工具函数（service 层使用）
func SetMessageHubRepoDB(r *MessageHubRepository, db *gorm.DB) {
	r.SetDB(context.Background(), db)
}

// Create 创建消息中台记录（唯一约束冲突返回 nil）
func (r *MessageHubRepository) Create(ctx context.Context, hub *model.MessageHub) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Create(hub).Error
}

// GetByID 按 ID 获取消息
func (r *MessageHubRepository) GetByID(ctx context.Context, id uint) (*model.MessageHub, error) {
	var hub model.MessageHub
	err := r.db.First(&hub, id).Error
	return &hub, err
}

// GetByMsgID 按消息 ID 获取消息
func (r *MessageHubRepository) GetByMsgID(ctx context.Context, msgID string) (*model.MessageHub, error) {
	var hub model.MessageHub
	err := r.db.Where("msg_id = ?", msgID).First(&hub).Error
	return &hub, err
}

// ListByConversation 按会话 ID 列出消息
func (r *MessageHubRepository) ListByConversation(ctx context.Context, conversationID string, page, pageSize int) ([]*model.MessageHub, int64, error) {
	var items []*model.MessageHub
	var total int64
	query := r.db.Model(&model.MessageHub{}).Where("conversation_id = ?", conversationID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := query.Order("sent_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	return items, total, err
}

// Update 更新消息中台记录
func (r *MessageHubRepository) Update(ctx context.Context, hub *model.MessageHub) error {
	return r.db.Save(hub).Error
}

// MarkReadByID 标记消息为已读
func (r *MessageHubRepository) MarkReadByID(ctx context.Context, id uint) error {
	now := time.Now()
	return r.db.Model(&model.MessageHub{}).Where("id = ?", id).
		Updates(map[string]any{
			"is_read": true,
			"read_at": now,
		}).Error
}

// GetByPlatformAccountMsgID 按 (platform, account_id, msg_id) 唯一键查询消息
// 用于 Push 幂等检查：消息已存在则返回 (msg, nil)，不存在返回 (nil, gorm.ErrRecordNotFound)
func (r *MessageHubRepository) GetByPlatformAccountMsgID(ctx context.Context, platform, accountID, msgID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var existing model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND msg_id = ?", platform, accountID, msgID).
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

// MarkReadByIDs 批量标记已读（ids 为空时无操作）
func (r *MessageHubRepository) MarkReadByIDs(ctx context.Context, ids []uint) error {
	if r == nil || r.db == nil || len(ids) == 0 {
		return nil
	}
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).Where("id IN ?", ids).
		Updates(map[string]any{
			"is_read": true,
			"read_at": now,
		}).Error
}

// HubListQuery 消息中台列表查询条件（与 service.ListQuery 字段对齐）
type HubListQuery struct {
	Platform       string
	AccountID      string
	ConversationID string
	SenderID       string
	Direction      string
	MsgType        string
	Keyword        string
	IsRead         *bool
	IsGroup        *bool
	StartTime      *time.Time
	EndTime        *time.Time
	Page           int
	PageSize       int
	OrderBy        string
}

// ListByHubQuery 按条件分页查询消息中台记录
func (r *MessageHubRepository) ListByHubQuery(ctx context.Context, q HubListQuery) ([]*model.MessageHub, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 200 {
		q.PageSize = 20
	}

	tx := r.db.WithContext(ctx).Model(&model.MessageHub{})
	if q.Platform != "" {
		tx = tx.Where("platform = ?", q.Platform)
	}
	if q.AccountID != "" {
		tx = tx.Where("account_id = ?", q.AccountID)
	}
	if q.ConversationID != "" {
		tx = tx.Where("conversation_id = ?", q.ConversationID)
	}
	if q.SenderID != "" {
		tx = tx.Where("sender_id = ?", q.SenderID)
	}
	if q.Direction != "" {
		tx = tx.Where("direction = ?", q.Direction)
	}
	if q.MsgType != "" {
		tx = tx.Where("msg_type = ?", q.MsgType)
	}
	if q.Keyword != "" {
		tx = tx.Where("content LIKE ?", "%"+q.Keyword+"%")
	}
	if q.IsRead != nil {
		tx = tx.Where("is_read = ?", *q.IsRead)
	}
	if q.IsGroup != nil {
		tx = tx.Where("is_group = ?", *q.IsGroup)
	}
	if q.StartTime != nil {
		tx = tx.Where("sent_at >= ?", *q.StartTime)
	}
	if q.EndTime != nil {
		tx = tx.Where("sent_at <= ?", *q.EndTime)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderBy := "sent_at DESC"
	switch q.OrderBy {
	case "sent_at ASC", "sent_at DESC", "created_at ASC", "created_at DESC":
		orderBy = q.OrderBy
	}

	var rows []*model.MessageHub
	if err := tx.Order(orderBy).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// HubStatsResult 消息中台统计结果
type HubStatsResult struct {
	Total       int64
	Inbound     int64
	Outbound    int64
	Unread      int64
	ByPlatform  map[string]int64
	ByDirection map[string]int64
	ByMsgType   map[string]int64
	ByAccount   map[string]int64
	Recent24h   int64
}

// GetHubStats 消息中台多维度统计
//
// 五层架构 §三.5：原 service 层多次 s.db 调用合并为单次 repo 方法。
// 行为保持与原实现一致：start/end 为 nil 时不限定时间窗口。
func (r *MessageHubRepository) GetHubStats(ctx context.Context, start, end *time.Time) (*HubStatsResult, error) {
	if r == nil || r.db == nil {
		return &HubStatsResult{
			ByPlatform: map[string]int64{}, ByDirection: map[string]int64{},
			ByMsgType: map[string]int64{}, ByAccount: map[string]int64{},
		}, nil
	}

	var total, inbound, outbound, unread int64

	countQuery := r.db.WithContext(ctx).Model(&model.MessageHub{}).Where("1 = 1")
	if start != nil {
		countQuery = countQuery.Where("sent_at >= ?", *start)
	}
	if end != nil {
		countQuery = countQuery.Where("sent_at <= ?", *end)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("direction = ?", "inbound").
		Count(&inbound)
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("direction = ?", "outbound").
		Count(&outbound)
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("(is_read = ? OR is_read IS NULL)", false).
		Count(&unread)

	stats := &HubStatsResult{
		Total: total, Inbound: inbound, Outbound: outbound, Unread: unread,
		ByPlatform: map[string]int64{}, ByDirection: map[string]int64{},
		ByMsgType: map[string]int64{}, ByAccount: map[string]int64{},
	}

	type pcount struct {
		Platform string
		C        int64
	}
	var pCounts []pcount
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("platform AS platform, COUNT(*) AS c").
		Group("platform").Scan(&pCounts)
	for _, p := range pCounts {
		stats.ByPlatform[p.Platform] = p.C
		stats.ByDirection["inbound_or_outbound"] += p.C
	}

	type dcount struct {
		Direction string
		C         int64
	}
	var dCounts []dcount
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("direction AS direction, COUNT(*) AS c").
		Group("direction").Scan(&dCounts)
	for _, d := range dCounts {
		stats.ByDirection[d.Direction] = d.C
	}

	type tcount struct {
		MsgType string
		C       int64
	}
	var tCounts []tcount
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("msg_type AS msg_type, COUNT(*) AS c").
		Group("msg_type").Scan(&tCounts)
	for _, t := range tCounts {
		stats.ByMsgType[t.MsgType] = t.C
	}

	type acount struct {
		AccountID string
		C         int64
	}
	var aCounts []acount
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("account_id AS account_id, COUNT(*) AS c").
		Group("account_id").Order("c DESC").Limit(50).Scan(&aCounts)
	for _, a := range aCounts {
		stats.ByAccount[a.AccountID] = a.C
	}

	threshold24h := time.Now().Add(-24 * time.Hour)
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("sent_at >= ?", threshold24h).
		Count(&stats.Recent24h)

	return stats, nil
}

// GetLastByPlatformAccount 取分区最新一条消息（按 sent_at DESC）
// 用于 Consume 在内存流为空时的 DB 兜底
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

// ListByConversationContext 按 (platform, account_id, sender_id OR receiver_id) 拉取消息
// 用于统一收件箱合并 message_hub 消息流（customer 可能是 sender 也可能是 receiver）
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

// ============== InboxConversationRepository ==============

// InboxConversationRepository 统一收件箱会话仓库
type InboxConversationRepository struct {
	db *gorm.DB
}

// NewInboxConversationRepository 创建收件箱会话仓库实例
func NewInboxConversationRepository() *InboxConversationRepository {
	return &InboxConversationRepository{db: _db.GetDB()}
}

// NewInboxConversationRepositoryWithDB 创建指定数据库连接的 InboxConversationRepository 实例
// 用于 service 层依赖注入与单元测试；db 为 nil 时所有方法做无操作短路。
func NewInboxConversationRepositoryWithDB(db *gorm.DB) *InboxConversationRepository {
	return &InboxConversationRepository{db: db}
}

// SetDB 注入 db（用于测试）
//
// 五层架构 §三.5 + §七：仓库方法必须首参为 ctx context.Context。
func (r *InboxConversationRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// SetInboxConversationRepoDB 工具函数（service 层使用）
func SetInboxConversationRepoDB(r *InboxConversationRepository, db *gorm.DB) {
	r.SetDB(context.Background(), db)
}

// FindByPlatformAccountCustomer 按平台/账号/客户查找会话
func (r *InboxConversationRepository) FindByPlatformAccountCustomer(ctx context.Context, platform, accountID, customerID string) (*model.InboxConversation, error) {
	var conv model.InboxConversation
	err := r.db.Where("platform = ? AND account_id = ? AND customer_id = ?",
		platform, accountID, customerID).First(&conv).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// Create 创建会话
func (r *InboxConversationRepository) Create(ctx context.Context, conv *model.InboxConversation) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Create(conv).Error
}

// UpdateLastMessage 更新最后消息字段（含 unread_count 自增）
func (r *InboxConversationRepository) UpdateLastMessage(ctx context.Context, id uint, lastMessage string, lastMessageAt time.Time, unreadInc int) error {
	// inbox_conversations 表的真实列名是 last_message_preview（varchar(500)）。
	// 早期代码错用 last_message，每次都报 SQLSTATE 42703。这里同时截断到 500 字符以匹配列宽。
	const previewMaxLen = 500
	if len(lastMessage) > previewMaxLen {
		lastMessage = lastMessage[:previewMaxLen]
	}
	return r.db.Model(&model.InboxConversation{}).Where("id = ?", id).
		Updates(map[string]any{
			"last_message_preview": lastMessage,
			"last_message_at":      lastMessageAt,
			"unread_count":         gorm.Expr("unread_count + ?", unreadInc),
		}).Error
}

// GetByID 按 ID 获取会话
func (r *InboxConversationRepository) GetByID(ctx context.Context, id uint) (*model.InboxConversation, error) {
	var conv model.InboxConversation
	err := r.db.First(&conv, id).Error
	return &conv, err
}

// Update 更新会话
func (r *InboxConversationRepository) Update(ctx context.Context, conv *model.InboxConversation) error {
	return r.db.Save(conv).Error
}

// ListByAccount 按账号列出会话
func (r *InboxConversationRepository) ListByAccount(ctx context.Context, platform, accountID string, page, pageSize int) ([]*model.InboxConversation, int64, error) {
	var items []*model.InboxConversation
	var total int64
	query := r.db.Model(&model.InboxConversation{}).
		Where("platform = ? AND account_id = ?", platform, accountID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := query.Order("pinned DESC, last_message_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	return items, total, err
}

// ============== InboxConversationRepository 扩展方法 ==============
// 以下方法服务于 InboxService 的五层架构：
//   - UpsertFromMessage / AssignTx：将原 service 层 db.Transaction 收敛到 repo
//   - ListByQuery / MarkRead / UpdateField / CountByAssignedToStatus / GetStats：替代 service 层零散 db 调用

// UpsertFromMessageInput upsert 入参（与 service 层字段一一对应）
type UpsertFromMessageInput struct {
	Platform           string
	AccountID          string
	CustomerID         string
	CustomerName       string
	ConversationID     string
	LastMessageID      uint
	LastMessagePreview string
	LastMessageAt      time.Time
	LastMessageFrom    string
}

// UpsertFromMessage 根据消息 upsert 会话（事务封装在 repo 内）
//
// 行为保持与原 service 层 upsertInternal 一致：
//   - 客户首条消息：unread_count=1，status=unread
//   - 客户后续消息：unread_count+1，closed 状态自动转 unread，assigned+无 assigned_to 也转 unread
//   - 非客户消息：不修改 unread_count
func (r *InboxConversationRepository) UpsertFromMessage(ctx context.Context, in UpsertFromMessageInput) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conv model.InboxConversation
		err := tx.Where("platform = ? AND account_id = ? AND customer_id = ?",
			in.Platform, in.AccountID, in.CustomerID).
			First(&conv).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			conv = model.InboxConversation{
				Platform:           in.Platform,
				AccountID:          in.AccountID,
				CustomerID:         in.CustomerID,
				CustomerName:       in.CustomerName,
				ConversationID:     in.ConversationID,
				Status:             "unread",
				UnreadCount:        0,
				TotalCount:         1,
				LastMessageID:      in.LastMessageID,
				LastMessagePreview: in.LastMessagePreview,
				LastMessageAt:      &in.LastMessageAt,
				LastMessageFrom:    in.LastMessageFrom,
			}
			if in.LastMessageFrom == "customer" {
				conv.UnreadCount = 1
			}
			return tx.Create(&conv).Error
		}
		if err != nil {
			return err
		}

		updates := map[string]any{
			"last_message_id":      in.LastMessageID,
			"last_message_preview": in.LastMessagePreview,
			"last_message_at":      in.LastMessageAt,
			"last_message_from":    in.LastMessageFrom,
			"total_count":          conv.TotalCount + 1,
			"customer_name":        firstNonEmpty(in.CustomerName, conv.CustomerName),
			"conversation_id":      firstNonEmpty(in.ConversationID, conv.ConversationID),
		}
		if in.LastMessageFrom == "customer" {
			updates["unread_count"] = conv.UnreadCount + 1
			if conv.Status == "closed" {
				updates["status"] = "unread"
				updates["closed_at"] = nil
			} else if conv.Status == "assigned" && conv.AssignedTo == "" {
				updates["status"] = "unread"
			}
		}
		return tx.Model(&model.InboxConversation{}).
			Where("id = ?", conv.ID).
			Updates(updates).Error
	})
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

// ListByQuery 按条件分页查询会话（行为与原 service.List 一致）
func (r *InboxConversationRepository) ListByQuery(ctx context.Context, q InboxConversationQuery) ([]*model.InboxConversation, int64, error) {
	if r == nil || r.db == nil {
		return []*model.InboxConversation{}, 0, nil
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 200 {
		q.PageSize = 20
	}

	tx := r.db.WithContext(ctx).Model(&model.InboxConversation{})
	if q.Platform != "" {
		tx = tx.Where("platform = ?", q.Platform)
	}
	if q.AccountID != "" {
		tx = tx.Where("account_id = ?", q.AccountID)
	}
	if q.CustomerID != "" {
		tx = tx.Where("customer_id = ?", q.CustomerID)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.AssignedTo != "" {
		tx = tx.Where("assigned_to = ?", q.AssignedTo)
	}
	if q.AssignedSOP > 0 {
		tx = tx.Where("assigned_to_sop = ?", q.AssignedSOP)
	}
	if q.Pinned != nil {
		tx = tx.Where("pinned = ?", *q.Pinned)
	}
	if q.Starred != nil {
		tx = tx.Where("starred = ?", *q.Starred)
	}
	if q.Muted != nil {
		tx = tx.Where("muted = ?", *q.Muted)
	}
	if q.Keyword != "" {
		tx = tx.Where("last_message_preview LIKE ?", "%"+q.Keyword+"%")
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderBy := "last_message_at DESC" // 默认按最新消息倒序
	switch q.OrderBy {
	case "pinned_first":
		orderBy = "pinned DESC, last_message_at DESC"
	case "unread_desc":
		orderBy = "unread_count DESC, last_message_at DESC"
	case "oldest_asc":
		orderBy = "last_message_at ASC"
	case "latest_desc":
		orderBy = "last_message_at DESC"
	}

	var list []*model.InboxConversation
	if err := tx.Order(orderBy).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// MarkRead 标记会话已读（重置未读计数 + 状态置为 open）
func (r *InboxConversationRepository) MarkRead(ctx context.Context, conversationID uint) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("id = ?", conversationID).
		Updates(map[string]any{
			"unread_count": 0,
			"status":       "open",
		}).Error
}

// UpdateField 更新单个字段（用于 Pin/Star/Mute/Tag 等开关型操作）
func (r *InboxConversationRepository) UpdateField(ctx context.Context, conversationID uint, field string, value any) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("id = ?", conversationID).
		Update(field, value).Error
}

// CountByAssignedToStatus 按 assigned_to + status 集合统计会话数（用于客服负载查询）
func (r *InboxConversationRepository) CountByAssignedToStatus(ctx context.Context, staffUserID string, statuses []string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("assigned_to = ? AND status IN ?", staffUserID, statuses).
		Count(&n).Error
	return n, err
}

// AssignTxInput 分配事务入参
type AssignTxInput struct {
	ConversationID uint
	Action         string // assign/reassign/release/close/reopen
	ToType         string // human/sop/ai
	ToUserID       string
	ToSOPID        uint
	OperatorID     string
	Remark         string
}

// AssignTxOutput 分配事务出参
//
// OldAssignedTo / NewAssignedTo 用于 service 层同步内存负载缓存（不属于 DB 操作）。
type AssignTxOutput struct {
	OldAssignedTo string
	NewAssignedTo string
	History       *model.InboxAssignment
}

// AssignTx 在单个事务内完成「更新会话 + 写入分配历史」
//
// 五层架构 §三.5：原 service 层 s.db.Transaction 收敛到 repo。
// 错误约定：会话不存在时返回 errors.New("conversation not found")（service 层据此映射 ErrInboxConversationMissing）。
func (r *InboxConversationRepository) AssignTx(ctx context.Context, in AssignTxInput) (*AssignTxOutput, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	out := &AssignTxOutput{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conv model.InboxConversation
		if err := tx.First(&conv, in.ConversationID).Error; err != nil {
			return errors.New("conversation not found")
		}
		out.OldAssignedTo = conv.AssignedTo

		updates := map[string]any{}
		now := time.Now()
		switch in.Action {
		case "assign":
			updates["status"] = "assigned"
			updates["assigned_at"] = &now
		case "reassign":
			updates["status"] = "assigned"
			updates["assigned_at"] = &now
		case "release":
			updates["status"] = "open"
			updates["assigned_to"] = ""
			updates["assigned_to_sop"] = 0
			updates["assigned_at"] = nil
		case "close":
			updates["status"] = "closed"
			updates["closed_at"] = &now
		case "reopen":
			updates["status"] = "unread"
			updates["closed_at"] = nil
			updates["assigned_to"] = ""
			updates["assigned_to_sop"] = 0
		}

		if in.Action == "assign" || in.Action == "reassign" {
			updates["assigned_to"] = ""
			updates["assigned_to_sop"] = 0
			switch in.ToType {
			case "human":
				updates["assigned_to"] = in.ToUserID
				out.NewAssignedTo = in.ToUserID
			case "sop":
				updates["assigned_to_sop"] = in.ToSOPID
			case "ai":
				// 暂不绑定具体 ID
			}
		}

		if err := tx.Model(&model.InboxConversation{}).
			Where("id = ?", in.ConversationID).
			Updates(updates).Error; err != nil {
			return err
		}

		hist := &model.InboxAssignment{
			ConversationID: conv.ID,
			Platform:       conv.Platform,
			AccountID:      conv.AccountID,
			CustomerID:     conv.CustomerID,
			Action:         in.Action,
			FromType:       inferFromType(conv.AssignedTo, conv.AssignedToSOP),
			FromUserID:     conv.AssignedTo,
			ToType:         in.ToType,
			ToUserID:       in.ToUserID,
			ToSOPID:        in.ToSOPID,
			OperatorID:     in.OperatorID,
			Remark:         in.Remark,
		}
		if err := tx.Create(hist).Error; err != nil {
			return err
		}
		out.History = hist
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// InboxStatsResult 收件箱统计结果（与 service.InboxStats 字段对齐）
type InboxStatsResult struct {
	Total        int64
	Unread       int64
	Open         int64
	Assigned     int64
	Closed       int64
	ByPlatform   map[string]int64
	ByAssignedTo map[string]int64
	OverdueCount int64
}

// GetStats 收件箱多维度统计（行为与原 service.GetStats 一致）
//
// 参数：
//   - validStatuses: 参与状态分布统计的状态集合
//   - activeStatuses: 客服活跃会话状态集合（用于 ByAssignedTo）
//   - from: last_message_from 过滤值（通常 "customer"）
//   - threshold: 超时阈值（last_message_at <= threshold 视为 overdue）
func (r *InboxConversationRepository) GetStats(ctx context.Context, validStatuses, activeStatuses []string, from string, threshold time.Time) (*InboxStatsResult, error) {
	if r == nil || r.db == nil {
		return &InboxStatsResult{
			ByPlatform: map[string]int64{}, ByAssignedTo: map[string]int64{},
		}, nil
	}
	stats := &InboxStatsResult{
		ByPlatform:   map[string]int64{},
		ByAssignedTo: map[string]int64{},
	}

	type sc struct {
		Status string
		C      int64
	}
	var scs []sc
	if err := r.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Select("status, COUNT(*) AS c").
		Where("status IN ?", validStatuses).
		Group("status").Scan(&scs).Error; err != nil {
		return nil, err
	}
	for _, s := range scs {
		switch s.Status {
		case "unread":
			stats.Unread = s.C
		case "open":
			stats.Open = s.C
		case "assigned":
			stats.Assigned = s.C
		case "closed":
			stats.Closed = s.C
		}
		stats.Total += s.C
	}

	type pc struct {
		Platform string
		C        int64
	}
	var pcs []pc
	r.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Select("platform AS platform, COUNT(*) AS c").
		Group("platform").Scan(&pcs)
	for _, p := range pcs {
		stats.ByPlatform[p.Platform] = p.C
	}

	type ac struct {
		AssignedTo string
		C          int64
	}
	var acs []ac
	r.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Select("assigned_to, COUNT(*) AS c").
		Where("assigned_to <> '' AND status IN ?", activeStatuses).
		Group("assigned_to").Scan(&acs)
	for _, a := range acs {
		stats.ByAssignedTo[a.AssignedTo] = a.C
	}

	var overdue int64
	r.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("status IN ? AND last_message_from = ? AND last_message_at <= ?",
			[]string{"unread", "open", "assigned"}, from, threshold).
		Count(&overdue)
	stats.OverdueCount = overdue

	return stats, nil
}

// ============== InboxAssignmentRepository ==============

// InboxAssignmentRepository 统一收件箱分配历史仓库
type InboxAssignmentRepository struct {
	db *gorm.DB
}

// NewInboxAssignmentRepository 创建分配历史仓库实例
func NewInboxAssignmentRepository() *InboxAssignmentRepository {
	return &InboxAssignmentRepository{db: _db.GetDB()}
}

// NewInboxAssignmentRepositoryWithDB 创建指定数据库连接的 InboxAssignmentRepository 实例
// 用于 service 层依赖注入与单元测试；db 为 nil 时所有方法做无操作短路。
func NewInboxAssignmentRepositoryWithDB(db *gorm.DB) *InboxAssignmentRepository {
	return &InboxAssignmentRepository{db: db}
}

// SetDB 注入 db（用于测试）
func (r *InboxAssignmentRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// ListByConversationID 按会话 ID 分页查询分配历史
func (r *InboxAssignmentRepository) ListByConversationID(ctx context.Context, conversationID uint, page, pageSize int) ([]*model.InboxAssignment, int64, error) {
	if r == nil || r.db == nil {
		return []*model.InboxAssignment{}, 0, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	tx := r.db.WithContext(ctx).Model(&model.InboxAssignment{})
	if conversationID > 0 {
		tx = tx.Where("conversation_id = ?", conversationID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.InboxAssignment
	if err := tx.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// AssignmentCount 分配计数（按 to_user_id 聚合）
type AssignmentCount struct {
	AssignedTo string
	N          int64
}

// GroupCountByToUserID 按 to_user_id 聚合统计分配次数（用于轮询分配选最闲客服）
func (r *InboxAssignmentRepository) GroupCountByToUserID(ctx context.Context, candidates []string, action string) ([]AssignmentCount, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var counts []AssignmentCount
	err := r.db.WithContext(ctx).Model(&model.InboxAssignment{}).
		Select("to_user_id AS assigned_to, COUNT(*) AS n").
		Where("to_user_id IN ? AND action = ?", candidates, action).
		Group("to_user_id").Scan(&counts).Error
	if err != nil {
		return nil, err
	}
	return counts, nil
}

// ============== 仓库内部辅助函数 ==============

// inferFromType 根据 assigned_to / assigned_to_sop 推断来源类型
func inferFromType(assignedTo string, assignedSOP uint) string {
	if assignedSOP > 0 {
		return "sop"
	}
	if assignedTo != "" {
		return "human"
	}
	return "system"
}

// firstNonEmpty 返回第一个非空字符串（trim 后非空）
func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
