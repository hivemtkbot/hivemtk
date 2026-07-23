package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"marketing/internal/websocket"
)

// VisitorChatService 访客端客服会话服务（ADR-010）
//
// 职责：
//   - 接收访客（匿名）通过 Web Widget 发送的消息
//   - 查找/创建会话（基于 visitor_id + channel_id）
//   - 调用 SmartCSOrchestrator 走 RAG + AI 决策 + 人工接管
//   - 通过 WebSocket 推送 AI/坐席消息给访客
//   - 提供历史消息、关闭会话、评分等访客侧 API
//
// 与 SmartCSOrchestrator.HandleIncomingWithAgent 的关系：
//   - 薄包装：构造 IncomingContext，复用已有的 9 步编排
//   - 差异：Platform=WebEmbed / AccountID=channel_id / SenderID=visitor_id
type VisitorChatService struct {
	db              *gorm.DB
	channelSvc      *ChatChannelService
	orchestrator    *SmartCSOrchestrator
	sessionSvc      *CustomerSessionService
	sessionRepo     *repository.CustomerSessionRepository
	messageRepo     *repository.SessionMessageRepository
	customerRepo    repository.CustomerRepository
	agentBindingSvc *ChannelAgentBindingService
	inboxConvRepo   *repository.InboxConversationRepository
}

// NewVisitorChatService 构造访客会话服务
// agentBindingSvc 可为 nil（测试/降级场景）：nil 时网页客服回退默认编排（与历史行为一致）
func NewVisitorChatService(ctx context.Context, db *gorm.DB, channelSvc *ChatChannelService, orchestrator *SmartCSOrchestrator, agentBindingSvc *ChannelAgentBindingService) *VisitorChatService {
	if db == nil {
		db = repository.NewCustomerSessionRepository().GetDB(ctx)
	}
	return &VisitorChatService{
		db:              db,
		channelSvc:      channelSvc,
		orchestrator:    orchestrator,
		sessionSvc:      NewCustomerSessionService(),
		sessionRepo:     repository.NewCustomerSessionRepository(),
		messageRepo:     repository.NewSessionMessageRepository(),
		customerRepo:    repository.NewCustomerRepository(),
		agentBindingSvc: agentBindingSvc,
		inboxConvRepo:   newInboxConversationRepo(db),
	}
}

// newInboxConversationRepo 构造收件箱会话仓储（绑定当前 db，缺失时回退全局默认库）
//
// 仓储初始化绑定 DB 的过程是同步轻量操作,使用 background ctx 隔离
func newInboxConversationRepo(db *gorm.DB) *repository.InboxConversationRepository {
	repo := repository.NewInboxConversationRepository()
	if db != nil {
		repo.SetDB(db)
	}
	return repo
}

// ensureVisitorCustomer 访客首次发消息时自动建档（CDP 客户中心）。
// 以 unified_id = "visitor:<channel>:<visitor>" 去重，保证 customers 表随真实访客对话增长。
func (s *VisitorChatService) ensureVisitorCustomer(ctx context.Context, req *VisitorSendMessageRequest, session *model.CustomerSession) error {
	_ = ctx
	if s.customerRepo == nil {
		return nil
	}
	unifiedID := "visitor:" + req.ChannelID + ":" + req.VisitorID
	if existing, _ := s.customerRepo.GetByUnifiedID(ctx, unifiedID); existing != nil {
		return nil
	}
	cust := &model.Customer{
		UnifiedID: unifiedID,
		Tags:      "[\"web_visitor\"]",
	}
	if err := s.customerRepo.Create(ctx, cust); err != nil {
		return fmt.Errorf("建档失败: %w", err)
	}
	return nil
}

// SetOrchestrator 注入编排器（用于循环依赖解耦）
func (s *VisitorChatService) SetOrchestrator(ctx context.Context, o *SmartCSOrchestrator) {
	s.orchestrator = o
}

// ============================================================================
// 会话管理
// ============================================================================

