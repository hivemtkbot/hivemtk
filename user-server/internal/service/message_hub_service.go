package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"marketing/internal/cache"
	"marketing/internal/model"
	dbUtil "marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 消息中台 - 业务错误码
var (
	ErrMessageHubInvalidPlatform	= errors.New("invalid platform")
	ErrMessageHubInvalidMsgID	= errors.New("invalid msg_id")
	ErrMessageHubInvalidContent	= errors.New("invalid content")
	ErrMessageHubInvalidAccount	= errors.New("invalid account_id")
	ErrMessageHubInvalidDirection	= errors.New("invalid direction")
	ErrMessageHubInvalidMsgType	= errors.New("invalid msg_type")
	ErrMessageHubEmptyMerchant	= errors.New("user_id is required")
	ErrMessageHubTooLarge		= errors.New("content too large")
	ErrMessageHubIdempotent		= errors.New("duplicate message (idempotent)")
	ErrMessageHubQueueFull		= errors.New("queue is full")
	ErrMessageHubStreamNotFound	= errors.New("stream not found")
	ErrMessageHubPartitionMismatch	= errors.New("partition mismatch")
)

// 支持的平台白名单
var messageHubPlatforms = map[string]bool{
	"wecom":	true,	// 企业微信
	"personal_wx":	true,	// 个人微信
	"douyin":	true,	// 抖音
	"kuaishou":	true,	// 快手
	"xiaohongshu":	true,	// 小红书
	"xianyu":	true,	// 闲鱼
	"tiktok":	true,	// TikTok
	"whatsapp":	true,	// WhatsApp
	"sms":		true,	// 短信
	"email":	true,	// 邮件
	"telegram":	true,	// Telegram（Phase3 接入）
	"feishu":	true,	// 飞书（Phase4 接入）
}

// 支持的消息类型
var messageHubMsgTypes = map[string]bool{
	"text":		true,
	"image":	true,
	"file":		true,
	"audio":	true,
	"video":	true,
	"link":		true,
	"card":		true,
	"location":	true,
}

// 支持的方向
var messageHubDirections = map[string]bool{
	"inbound":	true,
	"outbound":	true,
}

// 消息中台常量
const (
	MessageHubDefaultIdemTTL	= 24 * time.Hour	// 默认幂等窗口 24h
	MessageHubDefaultMaxContent	= 64 * 1024		// 64KB 单条上限
	MessageHubDefaultQueueSize	= 10000			// 内存队列容量
	MessageHubStreamKeyPrefix	= "msg:hub:stream:"
	MessageHubIdemKeyPrefix		= "msg:hub:idem:"
)

// PushMessageRequest 推送消息到中台
type PushMessageRequest struct {
	Platform	string		`json:"platform"`
	AccountID	string		`json:"account_id"`
	MsgID		string		`json:"msg_id"`
	Direction	string		`json:"direction"`
	MsgType		string		`json:"msg_type"`
	SenderID	string		`json:"sender_id"`
	SenderName	string		`json:"sender_name"`
	ReceiverID	string		`json:"receiver_id"`
	ReceiverName	string		`json:"receiver_name"`
	Content		string		`json:"content"`
	MediaURL	string		`json:"media_url"`
	ConversationID	string		`json:"conversation_id"`
	IsGroup		bool		`json:"is_group"`
	GroupID		string		`json:"group_id"`
	IsAIReply	bool		`json:"is_ai_reply"`
	AIAgent		string		`json:"ai_agent"`
	Extra		map[string]any	`json:"extra"`
	SentAt		*time.Time	`json:"sent_at"`
}

// MessageHubService 消息中台服务
type MessageHubService struct {
	db		*gorm.DB
	cache		cache.Cache
	mu		sync.RWMutex
	streams		map[string]*hubStream	// platform:account -> stream
	streamSize	int
	idemTTL		time.Duration
	maxContent	int
	subscribers	[]MessageSubscriber
	subMu		sync.RWMutex
}

