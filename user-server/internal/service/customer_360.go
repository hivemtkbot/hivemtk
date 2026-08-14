package service

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	_type "hivemtk-user/internal/pkg/utils/type"

	"hivemtk-user/internal/repository"

	"strconv"

	"time"

	"gorm.io/gorm"
)

type Customer360ServiceInterface interface {
	GetCustomer360(userID string) (*Customer360DTO, error)
	GetCustomerList(page, pageSize int, filters map[string]string) (map[string]*Customer360DTO, int64, error)
}

type Customer360Service struct {
	sessionRepo      *repository.CustomerSessionRepository
	messageRepo      *repository.SessionMessageRepository
	clueRepo         repository.ClueRepository
	orderRepo        repository.OrderRepository
	unifiedMsgRepo   repository.UnifiedMessageRepository
	unifiedReplyRepo repository.UnifiedReplyRepository
	customerRepo     repository.CustomerRepository
}

func NewCustomer360ServiceWithDB(db *gorm.DB) *Customer360Service {
	return &Customer360Service{
		sessionRepo:      repository.NewCustomerSessionRepositoryWithDB(db),
		messageRepo:      repository.NewSessionMessageRepositoryWithDB(db),
		clueRepo:         repository.NewClueRepositoryWithDB(db),
		orderRepo:        repository.NewOrderRepositoryWithDB(db),
		unifiedMsgRepo:   repository.NewUnifiedMessageRepositoryWithDB(db),
		unifiedReplyRepo: repository.NewUnifiedReplyRepositoryWithDB(db),
		customerRepo:     repository.NewCustomerRepository(),
	}
}

func NewCustomer360Service() *Customer360Service {
	return &Customer360Service{
		sessionRepo:      repository.NewCustomerSessionRepository(),
		messageRepo:      repository.NewSessionMessageRepository(),
		clueRepo:         repository.NewClueRepository(),
		orderRepo:        repository.NewOrderRepository(),
		unifiedMsgRepo:   repository.NewUnifiedMessageRepository(),
		unifiedReplyRepo: repository.NewUnifiedReplyRepository(),
		customerRepo:     repository.NewCustomerRepository(),
	}
}

type Customer360DTO struct {
	// 基本信息
	BasicInfo *CustomerBasicInfo `json:"basic_info"`

	// 会话统计
	SessionStats *SessionStatistics `json:"session_stats"`

	// 会话历史
	SessionHistory []*SessionHistoryItem `json:"session_history"`

	// 消息记录
	MessageHistory []*MessageHistoryItem `json:"message_history"`

	// 线索信息
	ClueInfo *ClueInfo `json:"clue_info,omitempty"`

	// 订单信息
	OrderInfo *OrderInfo `json:"order_info,omitempty"`

	// 互动统计
	InteractionStats *InteractionStats `json:"interaction_stats"`

	// 用户画像
	UserProfile *UserProfile `json:"user_profile"`
}

type CustomerBasicInfo struct {
	UserID         string `json:"user_id"`
	UserName       string `json:"user_name"`
	UserAvatar     string `json:"user_avatar"`
	UserPhone      string `json:"user_phone"`
	UserEmail      string `json:"user_email"`
	FirstSeenAt    string `json:"first_seen_at"`
	LastSeenAt     string `json:"last_seen_at"`
	SourcePlatform string `json:"source_platform"`
}

type SessionStatistics struct {
	TotalSessions   int64 `json:"total_sessions"`
	ActiveSessions  int64 `json:"active_sessions"`
	ClosedSessions  int64 `json:"closed_sessions"`
	TotalMessages   int64 `json:"total_messages"`
	AvgResponseTime int   `json:"avg_response_time"` // 平均响应时间（秒）
	AIReplies       int64 `json:"ai_replies"`
	HumanReplies    int64 `json:"human_replies"`
	UserRating      int   `json:"user_rating"`  // 用户评分
	RatingCount     int64 `json:"rating_count"` // 评分次数
}

type SessionHistoryItem struct {
	SessionID     string `json:"session_id"`
	Platform      string `json:"platform"`
	Status        string `json:"status"`
	MessageCount  int    `json:"message_count"`
	LastMessage   string `json:"last_message"`
	LastMessageAt string `json:"last_message_at"`
	HandlerType   string `json:"handler_type"`
	AgentName     string `json:"agent_name,omitempty"`
	Rating        int    `json:"rating,omitempty"`
}