// VisitorOpenSessionRequest 访客打开会话请求
//
// 注意（2026-07-18 私域部署修复）：ChannelID/VisitorID 不再用 binding:"required"。
// 原因：私域部署模式下，channel_id 由中间件 AppKeyResolve 从 X-Chat-Channel-Id header
// 或 X-Chat-App-Key 反查注入 ctx；visitor_id 从 X-Chat-Visitor-Id header 注入。
// controller 层 OpenSession 会在 binding 之后做软解析兜底，所以 binding 不能 required。
// 保留 json 字段以便兼容显式传 body 的调用方。
type VisitorOpenSessionRequest struct {
	ChannelID    string `json:"channel_id"`
	VisitorID    string `json:"visitor_id"`
	VisitorName  string `json:"visitor_name"`
	VisitorPhone string `json:"visitor_phone"`
	VisitorEmail string `json:"visitor_email"`
	VisitorMeta  string `json:"visitor_meta"` // JSON 字符串：来源页面、UA 等
	// Resume 控制：true=续接最近未结束会话；false=总是创建新会话
	Resume bool `json:"resume"`
}

// VisitorOpenSessionResult 访客打开会话结果
type VisitorOpenSessionResult struct {
	Session        *model.CustomerSession `json:"session"`
	IsNewSession   bool                   `json:"is_new_session"`
	OnlineAgentNum int                    `json:"online_agent_num"`
	WelcomeMessage string                 `json:"welcome_message"`
}

// resolveChannel 解析 channel（支持 channel_id / app_key / "default"）
//
// 私域部署（2026-07-21 修复）：前端嵌入路由 /chat/embed/:channel_ref 把 app_key
// 作为 path 参数透传过来，effectiveChannelId 收到的实际值可能是 channel_id 或
// app_key。这里按"先按 channel_id 查 → 再按 app_key 查 → 最后 default 兜底"
// 的顺序兼容三种取值，避免前端传 app_key 时报"渠道不存在"。
func (s *VisitorChatService) resolveChannel(ctx context.Context, channelRef string) (*model.ChatChannel, error) {
	ref := strings.TrimSpace(channelRef)
	if ref == "" {
		// 缺失时使用 default
		return s.channelSvc.GetOrCreateDefaultChannel(ctx)
	}
	if channel, err := s.channelSvc.GetByChannelID(ctx, ref); err == nil {
		return channel, nil
	} else if !strings.Contains(err.Error(), "渠道不存在") && !strings.Contains(err.Error(), "channel_id 不能为空") {
		// 非"不存在"错误：透传
		return nil, err
	}
	if channel, err := s.channelSvc.GetByAppKey(ctx, ref); err == nil {
		return channel, nil
	} else if !strings.Contains(err.Error(), "AppKey 无效") {
		// 非"无效"错误：透传
		return nil, err
	}
	if ref == "default" {
		return s.channelSvc.GetOrCreateDefaultChannel(ctx)
	}
	// 2026-07-21: 抖音/快手/小红书/咸鱼 4 平台卡片渠道自动创建
	// channel_ref 形如 "douyin_card" / "kuaishou_card" / "xiaohongshu_card" / "xianyu_card"
	if platform, ok := IsCardChannelRef(ref); ok {
		return s.channelSvc.GetOrCreateCardChannel(ctx, platform)
	}
	return nil, fmt.Errorf("渠道不存在: %s", ref)
}