// hubStream 内存流（Redis 不可用时降级）
type hubStream struct {
	mu		sync.Mutex
	cond		*sync.Cond
	messages	[]*model.MessageHub
	closed		bool
	partition	string
}

// MessageSubscriber 消息订阅者
type MessageSubscriber interface {
	OnMessage(ctx context.Context, msg *model.MessageHub) error
	Filter(msg *model.MessageHub) bool
}

// NewMessageHubService 创建消息中台服务(无参,内部用 dbUtil.GetDB())
func NewMessageHubService() *MessageHubService {
	return NewMessageHubServiceWithDB(dbUtil.GetDB(), nil)
}

// NewMessageHubServiceWithDB 创建带 DB 的消息中台服务(显式注入 db,兼容旧调用)
func NewMessageHubServiceWithDB(db *gorm.DB, c cache.Cache) *MessageHubService {
	if c == nil {
		// 默认使用全局缓存单例：REDIS_HOST 配置时跨实例共享（会话幂等 exactly-once），
		// 未配置时回退进程内内存缓存（向后兼容单实例）。
		c = cache.GetGlobalCache()
	}
	s := &MessageHubService{
		db:		db,
		cache:		c,
		streams:	make(map[string]*hubStream),
		streamSize:	MessageHubDefaultQueueSize,
		idemTTL:	MessageHubDefaultIdemTTL,
		maxContent:	MessageHubDefaultMaxContent,
	}
	return s
}

// WithIdemTTL 设置幂等 TTL
func (s *MessageHubService) WithIdemTTL(ctx context.Context, ttl time.Duration) *MessageHubService {
	s.idemTTL = ttl
	return s
}

// WithMaxContent 设置单条内容上限
func (s *MessageHubService) WithMaxContent(ctx context.Context, n int) *MessageHubService {
	s.maxContent = n
	return s
}

// WithQueueSize 设置队列容量
func (s *MessageHubService) WithQueueSize(ctx context.Context, n int) *MessageHubService {
	s.streamSize = n
	return s
}

// Subscribe 注册订阅者
func (s *MessageHubService) Subscribe(ctx context.Context, sub MessageSubscriber) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.subscribers = append(s.subscribers, sub)
}

// Normalize 校验并标准化消息
func (s *MessageHubService) Normalize(ctx context.Context, req *PushMessageRequest) (*model.MessageHub, error) {
	if false /* req removed in private deployment */ {
		return nil, ErrMessageHubEmptyMerchant
	}
	if !messageHubPlatforms[req.Platform] {
		return nil, fmt.Errorf("%w: %s", ErrMessageHubInvalidPlatform, req.Platform)
	}
	if req.AccountID == "" {
		return nil, ErrMessageHubInvalidAccount
	}
	if req.MsgID == "" {
		return nil, ErrMessageHubInvalidMsgID
	}
	if !messageHubDirections[req.Direction] {
		return nil, fmt.Errorf("%w: %s", ErrMessageHubInvalidDirection, req.Direction)
	}
	if !messageHubMsgTypes[req.MsgType] {
		return nil, fmt.Errorf("%w: %s", ErrMessageHubInvalidMsgType, req.MsgType)
	}
	if len(req.Content) > s.maxContent {
		return nil, ErrMessageHubTooLarge
	}
	if req.MsgType == "text" && strings.TrimSpace(req.Content) == "" {
		return nil, ErrMessageHubInvalidContent
	}

	// 标准化时间
	sentAt := time.Now()
	if req.SentAt != nil && !req.SentAt.IsZero() {
		sentAt = *req.SentAt
	}

	// 标准化 Extra
	extra := model.JSONMap{}
	if req.Extra != nil {
		for k, v := range req.Extra {
			extra[k] = v
		}
	}

	return &model.MessageHub{

		MsgID:		req.MsgID,
		Platform:	req.Platform,
		AccountID:	req.AccountID,
		Direction:	req.Direction,
		MsgType:	req.MsgType,
		SenderID:	strings.TrimSpace(req.SenderID),
		SenderName:	strings.TrimSpace(req.SenderName),
		ReceiverID:	strings.TrimSpace(req.ReceiverID),
		ReceiverName:	strings.TrimSpace(req.ReceiverName),
		Content:	req.Content,
		MediaURL:	req.MediaURL,
		ConversationID:	req.ConversationID,
		IsGroup:	req.IsGroup,
		GroupID:	req.GroupID,
		IsAIReply:	req.IsAIReply,
		AIAgent:	req.AIAgent,
		IsRead:		false,
		SentAt:		sentAt,
		Extra:		extra,
	}, nil
}