type MessageHistoryItem struct {
	SessionID    string  `json:"session_id"`
	Content      string  `json:"content"`
	SenderType   string  `json:"sender_type"` // user, ai, agent
	SenderName   string  `json:"sender_name"`
	ContentType  string  `json:"content_type"`
	CreatedAt    string  `json:"created_at"`
	AIConfidence float64 `json:"ai_confidence,omitempty"`
}

type ClueInfo struct {
	ClueID    string `json:"clue_id"`
	Source    string `json:"source"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Status    string `json:"status"` // new, contacted, qualified, converted, lost
	Level     string `json:"level"`  // hot, warm, cold
	Owner     string `json:"owner"`
	CreatedAt string `json:"created_at"`
}

type OrderInfo struct {
	TotalOrders     int64        `json:"total_orders"`
	TotalAmount     float64      `json:"total_amount"`
	LastOrderAt     string       `json:"last_order_at"`
	LastOrderID     string       `json:"last_order_id"`
	LastOrderAmount float64      `json:"last_order_amount"`
	Orders          []*OrderItem `json:"orders"`
}

type OrderItem struct {
	OrderID     string  `json:"order_id"`
	Amount      float64 `json:"amount"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	ProductName string  `json:"product_name"`
}

type InteractionStats struct {
	TotalInteractions int64 `json:"total_interactions"`
	DouyinCount       int64 `json:"douyin_count"`
	KuaishouCount     int64 `json:"kuaishou_count"`
	XiaohongshuCount  int64 `json:"xiaohongshu_count"`
	XianyuCount       int64 `json:"xianyu_count"`
	TiktokCount       int64 `json:"tiktok_count"`

	Last7Days  int64 `json:"last_7_days"`
	Last30Days int64 `json:"last_30_days"`

	AvgMessagesPerSession float64 `json:"avg_messages_per_session"`
	FirstResponseTime     int     `json:"first_response_time"` // 首次响应时间（秒）
}

type UserProfile struct {
	Tags              []string `json:"tags"`
	Interests         []string `json:"interests"`
	PurchasePower     string   `json:"purchase_power"` // high, medium, low
	ActivityLevel     string   `json:"activity_level"` // active, normal, silent
	RiskLevel         string   `json:"risk_level"`     // high, normal, low
	PreferredPlatform string   `json:"preferred_platform"`
	PreferredTime     string   `json:"preferred_time"`
}

func (s *Customer360Service) GetCustomer360(ctx context.Context, userID string) (*Customer360DTO, error) {
	dto := &Customer360DTO{}

	// 1. 获取该用户的会话（GetByUserID 直接走 user_id 索引，单 SQL 拉全该用户会话）
	userSessions, err := s.sessionRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(userSessions) == 0 {
		return nil, ErrCustomerNotFound
	}

	// 2. 填充基本信息
	dto.BasicInfo = s.buildBasicInfo(ctx, userSessions)

	// 3. 填充会话统计
	dto.SessionStats = s.buildSessionStats(ctx, userSessions)

	// 4. 填充会话历史
	dto.SessionHistory = s.buildSessionHistory(ctx, userSessions)

	// 5. 填充消息历史（批量拉所有 session 消息，按 session_id 分桶）
	dto.MessageHistory, _ = s.buildMessageHistory(ctx, userSessions)

	// 6. 填充线索信息（单次 SQL 拉取）
	dto.ClueInfo, _ = s.buildClueInfo(ctx, userID, userSessions)

	// 7. 填充订单信息（单次 SQL 拉取）
	dto.OrderInfo, _ = s.buildOrderInfo(ctx, userID, userSessions)

	// 8. 填充互动统计
	dto.InteractionStats = s.buildInteractionStats(ctx, userSessions)

	// 9. 填充用户画像
	dto.UserProfile = s.buildUserProfile(ctx, userSessions, dto.InteractionStats, dto.OrderInfo)

	return dto, nil
}