// OpenSession 打开（或续接）会话
func (s *VisitorChatService) OpenSession(ctx context.Context, req *VisitorOpenSessionRequest) (*VisitorOpenSessionResult, error) {
	if req == nil {
		return nil, errors.New("请求不能为空")
	}
	if strings.TrimSpace(req.ChannelID) == "" || strings.TrimSpace(req.VisitorID) == "" {
		return nil, errors.New("channel_id 和 visitor_id 必填")
	}

	// 1. 查询渠道
	//    私域部署（2026-07-17）：channel_id == "default" 时，若 DB 不存在则自动创建
	//    其他 channel_id 仍按正常流程（必须先在管理后台创建）
	channel, err := s.resolveChannel(ctx, req.ChannelID)
	if err != nil {
		return nil, err
	}
	if !ChatChannelIsActive(channel) {
		return nil, errors.New("渠道已禁用")
	}

	// 2. 续接最近会话
	if req.Resume {
		var existing model.CustomerSession
		err := s.db.WithContext(ctx).Where("platform = ? AND account_id = ? AND user_id = ?",
			model.PlatformWebEmbed, channel.ChannelID, req.VisitorID).
			Where("status NOT IN ?", []model.SessionStatus{
				model.SessionStatusResolved,
				model.SessionStatusClosed,
			}).
			Order("last_message_at DESC NULLS LAST, created_at DESC").
			First(&existing).Error
		if err == nil {
			// 找到未结束会话，复用
			online, _ := s.countOnlineAgents(ctx)
			return &VisitorOpenSessionResult{
				Session:        &existing,
				IsNewSession:   false,
				OnlineAgentNum: online,
				WelcomeMessage: channel.WelcomeMessage,
			}, nil
		}
		// gorm.ErrRecordNotFound 时继续创建
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("查询历史会话失败: %w", err)
		}
	}

	// 3. 创建新会话
	//    2026-07-21 卡片客服：解析 visitor_meta 中的 source/card_id，
	//    作为来源标签追加到 session.Tags（JSON 数组），便于客服工作台按来源筛选
	sessionTags := []string{"web_visitor"}
	if meta := strings.TrimSpace(req.VisitorMeta); meta != "" {
		var mv map[string]string
		if err := json.Unmarshal([]byte(meta), &mv); err == nil {
			if src := strings.TrimSpace(mv["source"]); src != "" {
				sessionTags = append(sessionTags, "source:"+src)
			}
			if cid := strings.TrimSpace(mv["card_id"]); cid != "" {
				sessionTags = append(sessionTags, "card_id:"+cid)
			}
		}
	}
	tagsJSON, _ := json.Marshal(sessionTags)
	session := &model.CustomerSession{
		SessionID:   generateSessionID(),
		Platform:    model.PlatformWebEmbed,
		AccountID:   channel.ChannelID, // AccountID 存 channel_id
		UserID:      req.VisitorID,
		UserName:    defaultIfEmpty(req.VisitorName, "访客"),
		UserPhone:   req.VisitorPhone,
		UserEmail:   req.VisitorEmail,
		Status:      model.SessionStatusPending,
		HandlerType: model.HandlerTypeAI, // 默认 AI
		Priority:    0,
		Tags:        string(tagsJSON),
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("创建会话失败: %w", err)
	}

	// 4. 累计 channel 计数
	_ = s.channelSvc.IncrementVisitorCount(ctx, channel.ChannelID)
	_ = s.channelSvc.IncrementSessionCount(ctx, channel.ChannelID)

	// 5. 推送新会话通知到所有在线坐席
	online, _ := s.countOnlineAgents(ctx)
	if online > 0 {
		// 广播给所有坐席客户端（不广播给访客，避免干扰）
		_ = websocket.BroadcastToAgents(websocket.TypeNewSession, map[string]any{
			"session": session,
			"channel": map[string]any{
				"channel_id":   channel.ChannelID,
				"channel_name": channel.ChannelName,
			},
		})
	}

	_ = session
	return &VisitorOpenSessionResult{
		Session:        session,
		IsNewSession:   true,
		OnlineAgentNum: online,
		WelcomeMessage: channel.WelcomeMessage,
	}, nil
}

// GetSessionByVisitorSessionID 访客通过 session_id 获取会话（仅校验归属）
//
// 2026-07-21 修复：channelID 入参既可能是 channel_id 也可能是 app_key
// （前端 embed 路由把 app_key 作为 path 透传）。这里把入参归一化为 channel_id 后再查。
func (s *VisitorChatService) GetSessionByVisitorSessionID(ctx context.Context, channelID, visitorID, sessionID string) (*model.CustomerSession, error) {
	channel, err := s.resolveChannel(ctx, channelID)
	if err != nil {
		return nil, errors.New("会话不存在或无权访问")
	}
	var session model.CustomerSession
	err = s.db.WithContext(ctx).Where("session_id = ? AND platform = ? AND account_id = ? AND user_id = ?",
		sessionID, model.PlatformWebEmbed, channel.ChannelID, visitorID).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("会话不存在或无权访问")
		}
		return nil, err
	}
	return &session, nil
}