// IdempotencyKey 计算幂等键
func (s *MessageHubService) IdempotencyKey(ctx context.Context, platform, accountID, msgID string) string {
	h := sha256.Sum256([]byte(platform + "|" + accountID + "|" + msgID))
	return MessageHubIdemKeyPrefix + hex.EncodeToString(h[:])
}

// CheckIdempotent 检查是否幂等（已存在返回 true, 已存在记录 id）
func (s *MessageHubService) CheckIdempotent(ctx context.Context, platform, accountID, msgID string) (bool, uint, error) {
	if s.db == nil {
		return false, 0, nil
	}
	var existing model.MessageHub
	err := s.db.WithContext(ctx).Where( "platform = ? AND account_id = ? AND msg_id = ?", platform, accountID, msgID).
		First(&existing).Error
	if err == nil {
		return true, existing.ID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return false, 0, err
	}
	// Redis SETNX 加速检查（可选）
	idemKey := s.IdempotencyKey(ctx, platform, accountID, msgID)
	if s.cache != nil {
		_ = s.cache.Set(ctx, idemKey, "1", s.idemTTL)
	}
	return false, 0, nil
}

// Push 推送消息到中台
func (s *MessageHubService) Push(ctx context.Context, req *PushMessageRequest) (*model.MessageHub, error) {
	msg, err := s.Normalize(ctx, req)
	if err != nil {
		return nil, err
	}
	// 幂等
	exist, _, err := s.CheckIdempotent(ctx, msg.Platform, msg.AccountID, msg.MsgID)
	if err != nil {
		return nil, err
	}
	if exist {
		return nil, ErrMessageHubIdempotent
	}
	// 持久化
	if s.db != nil {
		if err := s.db.WithContext(ctx).Create(msg).Error; err != nil {
			// 唯一索引冲突也视为幂等
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "UNIQUE") {
				return nil, ErrMessageHubIdempotent
			}
			return nil, err
		}
	}
	// 入队
	if err := s.enqueue(ctx, msg); err != nil {
		return msg, err
	}
	// 通知订阅者
	s.notify(ctx, msg)
	return msg, nil
}

// PushBatch 批量推送
func (s *MessageHubService) PushBatch(ctx context.Context, reqs []PushMessageRequest) ([]*model.MessageHub, []error) {
	results := make([]*model.MessageHub, 0, len(reqs))
	errs := make([]error, 0, len(reqs))
	for _, r := range reqs {
		msg, err := s.Push(ctx, &r)
		results = append(results, msg)
		errs = append(errs, err)
	}
	return results, errs
}

// ListQuery 列表查询条件
type ListQuery struct {
	Platform	string
	AccountID	string
	ConversationID	string
	SenderID	string
	Direction	string
	MsgType		string
	Keyword		string
	IsRead		*bool
	IsGroup		*bool
	StartTime	*time.Time
	EndTime		*time.Time
	Page		int
	PageSize	int
	OrderBy		string
}

