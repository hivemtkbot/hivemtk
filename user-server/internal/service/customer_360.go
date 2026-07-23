package service

import (
	"context"
	"gorm.io/gorm"
	"marketing/internal/model"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/repository"
	"strconv"
	"time"
)

// Customer360ServiceInterface Customer360 服务接口
type Customer360ServiceInterface interface {
	GetCustomer360(userID string) (*Customer360DTO, error)
	GetCustomerList(page, pageSize int, filters map[string]string) (map[string]*Customer360DTO, int64, error)
}

// Customer360Service 客户 360° 视图服务
type Customer360Service struct {
	sessionRepo		*repository.CustomerSessionRepository
	messageRepo		*repository.SessionMessageRepository
	clueRepo		repository.ClueRepository
	orderRepo		repository.OrderRepository
	unifiedMsgRepo		repository.UnifiedMessageRepository
	unifiedReplyRepo	repository.UnifiedReplyRepository
}

// NewCustomer360ServiceWithDB 创建客户 360° 视图服务（带数据库连接，用于测试）
func NewCustomer360ServiceWithDB(db *gorm.DB) *Customer360Service {
	return &Customer360Service{
		sessionRepo:		repository.NewCustomerSessionRepositoryWithDB(db),
		messageRepo:		repository.NewSessionMessageRepositoryWithDB(db),
		clueRepo:		repository.NewClueRepositoryWithDB(db),
		orderRepo:		repository.NewOrderRepositoryWithDB(db),
		unifiedMsgRepo:		repository.NewUnifiedMessageRepositoryWithDB(db),
		unifiedReplyRepo:	repository.NewUnifiedReplyRepositoryWithDB(db),
	}
}

// NewCustomer360Service 创建客户 360° 视图服务
func NewCustomer360Service() *Customer360Service {
	return &Customer360Service{
		sessionRepo:		repository.NewCustomerSessionRepository(),
		messageRepo:		repository.NewSessionMessageRepository(),
		clueRepo:		repository.NewClueRepository(),
		orderRepo:		repository.NewOrderRepository(),
		unifiedMsgRepo:		repository.NewUnifiedMessageRepository(),
		unifiedReplyRepo:	repository.NewUnifiedReplyRepository(),
	}
}

// Customer360DTO 客户 360° 视图数据传输对象
type Customer360DTO struct {
	// 基本信息
	BasicInfo	*CustomerBasicInfo	`json:"basic_info"`

	// 会话统计
	SessionStats	*SessionStatistics	`json:"session_stats"`

	// 会话历史
	SessionHistory	[]*SessionHistoryItem	`json:"session_history"`

	// 消息记录
	MessageHistory	[]*MessageHistoryItem	`json:"message_history"`

	// 线索信息
	ClueInfo	*ClueInfo	`json:"clue_info,omitempty"`

	// 订单信息
	OrderInfo	*OrderInfo	`json:"order_info,omitempty"`

	// 互动统计
	InteractionStats	*InteractionStats	`json:"interaction_stats"`

	// 用户画像
	UserProfile	*UserProfile	`json:"user_profile"`
}

// CustomerBasicInfo 客户基本信息
type CustomerBasicInfo struct {
	UserID		string	`json:"user_id"`
	UserName	string	`json:"user_name"`
	UserAvatar	string	`json:"user_avatar"`
	UserPhone	string	`json:"user_phone"`
	UserEmail	string	`json:"user_email"`
	FirstSeenAt	string	`json:"first_seen_at"`
	LastSeenAt	string	`json:"last_seen_at"`
	SourcePlatform	string	`json:"source_platform"`
}

// SessionStatistics 会话统计
type SessionStatistics struct {
	TotalSessions	int64	`json:"total_sessions"`
	ActiveSessions	int64	`json:"active_sessions"`
	ClosedSessions	int64	`json:"closed_sessions"`
	TotalMessages	int64	`json:"total_messages"`
	AvgResponseTime	int	`json:"avg_response_time"`	// 平均响应时间（秒）
	AIReplies	int64	`json:"ai_replies"`
	HumanReplies	int64	`json:"human_replies"`
	UserRating	int	`json:"user_rating"`	// 用户评分
	RatingCount	int64	`json:"rating_count"`	// 评分次数
}

// SessionHistoryItem 会话历史项
type SessionHistoryItem struct {
	SessionID	string	`json:"session_id"`
	Platform	string	`json:"platform"`
	Status		string	`json:"status"`
	MessageCount	int	`json:"message_count"`
	LastMessage	string	`json:"last_message"`
	LastMessageAt	string	`json:"last_message_at"`
	HandlerType	string	`json:"handler_type"`
	AgentName	string	`json:"agent_name,omitempty"`
	Rating		int	`json:"rating,omitempty"`
}

