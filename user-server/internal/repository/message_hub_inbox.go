package repository

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

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

// CreateWithInboxTx 原子地写入 message_hub + 调用 inboxRepo.UpsertFromMessageTx（同事务）。
//
// 修复（2026-08-05 审计 P1）：原 persistMessage 分两步调用 hubRepo.Create + inboxSvc.UpsertFromHubMessage，
// 两步跨表无事务，inbox_conversations 写入失败时仅 Warn 日志，导致"消息在 message_hub 但收件箱看不到"的极端不一致。
// 修复后两步在同一 tx 内完成，任一失败整体回滚。
//
// 入参：
//   - hub：message_hub 记录
//   - inboxRepo：收件箱仓库（与 hubRepo 共享同一 *gorm.DB 实例）
//   - input：inbox_conversations upsert 入参（由 service 层根据 msg 字段构造）
//
// 返回：
//   - message_hub 唯一键冲突时返回 nil（与 Create 行为一致，幂等视为成功）
//   - 其他错误正常返回（tx 已回滚）
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

// AckOutboundDeliveredBatch 批量将 (channel, accountID) 下、匹配给定 msg_id 且仍为 pending 的出站消息
// 标记为 delivered（通道B·状态上报）。
//
// 关键修正：msg_id 由 (channel+content) 生成，同一内容可出现在多个会话（复合唯一索引 (msg_id, conversation_id)
// 允许同 msg_id 跨会话存储）。旧实现用 GetByMsgID 仅取首行再 Update，导致跨会话的重复出站消息永远停留在
// pending → 污染 stuck_unreachable 监控。此处按 (channel, account_id, msg_id) 一次性更新所有 pending 行，
// 归属校验由 WHERE 的 platform/account_id 保证；仅翻转为 pending（failed 为终态，不改动）。
func (r *MessageHubRepository) AckOutboundDeliveredBatch(ctx context.Context, channel, accountID string, msgIDs []string) (int64, error) {
	if r.db == nil || len(msgIDs) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		Where("platform = ? AND account_id = ? AND direction = 'outbound' AND msg_id IN ? AND status = 'pending'", channel, accountID, msgIDs).
		Update("status", "delivered")
	return res.RowsAffected, res.Error
}

// GetByContentHash 按 canonical contentHash 获取消息（服务端权威去重 2026-08-07 修复）
//
// 背景：前端 patrol 上报的消息 msg_id 在历史曾用 algo1（channel+conv+content），
//   而服务端 ContentHashMsgID 已用 algo2（channel+content），导致同内容生成不同 msg_id
//   → 钩子2 GetByMsgID 漏检 → 同内容 AI 回复被反复入库为 inbound → 触发循环 AI。
//
// 参数 canonicalHash 应为服务端 ContentHashMsgID(channel, content) 的输出（algo2），
//   而非调用方传入的 event_id（可能是 algo1）。本方法直接按 hash 查全表。
func (r *MessageHubRepository) GetByContentHash(ctx context.Context, canonicalHash string) (*model.MessageHub, error) {
	if canonicalHash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var hub model.MessageHub
	err := r.db.Where("msg_id = ?", canonicalHash).First(&hub).Error
	return &hub, err
}

// GetByPlatformContent 按 platform + content 精确查重（md5 完全匹配），仅查 direction='outbound'。
// 用于服务端权威去重：判定当前上报消息是否为"平台已下发的 AI 回复回显"。
//   限 outbound 语义：回显 = 平台 outbound 的 echo，不得把其他会话客户发的相同内容 inbound
//   误判为自/他回显跳过（2026-08-07 第八轮修复：跨会话客户同 content 误跳过 bug）。
//   索引：idx_message_hub_platform_content (platform, md5(content))
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

// GetByPlatformContentNormalized 按 platform + 归一化内容（去所有空白后）查重。
// 兜底 DOM 中 AI 回复与 DB 落库内容存在空格/换行/emoji 编码差异时的回环：
//   例 DB "安全。 需要" vs DOM "安全。需要" → 去空格后 md5 一致 → 命中跳过。
// 仅查 direction=outbound，避免把客户发的相同内容也误跳过。
//   索引：idx_message_hub_platform_content (platform, md5(content)) 加速 platform 过滤，
//   归一化 md5 比较在 platform 范围内逐行计算（量小，可接受）。
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