// GetCustomer360ByCustomerID 按客户档案主键 id 获取客户 360° 视图。
//
// 前端调用 /api/customer/360/:id 与 /api/customer/:id 时传入的是 customers 表主键 id，
// 而底层会话表（customer_sessions）与客户档案的关联键是 one_id（= customers.unified_id），
// 并非会话的 user_id 字段。因此这里先按客户 id 解析出 unified_id，再按 one_id 查会话。
//
// 即使该客户当前没有关联会话（customer_sessions.one_id 为空是常见情况），也返回客户基本
// 档案（姓名/电话/邮箱/OneID 等），不再误报 404。
func (s *Customer360Service) GetCustomer360ByCustomerID(ctx context.Context, customerID string) (*Customer360DTO, error) {
	cust, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerNotFound
		}
		return nil, err
	}
	if cust == nil {
		return nil, ErrCustomerNotFound
	}

	dto := &Customer360DTO{}

	// 用客户的 OneID 查关联会话（会话 one_id = customers.unified_id）
	sessions, err := s.sessionRepo.GetByOneID(ctx, cust.UnifiedID)
	if err != nil {
		return nil, err
	}

	// 基本信息：优先从客户档案填充（不依赖会话是否存在）
	dto.BasicInfo = s.buildBasicInfoFromCustomer(cust, sessions)

	if len(sessions) == 0 {
		// 无会话时仍返回客户档案，仅会话相关字段留空，避免误报 404
		dto.SessionStats = &SessionStatistics{}
		dto.InteractionStats = &InteractionStats{}
		dto.UserProfile = &UserProfile{}
		return dto, nil
	}

	// 2. 填充会话统计
	dto.SessionStats = s.buildSessionStats(ctx, sessions)

	// 3. 填充会话历史
	dto.SessionHistory = s.buildSessionHistory(ctx, sessions)

	// 4. 填充消息历史（批量拉所有 session 消息，按 session_id 分桶）
	dto.MessageHistory, _ = s.buildMessageHistory(ctx, sessions)

	// 5. 填充线索信息（单次 SQL 拉取）
	dto.ClueInfo, _ = s.buildClueInfo(ctx, cust.UnifiedID, sessions)

	// 6. 填充订单信息（单次 SQL 拉取）
	dto.OrderInfo, _ = s.buildOrderInfo(ctx, cust.UnifiedID, sessions)

	// 7. 填充互动统计
	dto.InteractionStats = s.buildInteractionStats(ctx, sessions)

	// 8. 填充用户画像
	dto.UserProfile = s.buildUserProfile(ctx, sessions, dto.InteractionStats, dto.OrderInfo)

	return dto, nil
}

// buildBasicInfoFromCustomer 从客户档案构造基本展示信息；若有关联会话，补充首次/最近活跃时间。
func (s *Customer360Service) buildBasicInfoFromCustomer(cust *model.Customer, sessions []*model.CustomerSession) *CustomerBasicInfo {
	info := &CustomerBasicInfo{
		UserID:    cust.ID,
		UserPhone: cust.Phone,
		UserEmail: cust.Email,
	}
	if cust.UnifiedID != "" {
		info.SourcePlatform = cust.UnifiedID
	}
	if len(sessions) > 0 {
		var firstSeenAt, lastSeenAt string
		for _, session := range sessions {
			ts := session.CreatedAt.Format("2006-01-02 15:04:05")
			if firstSeenAt == "" || ts < firstSeenAt {
				firstSeenAt = ts
			}
			if lastSeenAt == "" || ts > lastSeenAt {
				lastSeenAt = ts
			}
		}
		info.FirstSeenAt = firstSeenAt
		info.LastSeenAt = lastSeenAt
		info.SourcePlatform = string(sessions[0].Platform)
		if info.UserName == "" {
			info.UserName = sessions[0].UserName
		}
		if info.UserPhone == "" {
			info.UserPhone = sessions[0].UserPhone
		}
		if info.UserEmail == "" {
			info.UserEmail = sessions[0].UserEmail
		}
	}
	return info
}

