package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

type MessageHubRepository struct {
	db *gorm.DB
}

func NewMessageHubRepository() *MessageHubRepository {
	return &MessageHubRepository{db: _db.GetDB()}
}

func NewMessageHubRepositoryWithDB(db *gorm.DB) *MessageHubRepository {
	return &MessageHubRepository{db: db}
}

func (r *MessageHubRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func SetMessageHubRepoDB(r *MessageHubRepository, db *gorm.DB) {
	r.SetDB(context.Background(), db)
}

func (r *MessageHubRepository) Create(ctx context.Context, hub *model.MessageHub) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Create(hub).Error
}

func (r *MessageHubRepository) ListRecentInboundBySender(ctx context.Context, platform, senderID string, limit int) ([]model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var hubs []model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND sender_id = ? AND direction = 'inbound'", platform, senderID).
		Order("sent_at DESC").
		Limit(limit).
		Find(&hubs).Error
	if err != nil {
		return nil, err
	}
	return hubs, nil
}

func (r *MessageHubRepository) CreateWithInboxTx(

	ctx context.Context,

	hub *model.MessageHub,

	inboxRepo *InboxConversationRepository,

	input UpsertFromMessageInput,

) error {
	if r == nil || r.db == nil {
		return nil
	}
	if inboxRepo == nil {
		return r.Create(ctx, hub)
	}

	// 幂等预检查：已落库的消息直接返回，避免并发竞态下事务内 INSERT 触发
	// duplicate key → PostgreSQL 将事务置为 aborted 状态 → commit 阶段报
	// "commit unexpectedly resulted in rollback"（即使代码 return nil 也救不回来）。
	if hub.MsgID != "" {
		var existing model.MessageHub
		if err := r.db.WithContext(ctx).Where("msg_id = ?", hub.MsgID).First(&existing).Error; err == nil {
			return nil
		}
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(hub).Error; err != nil {
			if isDuplicateKeyErr(err) {
				return nil
			}
			return err
		}
		return inboxRepo.UpsertFromMessageTx(ctx, tx, input)
	})
}

func (r *MessageHubRepository) GetByID(ctx context.Context, id uint) (*model.MessageHub, error) {
	var hub model.MessageHub
	err := r.db.First(&hub, id).Error
	return &hub, err
}

func (r *MessageHubRepository) GetByMsgID(ctx context.Context, msgID string) (*model.MessageHub, error) {
	var hub model.MessageHub
	err := r.db.Where("msg_id = ?", msgID).First(&hub).Error
	return &hub, err
}

func (r *MessageHubRepository) GetByContentHash(ctx context.Context, canonicalHash string) (*model.MessageHub, error) {
	if canonicalHash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var hub model.MessageHub
	err := r.db.Where("msg_id = ?", canonicalHash).First(&hub).Error
	return &hub, err
}

func (r *MessageHubRepository) GetByPlatformContent(ctx context.Context, platform, content string) (*model.MessageHub, error) {
	if platform == "" || content == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var hub model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND direction = 'outbound' AND md5(content) = md5(?)", platform, content).
		Order("sent_at DESC").
		First(&hub).Error
	return &hub, err
}

func (r *MessageHubRepository) GetByPlatformContentNormalized(ctx context.Context, platform, content string) (*model.MessageHub, error) {
	if platform == "" || content == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var hub model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND direction = 'outbound' AND md5(regexp_replace(content, '\\s+', '', 'g')) = md5(regexp_replace(?, '\\s+', '', 'g'))", platform, content).
		Order("sent_at DESC").
		First(&hub).Error
	return &hub, err
}

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

func (r *MessageHubRepository) Update(ctx context.Context, hub *model.MessageHub) error {
	return r.db.Save(hub).Error
}

func (r *MessageHubRepository) Delete(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.MessageHub{}).Error
}