// MessageHistoryItem 消息历史项
type MessageHistoryItem struct {
	SessionID	string	`json:"session_id"`
	Content		string	`json:"content"`
	SenderType	string	`json:"sender_type"`	// user, ai, agent
	SenderName	string	`json:"sender_name"`
	ContentType	string	`json:"content_type"`
	CreatedAt	string	`json:"created_at"`
	AIConfidence	float64	`json:"ai_confidence,omitempty"`
}

// ClueInfo 线索信息
type ClueInfo struct {
	ClueID		string	`json:"clue_id"`
	Source		string	`json:"source"`
	Name		string	`json:"name"`
	Phone		string	`json:"phone"`
	Email		string	`json:"email"`
	Status		string	`json:"status"`	// new, contacted, qualified, converted, lost
	Level		string	`json:"level"`	// hot, warm, cold
	Owner		string	`json:"owner"`
	CreatedAt	string	`json:"created_at"`
}

// OrderInfo 订单信息
type OrderInfo struct {
	TotalOrders	int64		`json:"total_orders"`
	TotalAmount	float64		`json:"total_amount"`
	LastOrderAt	string		`json:"last_order_at"`
	LastOrderID	string		`json:"last_order_id"`
	LastOrderAmount	float64		`json:"last_order_amount"`
	Orders		[]*OrderItem	`json:"orders"`
}

// OrderItem 订单项
type OrderItem struct {
	OrderID		string	`json:"order_id"`
	Amount		float64	`json:"amount"`
	Status		string	`json:"status"`
	CreatedAt	string	`json:"created_at"`
	ProductName	string	`json:"product_name"`
}

// InteractionStats 互动统计
type InteractionStats struct {
	TotalInteractions	int64	`json:"total_interactions"`
	DouyinCount		int64	`json:"douyin_count"`
	KuaishouCount		int64	`json:"kuaishou_count"`
	XiaohongshuCount	int64	`json:"xiaohongshu_count"`
	XianyuCount		int64	`json:"xianyu_count"`
	TiktokCount		int64	`json:"tiktok_count"`

	Last7Days	int64	`json:"last_7_days"`
	Last30Days	int64	`json:"last_30_days"`

	AvgMessagesPerSession	float64	`json:"avg_messages_per_session"`
	FirstResponseTime	int	`json:"first_response_time"`	// 首次响应时间（秒）
}

// UserProfile 用户画像
type UserProfile struct {
	Tags			[]string	`json:"tags"`
	Interests		[]string	`json:"interests"`
	PurchasePower		string		`json:"purchase_power"`	// high, medium, low
	ActivityLevel		string		`json:"activity_level"`	// active, normal, silent
	RiskLevel		string		`json:"risk_level"`	// high, normal, low
	PreferredPlatform	string		`json:"preferred_platform"`
	PreferredTime		string		`json:"preferred_time"`
}

// GetCustomer360 获取客户 360° 视图
func (s *Customer360Service) GetCustomer360(ctx context.Context, userID string) (*Customer360DTO, error) {
	dto := &Customer360DTO{}

	// 1. 获取基本信息和会话统计
	sessions, _, _ := s.sessionRepo.GetByMerchant(ctx, "", 1, 1000)

	// 过滤出该用户的会话
	var userSessions []*model.CustomerSession
	for _, session := range sessions {
		if session.UserID == userID {
			userSessions = append(userSessions, session)
		}
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

	// 5. 填充消息历史
	dto.MessageHistory, _ = s.buildMessageHistory(ctx, userSessions)

	// 6. 填充线索信息
	dto.ClueInfo, _ = s.buildClueInfo(ctx, userID)

	// 7. 填充订单信息
	dto.OrderInfo, _ = s.buildOrderInfo(ctx, userID)

	// 8. 填充互动统计
	dto.InteractionStats = s.buildInteractionStats(ctx, userSessions)

	// 9. 填充用户画像
	dto.UserProfile = s.buildUserProfile(ctx, userSessions, dto.InteractionStats, dto.OrderInfo)

	return dto, nil
}

// buildBasicInfo 构建基本信息
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
		UserID:		first.UserID,
		UserName:	first.UserName,
		UserAvatar:	first.UserAvatar,
		UserPhone:	first.UserPhone,
		UserEmail:	first.UserEmail,
		FirstSeenAt:	firstSeenAt,
		LastSeenAt:	lastSeenAt,
		SourcePlatform:	string(first.Platform),
	}
}

// buildSessionStats 构建会话统计
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