// ============================================================================
// 消息收发
// ============================================================================

// VisitorSendMessageRequest 访客发送消息请求
type VisitorSendMessageRequest struct {
	ChannelID   string `json:"channel_id" binding:"required"`
	VisitorID   string `json:"visitor_id" binding:"required"`
	SessionID   string `json:"session_id" binding:"required"`
	Content     string `json:"content" binding:"required"`
	ContentType string `json:"content_type"`
	// 2026-07-17: 附件支持（走七牛直传）
	//   - MediaURL: 访客上传到七牛后获得的 CDN URL
	//   - MediaType: image / file / audio / video
	//   - MediaName: 原始文件名
	//   - MediaSize: 文件大小（字节）
	MediaURL  string `json:"media_url"`
	MediaType string `json:"media_type"`
	MediaName string `json:"media_name"`
	MediaSize int64  `json:"media_size"`
}

// VisitorSendMessageResult 访客发送消息结果
type VisitorSendMessageResult struct {
	UserMessage    *model.SessionMessage `json:"user_message"`
	AIReplied      bool                  `json:"ai_replied"`
	AIResponse     *model.SessionMessage `json:"ai_response,omitempty"`
	Transferred    bool                  `json:"transferred"`
	TransferReason string                `json:"transfer_reason,omitempty"`
	Confidence     float64               `json:"confidence"`
	HandlerType    string                `json:"handler_type"`
	SuggestionID   uint                  `json:"suggestion_id,omitempty"`
}