func (r *MessageHubRepository) MarkReadByID(ctx context.Context, id uint) error {
	now := time.Now()
	return r.db.Model(&model.MessageHub{}).Where("id = ?", id).
		Updates(map[string]any{
			"is_read": true,
			"read_at": now,
		}).Error
}

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

// UpdateMsgID 回写平台消息 ID（ChatbotX 模式移植 T2）。
//
// 业务背景：出站消息落库时平台尚未返回消息 ID（历史实现自造 wa-out-{UnixNano}
// 占位），发送成功后平台返回 wamid / message_id，必须回写用于：
//  1. echo 精确去重（入站回显按 platform+msg_id 命中 outgoing 行即拦截）
//  2. 状态回执对账（WA statuses 按 wamid 定位消息行）
//  3. 撤回/引用等平台能力的基础
//
// best-effort：回写失败仅影响对账，不阻断发送链路（调用方自行 WARN）。
// 仅更新 direction='outgoing' 行，防御性避免误改入站行。
func (r *MessageHubRepository) UpdateMsgID(ctx context.Context, id uint, platformMsgID string) error {
	if r == nil || r.db == nil || platformMsgID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ? AND direction = ?", id, "outbound").
		Update("msg_id", platformMsgID).Error
}

// GetOutgoingByPlatformMsgID 按平台消息 ID 精确查找出站行（echo 拦截用）。
// 范围限定单账号 + 出站方向，避免跨账号/跨方向的偶然 ID 碰撞误判。
func (r *MessageHubRepository) GetOutgoingByPlatformMsgID(ctx context.Context, platform, accountID, msgID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil || msgID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var existing model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND msg_id = ? AND direction = ?", platform, accountID, msgID, "outbound").
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

func (r *MessageHubRepository) GetByDedupHash(ctx context.Context, platform, dedupHash string) (*model.MessageHub, error) {
	if r == nil || r.db == nil || dedupHash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var existing model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND dedup_hash = ?", platform, dedupHash).
		Order("id DESC").
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

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

type HubListQuery struct {
	Platform       string
	Status         string
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
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
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
	case "sent_at ASC", "sent_at DESC", "created_at ASC", "created_at DESC", "id ASC", "id DESC":
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

func (r *MessageHubRepository) GetLastByConversation(ctx context.Context, conversationID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if conversationID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var msg model.MessageHub
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("sent_at DESC").First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// SetDB 注入 db（用于测试）
//
// 五层架构 §三.5 + §七：仓库方法必须首参为 ctx context.Context。
func (r *InboxConversationRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
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
		return r.UpsertFromMessageTx(ctx, tx, in)
	})
}

// UpsertFromMessageTx 与 UpsertFromMessage 等价，但接受外部 tx（用于跨表事务）。
// 调用方负责事务边界（如 MessageHubRepository.CreateWithInboxTx）。
// tx 为 nil 时返回 nil（防御性短路，与 UpsertFromMessage 行为一致）。
func (r *InboxConversationRepository) UpsertFromMessageTx(ctx context.Context, tx *gorm.DB, in UpsertFromMessageInput) error {
	if r == nil || tx == nil {
		return nil
	}
	tx = tx.WithContext(ctx)

	in.LastMessagePreview = sanitizeUTF8(in.LastMessagePreview)
	in.CustomerName = sanitizeUTF8(in.CustomerName)
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
		} else {

			conv.Status = "open"
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
	} else {

		updates["unread_count"] = 0
		if conv.Status == "unread" {
			updates["status"] = "open"
		}
	}
	return tx.Model(&model.InboxConversation{}).
		Where("id = ?", conv.ID).
		Updates(updates).Error
}

// DeleteOrphanInboxByConversation 删除同一 (platform, account_id, conversation_id)
// 下、customer_id 与 keepCustomerID 不一致的孤儿收件箱行。群聊 sender_id 被时间戳
// 污染后，同一会话会生成多条 customer_id 各异的碎片行，合并为按 conversation_id
// 归属的单行后，其余碎片需清理以免在收件箱 UI 重复出现。
func (r *InboxConversationRepository) DeleteOrphanInboxByConversation(ctx context.Context, platform, accountID, conversationID, keepCustomerID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND conversation_id = ? AND customer_id <> ?",
			platform, accountID, conversationID, keepCustomerID).
		Delete(&model.InboxConversation{})
	return res.RowsAffected, res.Error
}

