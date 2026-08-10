package repository

import (
	"context"

	"fmt"

	"time"

	"hivemtk-user/internal/model"

	_db "hivemtk-user/internal/pkg/db"

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
		// 无 inboxRepo 时退化为单表 Create（兼容老路径）
		return r.Create(ctx, hub)
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(hub).Error; err != nil {
			// 唯一键冲突视为幂等成功（与 Create 行为一致）
			if isDuplicateKeyErr(err) {
				return nil
			}
			return err
		}
		// 调用 inboxRepo 的 tx 版本，复用同一个 tx
		return inboxRepo.UpsertFromMessageTx(tx, input)
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

func (r *MessageHubRepository) AckOutboundDeliveredBatch(ctx context.Context, channel, accountID string, msgIDs []string) (int64, error) {
	if r.db == nil || len(msgIDs) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		// 翻转范围覆盖 pending 与 inflight：ack 可能在「认领→超时回收→重新认领」的间隙到达，
		// 此时行已回退为 pending；两种状态都翻 delivered 保证幂等且绝不漏翻（at-least-once 安全）。
		Where("platform = ? AND account_id = ? AND direction = 'outbound' AND msg_id IN ? AND status IN ('pending','inflight')", channel, accountID, msgIDs).
		Update("status", "delivered")
	return res.RowsAffected, res.Error
}

func (r *MessageHubRepository) ClaimPendingOutbound(ctx context.Context, channel, accountID string, limit int, claimTimeout time.Duration) ([]model.MessageHub, error) {
	if r == nil || r.db == nil || limit <= 0 {
		return nil, nil
	}
	cutoff := time.Now().Add(-claimTimeout)
	// 1) 回收超时 inflight → pending（惰性 reaper）。
	//    注意：claimed_at 为 NULL 的 pending 行（新入队、从未被认领）必须显式排除，
	//    否则 `claimed_at < cutoff` 对 NULL 求值为 unknown → 整条 WHERE 失效、回收 0 行（虽无害，
	//    但语义上不应依赖 NULL 比较）。这里已用 `status = 'inflight'` 精确锁定目标行。
	if err := r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		Where("platform = ? AND account_id = ? AND direction = 'outbound' AND status = 'inflight' AND claimed_at IS NOT NULL AND claimed_at < ?", channel, accountID, cutoff).
		Updates(map[string]any{"status": "pending", "claimed_at": nil}).Error; err != nil {
		fmt.Printf("[ClaimPendingOutbound] 回收超时 inflight 失败（继续认领）: %v\n", err)
	}
	// 2) 原子认领 top-N pending → inflight。
	//    关键并发安全：子查询必须加 `FOR UPDATE SKIP LOCKED`。
	//    否则 PostgreSQL 在 READ COMMITTED 下，UPDATE...RETURNING 仅按 `id IN (...)` 重新判定，
	//    不复核子查询的 `status='pending'`——若并发轮询 T1 已把某 pending 翻为 inflight 并提交，
	//    T2 仍会按各自快照算出的同一 id 集合再次翻该行为 inflight 并 RETURNING → 同一条被两个
	//    轮询同时认领 → 重复下发。`FOR UPDATE SKIP LOCKED` 让 T2 的子查询跳过被 T1 锁定的行，
	//    实现服务端权威互斥（多标签页并发轮询也安全）。
	list := make([]model.MessageHub, 0, limit)
	const q = `UPDATE message_hub SET status = 'inflight', claimed_at = now()
		WHERE id IN (
			SELECT id FROM message_hub
			WHERE platform = ? AND account_id = ? AND direction = 'outbound' AND status = 'pending'
			ORDER BY id ASC LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`
	if err := r.db.WithContext(ctx).Raw(q, channel, accountID, limit).Scan(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
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
	Status         string // 可选：按消息状态过滤（如 "failed" 待补发）
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

func (r *MessageHubRepository) HasUnrepliedCustomerMessage(ctx context.Context, conversationID string, replyWindow time.Duration) (unreplied bool, withinWindow bool, err error) {
	if r == nil || r.db == nil {
		return false, false, nil
	}
	if conversationID == "" {
		return false, false, nil
	}
	var last model.MessageHub
	// 按 conversation_id 查最新一条消息（不分 platform / account_id，因为同一 conversation_id 唯一）
	if err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("sent_at DESC").
		Limit(1).
		First(&last).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 会话无消息 → 视为未回复且在窗口内（首条消息触发）
			return true, true, nil
		}
		// 查询失败：保守视为已回复（不触发 AI，避免误触发）
		return false, false, err
	}
	// 最后一条消息方向判断：
	//   - outbound（平台自己发的）→ 已回复，不触发 AI
	//   - inbound（客户发的）→ 未回复，继续判断 5min 窗口
	if last.Direction != "inbound" {
		return false, false, nil // 平台自己发的 → 不回复
	}
	// 最后一条是客户消息 → 未回复，判断是否在 5 分钟窗口内
	cutoff := time.Now().Add(-replyWindow)
	if last.SentAt.Before(cutoff) {
		// 最后一条客户消息超过 5 分钟 → 历史消息不触发
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
