package service

import (
	"context"

	"fmt"

	"sync"

	"time"

	"hivemtk-user/internal/dto"
)

type InboxChannel string

const (
	InboxChannelWeChat InboxChannel = "wechat"

	InboxChannelDouyin InboxChannel = "douyin"

	InboxChannelXiaohongshu InboxChannel = "xiaohongshu"

	InboxChannelEmail InboxChannel = "email"

	InboxChannelWeb InboxChannel = "web"
)

type InboxMessage struct {
	MessageID    string       `json:"message_id"`
	Channel      InboxChannel `json:"channel"`
	ChannelLabel string       `json:"channel_label"`
	UnifiedID    string       `json:"unified_id"`    // OneID
	CustomerID   string       `json:"customer_id"`   // 客户内部ID
	CustomerName string       `json:"customer_name"` // 客户展示名
	SenderID     string       `json:"sender_id"`
	SenderName   string       `json:"sender_name"`
	Content      string       `json:"content"`
	ContentType  string       `json:"content_type"`
	ReceivedAt   time.Time    `json:"received_at"`
	IsRead       bool         `json:"is_read"`
	IsReplied    bool         `json:"is_replied"`
	IsInbound    bool         `json:"is_inbound"` // true=客户→我们 false=我们→客户
	// 旅程相关（自动启动后填充）
	JourneyStage string `json:"journey_stage,omitempty"`
	JourneyLabel string `json:"journey_label,omitempty"`
	// AI 谈单相关
	AIHandled  bool   `json:"ai_handled"`
	TransferTo string `json:"transfer_to,omitempty"`
}

type InboxThread struct {
	UnifiedID       string          `json:"unified_id"`
	CustomerID      string          `json:"customer_id"`
	CustomerName    string          `json:"customer_name"`
	Phone           string          `json:"phone"`
	Email           string          `json:"email"`
	Channels        []InboxChannel  `json:"channels"` // 该客户使用的渠道
	ChannelLabels   []string        `json:"channel_labels"`
	TotalMessages   int             `json:"total_messages"`
	UnreadCount     int             `json:"unread_count"`
	LastMessageAt   time.Time       `json:"last_message_at"`
	LastMessage     string          `json:"last_message"`
	LastChannel     InboxChannel    `json:"last_channel"`
	JourneyStage    string          `json:"journey_stage"`
	JourneyLabel    string          `json:"journey_label"`
	OwnerID         string          `json:"owner_id,omitempty"`
	Tags            []string        `json:"tags"`
	HasOpenFollowup bool            `json:"has_open_followup"`
	FollowupDueAt   *time.Time      `json:"followup_due_at,omitempty"`
	RecentMessages  []*InboxMessage `json:"recent_messages,omitempty"` // 最近 N 条
}

type InboxSummary struct {
	Threads       []*InboxThread `json:"threads"`
	TotalThreads  int            `json:"total_threads"`
	UnreadThreads int            `json:"unread_threads"`
	TotalUnread   int            `json:"total_unread"`
	ChannelStats  map[string]int `json:"channel_stats"` // 各渠道未读数
	GeneratedAt   time.Time      `json:"generated_at"`
}

type InboxFilter struct {
	Channel       InboxChannel // 按渠道过滤
	OnlyUnread    bool         // 仅未读
	OnlyAIHandled bool         // 仅 AI 处理
	OnlyFollowup  bool         // 仅有待办跟进
	JourneyStage  JourneyStage // 按旅程阶段
	OwnerID       string       // 按销售
	Keyword       string       // 关键词
	Limit         int
	Offset        int
}

type UnifiedInboxService struct {
	mu sync.RWMutex

	// 消息存储（按 OneID 分组）
	messages map[string][]*InboxMessage // unifiedID → messages

	// 客户身份映射（OneID → 客户身份）
	customerByUnifiedID map[string]*OneIDCustomerLite

	// 身份索引（phone/email/openID → unifiedID）
	phoneIndex  map[string]string
	emailIndex  map[string]string
	wechatIndex map[string]string
	douyinIndex map[string]string
	xhsIndex    map[string]string

	// 已读状态
	readSet map[string]bool // messageID → isRead

	// 联动组件
	journey  *CustomerJourneyService
	followup *FollowUpService
	tagger   *AITagger
}