// sanitizeUTF8 将字符串中的非法 UTF-8 字节序列替换为 Unicode 替换符（U+FFFD），
// 避免将损坏内容写入 Postgres（utf8 编码）时报 "invalid byte sequence" 错误。
// 这是防御性清洗：桥接/历史消息可能携带截断的多字节序列（如 0xe8 0x81）。
func sanitizeUTF8(s string) string {
	if s == "" {
		return s
	}
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "�")
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

	// 默认置顶优先（收件箱标准语义），其余按 id 倒序；显式OrderBy仍可覆盖
	orderBy := "pinned DESC, id DESC"
	switch q.OrderBy {
	case "pinned_first":
		orderBy = "pinned DESC, last_message_at DESC"
	case "id_desc":
		orderBy = "id DESC"
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

	r.attachLatestMessages(ctx, list)
	return list, total, nil
}

// attachLatestMessages 用关联查询（DISTINCT ON conversation_id）读取每个会话的最新一条 message_hub 消息，
// 覆盖收件箱会话上的 last_message_* 冗余字段，避免与消息表不一致。
// 注意：仅覆盖有对应 message_hub 记录的会话；无消息记录的会话保留原字段。
func (r *InboxConversationRepository) attachLatestMessages(ctx context.Context, list []*model.InboxConversation) {
	if len(list) == 0 {
		return
	}
	ids := make([]string, 0, len(list))
	for _, c := range list {
		if c.ConversationID != "" {
			ids = append(ids, c.ConversationID)
		}
	}
	if len(ids) == 0 {
		return
	}
	type latestHub struct {
		ConversationID string
		ID             uint
		Content        string
		SentAt         *time.Time
		Direction      string
		IsAIReply      bool
	}
	var rows []latestHub
	if err := r.db.WithContext(ctx).
		Table("message_hub").
		Select("DISTINCT ON (conversation_id) conversation_id, id, content, sent_at, direction, is_ai_reply").
		Where("conversation_id IN ?", ids).
		Order("conversation_id, sent_at DESC").
		Find(&rows).Error; err != nil {
		return
	}
	byConv := make(map[string]*latestHub, len(rows))
	for i := range rows {
		byConv[rows[i].ConversationID] = &rows[i]
	}
	for _, c := range list {
		l, ok := byConv[c.ConversationID]
		if !ok {
			continue
		}
		c.LastMessageID = l.ID
		preview := l.Content
		if len(preview) > 200 {
			preview = preview[:200]
		}
		c.LastMessagePreview = preview
		c.LastMessageAt = l.SentAt
		switch {
		case l.Direction == "inbound":
			c.LastMessageFrom = "customer"
		case l.IsAIReply:
			c.LastMessageFrom = "ai"
		default:
			c.LastMessageFrom = "staff"
		}
	}
	r.attachSessionMessageFallback(ctx, list)
}