// SendMessage 访客发送消息（核心入口）
func (s *VisitorChatService) SendMessage(ctx context.Context, req *VisitorSendMessageRequest) (*VisitorSendMessageResult, error) {
	if req == nil {
		return nil, errors.New("请求不能为空")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, errors.New("消息内容不能为空")
	}

	// 1. 校验会话归属
	session, err := s.GetSessionByVisitorSessionID(ctx, req.ChannelID, req.VisitorID, req.SessionID)
	if err != nil {
		return nil, err
	}

	// 2. 校验渠道
	channel, err := s.resolveChannel(ctx, req.ChannelID)
	if err != nil {
		return nil, err
	}

	// 3. 落库访客消息
	contentType := model.MessageTypeText
	if req.ContentType != "" {
		contentType = model.MessageType(req.ContentType)
	}
	// 2026-07-17: 附件消息（image/file/audio/video）
	if req.MediaURL != "" {
		switch req.MediaType {
		case "image":
			contentType = model.MessageTypeImage
		case "file":
			contentType = model.MessageTypeFile
		case "audio":
			contentType = model.MessageTypeAudio
		case "video":
			contentType = model.MessageTypeVideo
		}
	}
	userMsg := &model.SessionMessage{
		SessionID:   session.SessionID,
		Content:     req.Content,
		ContentType: contentType,
		MediaURL:    req.MediaURL,
		SenderType:  "user",
		SenderID:    req.VisitorID,
		SenderName:  session.UserName,
	}
	if err := s.messageRepo.Create(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("保存访客消息失败: %w", err)
	}
	_ = s.sessionRepo.UpdateLastMessage(ctx, session.ID, req.Content, "user")

	// 3.5 访客首次发消息自动建档（CDP 客户中心）：保证 customers 表随真实访客对话增长
	if err := s.ensureVisitorCustomer(ctx, req, session); err != nil {
		logger.Warnf("[Visitor] 访客建档失败（不影响消息发送）: %v", err)
	}

	// 3.6 同步到统一收件箱（商户坐席可见）：此前 web_embed 会话只写 customer_sessions，
	// 未写入 inbox_conversations，导致商户「统一收件箱」看不到网页客服消息、核心链路断点。
	// 这里补齐同步（与 WebhookService.upsertInboxFromHub 保持一致的幂等 upsert 语义）。
	s.syncToInbox(ctx, session, req.Content)

	// 4. 通过 WebSocket 实时推送给坐席
	if session.AgentID > 0 {
		_ = websocket.SendToAgent(websocket.TypeNewMessage, map[string]any{
			"session_id": session.SessionID,
			"message":    userMsg,
		}, session.AgentID)
	}

	// 5. 调用 SmartCSOrchestrator
	if s.orchestrator == nil {
		// 编排器未注入，返回降级结果（仅消息已保存）
		return &VisitorSendMessageResult{
			UserMessage: userMsg,
			HandlerType: string(session.HandlerType),
		}, nil
	}

	// 5.1 NLP 关键词自动转人工（2026-07-17）
	//   前端不再暴露"转人工"按钮。对用户永远显示"在线客服"。
	//   后端基于关键词命中（"人工"/"真人"/"转人工" 等）自动触发转人工流程。
	//   命中后：
	//     - 跳过 orchestrator AI 推理
	//     - 直接走自动分配（session.AutoAssign）
	//     - 推送 agent_joined WebSocket 通知给访客
	//     - 广播给所有坐席
	if shouldForceTransferByKeywords(req.Content) {
		// 1) 自动分配（如果失败则标记为 waiting）
		if err := s.sessionSvc.AutoAssign(ctx, session.ID); err != nil {
			_ = s.sessionRepo.UpdateStatus(ctx, session.ID, model.SessionStatusWaiting)
			// 2026-07-17: 分配失败时也要更新 handler_type 为 human
			//   之前只改 status，session.handler_type 仍是 "ai"，导致前端列表展示错误
			_ = s.db.WithContext(ctx).Model(&model.CustomerSession{}).
				Where("id = ?", session.ID).
				Update("handler_type", model.HandlerTypeHuman).Error
		}
		// 2) 推送新会话 / 转人工通知给坐席
		_ = websocket.BroadcastToAll(websocket.TypeNewSession, map[string]any{
			"session":    session,
			"need_human": true,
			"reason":     "关键词命中自动转人工：" + req.Content,
		})
		// 3) 推送 agent_joined 给访客（访客侧会显示"客服正在接入..."）
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
			"session_id": session.SessionID,
			"handler":    "human",
			"reason":     "正在为您接入人工客服，请稍候...",
		}, session.SessionID)
		// 4) 系统消息落库
		sysMsg := &model.SessionMessage{
			SessionID:  session.SessionID,
			Content:    "【系统】访客请求人工客服，正在为您接入...",
			SenderType: "system",
			SenderID:   "system",
			SenderName: "系统",
		}
		_ = s.messageRepo.Create(ctx, sysMsg)
		return &VisitorSendMessageResult{
			UserMessage:    userMsg,
			AIReplied:      false,
			Transferred:    true,
			TransferReason: "关键词命中自动转人工",
			HandlerType:    string(model.HandlerTypeHuman),
		}, nil
	}

	// 5.2 通知访客「AI 正在思考」，避免 RAG+推理耗时期间用户以为卡死
	// 配合前端 typing 指示器；defer 确保正常返回与编排出错提前返回时都会清除
	_ = websocket.SendToVisitor(websocket.TypeAITyping, map[string]any{"typing": true}, session.SessionID)
	defer websocket.SendToVisitor(websocket.TypeAITyping, map[string]any{"typing": false}, session.SessionID)

	in := &IncomingContext{
		Platform:   model.PlatformWebEmbed,
		AccountID:  channel.ChannelID,
		SenderID:   req.VisitorID,
		SenderName: session.UserName,
		Content:    req.Content,
		MessageID:  strconv.FormatUint(uint64(userMsg.ID), 10),
	}
	// 多 AI 智能体路由：网页客服与 webhook 保持一致，按 (渠道类型, 渠道账号) 加载绑定的智能体上下文。
	// 这样「网页客服 AI 自动回复」才会真正接入 SmartCSOrchestrator（默认行为需绑定智能体，否则回退人工）。
	var agentCtxFromChannel *AgentContext
	if s.agentBindingSvc != nil {
		agentCtxFromChannel, _ = s.agentBindingSvc.LoadAgentForChannel(ctx, NormalizeChannelType(string(model.PlatformWebEmbed)), channel.ChannelID)
	}
	handleResult, err := s.orchestrator.HandleIncomingWithAgent(ctx, in, agentCtxFromChannel)
	if err != nil {
		// 编排失败不影响访客消息已保存
		return &VisitorSendMessageResult{
			UserMessage: userMsg,
			HandlerType: string(model.HandlerTypeAI),
		}, nil
	}

	result := &VisitorSendMessageResult{
		UserMessage:    userMsg,
		AIReplied:      handleResult.AIReplied,
		Confidence:     handleResult.Confidence,
		Transferred:    handleResult.Transferred,
		TransferReason: handleResult.TransferReason,
		HandlerType:    string(handleResult.HandlerType),
		SuggestionID:   handleResult.SuggestionID,
	}

	// 6. 推送给访客的 WebSocket
	notifyPayload := map[string]any{
		"session_id":  session.SessionID,
		"handler":     handleResult.HandlerType,
		"transferred": handleResult.Transferred,
	}
	if handleResult.AIReplied && handleResult.Reply != "" {
		// 落库 AI 回复（带 delivered_at=now，标记已通过 HTTP 投递给访客，避免离线消息重发）
		//
		// sender_id 必须填 "ai_assistant"：与 orchestrator.saveOutboundMessage 的 sender_id 保持一致，
		// 这样 5 秒内的去重检查才能匹配，visitor 端 + orchestrator 不会双保存。
		now := time.Now()
		aiMsg := &model.SessionMessage{
			SessionID:    session.SessionID,
			Content:      handleResult.Reply,
			ContentType:  model.MessageTypeText,
			SenderType:   "ai",
			SenderID:     "ai_assistant",
			SenderName:   "智能助手",
			AIConfidence: handleResult.Confidence,
			AISource:     "rag",
			DeliveredAt:  &now,
		}
		_ = s.messageRepo.Create(ctx, aiMsg)
		_ = s.sessionRepo.UpdateLastMessage(ctx, session.ID, handleResult.Reply, "ai")
		_ = s.sessionRepo.IncrementAIReplyCount(ctx, session.ID)
		result.AIResponse = aiMsg

		// 不再通过 WebSocket 推送 AI 回复给访客：
		//   - HTTP 响应已带 ai_response，前端可立即渲染
		//   - 重复推送会导致访客侧 UI 出现两条相同 AI 消息
		//   - 如访客已离线，AI 消息已落库且 delivered_at 已置，
		//     重连时 offline-messages 接口会过滤掉 delivered_at NOT NULL
	}

	if handleResult.Transferred {
		// 转人工通知仍走 WebSocket（HTTP 响应只返回 transfer flag，system 消息由 ws 推）
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, notifyPayload, session.SessionID)
	}

	// 7. 推送会话状态变化给坐席
	if session.AgentID > 0 {
		_ = websocket.SendToAgent(websocket.TypeSessionUpdate, map[string]any{
			"session_id":   session.SessionID,
			"handler_type": handleResult.HandlerType,
			"transferred":  handleResult.Transferred,
		}, session.AgentID)
	}

	_ = time.Now
	return result, nil
}