type OneIDCustomerLite struct {
	UnifiedID     string    `json:"unified_id"`
	CustomerID    string    `json:"customer_id"`
	CustomerName  string    `json:"customer_name"`
	Phone         string    `json:"phone"`
	Email         string    `json:"email"`
	WechatOpenID  string    `json:"wechat_open_id"`
	DouyinOpenID  string    `json:"douyin_open_id"`
	XiaohongshuID string    `json:"xiaohongshu_id"`
	OwnerID       string    `json:"owner_id"`
	Tags          []string  `json:"tags"`
	CreatedAt     time.Time `json:"created_at"`
}

func NewUnifiedInboxService(journey *CustomerJourneyService, followup *FollowUpService, tagger *AITagger) *UnifiedInboxService {
	return &UnifiedInboxService{
		messages:            make(map[string][]*InboxMessage),
		customerByUnifiedID: make(map[string]*OneIDCustomerLite),
		phoneIndex:          make(map[string]string),
		emailIndex:          make(map[string]string),
		wechatIndex:         make(map[string]string),
		douyinIndex:         make(map[string]string),
		xhsIndex:            make(map[string]string),
		readSet:             make(map[string]bool),
		journey:             journey,
		followup:            followup,
		tagger:              tagger,
	}
}

func (s *UnifiedInboxService) IngestMessage(ctx context.Context, msg *InboxMessage) (*InboxMessage, error) {
	if msg == nil {
		return nil, fmt.Errorf("消息为空")
	}
	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = time.Now()
	}
	if msg.MessageID == "" {
		msg.MessageID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	if msg.ContentType == "" {
		msg.ContentType = "text"
	}
	if msg.ChannelLabel == "" {
		msg.ChannelLabel = s.channelLabel(ctx, msg.Channel)
	}

	// === 1. OneID 解析：找/创建客户 ===
	cust, unifiedID, err := s.resolveOneID(ctx, msg)
	if err != nil {
		return nil, err
	}
	msg.UnifiedID = unifiedID
	msg.CustomerID = cust.CustomerID
	if msg.CustomerName == "" {
		msg.CustomerName = cust.CustomerName
	}

	// === 2. 自动绑定新渠道身份 ===
	if err := s.bindIdentity(ctx, cust, msg); err != nil {
		return nil, err
	}

	// === 3. 旅程自动启动 ===
	if s.journey != nil {
		// 记录互动（独立于阶段迁移，弥补 Transition 不增加 Touches 的问题）
		s.journey.Touch(ctx, cust.CustomerID, string(msg.Channel))
		_, _ = s.journey.Transition(ctx, cust.CustomerID, StageLead, "inbox", "system",
			"统一收件箱自动启动旅程: "+string(msg.Channel), nil)
		state := s.journey.GetState(ctx, cust.CustomerID)
		msg.JourneyStage = string(state.CurrentStage)
		if meta, ok := StageMetas[state.CurrentStage]; ok && meta != nil {
			msg.JourneyLabel = meta.Label
		}
	}

	// === 4. 记录到收件箱 ===
	s.mu.Lock()
	s.messages[unifiedID] = append(s.messages[unifiedID], msg)
	// 限制单客户最多保留 500 条
	if len(s.messages[unifiedID]) > 500 {
		s.messages[unifiedID] = s.messages[unifiedID][len(s.messages[unifiedID])-500:]
	}
	s.mu.Unlock()

	// === 5. 联动 AI 谈单 / 跟进 ===
	if err := s.triggerDownstream(ctx, cust, msg); err != nil {
		return msg, err
	}

	return msg, nil
}