func (s *Customer360Service) buildBasicInfo(ctx context.Context, sessions []*model.CustomerSession) *CustomerBasicInfo {
	if len(sessions) == 0 {
		return nil
	}

	// 取第一个会话的信息
	first := sessions[0]
	var firstSeenAt, lastSeenAt string

	for _, session := range sessions {
		if firstSeenAt == "" || session.CreatedAt.Format("2006-01-02 15:04:05") < firstSeenAt {
			firstSeenAt = session.CreatedAt.Format("2006-01-02 15:04:05")
		}
		if lastSeenAt == "" || session.CreatedAt.Format("2006-01-02 15:04:05") > lastSeenAt {
			lastSeenAt = session.CreatedAt.Format("2006-01-02 15:04:05")
		}
	}

	return &CustomerBasicInfo{
		UserID:         first.UserID,
		UserName:       first.UserName,
		UserAvatar:     first.UserAvatar,
		UserPhone:      first.UserPhone,
		UserEmail:      first.UserEmail,
		FirstSeenAt:    firstSeenAt,
		LastSeenAt:     lastSeenAt,
		SourcePlatform: string(first.Platform),
	}
}

func (s *Customer360Service) buildSessionStats(ctx context.Context, sessions []*model.CustomerSession) *SessionStatistics {
	if len(sessions) == 0 {
		return nil
	}

	stats := &SessionStatistics{}
	var totalResponseTime int
	var ratingSum int
	var ratingCount int64

	for _, session := range sessions {
		stats.TotalSessions++

		switch session.Status {
		case model.SessionStatusPending, model.SessionStatusAIHandling, model.SessionStatusHumanHandling, model.SessionStatusWaiting:
			stats.ActiveSessions++
		case model.SessionStatusResolved, model.SessionStatusClosed:
			stats.ClosedSessions++
		}

		stats.TotalMessages += int64(session.MessageCount)
		stats.AIReplies += int64(session.AIReplyCount)
		stats.HumanReplies += int64(session.HumanReplyCount)

		if session.AvgResponseTime > 0 {
			totalResponseTime += session.AvgResponseTime
		}

		if session.Rating > 0 {
			ratingSum += session.Rating
			ratingCount++
		}
	}

	if stats.TotalSessions > 0 {
		stats.AvgResponseTime = totalResponseTime / int(stats.TotalSessions)
	}

	if ratingCount > 0 {
		stats.UserRating = ratingSum / int(ratingCount)
		stats.RatingCount = ratingCount
	}

	return stats
}

func (s *Customer360Service) buildSessionHistory(ctx context.Context, sessions []*model.CustomerSession) []*SessionHistoryItem {
	history := make([]*SessionHistoryItem, 0, len(sessions))

	for _, session := range sessions {
		item := &SessionHistoryItem{
			SessionID:    session.SessionID,
			Platform:     string(session.Platform),
			Status:       string(session.Status),
			MessageCount: session.MessageCount,
			LastMessage:  session.LastMessage,
			HandlerType:  string(session.HandlerType),
			AgentName:    session.AgentName,
			Rating:       session.Rating,
		}

		if session.LastMessageAt != nil {
			item.LastMessageAt = session.LastMessageAt.Format("2006-01-02 15:04:05")
		}

		history = append(history, item)
	}

	return history
}

func (s *Customer360Service) buildMessageHistory(ctx context.Context, sessions []*model.CustomerSession) ([]*MessageHistoryItem, error) {
	history := make([]*MessageHistoryItem, 0)
	if len(sessions) == 0 {
		return history, nil
	}

	// 收集所有 sessionID（按出现顺序去重）
	sessionIDs := make([]string, 0, len(sessions))
	seen := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session.SessionID == "" {
			continue
		}
		if _, ok := seen[session.SessionID]; ok {
			continue
		}
		seen[session.SessionID] = struct{}{}
		sessionIDs = append(sessionIDs, session.SessionID)
	}
	if len(sessionIDs) == 0 {
		return history, nil
	}

	// 单次 SQL 拉所有 session 的消息
	messageMap, err := s.messageRepo.ListBySessionIDsBatch(ctx, sessionIDs, 100)
	if err != nil {
		return history, err
	}

	// 按 sessions 顺序拼接，保证 message_history 与 session_history 顺序对齐
	for _, session := range sessions {
		messages := messageMap[session.SessionID]
		for _, msg := range messages {
			history = append(history, &MessageHistoryItem{
				SessionID:    msg.SessionID,
				Content:      msg.Content,
				SenderType:   msg.SenderType,
				SenderName:   msg.SenderName,
				ContentType:  string(msg.ContentType),
				CreatedAt:    msg.CreatedAt.Format("2006-01-02 15:04:05"),
				AIConfidence: msg.AIConfidence,
			})
		}
	}

	return history, nil
}