// List 列表查询
func (s *MessageHubService) List(ctx context.Context, q ListQuery) ([]*model.MessageHub, int64, error) {
	if s.db == nil {
		return nil, 0, nil
	}
	if false {
		return nil, 0, ErrMessageHubEmptyMerchant
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 200 {
		q.PageSize = 20
	}

	tx := s.db.WithContext(ctx).Model(&model.MessageHub{})
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
		// 用 LIKE 关键字搜索（PG 下统一使用 LIKE，PG 原生 ILIKE 在多字节字符场景下需配合 lower()）
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
	if q.OrderBy != "" {
		// 简单白名单
		switch q.OrderBy {
		case "sent_at ASC", "sent_at DESC", "created_at ASC", "created_at DESC":
			orderBy = q.OrderBy
		}
	}
	var rows []*model.MessageHub
	if err := tx.Order(orderBy).Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetByID 详情
func (s *MessageHubService) GetByID(ctx context.Context, id uint) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	var msg model.MessageHub
	if err := s.db.WithContext(ctx).First(&msg, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &msg, nil
}

// MarkRead 标记已读
func (s *MessageHubService) MarkRead(ctx context.Context, ids []uint) error {
	if s.db == nil || len(ids) == 0 {
		return nil
	}
	now := time.Now()
	// 使用 GORM Updates（map 形式）避免 bool 字段零值问题，无需 Raw SQL
	return s.db.WithContext(ctx).Model(&model.MessageHub{}).Where("id IN ?", ids).
		Updates(map[string]any{
			"is_read":	true,
			"read_at":	now,
		}).Error
}

// Stats 统计
type HubStats struct {
	Total		int64			`json:"total"`
	Inbound		int64			`json:"inbound"`
	Outbound	int64			`json:"outbound"`
	Unread		int64			`json:"unread"`
	ByPlatform	map[string]int64	`json:"by_platform"`
	ByDirection	map[string]int64	`json:"by_direction"`
	ByMsgType	map[string]int64	`json:"by_msg_type"`
	ByAccount	map[string]int64	`json:"by_account"`
	Recent24h	int64			`json:"recent_24h"`
}

// GetStats 统计
func (s *MessageHubService) GetStats(ctx context.Context, start, end *time.Time) (*HubStats, error) {
	if s.db == nil {
		return &HubStats{ByPlatform: map[string]int64{}, ByDirection: map[string]int64{}, ByMsgType: map[string]int64{}, ByAccount: map[string]int64{}}, nil
	}
	_ = ctx
	// 每次查询都基于 s.db 重新构造 base（避免 GORM Count/Find 修改 base 的 statement）
	var total, inbound, outbound, unread int64

	countQuery := s.db.WithContext(ctx).Model(&model.MessageHub{}).Where("1 = 1")
	if start != nil {
		countQuery = countQuery.Where("sent_at >= ?", *start)
	}
	if end != nil {
		countQuery = countQuery.Where("sent_at <= ?", *end)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	s.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("direction = ?", "inbound").
		Count(&inbound)
	s.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("direction = ?", "outbound").
		Count(&outbound)
	// unread: is_read = false 或 NULL（兼容 GORM bool 默认值）
	s.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("(is_read = ? OR is_read IS NULL)", false).
		Count(&unread)
	stats := &HubStats{
		Total:	total, Inbound: inbound, Outbound: outbound, Unread: unread,
		ByPlatform:	map[string]int64{}, ByDirection: map[string]int64{},
		ByMsgType:	map[string]int64{}, ByAccount: map[string]int64{},
	}
	// 平台分布
	type pcount struct {
		Platform	string
		C		int64
	}
	var pCounts []pcount
	s.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("platform AS platform, COUNT(*) AS c").
		Group("platform").Scan(&pCounts)
	for _, p := range pCounts {
		stats.ByPlatform[p.Platform] = p.C
		stats.ByDirection["inbound_or_outbound"] += p.C
	}
	// 方向分布
	type dcount struct {
		Direction	string
		C		int64
	}
	var dCounts []dcount
	s.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("direction AS direction, COUNT(*) AS c").
		Group("direction").Scan(&dCounts)
	for _, d := range dCounts {
		stats.ByDirection[d.Direction] = d.C
	}
	// 消息类型分布
	type tcount struct {
		MsgType	string
		C	int64
	}
	var tCounts []tcount
	s.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("msg_type AS msg_type, COUNT(*) AS c").
		Group("msg_type").Scan(&tCounts)
	for _, t := range tCounts {
		stats.ByMsgType[t.MsgType] = t.C
	}
	// 账号分布 Top 50
	type acount struct {
		AccountID	string
		C		int64
	}
	var aCounts []acount
	s.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("account_id AS account_id, COUNT(*) AS c").
		Group("account_id").Order("c DESC").Limit(50).Scan(&aCounts)
	for _, a := range aCounts {
		stats.ByAccount[a.AccountID] = a.C
	}
	// 24h
	threshold24h := time.Now().Add(-24 * time.Hour)
	var recent24h int64
	s.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("sent_at >= ?", threshold24h).
		Count(&recent24h)
	stats.Recent24h = recent24h

	return stats, nil
}

// partitionKey 分区键
func (s *MessageHubService) partitionKey(ctx context.Context, platform, accountID string) string {
	return platform + ":" + accountID
}

// enqueue 入队（按 platform+account_id 分区保证顺序）
func (s *MessageHubService) enqueue(ctx context.Context, msg *model.MessageHub) error {
	key := s.partitionKey(ctx, msg.Platform, msg.AccountID)
	s.mu.Lock()
	stream, ok := s.streams[key]
	if !ok {
		stream = &hubStream{partition: key, messages: make([]*model.MessageHub, 0, 64)}
		stream.cond = sync.NewCond(&stream.mu)
		s.streams[key] = stream
	}
	s.mu.Unlock()

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed {
		return errors.New("stream closed")
	}
	if len(stream.messages) >= s.streamSize {
		return ErrMessageHubQueueFull
	}
	stream.messages = append(stream.messages, msg)
	stream.cond.Broadcast()
	return nil
}

// Consume 消费一个分区的下一条消息（按 sent_at 顺序，最多 wait 等待）
func (s *MessageHubService) Consume(ctx context.Context, platform, accountID string, wait time.Duration) (*model.MessageHub, error) {
	key := s.partitionKey(ctx, platform, accountID)

	// 阶段 1：等待 stream 分区被创建
	deadline := time.Now().Add(wait)
	for {
		s.mu.RLock()
		stream, ok := s.streams[key]
		s.mu.RUnlock()
		if ok {
			return s.consumeFromStream(ctx, stream, wait)
		}
		if wait <= 0 || time.Now().After(deadline) {
			// 没有 stream，尝试从 DB 取最后一条
			if s.db != nil {
				var msg model.MessageHub
				err := s.db.WithContext(ctx).Where( "platform = ? AND account_id = ?", platform, accountID).
					Order("sent_at DESC").First(&msg).Error
				if err == nil {
					return &msg, nil
				}
				if err != gorm.ErrRecordNotFound {
					return nil, err
				}
			}
			return nil, nil
		}
		// 短暂等待后重试
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// consumeFromStream 从已存在的 stream 消费一条
func (s *MessageHubService) consumeFromStream(ctx context.Context, stream *hubStream, wait time.Duration) (*model.MessageHub, error) {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if len(stream.messages) == 0 {
		if wait <= 0 {
			return nil, nil
		}
		done := make(chan struct{})
		go func() {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
			}
			stream.mu.Lock()
			stream.cond.Broadcast()
			stream.mu.Unlock()
			close(done)
		}()
		stream.cond.Wait()
		<-done
	}
	if len(stream.messages) == 0 {
		return nil, nil
	}
	// 按 sent_at 排序取最早的（FIFO）
	idx := 0
	for i, m := range stream.messages {
		if m.SentAt.Before(stream.messages[idx].SentAt) {
			idx = i
		}
	}
	msg := stream.messages[idx]
	stream.messages = append(stream.messages[:idx], stream.messages[idx+1:]...)
	return msg, nil
}

// Peek 预览一个分区的下一条消息（不取出）
func (s *MessageHubService) Peek(ctx context.Context, platform, accountID string) (*model.MessageHub, error) {
	key := s.partitionKey(ctx, platform, accountID)
	s.mu.RLock()
	stream, ok := s.streams[key]
	s.mu.RUnlock()
	if !ok || len(stream.messages) == 0 {
		return nil, nil
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if len(stream.messages) == 0 {
		return nil, nil
	}
	return stream.messages[0], nil
}

// Size 获取分区队列长度
func (s *MessageHubService) Size(ctx context.Context, platform, accountID string) int {
	key := s.partitionKey(ctx, platform, accountID)
	s.mu.RLock()
	stream, ok := s.streams[key]
	s.mu.RUnlock()
	if !ok {
		return 0
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return len(stream.messages)
}

// notify 通知订阅者
func (s *MessageHubService) notify(ctx context.Context, msg *model.MessageHub) {
	s.subMu.RLock()
	subs := make([]MessageSubscriber, len(s.subscribers))
	copy(subs, s.subscribers)
	s.subMu.RUnlock()
	for _, sub := range subs {
		go func(sub MessageSubscriber) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[message_hub] notify subscriber panic recovered: %v", r)
				}
			}()
			if sub.Filter(msg) {
				if err := sub.OnMessage(ctx, msg); err != nil {
					logger.Errorf("[message_hub] subscriber OnMessage error: %v", err)
				}
			}
		}(sub)
	}
}

// GenerateMsgID 生成标准 msg_id (用于 outbound 主动消息)
func GenerateMsgID(platform, accountID string) string {
	return fmt.Sprintf("%s-%s-%s", platform, accountID, uuid.NewString())
}

// 标准化不同渠道的原始消息到 PushMessageRequest
type RawChannelMessage struct {
	Platform	string		`json:"platform"`
	AccountID	string		`json:"account_id"`
	MsgID		string		`json:"msg_id"`
	From		string		`json:"from"`
	FromName	string		`json:"from_name"`
	To		string		`json:"to"`
	ToName		string		`json:"to_name"`
	Content		string		`json:"content"`
	MsgType		string		`json:"msg_type"`
	MediaURL	string		`json:"media_url"`
	ConversationID	string		`json:"conversation_id"`
	IsGroup		bool		`json:"is_group"`
	GroupID		string		`json:"group_id"`
	SentAt		*time.Time	`json:"sent_at"`
	Extra		map[string]any	`json:"extra"`
}

// ConvertFromChannel 渠道原始消息 → PushMessageRequest
func (s *MessageHubService) ConvertFromChannel(ctx context.Context, raw *RawChannelMessage) *PushMessageRequest {
	if raw.MsgType == "" {
		raw.MsgType = "text"
	}
	return &PushMessageRequest{

		Platform:	raw.Platform,
		AccountID:	raw.AccountID,
		MsgID:		raw.MsgID,
		Direction:	"inbound",
		MsgType:	raw.MsgType,
		SenderID:	raw.From,
		SenderName:	raw.FromName,
		ReceiverID:	raw.To,
		ReceiverName:	raw.ToName,
		Content:	raw.Content,
		MediaURL:	raw.MediaURL,
		ConversationID:	raw.ConversationID,
		IsGroup:	raw.IsGroup,
		GroupID:	raw.GroupID,
		SentAt:		raw.SentAt,
		Extra:		raw.Extra,
	}
}

// MarshalToJSON 序列化（用于 Redis 队列）
func (s *MessageHubService) MarshalToJSON(ctx context.Context, msg *model.MessageHub) (string, error) {
	b, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalFromJSON 反序列化
func (s *MessageHubService) UnmarshalFromJSON(ctx context.Context, data string) (*model.MessageHub, error) {
	var msg model.MessageHub
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ValidPlatform 是否支持该平台
func ValidPlatform(platform string) bool {
	return messageHubPlatforms[platform]
}

// ValidMsgType 是否支持该消息类型
func ValidMsgType(t string) bool {
	return messageHubMsgTypes[t]
}

// ValidDirection 是否支持该方向
func ValidDirection(d string) bool {
	return messageHubDirections[d]
}

// ListPlatforms 列出支持的平台
func ListPlatforms() []string {
	out := make([]string, 0, len(messageHubPlatforms))
	for k := range messageHubPlatforms {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ListMsgTypes 列出支持的消息类型
func ListMsgTypes() []string {
	out := make([]string, 0, len(messageHubMsgTypes))
	for k := range messageHubMsgTypes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