func (s *UnifiedInboxService) resolveOneID(ctx context.Context, msg *InboxMessage) (*OneIDCustomerLite, string, error) {
	// 1) 按 sender_id 找（sender_id 通常是平台 OpenID）
	if msg.SenderID != "" {
		uid, ok := s.findUnifiedByPlatformID(ctx, msg.Channel, msg.SenderID)
		if ok {
			s.mu.RLock()
			cust := s.customerByUnifiedID[uid]
			s.mu.RUnlock()
			if cust != nil {
				return cust, uid, nil
			}
		}
	}

	// 2) 尝试从消息内容提取手机号/邮箱（粗略正则）
	phone, email := extractContactFromText(msg.Content)
	if phone != "" {
		phone = NormalizePhone(phone)
		s.mu.RLock()
		uid, ok := s.phoneIndex[phone]
		s.mu.RUnlock()
		if ok {
			s.mu.RLock()
			cust := s.customerByUnifiedID[uid]
			s.mu.RUnlock()
			if cust != nil {
				return cust, uid, nil
			}
		}
	}
	if email != "" {
		email = NormalizeEmail(email)
		s.mu.RLock()
		uid, ok := s.emailIndex[email]
		s.mu.RUnlock()
		if ok {
			s.mu.RLock()
			cust := s.customerByUnifiedID[uid]
			s.mu.RUnlock()
			if cust != nil {
				return cust, uid, nil
			}
		}
	}

	// 3) 找不到 → 检查是否有现成客户只用 phone/email 标识（没 OpenID）
	// 关键：客户先在微信匿名聊天"你好"（无手机号），后在抖音说"联系我 13800138000"
	// 第一次来时未提取到 phone/email → phoneIndex 中无此 key
	// 第二次来时 phone 出现在内容里 → 应能合并到第一次的客户
	// 创建新客户前，反向查询 phoneIndex/emailIndex 是否有相同 key
	//       （即使之前没有该 key，也要查是否已存在相同 phone/email 的客户）
	if phone != "" {
		s.mu.RLock()
		uid, ok := s.phoneIndex[phone]
		s.mu.RUnlock()
		if ok {
			s.mu.RLock()
			cust := s.customerByUnifiedID[uid]
			s.mu.RUnlock()
			if cust != nil {
				return cust, uid, nil
			}
		}
	}
	if email != "" {
		s.mu.RLock()
		uid, ok := s.emailIndex[email]
		s.mu.RUnlock()
		if ok {
			s.mu.RLock()
			cust := s.customerByUnifiedID[uid]
			s.mu.RUnlock()
			if cust != nil {
				return cust, uid, nil
			}
		}
	}

	// 4) 实在找不到 → 创建新客户
	cust := &OneIDCustomerLite{
		UnifiedID:    fmt.Sprintf("oneid-%d", time.Now().UnixNano()),
		CustomerID:   fmt.Sprintf("cust-%d", time.Now().UnixNano()),
		CustomerName: msg.SenderName,
		Phone:        phone,
		Email:        email,
		CreatedAt:    time.Now(),
		Tags:         []string{},
	}
	// 按渠道填 OpenID
	switch msg.Channel {
	case InboxChannelWeChat:
		cust.WechatOpenID = msg.SenderID
	case InboxChannelDouyin:
		cust.DouyinOpenID = msg.SenderID
	case InboxChannelXiaohongshu:
		cust.XiaohongshuID = msg.SenderID
	}

	s.mu.Lock()
	s.customerByUnifiedID[cust.UnifiedID] = cust
	if cust.Phone != "" {
		s.phoneIndex[cust.Phone] = cust.UnifiedID
	}
	if cust.Email != "" {
		s.emailIndex[cust.Email] = cust.UnifiedID
	}
	if cust.WechatOpenID != "" {
		s.wechatIndex[cust.WechatOpenID] = cust.UnifiedID
	}
	if cust.DouyinOpenID != "" {
		s.douyinIndex[cust.DouyinOpenID] = cust.UnifiedID
	}
	if cust.XiaohongshuID != "" {
		s.xhsIndex[cust.XiaohongshuID] = cust.UnifiedID
	}
	s.mu.Unlock()

	return cust, cust.UnifiedID, nil
}

