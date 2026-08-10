// 拆分自 message_hub_inbox.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
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
		return r.UpsertFromMessageTx(tx, in)
	})
}

// UpsertFromMessageTx 与 UpsertFromMessage 等价，但接受外部 tx（用于跨表事务）。
// 调用方负责事务边界（如 MessageHubRepository.CreateWithInboxTx）。
// tx 为 nil 时返回 nil（防御性短路，与 UpsertFromMessage 行为一致）。
func (r *InboxConversationRepository) UpsertFromMessageTx(tx *gorm.DB, in UpsertFromMessageInput) error {
	if r == nil || tx == nil {
		return nil
	}
	// 清洗非法 UTF-8：message_hub.content 等来源可能携带损坏字节（如 0xe8 0x81 截断的多字节序列），
	// 直接写入 inbox_conversations.last_message_preview 会触发 PG "invalid byte sequence for
	// encoding UTF8" 错误，阻断收件箱同步（reconcile backfill 曾因此失败）。此处防御性清洗，保证同步不中断。
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
			// 首条即我方（AI/坐席）消息 → 已是“已读/待处理”态
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
		// 我方（AI/坐席）已回复 → 视为已读：未读清零；仅“未读”态转“待处理/消息池”
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

	orderBy := "pinned DESC, last_message_at DESC" // 默认：置顶优先，其次按最后消息时间倒序
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
	// 关联查询读取每条会话在 message_hub 中的“最后一条”消息，覆盖冗余的 last_message_* 字段，
	// 保证列表展示的最新内容（客户上报 / AI 回复）始终来自消息事实源。
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
}

// ============== 收件箱对账（Reconcile） ==============
// 用于修正历史数据：以 message_hub 的“最后一条消息”为事实源，重算 inbox_conversations 的未读/状态。

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