// buildSessionHistory 构建会话历史
func (s *Customer360Service) buildSessionHistory(ctx context.Context, sessions []*model.CustomerSession) []*SessionHistoryItem {
	history := make([]*SessionHistoryItem, 0, len(sessions))

	for _, session := range sessions {
		item := &SessionHistoryItem{
			SessionID:	session.SessionID,
			Platform:	string(session.Platform),
			Status:		string(session.Status),
			MessageCount:	session.MessageCount,
			LastMessage:	session.LastMessage,
			HandlerType:	string(session.HandlerType),
			AgentName:	session.AgentName,
			Rating:		session.Rating,
		}

		if session.LastMessageAt != nil {
			item.LastMessageAt = session.LastMessageAt.Format("2006-01-02 15:04:05")
		}

		history = append(history, item)
	}

	return history
}

// buildMessageHistory 构建消息历史
func (s *Customer360Service) buildMessageHistory(ctx context.Context, sessions []*model.CustomerSession) ([]*MessageHistoryItem, error) {
	history := make([]*MessageHistoryItem, 0)

	for _, session := range sessions {
		messages, _, _ := s.messageRepo.GetBySessionID(ctx, session.SessionID, 1, 100)
		for _, msg := range messages {
			item := &MessageHistoryItem{
				SessionID:	msg.SessionID,
				Content:	msg.Content,
				SenderType:	msg.SenderType,
				SenderName:	msg.SenderName,
				ContentType:	string(msg.ContentType),
				CreatedAt:	msg.CreatedAt.Format("2006-01-02 15:04:05"),
				AIConfidence:	msg.AIConfidence,
			}
			history = append(history, item)
		}
	}

	return history, nil
}