// syncToInbox 将网页客服会话同步到统一收件箱（inbox_conversations），
// 使商户坐席能在「统一收件箱」看到并回复访客消息。
// 与 WebhookService.upsertInboxFromHub 保持一致的幂等 upsert 语义：
//   - 同一 (platform, account_id, customer_id) 已存在则仅更新最后消息 + 未读 +1
//   - 不存在则新建一条 unread 会话
func (s *VisitorChatService) syncToInbox(ctx context.Context, session *model.CustomerSession, content string) {
	if s.inboxConvRepo == nil {
		s.inboxConvRepo = newInboxConversationRepo(s.db)
	}
	if s.inboxConvRepo == nil {
		return
	}
	now := time.Now()
	conv, err := s.inboxConvRepo.FindByPlatformAccountCustomer(ctx, string(model.PlatformWebEmbed), session.AccountID, session.UserID)
	if err == nil && conv != nil {
		_ = s.inboxConvRepo.UpdateLastMessage(ctx, conv.ID, content, now, 1)
		return
	}
	newConv := &model.InboxConversation{
		Platform:           string(model.PlatformWebEmbed),
		AccountID:          session.AccountID,
		CustomerID:         session.UserID,
		CustomerName:       session.UserName,
		ConversationID:     session.SessionID,
		LastMessagePreview: content,
		LastMessageAt:      &now,
		UnreadCount:        1,
		TotalCount:         1,
		Status:             "unread",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_ = s.inboxConvRepo.Create(ctx, newConv)
}

// ============================================================================
// 离线消息 / 历史
// ============================================================================

// GetMessages 访客获取历史消息
func (s *VisitorChatService) GetMessages(ctx context.Context, channelID, visitorID, sessionID string, page, pageSize int) ([]*model.SessionMessage, int64, error) {
	if _, err := s.GetSessionByVisitorSessionID(ctx, channelID, visitorID, sessionID); err != nil {
		return nil, 0, err
	}
	return s.messageRepo.GetBySessionID(ctx, sessionID, page, pageSize)
}

// GetLatestActiveSession 获取访客最近一次未结束会话（用于离线消息续接）
//
// 2026-07-21 修复：channelID 入参既可能是 channel_id 也可能是 app_key，
// 这里统一通过 resolveChannel 归一化为 channel_id 后再查询。
func (s *VisitorChatService) GetLatestActiveSession(ctx context.Context, channelID, visitorID string) (*model.CustomerSession, error) {
	channel, err := s.resolveChannel(ctx, channelID)
	if err != nil {
		return nil, nil // 渠道不存在视为无活跃会话
	}
	var session model.CustomerSession
	err = s.db.WithContext(ctx).Where("platform = ? AND account_id = ? AND user_id = ?",
		model.PlatformWebEmbed, channel.ChannelID, visitorID).
		Where("status NOT IN ?", []model.SessionStatus{
			model.SessionStatusResolved,
			model.SessionStatusClosed,
		}).
		Order("last_message_at DESC NULLS LAST, created_at DESC").
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 无活跃会话不视为错误
		}
		return nil, err
	}
	return &session, nil
}