func (s *Customer360Service) buildClueInfo(ctx context.Context, userID string, userSessions []*model.CustomerSession) (*ClueInfo, error) {
	// 从已加载的 userSessions 提取 phone / email / accountID
	var userPhone, userEmail, accountID string
	for _, session := range userSessions {
		if session.UserID == userID {
			userPhone = session.UserPhone
			userEmail = session.UserEmail
			accountID = session.AccountID
			break
		}
	}

	if userPhone == "" && userEmail == "" && accountID == "" {
		return nil, nil
	}

	// 单次 SQL 拉取所有线索（按 type 内存去重）
	clues, err := s.clueRepo.ListByAccounts(ctx, []string{accountID, userPhone, userEmail})
	if err != nil {
		return nil, err
	}

	if len(clues) == 0 {
		return nil, nil
	}

	// 复用 listByAccounts 的 create_time DESC 排序，取首条即最新
	latestClue := clues[0]
	clueInfo := &ClueInfo{
		ClueID:    latestClue.ID,
		Source:    "platform",
		Name:      latestClue.Name,
		Phone:     latestClue.Account,
		Status:    "new",
		Level:     "warm",
		CreatedAt: time.Unix(latestClue.CreateTime, 0).Format("2006-01-02 15:04:05"),
	}

	// 根据 IsVerify 设置状态
	if latestClue.IsVerify == 1 {
		clueInfo.Status = "qualified"
	}

	return clueInfo, nil
}

func (s *Customer360Service) buildOrderInfo(ctx context.Context, userID string, userSessions []*model.CustomerSession) (*OrderInfo, error) {
	// 从已加载的 userSessions 提取 accountID
	var accountID string
	for _, session := range userSessions {
		if session.UserID == userID {
			accountID = session.AccountID
			break
		}
	}

	if accountID == "" {
		return &OrderInfo{
			Orders: make([]*OrderItem, 0),
		}, nil
	}

	// 单次 SQL 拉取所有订单
	userOrders, err := s.orderRepo.ListByAccountIDs(ctx, []string{accountID})
	if err != nil {
		return &OrderInfo{
			Orders: make([]*OrderItem, 0),
		}, nil
	}

	if len(userOrders) == 0 {
		return &OrderInfo{
			Orders: make([]*OrderItem, 0),
		}, nil
	}

	// 计算统计数据（ListByAccountIDs 已按 create_time DESC 排序，首条即最新）
	var totalAmount float64
	lastOrder := userOrders[0]
	orderItems := make([]*OrderItem, 0, len(userOrders))

	for _, order := range userOrders {
		amount, _ := strconv.ParseFloat(order.Price, 64)
		totalAmount += amount

		orderItems = append(orderItems, &OrderItem{
			OrderID:     order.ID,
			Amount:      amount,
			Status:      orderStatusToString(order.Status),
			CreatedAt:   time.Unix(order.CreateTime, 0).Format("2006-01-02 15:04:05"),
			ProductName: "平台商品", // In production, this would query the product table for actual name
		})
	}

	orderInfo := &OrderInfo{
		TotalOrders: int64(len(userOrders)),
		TotalAmount: totalAmount,
		Orders:      orderItems,
	}

	// 填充最新订单信息
	if lastOrder != nil {
		orderInfo.LastOrderID = lastOrder.ID
		orderInfo.LastOrderAt = time.Unix(lastOrder.CreateTime, 0).Format("2006-01-02 15:04:05")
		amount, _ := strconv.ParseFloat(lastOrder.Price, 64)
		orderInfo.LastOrderAmount = amount
	}

	return orderInfo, nil
}

func orderStatusToString(status _type.OrderStatusType) string {
	switch status {
	case _type.OrderStatusPending:
		return "pending" // 待支付
	case _type.OrderStatusSuccess:
		return "success" // 已支付/成功
	case _type.OrderStatusForceSuccess:
		return "success" // 强制成功
	case _type.OrderStatusTimeout:
		return "timeout" // 超时
	case _type.OrderStatusForceClose:
		return "closed" // 强制关闭
	default:
		return "unknown"
	}
}