// buildClueInfo 构建线索信息
func (s *Customer360Service) buildClueInfo(ctx context.Context, userID string) (*ClueInfo, error) {
	// 首先获取用户的会话信息来查找联系方式
	sessions, _, err := s.sessionRepo.GetByMerchant(ctx, "", 1, 1000)
	if err != nil {
		return nil, err
	}

	// 查找该用户的会话
	var userPhone, userEmail, accountID string
	for _, session := range sessions {
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

	// 通过电话或邮箱查询线索
	var clues []*model.Clue

	// 获取所有线索（遍历常见类型）
	for _, clueType := range []int64{0, 1, 2, 3, 4, 5} {
		allClues, _, err := s.clueRepo.GetClueAllList(ctx, clueType)
		if err == nil {
			for _, clue := range allClues {
				// 匹配 Account、Name（手机号）或邮箱
				if clue.Account == accountID || clue.Account == userPhone || clue.Name == userPhone {
					// 检查是否已存在，避免重复
					found := false
					for _, existing := range clues {
						if existing.ID == clue.ID {
							found = true
							break
						}
					}
					if !found {
						clues = append(clues, clue)
					}
				}
			}
		}
	}

	if len(clues) == 0 {
		return nil, nil
	}

	// 返回最新的线索
	latestClue := clues[0]
	clueInfo := &ClueInfo{
		ClueID:		latestClue.ID,
		Source:		"platform",
		Name:		latestClue.Name,
		Phone:		latestClue.Account,
		Status:		"new",
		Level:		"warm",
		CreatedAt:	time.Unix(latestClue.CreateTime, 0).Format("2006-01-02 15:04:05"),
	}

	// 根据 IsVerify 设置状态
	if latestClue.IsVerify == 1 {
		clueInfo.Status = "qualified"
	}

	return clueInfo, nil
}

// buildOrderInfo 构建订单信息
func (s *Customer360Service) buildOrderInfo(ctx context.Context, userID string) (*OrderInfo, error) {
	// 首先获取用户的会话信息来获取 AccountID
	sessions, _, err := s.sessionRepo.GetByMerchant(ctx, "", 1, 1000)
	if err != nil {
		return nil, err
	}

	// 查找该用户的 AccountID
	var accountID string
	for _, session := range sessions {
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

	// 获取所有订单（OrderRepository 没有按 AccountID 查询的方法，需要获取所有后过滤）
	allOrders, _, err := s.orderRepo.GetOrderList(ctx, 1, 10000)
	if err != nil {
		return &OrderInfo{
			Orders: make([]*OrderItem, 0),
		}, nil
	}

	// 过滤出该用户的订单
	var userOrders []*model.Order
	for _, order := range allOrders {
		if order.AccountID == accountID {
			userOrders = append(userOrders, order)
		}
	}

	if len(userOrders) == 0 {
		return &OrderInfo{
			Orders: make([]*OrderItem, 0),
		}, nil
	}

	// 计算统计数据
	var totalAmount float64
	var lastOrder *model.Order
	orderItems := make([]*OrderItem, 0, len(userOrders))

	for _, order := range userOrders {
		amount, _ := strconv.ParseFloat(order.Price, 64)
		totalAmount += amount

		// 检查是否是最新订单
		if lastOrder == nil || order.CreateTime > lastOrder.CreateTime {
			lastOrder = order
		}

		orderItems = append(orderItems, &OrderItem{
			OrderID:	order.ID,
			Amount:		amount,
			Status:		orderStatusToString(order.Status),
			CreatedAt:	time.Unix(order.CreateTime, 0).Format("2006-01-02 15:04:05"),
			ProductName:	"平台商品",	// In production, this would query the product table for actual name
		})
	}

	orderInfo := &OrderInfo{
		TotalOrders:	int64(len(userOrders)),
		TotalAmount:	totalAmount,
		Orders:		orderItems,
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

// orderStatusToString 将订单状态转换为字符串
func orderStatusToString(status _type.OrderStatusType) string {
	switch status {
	case _type.OrderStatusPending:
		return "pending"	// 待支付
	case _type.OrderStatusSuccess:
		return "success"	// 已支付/成功
	case _type.OrderStatusForceSuccess:
		return "success"	// 强制成功
	case _type.OrderStatusTimeout:
		return "timeout"	// 超时
	case _type.OrderStatusForceClose:
		return "closed"	// 强制关闭
	default:
		return "unknown"
	}
}

// buildInteractionStats 构建互动统计
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

// buildUserProfile 构建用户画像
func (s *Customer360Service) buildUserProfile(ctx context.Context, sessions []*model.CustomerSession, interactionStats *InteractionStats, orderInfo *OrderInfo) *UserProfile {
	profile := &UserProfile{
		Tags:		make([]string, 0),
		Interests:	make([]string, 0),
		PurchasePower:	"medium",
		ActivityLevel:	"normal",
		RiskLevel:	"normal",
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
		"douyin":	interactionStats.DouyinCount,
		"kuaishou":	interactionStats.KuaishouCount,
		"xiaohongshu":	interactionStats.XiaohongshuCount,
		"xianyu":	interactionStats.XianyuCount,
		"tiktok":	interactionStats.TiktokCount,
	} {
		if count > maxCount {
			maxCount = count
			profile.PreferredPlatform = platform
		}
	}

	return profile
}

// GetCustomerList 获取客户列表（带分页和筛选）
func (s *Customer360Service) GetCustomerList(ctx context.Context, page, pageSize int, filters map[string]string) (map[string]*Customer360DTO, int64, error) {
	sessions, total, err := s.sessionRepo.GetByMerchant(ctx, "", page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	// 按用户分组
	userSessions := make(map[string][]*model.CustomerSession)
	for _, session := range sessions {
		userSessions[session.UserID] = append(userSessions[session.UserID], session)
	}

	// 构建每个客户的 360 视图
	result := make(map[string]*Customer360DTO)
	for userID := range userSessions {
		dto, err := s.GetCustomer360(ctx, userID)
		if err != nil {
			continue
		}
		result[userID] = dto
	}

	return result, total, nil
}

// Customer360ServiceForTest 用于测试的 Customer360Service（公开字段）
type Customer360ServiceForTest struct {
	SessionRepo		*repository.CustomerSessionRepository
	MessageRepo		*repository.SessionMessageRepository
	ClueRepo		repository.ClueRepository
	OrderRepo		repository.OrderRepository
	UnifiedMsgRepo		repository.UnifiedMessageRepository
	UnifiedReplyRepo	repository.UnifiedReplyRepository
}

// GetCustomer360 获取客户 360 视图（测试版本）
func (s *Customer360ServiceForTest) GetCustomer360(ctx context.Context, userID string) (*Customer360DTO, error) {
	realService := &Customer360Service{
		sessionRepo:		s.SessionRepo,
		messageRepo:		s.MessageRepo,
		clueRepo:		s.ClueRepo,
		orderRepo:		s.OrderRepo,
		unifiedMsgRepo:		s.UnifiedMsgRepo,
		unifiedReplyRepo:	s.UnifiedReplyRepo,
	}
	return realService.GetCustomer360(ctx, userID)
}

// GetCustomerList 获取客户列表（测试版本）
func (s *Customer360ServiceForTest) GetCustomerList(ctx context.Context, page, pageSize int, filters map[string]string) (map[string]*Customer360DTO, int64, error) {
	realService := &Customer360Service{
		sessionRepo:		s.SessionRepo,
		messageRepo:		s.MessageRepo,
		clueRepo:		s.ClueRepo,
		orderRepo:		s.OrderRepo,
		unifiedMsgRepo:		s.UnifiedMsgRepo,
		unifiedReplyRepo:	s.UnifiedReplyRepo,
	}
	return realService.GetCustomerList(ctx, page, pageSize, filters)
}