// Delete 按 id 删除单条消息中台记录（统一收件箱消息删除）
func (r *MessageHubRepository) Delete(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.MessageHub{}).Error
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

// GetLastByConversation 取会话最新一条消息（按 sent_at DESC）。
//
// 2026-08-05 新增（用户科学方案）：
//   - 用于 persistMessage 时序锚点判断：新消息 timestamp 与锚点比较，早于锚点 → backfill
//   - 用于 HandleIngressBatch 回复判断：会话最后一条消息方向决定是否触发 AI
//   - outbound（平台自己发的）→ 不回复
//   - inbound（客户发的）+ 5min 内 → 回复
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

// HasUnrepliedCustomerMessage 判断会话最后一条消息方向，决定是否需要回复。
//
// 2026-08-05 重构（用户科学方案）：
//   - 查询会话最后一条消息方向（不排除任何消息）
//   - 最后一条 outbound（平台自己发的）→ 已回复，不触发 AI
//   - 最后一条 inbound（客户发的）+ 5min 内 → 未回复，触发 AI
//   - 最后一条 inbound + 5min 外 → 历史消息，不触发
//   - 无消息 → 视为未回复（首条消息触发）
//
// 用户诉求："是否回消息依据是最后一条是不是平台自己发的 是则不发送"
//
// 入参：
//   - conversationID：会话唯一标识
//   - replyWindow：回复判断窗口（5 分钟），超过则视为历史消息不再触发
//
// 返回：
//   - unreplied=true + withinWindow=true → 最后一条是客户消息且在 5min 内 → 触发 AI
//   - unreplied=true + withinWindow=false → 最后一条是客户消息但超过 5min → 历史消息不触发
//   - unreplied=false → 最后一条是平台自己发的（outbound）→ 不触发
//   - 查询失败 → 保守返回 false（避免对历史存量消息逐一自动回复）
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

// GetLastOutboundByConversation 查询会话最后一条平台发出消息（direction=outbound）。
//
// 2026-08-05 新增（防回环）：
//   - 前端 isSelfMessage 判定可能失效，导致 AI 发送的 outbound 消息被误判为 customer（inbound）
//   - 后端在入库前查最后一条 outbound，若内容与当前 inbound 相同 → 回环消息，跳过 AI 触发
//   - 仅查 5 分钟内的 outbound（超过 5 分钟的相同内容大概率是客户新消息而非回环）
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

// GetRecentOutboundsByConversation / ExistsInboundByContent 已于 2026-08-07 删除。
// 回显检测现由 contentHash (FNV-1a) + GetByMsgID 统一处理 ——
// 前端 patrol 抓取的 AI 回复气泡 msg_id 与服务端 ContentHashMsgID 逐字节一致，
// GetByMsgID 命中即跳过，无需 sender_type 或内容精确匹配辅助函数。

// GetLastInboundByConversation 查询会话最后一条客户消息（direction=inbound）。
//
// 用于 AI 回复完成后补触发：释放 ai_processing 标记后，若仍有未回复的客户消息
// （AI 推理期间用户发的新消息），需要获取该消息完整内容来构造触发事件。
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

// FindNullConversationIDRows 返回 conversation_id 为 NULL 或空串的 message_hub 行。
//
// 这类脏数据产生自统一收件箱特性上线前的历史消息（ingest 兜底 conversation_id 的逻辑
// 于 2026-08-06 才加入），会导致 sync_gap 误报一个“空会话”分组。回填时按 ingest 兜底规则
// 派生 platform:account_id 修正。
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

// UpdateConversationID 修正单行 message_hub 的 conversation_id（用于回填 NULL/空脏数据）。
func (r *MessageHubRepository) UpdateConversationID(ctx context.Context, id uint, conversationID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		Where("id = ?", id).
		Update("conversation_id", conversationID).Error
}