func (s *Customer360Service) buildInteractionStats(ctx context.Context, sessions []*model.CustomerSession) *InteractionStats {
	stats := &InteractionStats{}

	platformCount := make(map[string]int64)
	var firstResponseTimeTotal int
	var firstResponseTimeCount int

	for _, session := range sessions {
		platformCount[string(session.Platform)]++
		stats.TotalInteractions++

		if session.AvgResponseTime > 0 {
			firstResponseTimeTotal += session.AvgResponseTime
			firstResponseTimeCount++
		}
	}

	stats.DouyinCount = platformCount[string(model.PlatformDouyin)]
	stats.KuaishouCount = platformCount[string(model.PlatformKuaishou)]
	stats.XiaohongshuCount = platformCount[string(model.PlatformXiaohongshu)]
	stats.XianyuCount = platformCount[string(model.PlatformXianyu)]
	stats.TiktokCount = platformCount[string(model.PlatformTiktok)]

	if len(sessions) > 0 {
		stats.AvgMessagesPerSession = float64(stats.TotalInteractions) / float64(len(sessions))
	}

	if firstResponseTimeCount > 0 {
		stats.FirstResponseTime = firstResponseTimeTotal / firstResponseTimeCount
	}

	return stats
}

func (s *Customer360Service) buildUserProfile(ctx context.Context, sessions []*model.CustomerSession, interactionStats *InteractionStats, orderInfo *OrderInfo) *UserProfile {
	profile := &UserProfile{
		Tags:          make([]string, 0),
		Interests:     make([]string, 0),
		PurchasePower: "medium",
		ActivityLevel: "normal",
		RiskLevel:     "normal",
	}

	// 根据互动频率判断活跃度
	if interactionStats.TotalInteractions > 20 {
		profile.ActivityLevel = "active"
	} else if interactionStats.TotalInteractions < 5 {
		profile.ActivityLevel = "silent"
	}

	// 根据订单金额判断购买力
	if orderInfo != nil && orderInfo.TotalAmount > 10000 {
		profile.PurchasePower = "high"
	} else if orderInfo != nil && orderInfo.TotalAmount < 1000 {
		profile.PurchasePower = "low"
	}

	// Analyze conversation content for interests and tags
	// In production, this would use AI to extract interests and tags from conversation content

	// 判断首选平台
	maxCount := int64(0)
	for platform, count := range map[string]int64{
		"douyin":      interactionStats.DouyinCount,
		"kuaishou":    interactionStats.KuaishouCount,
		"xiaohongshu": interactionStats.XiaohongshuCount,
		"xianyu":      interactionStats.XianyuCount,
		"tiktok":      interactionStats.TiktokCount,
	} {
		if count > maxCount {
			maxCount = count
			profile.PreferredPlatform = platform
		}
	}

	return profile
}