// GetRecentClosedSessions 获取访客最近 7 天已结束会话（离线消息列表）
//
// 2026-07-21 修复：channelID 入参既可能是 channel_id 也可能是 app_key。
func (s *VisitorChatService) GetRecentClosedSessions(ctx context.Context, channelID, visitorID string, limit int) ([]*model.CustomerSession, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	channel, err := s.resolveChannel(ctx, channelID)
	if err != nil {
		return []*model.CustomerSession{}, nil // 渠道不存在视为无历史
	}
	var sessions []*model.CustomerSession
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	err = s.db.WithContext(ctx).Where("platform = ? AND account_id = ? AND user_id = ?",
		model.PlatformWebEmbed, channel.ChannelID, visitorID).
		Where("status IN ?", []model.SessionStatus{
			model.SessionStatusResolved,
			model.SessionStatusClosed,
		}).
		Where("created_at > ?", sevenDaysAgo).
		Order("created_at DESC").
		Limit(limit).
		Find(&sessions).Error
	return sessions, err
}

// GetOfflineMessages 拉取访客离线期间未投递的坐席/AI 回复消息
//
// 离线消息定义：sender_type IN ('ai', 'agent') AND delivered_at IS NULL
// 用户自己发的消息不算"离线消息"（用户自己当然知道）
//
// 调用方应在拉取后调用 MarkMessagesDelivered 标记为已投递，避免重复拉取。
func (s *VisitorChatService) GetOfflineMessages(ctx context.Context, channelID, visitorID, sessionID string) ([]*model.SessionMessage, error) {
	if _, err := s.GetSessionByVisitorSessionID(ctx, channelID, visitorID, sessionID); err != nil {
		return nil, err
	}

	var messages []*model.SessionMessage
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).
		Where("sender_type IN ?", []string{"ai", "agent"}).
		Where("delivered_at IS NULL").
		Order("created_at ASC").
		Find(&messages).Error
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// MarkMessagesDelivered 批量标记消息已投递（WebSocket 拉到离线消息后调用）
func (s *VisitorChatService) MarkMessagesDelivered(ctx context.Context, sessionID string, messageIDs []uint) error {
	if len(messageIDs) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.WithContext(ctx).Model(&model.SessionMessage{}).
		Where("session_id = ? AND id IN ?", sessionID, messageIDs).
		Update("delivered_at", &now).Error
}