func (s *UnifiedInboxService) bindIdentity(ctx context.Context, cust *OneIDCustomerLite, msg *InboxMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch msg.Channel {
	case InboxChannelWeChat:
		if cust.WechatOpenID == "" && msg.SenderID != "" {
			cust.WechatOpenID = msg.SenderID
			s.wechatIndex[msg.SenderID] = cust.UnifiedID
		}
	case InboxChannelDouyin:
		if cust.DouyinOpenID == "" && msg.SenderID != "" {
			cust.DouyinOpenID = msg.SenderID
			s.douyinIndex[msg.SenderID] = cust.UnifiedID
		}
	case InboxChannelXiaohongshu:
		if cust.XiaohongshuID == "" && msg.SenderID != "" {
			cust.XiaohongshuID = msg.SenderID
			s.xhsIndex[msg.SenderID] = cust.UnifiedID
		}
	}
	// 从消息内容补充 phone/email
	if phone, email := extractContactFromText(msg.Content); phone != "" || email != "" {
		if cust.Phone == "" && phone != "" {
			phone = NormalizePhone(phone)
			cust.Phone = phone
			s.phoneIndex[phone] = cust.UnifiedID
		}
		if cust.Email == "" && email != "" {
			email = NormalizeEmail(email)
			cust.Email = email
			s.emailIndex[email] = cust.UnifiedID
		}
	}
	return nil
}

func (s *UnifiedInboxService) findUnifiedByPlatformID(ctx context.Context, channel InboxChannel, senderID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch channel {
	case InboxChannelWeChat:
		uid, ok := s.wechatIndex[senderID]
		return uid, ok
	case InboxChannelDouyin:
		uid, ok := s.douyinIndex[senderID]
		return uid, ok
	case InboxChannelXiaohongshu:
		uid, ok := s.xhsIndex[senderID]
		return uid, ok
	}
	return "", false
}

func (s *UnifiedInboxService) triggerDownstream(ctx context.Context, cust *OneIDCustomerLite, msg *InboxMessage) error {
	if !msg.IsInbound {
		return nil // 只对客户→我们的消息触发
	}
	// AI 自动打标签（基于消息内容）
	if s.tagger != nil {
		// 简单的意图识别：基于关键词
		intent := detectIntentFromText(msg.Content)
		s.tagger.TagFromSalesResponse(ctx, cust.CustomerID, &SalesResponse{
			Reply: "",
			Intent: &dto.RecognizeResult{
				IntentType: intent,
				Confidence: 0.7,
			},
		})
	}
	return nil
}