func (s *Customer360Service) GetCustomerList(ctx context.Context, page, pageSize int, filters map[string]string) (map[string]*Customer360DTO, int64, error) {
	sessions, total, err := s.sessionRepo.GetByMerchant(ctx, "", page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	// 按用户分组
	userSessionsMap := make(map[string][]*model.CustomerSession)
	for _, session := range sessions {
		userSessionsMap[session.UserID] = append(userSessionsMap[session.UserID], session)
	}

	// 收集该页所有 sessionID / userID / accountID
	allSessionIDs := make([]string, 0, len(sessions))
	seenSess := make(map[string]struct{}, len(sessions))
	userIDs := make([]string, 0, len(userSessionsMap))
	accountIDs := make([]string, 0, len(userSessionsMap))
	seenAcct := make(map[string]struct{}, len(userSessionsMap))

	for _, session := range sessions {
		if session.SessionID != "" {
			if _, ok := seenSess[session.SessionID]; !ok {
				seenSess[session.SessionID] = struct{}{}
				allSessionIDs = append(allSessionIDs, session.SessionID)
			}
		}
		if session.AccountID != "" {
			if _, ok := seenAcct[session.AccountID]; !ok {
				seenAcct[session.AccountID] = struct{}{}
				accountIDs = append(accountIDs, session.AccountID)
			}
		}
	}
	for uid := range userSessionsMap {
		userIDs = append(userIDs, uid)
	}

	// 单次 SQL 拉所有 session 的消息
	messageMap, _ := s.messageRepo.ListBySessionIDsBatch(ctx, allSessionIDs, 100)

	// 单次 SQL 拉所有相关线索（按 account / phone / email 三类 key）
	allAccounts := make([]string, 0, len(accountIDs)+len(userSessionsMap))
	allAccounts = append(allAccounts, accountIDs...)
	for _, ss := range sessions {
		allAccounts = append(allAccounts, ss.AccountID, ss.UserPhone, ss.UserEmail)
	}
	clueMap := s.indexCluesByAccount(ctx, allAccounts)

	// 单次 SQL 拉所有相关订单
	orders, _ := s.orderRepo.ListByAccountIDs(ctx, accountIDs)
	orderMap := make(map[string][]*model.Order, len(accountIDs))
	for _, o := range orders {
		orderMap[o.AccountID] = append(orderMap[o.AccountID], o)
	}

	// 内存组装每个客户的 360 视图
	result := make(map[string]*Customer360DTO, len(userSessionsMap))
	for userID, userSessions := range userSessionsMap {
		dto := s.assembleCustomer360DTO(userSessions, messageMap, clueMap, orderMap)
		if dto == nil {
			continue
		}
		result[userID] = dto
	}

	return result, total, nil
}

func (s *Customer360Service) indexCluesByAccount(ctx context.Context, accounts []string) map[string][]*model.Clue {
	out := make(map[string][]*model.Clue)
	if len(accounts) == 0 {
		return out
	}
	clues, err := s.clueRepo.ListByAccounts(ctx, accounts)
	if err != nil {
		return out
	}
	for _, c := range clues {
		if c.Account != "" {
			out[c.Account] = append(out[c.Account], c)
		}
		if c.Name != "" {
			out[c.Name] = append(out[c.Name], c)
		}
	}
	return out
}

func (s *Customer360Service) assembleCustomer360DTO(

	userSessions []*model.CustomerSession,

	messageMap map[string][]*model.SessionMessage,

	clueMap map[string][]*model.Clue,

	orderMap map[string][]*model.Order,

) *Customer360DTO {
	if len(userSessions) == 0 {
		return nil
	}
	dto := &Customer360DTO{}
	dto.BasicInfo = s.buildBasicInfo(nil, userSessions)
	dto.SessionStats = s.buildSessionStats(nil, userSessions)
	dto.SessionHistory = s.buildSessionHistory(nil, userSessions)
	dto.MessageHistory = s.buildMessageHistoryFromMap(userSessions, messageMap)
	dto.ClueInfo = s.buildClueInfoFromMap(userSessions, clueMap)
	dto.OrderInfo = s.buildOrderInfoFromMap(userSessions, orderMap)
	dto.InteractionStats = s.buildInteractionStats(nil, userSessions)
	dto.UserProfile = s.buildUserProfile(nil, userSessions, dto.InteractionStats, dto.OrderInfo)
	return dto
}

func (s *Customer360Service) buildMessageHistoryFromMap(sessions []*model.CustomerSession, messageMap map[string][]*model.SessionMessage) []*MessageHistoryItem {
	history := make([]*MessageHistoryItem, 0)
	for _, session := range sessions {
		messages := messageMap[session.SessionID]
		for _, msg := range messages {
			history = append(history, &MessageHistoryItem{
				SessionID:    msg.SessionID,
				Content:      msg.Content,
				SenderType:   msg.SenderType,
				SenderName:   msg.SenderName,
				ContentType:  string(msg.ContentType),
				CreatedAt:    msg.CreatedAt.Format("2006-01-02 15:04:05"),
				AIConfidence: msg.AIConfidence,
			})
		}
	}
	return history
}

func (s *Customer360Service) buildClueInfoFromMap(userSessions []*model.CustomerSession, clueMap map[string][]*model.Clue) *ClueInfo {
	if len(userSessions) == 0 {
		return nil
	}
	first := userSessions[0]
	// 合并查询的 keys：accountID / phone / email
	keys := []string{first.AccountID, first.UserPhone, first.UserEmail}
	var latest *model.Clue
	for _, k := range keys {
		for _, c := range clueMap[k] {
			if latest == nil || c.CreateTime > latest.CreateTime {
				latest = c
			}
		}
	}
	if latest == nil {
		return nil
	}
	clueInfo := &ClueInfo{
		ClueID:    latest.ID,
		Source:    "platform",
		Name:      latest.Name,
		Phone:     latest.Account,
		Status:    "new",
		Level:     "warm",
		CreatedAt: time.Unix(latest.CreateTime, 0).Format("2006-01-02 15:04:05"),
	}
	if latest.IsVerify == 1 {
		clueInfo.Status = "qualified"
	}
	return clueInfo
}

func (s *Customer360Service) buildOrderInfoFromMap(userSessions []*model.CustomerSession, orderMap map[string][]*model.Order) *OrderInfo {
	if len(userSessions) == 0 {
		return &OrderInfo{Orders: make([]*OrderItem, 0)}
	}
	accountID := userSessions[0].AccountID
	if accountID == "" {
		return &OrderInfo{Orders: make([]*OrderItem, 0)}
	}
	userOrders := orderMap[accountID]
	if len(userOrders) == 0 {
		return &OrderInfo{Orders: make([]*OrderItem, 0)}
	}
	var totalAmount float64
	lastOrder := userOrders[0] // ListByAccountIDs 已按 create_time DESC 排序
	orderItems := make([]*OrderItem, 0, len(userOrders))
	for _, order := range userOrders {
		amount, _ := strconv.ParseFloat(order.Price, 64)
		totalAmount += amount
		orderItems = append(orderItems, &OrderItem{
			OrderID:     order.ID,
			Amount:      amount,
			Status:      orderStatusToString(order.Status),
			CreatedAt:   time.Unix(order.CreateTime, 0).Format("2006-01-02 15:04:05"),
			ProductName: "平台商品",
		})
	}
	orderInfo := &OrderInfo{
		TotalOrders: int64(len(userOrders)),
		TotalAmount: totalAmount,
		Orders:      orderItems,
	}
	if lastOrder != nil {
		orderInfo.LastOrderID = lastOrder.ID
		orderInfo.LastOrderAt = time.Unix(lastOrder.CreateTime, 0).Format("2006-01-02 15:04:05")
		amount, _ := strconv.ParseFloat(lastOrder.Price, 64)
		orderInfo.LastOrderAmount = amount
	}
	return orderInfo
}

// Customer360ServiceForTest 用于测试的 Customer360Service（公开字段）
type Customer360ServiceForTest struct {
	SessionRepo      *repository.CustomerSessionRepository
	MessageRepo      *repository.SessionMessageRepository
	ClueRepo         repository.ClueRepository
	OrderRepo        repository.OrderRepository
	UnifiedMsgRepo   repository.UnifiedMessageRepository
	UnifiedReplyRepo repository.UnifiedReplyRepository
}

// GetCustomer360 获取客户 360 视图（测试版本）
func (s *Customer360ServiceForTest) GetCustomer360(ctx context.Context, userID string) (*Customer360DTO, error) {
	realService := &Customer360Service{
		sessionRepo:      s.SessionRepo,
		messageRepo:      s.MessageRepo,
		clueRepo:         s.ClueRepo,
		orderRepo:        s.OrderRepo,
		unifiedMsgRepo:   s.UnifiedMsgRepo,
		unifiedReplyRepo: s.UnifiedReplyRepo,
	}
	return realService.GetCustomer360(ctx, userID)
}

// GetCustomerList 获取客户列表（测试版本）
func (s *Customer360ServiceForTest) GetCustomerList(ctx context.Context, page, pageSize int, filters map[string]string) (map[string]*Customer360DTO, int64, error) {
	realService := &Customer360Service{
		sessionRepo:      s.SessionRepo,
		messageRepo:      s.MessageRepo,
		clueRepo:         s.ClueRepo,
		orderRepo:        s.OrderRepo,
		unifiedMsgRepo:   s.UnifiedMsgRepo,
		unifiedReplyRepo: s.UnifiedReplyRepo,
	}
	return realService.GetCustomerList(ctx, page, pageSize, filters)
}