// attachSessionMessageFallback 对仍无 last_message_preview 的会话，从 session_messages
// 读取其最新一条消息内容补全预览（截断到 200 字符）。
func (r *InboxConversationRepository) attachSessionMessageFallback(ctx context.Context, list []*model.InboxConversation) {
	missing := make([]*model.InboxConversation, 0, len(list))
	for _, c := range list {
		if c.ConversationID != "" && (c.LastMessagePreview == "") {
			missing = append(missing, c)
		}
	}
	if len(missing) == 0 {
		return
	}
	ids := make([]string, 0, len(missing))
	for _, c := range missing {
		ids = append(ids, c.ConversationID)
	}
	type latestSM struct {
		SessionID string
		Content   string
	}
	var rows []latestSM
	if err := r.db.WithContext(ctx).
		Table("session_messages").
		Select("DISTINCT ON (session_id) session_id, content").
		Where("session_id IN ?", ids).
		Order("session_id, id DESC").
		Find(&rows).Error; err != nil {
		return
	}
	bySession := make(map[string]string, len(rows))
	for i := range rows {
		content := rows[i].Content
		if len(content) > 200 {
			content = content[:200]
		}
		bySession[rows[i].SessionID] = content
	}
	for _, c := range missing {
		if preview, ok := bySession[c.ConversationID]; ok && preview != "" {
			c.LastMessagePreview = preview
		}
	}
}

// ReconcileUnread 以 message_hub 最后一条消息为准，批量重算全部会话的未读计数与状态。
//   - 最后一条是我方（AI/坐席）消息 → 未读清零；原“未读”态转“待处理/消息池”。
//   - 最后一条是客户消息 → 未读记为 1（至少有一条未读）；已分配/已关闭保持不变，其余转“未读”。
//
// 仅更新存在 message_hub 记录的会话；无消息记录的会话保持原状。
func (r *InboxConversationRepository) ReconcileUnread(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Exec(`
		WITH latest AS (
			SELECT DISTINCT ON (conversation_id) conversation_id, direction
			FROM message_hub
			ORDER BY conversation_id, sent_at DESC
		)
		UPDATE inbox_conversations ic
		SET unread_count = CASE
		        WHEN l.direction = 'inbound' THEN GREATEST(COALESCE(ic.unread_count, 0), 1)
		        ELSE 0
		    END,
		    status = CASE
		        WHEN l.direction = 'inbound'
		            THEN CASE WHEN ic.status IN ('assigned', 'closed') THEN ic.status ELSE 'unread' END
		        ELSE CASE WHEN ic.status = 'unread' THEN 'open' ELSE ic.status END
		    END
		FROM latest l
		WHERE ic.conversation_id = l.conversation_id
	`)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// FindOverdueConversations 查询“超时未响应”的会话：
// 最后一条是客户消息、且早于 threshold、且仍处于活跃态（未读/待处理/已分配）。
func (r *InboxConversationRepository) FindOverdueConversations(ctx context.Context, threshold time.Time, limit int) ([]*model.InboxConversation, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var list []*model.InboxConversation
	q := r.db.WithContext(ctx).
		Where("last_message_from = ?", "customer").
		Where("last_message_at IS NOT NULL AND last_message_at < ?", threshold).
		Where("status IN ?", []string{"unread", "open", "assigned"})
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Order("last_message_at ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListOnlineAgentIDs 返回当前在线（status='online'）的坐席 agent_id 列表（字符串形式，可直接用于分配）。
func (r *InboxConversationRepository) ListOnlineAgentIDs(ctx context.Context) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var ids []string
	if err := r.db.WithContext(ctx).
		Table("agent_statuses").
		Where("status = ?", "online").
		Pluck("CAST(agent_id AS TEXT)", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
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
	Action         string
	ToType         string
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
		case "close":
			updates["status"] = "closed"
			updates["closed_at"] = &now
		case "reopen":
			updates["status"] = "unread"
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

			}
		}

		// 先清掉需要置 NULL 的字段（硬编码 SET NULL，绕开 GORM map nil/参数绑定歧义）
		switch in.Action {
		case "release":
			if err := tx.Model(&model.InboxConversation{}).
				Where("id = ?", in.ConversationID).
				UpdateColumn("assigned_at", gorm.Expr("NULL")).Error; err != nil {
				return err
			}
		case "reopen":
			if err := tx.Model(&model.InboxConversation{}).
				Where("id = ?", in.ConversationID).
				UpdateColumn("closed_at", gorm.Expr("NULL")).Error; err != nil {
				return err
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