// FindConversationIDsMissingInbox 返回 message_hub 中存在、但 inbox_conversations 中缺失的
// conversation_id（按 conversation_id 左连接）。用于回填历史会话：统一收件箱特性上线前已积累
// 消息的会话，只有新消息才触发同步，导致历史缺口。注意：群聊/聚合会话的 customer_id 已统一为
// conversation_id（见 service.inboxCustomerID），这类碎片合并由 reconcileBackfill 步骤 3 处理，
// 此处仅覆盖 1:1 等按 conversation_id 缺失的会话。
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

// FindLatestByConversation 取会话中 id 最大（最新）的一条消息，作为回填收件箱的代表消息。
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

// SyncGapConv 一个 sync_gap 会话的去重键与规范客户键（与 monitor sync_gap 判定一致）。
type SyncGapConv struct {
	Platform       string
	AccountID      string
	ConversationID string
	CustomerID     string
}

// FindSyncGapConversations 返回 message_hub 中存在、但 inbox_conversations 中缺失
// 规范客户键收件箱行的会话（与 monitor 的 sync_gap 检测口径完全一致）。用于 backfill
// 步骤 5 全量修复：把被时间戳污染的碎片收敛为按规范 customer_id 归属的单行并清理孤儿。
//
// 规范客户键判定（与 service.inboxCustomerID、monitor.sync_gap 三元组完全一致）：
//  1. 群聊（is_group）且 conversation_id 非空 → conversation_id
//  2. sender_id（入站）或 receiver_id（出站）形如 "conversation_id <时间后缀>" → conversation_id
//  3. 出站取 receiver_id，入站取 sender_id
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

// NormalizePollutedConversationIDs 把被时间戳/状态串污染的 conversation_id 还原为规范键。
//
// 历史 bug：早期曾用 split_part(conversation_id,' ',1) 把 "conv:AI 修炼场 5 37分钟前"
// 错切成 "conv:AI"，导致多个不同群被折叠进同一短键、收件箱/追踪严重碎片化。
// 由于 sender_id/receiver_id 仍保留完整被污染标题（如 "conv:AI 修炼场 5 37分钟前"），
// 这里以其为事实源还原 conversation_id：取与 conversation_id 前缀匹配的 sender/receiver，
// 再去掉【末尾】时间戳/状态 token（标题本身可能含空格，绝不能按首个空格切分）。
// 该逻辑对已是规范键的行幂等（CASE 回退为原值），可重复安全执行。
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

// NormalizePollutedTraceConversationIDs 清洗 message_trace 表被时间戳污染的 conversation_id
// 末尾时间戳 token（与 message_hub 归一口径一致），避免链路追踪树按旧键分裂。
//
// 注意：message_trace 无 sender_id/receiver_id，且历史上 split_part 已把 conversation_id 错切为
// 首 token 短键（如 "conv:旭"），这类短键在 message_hub 中无唯一可映射的全标题，属不可逆历史数据，
// 不能臆造还原——仅做末尾时间戳清洗（对当前无空格的短键为幂等 no-op）。前向正确性由追踪从实时消息
// 取值保证：message_hub 修复后，新产生的 trace 自然使用规范 conversation_id。
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

// DeletePollutedInboxRows 删除被时间戳污染的孤儿收件箱行（customer_id 或 conversation_id
// 带空格后缀）。归一 conversation_id 后这些行不再被任何 message_hub 引用，必须清理以免
// 在收件箱 UI 造成重复/陈旧会话。
func (r *InboxConversationRepository) DeletePollutedInboxRows(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("conversation_id LIKE ? OR customer_id LIKE ?", "conv:% %", "conv:% %").
		Delete(&model.InboxConversation{})
	return res.RowsAffected, res.Error
}

// DeleteOrphanConvInboxRows 删除收件箱中被早期错切（split_part 首 token）产生的 conv: 短键
// 孤儿行——这些 conversation_id 在 message_hub 归一后已不存在（如 "conv:AI" 已还原为
// "conv:AI 修炼场 5" 等）。与 DeletePollutedInboxRows 不同，这里按“conversation_id 是否仍被
// message_hub 引用”判定孤儿，能正确清理无空格的短键折叠残留。限定 conv: 前缀，避免误删其它渠道
// 合法会话（非群聊 conv: 前缀的会话不应被此逻辑触碰）。
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

	orderBy := "id DESC" // 默认：会话 id 倒序（新建会话在前）
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