// ============================================================================
// 访客主动操作
// ============================================================================

// RequestHumanTransfer 访客主动转人工
func (s *VisitorChatService) RequestHumanTransfer(ctx context.Context, channelID, visitorID, sessionID, reason string) error {
	session, err := s.GetSessionByVisitorSessionID(ctx, channelID, visitorID, sessionID)
	if err != nil {
		return err
	}

	// 落库访客消息
	transferMsg := &model.SessionMessage{
		SessionID:  session.SessionID,
		Content:    "【访客请求转人工】" + defaultIfEmpty(reason, ""),
		SenderType: "user",
		SenderID:   visitorID,
		SenderName: session.UserName,
	}
	_ = s.messageRepo.Create(ctx, transferMsg)

	// 尝试自动分配
	if err := s.sessionSvc.AutoAssign(ctx, session.ID); err != nil {
		// 无在线坐席，标记待人工
		_ = s.sessionRepo.UpdateStatus(ctx, session.ID, model.SessionStatusWaiting)
	}

	// 通知所有坐席
	_ = websocket.BroadcastToAll(websocket.TypeNewSession, map[string]any{
		"session":    session,
		"need_human": true,
		"reason":     reason,
	})

	// 通知访客
	_ = websocket.SendToVisitor(websocket.TypeMessage, map[string]any{
		"session_id": session.SessionID,
		"system_msg": "正在为您转接人工客服，请稍候...",
	}, session.SessionID)

	return nil
}

// CloseSession 访客主动关闭会话
func (s *VisitorChatService) CloseSession(ctx context.Context, channelID, visitorID, sessionID string) error {
	session, err := s.GetSessionByVisitorSessionID(ctx, channelID, visitorID, sessionID)
	if err != nil {
		return err
	}
	return s.sessionSvc.UpdateSessionStatus(ctx, session.ID, model.SessionStatusClosed)
}

// RateSession 访客评分
func (s *VisitorChatService) RateSession(ctx context.Context, channelID, visitorID, sessionID string, rating int, comment string) error {
	session, err := s.GetSessionByVisitorSessionID(ctx, channelID, visitorID, sessionID)
	if err != nil {
		return err
	}
	if rating < 1 || rating > 5 {
		return errors.New("评分必须在 1-5 之间")
	}
	return s.sessionSvc.RateSession(ctx, session.ID, rating, comment)
}

// ============================================================================
// 辅助方法
// ============================================================================

// countOnlineAgents 统计在线坐席数
func (s *VisitorChatService) countOnlineAgents(ctx context.Context) (int, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.AgentStatus{}).
		Where("status IN ?", []string{"online", "busy"}).
		Count(&count).Error
	return int(count), err
}

// CountAvailableAgents 公开方法：可用坐席数
func (s *VisitorChatService) CountAvailableAgents(ctx context.Context) (int, error) {
	return s.countOnlineAgents(ctx)
}

// ============================================================================
// NLP 关键词自动转人工（2026-07-17）
// ============================================================================

// transferKeywords 触发自动转人工的关键词（私域部署基线）
// 1) 与 config.yaml `chat.transfer_keywords` 同步
// 2) 兜底：当 config 不可用时使用以下默认值
// 3) 中文/英文双语覆盖
var transferKeywords = []string{
	"人工", "真人", "转人工", "找人", "客服", "operator", "human", "agent",
}

// shouldForceTransferByKeywords 判断用户消息是否命中"转人工"关键词
//
// 设计：
//   - 大小写不敏感（统一转小写匹配）
//   - 子串匹配（无需分词；词组内包含关键词即命中）
//   - 多语言支持：中文 2-4 字关键词 / 英文单词
func shouldForceTransferByKeywords(content string) bool {
	if content == "" {
		return false
	}
	lc := strings.ToLower(content)
	for _, kw := range transferKeywords {
		if kw == "" {
			continue
		}
		if strings.Contains(lc, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