func (s *UnifiedInboxService) ListThreads(ctx context.Context, filter InboxFilter) *InboxSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threads := make([]*InboxThread, 0)
	channelStats := make(map[string]int)
	totalUnread := 0
	unreadThreads := 0

	for unifiedID, msgs := range s.messages {
		if len(msgs) == 0 {
			continue
		}
		cust, ok := s.customerByUnifiedID[unifiedID]
		if !ok {
			continue
		}

		// 渠道去重
		channelSet := make(map[InboxChannel]bool)
		channelLabels := make([]string, 0)
		for _, m := range msgs {
			if !channelSet[m.Channel] {
				channelSet[m.Channel] = true
				channelLabels = append(channelLabels, m.ChannelLabel)
				if !m.IsRead {
					channelStats[string(m.Channel)]++
				}
			}
		}
		channels := make([]InboxChannel, 0, len(channelSet))
		for c := range channelSet {
			channels = append(channels, c)
		}

		// 最后一条
		last := msgs[len(msgs)-1]
		unread := 0
		for _, m := range msgs {
			if !m.IsRead {
				unread++
			}
		}
		totalUnread += unread
		if unread > 0 {
			unreadThreads++
		}

		// 旅程
		stage := ""
		stageLabel := ""
		hasFollowup := false
		var followupDue *time.Time
		if s.journey != nil {
			state := s.journey.GetState(ctx, cust.CustomerID)
			stage = string(state.CurrentStage)
			if meta, ok := StageMetas[state.CurrentStage]; ok && meta != nil {
				stageLabel = meta.Label
			}
		}
		if s.followup != nil {
			pending := s.followup.ListPending(ctx, cust.OwnerID, 0)
			for _, p := range pending {
				if p.CustomerID == cust.CustomerID {
					hasFollowup = true
					t := p.DueAt
					followupDue = &t
					break
				}
			}
		}

		// 最近 N 条
		recentN := 5
		if len(msgs) < recentN {
			recentN = len(msgs)
		}
		recent := make([]*InboxMessage, 0, recentN)
		for i := len(msgs) - recentN; i < len(msgs); i++ {
			recent = append(recent, msgs[i])
		}

		thread := &InboxThread{
			UnifiedID:       unifiedID,
			CustomerID:      cust.CustomerID,
			CustomerName:    cust.CustomerName,
			Phone:           cust.Phone,
			Email:           cust.Email,
			Channels:        channels,
			ChannelLabels:   channelLabels,
			TotalMessages:   len(msgs),
			UnreadCount:     unread,
			LastMessageAt:   last.ReceivedAt,
			LastMessage:     inboxTruncate(last.Content, 80),
			LastChannel:     last.Channel,
			JourneyStage:    stage,
			JourneyLabel:    stageLabel,
			OwnerID:         cust.OwnerID,
			Tags:            cust.Tags,
			HasOpenFollowup: hasFollowup,
			FollowupDueAt:   followupDue,
			RecentMessages:  recent,
		}
		threads = append(threads, thread)
	}

	// 过滤
	filtered := make([]*InboxThread, 0, len(threads))
	for _, t := range threads {
		if filter.Channel != "" {
			hasChannel := false
			for _, c := range t.Channels {
				if c == filter.Channel {
					hasChannel = true
					break
				}
			}
			if !hasChannel {
				continue
			}
		}
		if filter.OnlyUnread && t.UnreadCount == 0 {
			continue
		}
		if filter.OnlyFollowup && !t.HasOpenFollowup {
			continue
		}
		if filter.JourneyStage != "" && t.JourneyStage != string(filter.JourneyStage) {
			continue
		}
		if filter.OwnerID != "" && t.OwnerID != filter.OwnerID {
			continue
		}
		if filter.Keyword != "" {
			if !containsFold(t.CustomerName, filter.Keyword) &&
				!containsFold(t.LastMessage, filter.Keyword) {
				continue
			}
		}
		filtered = append(filtered, t)
	}

	// 按最后消息时间倒序
	for i := 0; i < len(filtered); i++ {
		for j := i + 1; j < len(filtered); j++ {
			if filtered[j].LastMessageAt.After(filtered[i].LastMessageAt) {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}

	// 分页
	offset := filter.Offset
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	end := offset + limit
	if offset > len(filtered) {
		offset = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	paged := filtered[offset:end]

	return &InboxSummary{
		Threads:       paged,
		TotalThreads:  len(filtered),
		UnreadThreads: unreadThreads,
		TotalUnread:   totalUnread,
		ChannelStats:  channelStats,
		GeneratedAt:   time.Now(),
	}
}

func (s *UnifiedInboxService) GetThread(ctx context.Context, unifiedID string) (*InboxThread, []*InboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cust, ok := s.customerByUnifiedID[unifiedID]
	if !ok {
		return nil, nil, fmt.Errorf("客户不存在: %s", unifiedID)
	}
	msgs := s.messages[unifiedID]
	if len(msgs) == 0 {
		return &InboxThread{UnifiedID: unifiedID, CustomerID: cust.CustomerID, CustomerName: cust.CustomerName}, nil, nil
	}
	channelSet := make(map[InboxChannel]bool)
	channelLabels := make([]string, 0)
	for _, m := range msgs {
		if !channelSet[m.Channel] {
			channelSet[m.Channel] = true
			channelLabels = append(channelLabels, m.ChannelLabel)
		}
	}
	channels := make([]InboxChannel, 0, len(channelSet))
	for c := range channelSet {
		channels = append(channels, c)
	}
	last := msgs[len(msgs)-1]
	stage := ""
	stageLabel := ""
	if s.journey != nil {
		state := s.journey.GetState(ctx, cust.CustomerID)
		stage = string(state.CurrentStage)
		if meta, ok := StageMetas[state.CurrentStage]; ok && meta != nil {
			stageLabel = meta.Label
		}
	}
	thread := &InboxThread{
		UnifiedID:     unifiedID,
		CustomerID:    cust.CustomerID,
		CustomerName:  cust.CustomerName,
		Phone:         cust.Phone,
		Email:         cust.Email,
		Channels:      channels,
		ChannelLabels: channelLabels,
		TotalMessages: len(msgs),
		LastMessageAt: last.ReceivedAt,
		LastMessage:   inboxTruncate(last.Content, 80),
		LastChannel:   last.Channel,
		JourneyStage:  stage,
		JourneyLabel:  stageLabel,
		OwnerID:       cust.OwnerID,
		Tags:          cust.Tags,
	}
	// 复制消息（避免外部修改）
	msgCopy := make([]*InboxMessage, len(msgs))
	for i, m := range msgs {
		cp := *m
		msgCopy[i] = &cp
	}
	return thread, msgCopy, nil
}

func (s *UnifiedInboxService) MarkRead(ctx context.Context, unifiedID string, messageIDs []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, id := range messageIDs {
		if !s.readSet[id] {
			s.readSet[id] = true
			count++
		}
	}
	// 同步到 messages
	if msgs, ok := s.messages[unifiedID]; ok {
		readSet := make(map[string]bool)
		for _, id := range messageIDs {
			readSet[id] = true
		}
		for _, m := range msgs {
			if readSet[m.MessageID] {
				m.IsRead = true
			}
		}
	}
	return count
}

func (s *UnifiedInboxService) MergeAccounts(ctx context.Context, primaryUnifiedID, secondaryUnifiedID string) error {
	if primaryUnifiedID == secondaryUnifiedID {
		return fmt.Errorf("不能合并自身")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	primary, ok1 := s.customerByUnifiedID[primaryUnifiedID]
	secondary, ok2 := s.customerByUnifiedID[secondaryUnifiedID]
	if !ok1 || !ok2 {
		return fmt.Errorf("客户不存在")
	}
	// 合并身份标识
	if primary.Phone == "" && secondary.Phone != "" {
		primary.Phone = secondary.Phone
		s.phoneIndex[secondary.Phone] = primaryUnifiedID
	}
	if primary.Email == "" && secondary.Email != "" {
		primary.Email = secondary.Email
		s.emailIndex[secondary.Email] = primaryUnifiedID
	}
	if primary.WechatOpenID == "" && secondary.WechatOpenID != "" {
		primary.WechatOpenID = secondary.WechatOpenID
		s.wechatIndex[secondary.WechatOpenID] = primaryUnifiedID
	}
	if primary.DouyinOpenID == "" && secondary.DouyinOpenID != "" {
		primary.DouyinOpenID = secondary.DouyinOpenID
		s.douyinIndex[secondary.DouyinOpenID] = primaryUnifiedID
	}
	if primary.XiaohongshuID == "" && secondary.XiaohongshuID != "" {
		primary.XiaohongshuID = secondary.XiaohongshuID
		s.xhsIndex[secondary.XiaohongshuID] = primaryUnifiedID
	}
	// 合并消息
	if sec, ok := s.messages[secondaryUnifiedID]; ok {
		s.messages[primaryUnifiedID] = append(s.messages[primaryUnifiedID], sec...)
		delete(s.messages, secondaryUnifiedID)
	}
	// 合并标签
	for _, t := range secondary.Tags {
		found := false
		for _, pt := range primary.Tags {
			if pt == t {
				found = true
				break
			}
		}
		if !found {
			primary.Tags = append(primary.Tags, t)
		}
	}
	// 删除次要客户
	delete(s.customerByUnifiedID, secondaryUnifiedID)
	return nil
}
