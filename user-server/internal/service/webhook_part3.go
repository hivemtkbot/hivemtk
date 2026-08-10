// 拆分自 webhook.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/trace"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"
	"strconv"
	"strings"
	"time"
)

func (s *WebhookService) dispatchFeishu(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB(ctx)
	var fsPayload struct {
		Challenge string `json:"challenge"`
		Type      string `json:"type"`
		Header    *struct {
			EventType    string `json:"event_type"`
			AppID        string `json:"app_id"`
			TenantKey    string `json:"tenant_key"`
			EventID      string `json:"event_id"`
			Token        string `json:"token"`
			CreateTime   int64  `json:"create_time"`
			AppSecretVer int    `json:"app_secret_ver"`
		} `json:"header,omitempty"`
		Event *struct {
			Sender *struct {
				SenderID *struct {
					UnionID string `json:"union_id"`
					UserID  string `json:"user_id"`
					OpenID  string `json:"open_id"`
				} `json:"sender_id"`
				SenderType string `json:"sender_type"`
				TenantKey  string `json:"tenant_key"`
			} `json:"sender"`
			Message *struct {
				MessageID   string `json:"message_id"`
				ChatID      string `json:"chat_id"`
				ChatType    string `json:"chat_type"`
				MessageType string `json:"message_type"`
				Content     string `json:"content"` // JSON 字符串
				CreateTime  int64  `json:"create_time"`
			} `json:"message"`
		} `json:"event,omitempty"`
	}
	if err := json.Unmarshal(raw, &fsPayload); err != nil {
		return nil, fmt.Errorf("feishu parse: %w", err)
	}
	// URL 验证挑战：返回 challenge 字段
	if fsPayload.Challenge != "" && (fsPayload.Type == "url_verification" || fsPayload.Header == nil) {
		// 不入库；返回 challenge 即可
		return nil, nil
	}
	if fsPayload.Event == nil || fsPayload.Event.Message == nil {
		return nil, nil
	}
	m := fsPayload.Event.Message
	// 解析 content JSON 字符串
	var contentObj struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(m.Content), &contentObj)
	content := contentObj.Text
	if content == "" {
		content = "[" + m.MessageType + "]"
	}
	senderID := ""
	if fsPayload.Event.Sender != nil && fsPayload.Event.Sender.SenderID != nil {
		senderID = fsPayload.Event.Sender.SenderID.OpenID
		if senderID == "" {
			senderID = fsPayload.Event.Sender.SenderID.UserID
		}
		if senderID == "" {
			senderID = fsPayload.Event.Sender.SenderID.UnionID
		}
	}
	hub := &model.MessageHub{
		Platform:       "feishu",
		AccountID:      accountID,
		MsgID:          m.MessageID,
		Direction:      "inbound",
		SenderID:       senderID,
		ConversationID: m.ChatID,
		MsgType:        m.MessageType,
		Content:        content,
		SentAt:         time.Now(),
		IsGroup:        m.ChatType == "group",
		GroupID:        m.ChatID,
	}
	if err := s.messageHubRepo.Create(ctx, hub); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
			return nil, err
		}
	}
	s.upsertInboxFromHub(ctx, hub, "")
	// 回填标准化字段，供下游 AI 编排与出站复用：
	// ParsePayload 只能取到飞书 content 的原始 JSON 串、且取不到嵌套的 sender open_id，
	// 否则 AI 拿到 `{"text":"..."}` 这样的 JSON 串、出站目标 open_id 为空导致回复失败。
	p.Content = content
	p.Sender = senderID
	p.ChatID = m.ChatID
	return hub, nil
}

// upsertInboxFromHub 写入收件箱会话
func (s *WebhookService) upsertInboxFromHub(ctx context.Context, hub *model.MessageHub, customerName string) {
	if s.inboxConvRepo == nil || hub == nil {
		return
	}
	conv, err := s.inboxConvRepo.FindByPlatformAccountCustomer(ctx, hub.Platform, hub.AccountID, hub.SenderID)
	if err == nil && conv != nil {
		// 更新最后消息（atomic 自增 unread_count）
		_ = s.inboxConvRepo.UpdateLastMessage(ctx, conv.ID, hub.Content, hub.SentAt, 1)
		return
	}
	newConv := &model.InboxConversation{
		Platform:           hub.Platform,
		AccountID:          hub.AccountID,
		CustomerID:         hub.SenderID,
		CustomerName:       customerName,
		LastMessagePreview: hub.Content,
		LastMessageAt:      &hub.SentAt,
		UnreadCount:        1,
		Status:             "active",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	_ = s.inboxConvRepo.Create(ctx, newConv)
}

func (s *WebhookService) dispatchToUnified(ctx context.Context, um *model.UnifiedMessage) error {
	s.ensureReposFromDB(ctx)
	if s.unifiedMsgRepo == nil {
		return errors.New("unified message repo nil")
	}
	// 直接 Create + 唯一冲突忽略：避免「先查后插」在并发下产生的竞态窗口
	// （先查判不存在后、插入前另一协程已插入，导致重复写盘）。
	// 依赖 UnifiedMessage.MessageID 唯一约束兜底幂等（与同文件其他去重/插入模式一致）。
	if err := s.unifiedMsgRepo.Create(ctx, um); err != nil {
		// 唯一冲突（已存在）视为成功，不做覆盖
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			return nil
		}
		return err
	}
	return nil
}

func (s *WebhookService) retryWithBackoff(ctx context.Context, job *webhookJob, payload *ParsedPayload, origErr error) {
	delays := []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}
	for i := 0; i < WebhookMaxRetries; i++ {
		// 防御性守卫：若 delays 被裁短或常量被改动，至少不会 panic 越界。
		if i >= len(delays) {
			i = len(delays) - 1
		}
		time.Sleep(delays[i])
		um := s.ToUnifiedMessage(ctx, job.channel, job.account, payload)
		if err := s.dispatchToUnified(ctx, um); err == nil {
			s.markProcessed(ctx, job.event)
			return
		}
	}
	logger.Errorf("[Webhook] 多次重试失败 event=%s err=%v", job.event.EventID, origErr)
}

func (s *WebhookService) markProcessed(ctx context.Context, evt *model.WebhookEvent) {
	now := time.Now()
	evt.Processed = true
	evt.ProcessedAt = &now
	if s.eventRepo != nil && s.db != nil {
		_ = s.eventRepo.Update(ctx, evt)
	}
}

// shouldTriggerAI 是否触发 智能体
func (s *WebhookService) shouldTriggerAI(ctx context.Context, channel WebhookChannel, accountID string) bool {
	if s.salesEngine == nil {
		return false
	}
	accID, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || accID == 0 {
		return false
	}
	switch channel {
	case ChannelWeCom:
		if s.wecomRepo == nil {
			return false
		}
		acc, err := s.wecomRepo.GetByID(ctx, uint(accID))
		if err != nil {
			return false
		}
		return acc.AIAgentEnabled
	case ChannelFeishu:
		if s.feishuRepo == nil {
			return false
		}
		acc, err := s.feishuRepo.GetByID(ctx, uint(accID))
		if err != nil {
			return false
		}
		return acc.AIAgentEnabled
	case ChannelTelegram:
		if s.telegramRepo == nil {
			return false
		}
		acc, err := s.telegramRepo.GetByID(ctx, uint(accID))
		if err != nil {
			return false
		}
		return acc.AIAgentEnabled && acc.Status == 1
	case ChannelWhatsapp:
		if s.waRepo == nil {
			return false
		}
		acc, err := s.waRepo.GetByID(ctx, uint(accID))
		if err != nil {
			return false
		}
		return acc.AIAgentEnabled
	default:
		return false
	}
}

// triggerSalesEngine 触发 智能体处理入站消息
// 优先走 SmartCSOrchestrator（统一编排：会话/消息/AI决策/转人工/建议保存）
// 未注入 smartOrchestrator 时回退到 salesEngine.Handle 直接调用
//
// 多 AI 智能体路由（MULTI_AI_AGENT_DESIGN）：
//   - 若 agentBindingSvc 已注入，先按 (channel_type, account_id) 查询绑定的智能体上下文
//   - 智能体上下文非 nil 时调用 HandleWithAgent 按智能体配置执行
//   - 智能体上下文为 nil 时回退到 DefaultSalesEngineConfig 默认行为
//   - 在主流程之前 publish customer.message.received 事件，由 AgentRuntime.EventSubscriber 异步消费
func (s *WebhookService) triggerSalesEngine(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, hubMsg *model.MessageHub) {
	// 0. 发布事件到 Event Bus（异步,失败不影响主流程）
	//    AgentRuntime 订阅此事件,实现 L1 入口层与 L4 AI 引擎层解耦
	//    即使 AgentRuntime 未启动,事件会被 event.Publish 静默丢弃
	{
		customerID := ""
		sessionID := ""
		if hubMsg != nil {
			customerID = hubMsg.SenderID
			if hubMsg.ConversationID != "" {
				sessionID = hubMsg.ConversationID
			}
		}
		agent_runtime.PublishCustomerMessage(string(channel), accountID, customerID, sessionID, p.Content, p.EventID)
	}

	// 分支 A：智能体统一编排器（推荐路径，9 步编排完整闭环）
	if s.smartOrchestrator != nil {
		s.triggerSmartOrchestrator(ctx, channel, accountID, p, hubMsg)
		return
	}
	// 分支 B：回退路径（仅 SalesEngine 8 步链路，无会话/座席联动）
	if s.salesEngine == nil {
		return
	}
	// 继承上游 trace_id（如有），保证全链路日志可串联；
	// 上游未注入时，WithTraceID 自动生成新 ID（不丢可观测性）。
	parentCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(ctx); parentTraceID != "" {
		parentCtx = trace.NewContextWithTraceID(parentCtx, parentTraceID)
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	ctx = logger.WithModule(ctx, "webhook")

	// 多 AI 智能体路由：加载渠道账号绑定的智能体上下文
	agentCtx, _ := s.loadAgentForChannel(ctx, channel, accountID)

	// 按 channel 构造请求
	req := &SalesRequest{
		SessionID:   p.ChatID,
		UserMessage: p.Content,
		Platform:    string(channel),
		AutoExecute: true,
		Config:      DefaultSalesEngineConfig(),
	}
	// 填充 customerID（oneID）
	if hubMsg != nil {
		req.CustomerID = hubMsg.SenderID
		req.OneID = hubMsg.SenderID
	}
	// 按智能体上下文执行（agentCtx == nil 时 HandleWithAgent 内部回退到 Handle）
	resp, err := s.salesEngine.HandleWithAgent(ctx, req, agentCtx)
	if err != nil {
		logger.Errorf("[Webhook] sales engine error: %v", err)
		return
	}
	if resp == nil || resp.Reply == "" {
		return
	}
	if resp.TransferredToHuman {
		// 转人工：仅记录，不出站
		logger.Infof("[Webhook] transferred to human: %s", resp.TransferReason)
		return
	}
	// 出站：按 channel 调用对应 Service（文本 + 结构化富卡片）
	s.sendOutbound(ctx, channel, accountID, p, resp.Reply, hubMsg, RichCardsFromDTO(resp.Cards))
}

// loadAgentForChannel 加载渠道账号绑定的智能体上下文
// agentBindingSvc 未注入或未绑定时返回 (nil, nil)，调用方回退默认配置
func (s *WebhookService) loadAgentForChannel(ctx context.Context, channel WebhookChannel, accountID string) (*AgentContext, error) {
	if s.agentBindingSvc == nil {
		return nil, nil
	}
	channelType := NormalizeChannelType(string(channel))
	return s.agentBindingSvc.LoadAgentForChannel(ctx, channelType, accountID)
}

// triggerSmartOrchestrator 智能体统一编排器路径
// 调用 SmartCSOrchestrator.HandleIncomingWithAgent 完成会话/消息/AI决策/转人工，再按结果出站
// 多 AI 智能体路由：先加载渠道账号绑定的智能体上下文，传给编排器
//   - 编排器内部会按座席挂载智能体覆盖（座席挂载 > 渠道绑定 > 默认）
//   - agentBindingSvc 未注入或未绑定时 agentCtx=nil，回退默认配置
//
// 修复（2026-08-05）：增加 panic recover + 显式告警日志。
// 历史 bug：智能体未注入时仅 silent return，下游无任何反馈导致会话/AI 回复全链路静默缺失
// （典型现场：message_hub 有 inbound 记录但 customer_sessions/ai_suggestions 均为空）。
// 修复后 nil orchestrator 转为 Error 级别日志，panic 转 Error + 堆栈，便于线上定位。
func (s *WebhookService) triggerSmartOrchestrator(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, hubMsg *model.MessageHub) {
	defer func() {
		if r := recover(); r != nil {
			logger.Ctx(ctx).Error().
				Interface("panic", r).
				Str("channel", string(channel)).
				Str("account_id", accountID).
				Str("event_id", p.EventID).
				Msg("[Webhook] triggerSmartOrchestrator panic recovered — AI 链路已断开，请查 root cause")
		}
	}()
	if s.smartOrchestrator == nil {
		logger.Ctx(ctx).Error().
			Str("channel", string(channel)).
			Str("account_id", accountID).
			Str("event_id", p.EventID).
			Str("conv_id", hubMsg.ConversationID).
			Msg("[Webhook] smartOrchestrator 未注入 — 桥接入站消息不会创建 customer_sessions / 不会生成 AI 回复。请检查 router.Setup() 中 webhookCtrl.SetSmartOrchestrator(orchestrator) 是否先于 bridgeIngressSvc.SetAITrigger(webhookSvc) 执行。")
		return
	}
	logger.Ctx(ctx).Info().
		Str("channel", string(channel)).
		Str("account_id", accountID).
		Str("event_id", p.EventID).
		Msg("[Webhook] triggerSmartOrchestrator start")
	// 轻量同步准备：加载智能体路由上下文 + 构造 IncomingContext（快速，不阻塞接入 worker）
	// 继承上游 trace_id（路由上下文在加载智能体时产生的日志可与主链路串联）
	routeCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(ctx); parentTraceID != "" {
		routeCtx = trace.NewContextWithTraceID(routeCtx, parentTraceID)
	}
	routeCtx = logger.WithModule(routeCtx, "webhook")
	agentCtx, _ := s.loadAgentForChannel(routeCtx, channel, accountID)

	in := &IncomingContext{
		Platform:  model.Platform(channel),
		AccountID: accountID,
		SenderID:  p.Sender,
		Content:   p.Content,
		MessageID: p.EventID,
	}
	if hubMsg != nil {
		in.OneID = hubMsg.SenderID
		in.SenderName = hubMsg.SenderName
		in.MediaURL = hubMsg.MediaURL
		// 群聊元数据透传：群聊客户消息 SenderID=群 id，AI 编排需按群建独立会话
		in.IsGroup = hubMsg.IsGroup
		in.GroupID = hubMsg.GroupID
		in.GroupName = groupNameFromHub(hubMsg)
	}
	// Extra 兜底：从原始 payload 抽取 sender_name / media_url
	if p.Extra != nil {
		if v, ok := p.Extra["sender_name"]; ok {
			if name, _ := v.(string); name != "" {
				in.SenderName = name
			}
		}
		if v, ok := p.Extra["media_url"]; ok {
			if url, _ := v.(string); url != "" {
				in.MediaURL = url
			}
		}
	}

	// 将重 LLM 生成从接入 worker 解耦。
	// 接入 worker 完成轻量准备后立即返回，AI 生成由独立有界池（replySem）执行，
	// 推理饱和时 AI 任务排队等待而非丢弃，避免 4 worker 同步跑 LLM 成为被动回复吞吐天花板。
	go s.runAIGeneration(ctx, channel, accountID, p, hubMsg, in, agentCtx)
}

// TriggerInboundAI 实现 service.AITrigger 接口：供网页桥接入站消息触发 AI 客服。
//
// 复用与 web 私信同源的同步主链路 triggerSmartOrchestrator，确保抖音/小红书/TikTok
// 网页桥接的新消息能像 web 私信一样被 AI 及时处理，并原路经 WebSocket 回写扩展。
// 若智能体编排器（smartOrchestrator）未注入，则安全空转（仅落库，不回复）。
//
// opts 透传群聊/发送者元数据（senderName/isGroup/groupID/groupName）：
// 群聊中客户消息 sender_id 聚合为群 id，AI 编排需据此区分成员（见 AITrigger 注释）。
//
// 修复（2026-08-05）：入口加 panic recover。TriggerInboundAI 由 inbox_ingress.HandleIngressMessage
// 同步调用，任何 panic 会冒泡到 bridge.readPump goroutine → runtime，整体进程被杀。
// 修复后 panic 转 Error + 堆栈，进程存活，业务可继续入站。
func (s *WebhookService) TriggerInboundAI(ctx context.Context, channel, accountID, conversationID, customerID, content, eventID string, opts ...TriggerInboundOption) {
	defer func() {
		if r := recover(); r != nil {
			logger.Ctx(ctx).Error().
				Interface("panic", r).
				Str("channel", channel).
				Str("account_id", accountID).
				Str("conv_id", conversationID).
				Str("event_id", eventID).
				Str("sender", customerID).
				Msg("[Webhook] TriggerInboundAI panic recovered — AI 链路已断开，请查 root cause")
		}
	}()
	// 复用与 Receive 一致的幂等去重 + 限流守卫，避免网页桥接入站绕过幂等/限流。
	if eventID != "" && s.isDuplicate(ctx, eventID) {
		logger.Ctx(ctx).Debug().Str("event_id", eventID).Msg("[Webhook] TriggerInboundAI duplicate, skip")
		return
	}
	rateKey := string(channel) + ":" + accountID
	if !s.allowRate(ctx, rateKey) {
		logger.Ctx(ctx).Warn().Str("channel", channel).Str("account_id", accountID).
			Msg("[Webhook] TriggerInboundAI rate limited, skip")
		return
	}
	// 解析透传元数据（群聊/发送者）
	meta := &TriggerInboundMeta{}
	for _, opt := range opts {
		if opt != nil {
			opt(meta)
		}
	}
	p := &ParsedPayload{
		EventID: eventID,
		Sender:  customerID,
		Content: content,
	}
	if meta.SenderName != "" {
		p.Extra = map[string]any{"sender_name": meta.SenderName}
	}
	hubMsg := &model.MessageHub{
		MsgID:          eventID,
		Platform:       channel,
		AccountID:      accountID,
		ConversationID: conversationID,
		SenderID:       customerID,
		SenderName:     meta.SenderName,
		Content:        content,
		Direction:      "inbound",
		IsGroup:        meta.IsGroup,
		GroupID:        meta.GroupID,
		IsRead:         true,
		SentAt:         time.Now(),
		Extra:          map[string]any{"sender_name": meta.SenderName},
	}
	if meta.IsGroup {
		if hubMsg.Extra == nil {
			hubMsg.Extra = model.JSONMap{}
		}
		hubMsg.Extra["is_group"] = true
		if meta.GroupID != "" {
			hubMsg.Extra["group_id"] = meta.GroupID
		}
		if meta.GroupName != "" {
			hubMsg.Extra["group_name"] = meta.GroupName
		}
	}
	s.triggerSmartOrchestrator(ctx, WebhookChannel(channel), accountID, p, hubMsg)
}

// runAIGeneration 在独立有界池中执行 AI 生成与出站。
func (s *WebhookService) runAIGeneration(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, hubMsg *model.MessageHub, in *IncomingContext, agentCtx *AgentContext) {
	// 修复（2026-08-05）：goroutine 入口加 panic recover。
	// 历史 bug：runAIGeneration 在 triggerSmartOrchestrator 中以 go 启动，
	// 内部任意 panic（LLM 客户端 NPE / DB 连接重置 / nil 指针）会冒泡到 runtime 杀掉整个进程，
	// 或被 gin recover 后只输出 200 行 panic 堆栈却没有任何业务级错误日志，
	// 导致 message_hub 有 inbound 记录但 customer_sessions/ai_suggestions 全空
	// （典型现场：小红书 bridge 5 条消息入库 0 个 session/AI 回复）。
	// 修复后 panic 转 Error + 堆栈，进程不会被杀，业务可观测性可定位。
	defer func() {
		if r := recover(); r != nil {
			logger.Ctx(ctx).Error().
				Interface("panic", r).
				Str("channel", string(channel)).
				Str("account_id", accountID).
				Str("event_id", p.EventID).
				Str("conv_id", hubMsg.ConversationID).
				Str("sender", p.Sender).
				Msg("[Webhook] runAIGeneration panic recovered — AI 链路已断开，请查 root cause")
		}
	}()
	// 修复（2026-08-05 P0 大扫除）：replySem 加超时获取。
	// 历史 bug：`s.replySem <- struct{}{}` 是阻塞写，AI 任务排队满时 goroutine 全部阻塞在 sema 上 → 越积越多 → OOM。
	// 修复后超 5s 仍拿不到 sema → 当前 AI 任务跳过 + Error 日志，依赖下轮 inbound 重试。
	const replySemTimeout = 5 * time.Second
	select {
	case s.replySem <- struct{}{}:
		defer func() { <-s.replySem }()
	case <-time.After(replySemTimeout):
		logger.Ctx(ctx).Error().
			Str("channel", string(channel)).
			Str("account_id", accountID).
			Str("event_id", p.EventID).
			Dur("timeout", replySemTimeout).
			Int("sem_capacity", cap(s.replySem)).
			Msg("[Webhook] runAIGeneration replySem 满 / 阻塞超时 — 跳过本轮 AI 推理，避免 goroutine 堆积 OOM；依赖下轮 inbound 重试")
		return
	case <-ctx.Done():
		logger.Ctx(ctx).Warn().Str("channel", string(channel)).Str("event_id", p.EventID).Msg("[Webhook] runAIGeneration ctx 取消，跳过")
		return
	}

	// 继承上游 trace_id（如有），保证全链路日志可串联；
	// 上游未注入时，WithTraceID 自动生成新 ID（不丢可观测性）。
	parentCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(ctx); parentTraceID != "" {
		parentCtx = trace.NewContextWithTraceID(parentCtx, parentTraceID)
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	ctx = logger.WithModule(ctx, "webhook")

	// 构造追踪载体并随 ctx 透传：工具层 observer 据此自动继承会话/渠道维度，
	// 以「观察者模式」零侵入采集 agent 多轮 / 多工具调用（见 tracing 包）。
	traceCarrier := &tracing.Carrier{
		TraceID:        hubMsg.TraceID,
		ConversationID: hubMsg.ConversationID,
		AccountID:      hubMsg.AccountID,
		Channel:        string(channel),
	}
	ctx = tracing.WithCarrier(ctx, traceCarrier)
	// 初始化自学习召回容器：RAG 检索时把召回的 chunk 累积到此，供下方 ai_dispatch 埋点写入 trace
	ctx = tracing.InitRecalledChunks(ctx)

	// 本地推理偶发超时，最多重试 WebhookMaxRetries 次
	var result *HandleResult
	var err error
	aiSpan := tracing.Start(ctx, tracing.NodeAIDispatch).
		Input(map[string]any{
			"channel":     string(channel),
			"account_id":  accountID,
			"conv_id":     hubMsg.ConversationID,
			"event_id":    p.EventID,
			"content_len": len(p.Content),
			"sender":      p.Sender,
			// 记录本轮真实用户问题（精确），供自学习聚合使用；
			// 否则回退到 message_hub「按会话取最近 inbound」会在多轮对话中错配 query/reply。
			"message": map[string]any{"content": p.Content, "sender": p.Sender},
		}).
		Expected("AI 编排器生成回复并决策（自动回复 / 转人工 / 接管）")
	for attempt := 0; attempt <= WebhookMaxRetries; attempt++ {
		result, err = s.smartOrchestrator.HandleIncomingWithAgent(ctx, in, agentCtx)
		if err == nil {
			break
		}
		logger.Ctx(ctx).Warn().
			Err(err).
			Str("event_id", p.EventID).
			Int("attempt", attempt).
			Msg("orchestrator handle retry")
	}
	// 节点2 AI 处理：入参（渠道/账号/会话/内容）→ 出参（决策/回复）→ 响应时间 / 异常
	aiAbnormal := ""
	aiStatus := tracing.StatusOk
	if err != nil {
		aiStatus = tracing.StatusAbnormal
		aiAbnormal = "AI 编排器在重试后仍失败：" + err.Error()
	} else if result == nil {
		aiStatus = tracing.StatusAbnormal
		aiAbnormal = "AI 编排器返回 nil 结果（无回复决策）"
	}
	// 自学习关联：把本次 AI 处理涉及的知识库 chunk 写入 trace，供后续打分动态调整其权重
	recalledChunkIDs := tracing.RecalledChunksOf(ctx)
	seenChunk := make(map[string]struct{})
	uniqRecalled := make([]string, 0, len(recalledChunkIDs))
	for _, id := range recalledChunkIDs {
		if _, ok := seenChunk[id]; !ok {
			seenChunk[id] = struct{}{}
			uniqRecalled = append(uniqRecalled, id)
		}
	}
	aiOutput := map[string]any{
		"ai_failed":          aiStatus == tracing.StatusAbnormal,
		"recalled_chunk_ids": uniqRecalled,
	}
	if result != nil {
		replyText := result.Reply
		if len(replyText) > 3000 {
			replyText = replyText[:3000]
		}
		aiOutput["ai_replied"] = result.AIReplied
		aiOutput["transferred"] = result.Transferred
		aiOutput["handler_type"] = result.HandlerType
		aiOutput["confidence"] = result.Confidence
		aiOutput["reply"] = replyText
		aiOutput["reply_len"] = len(result.Reply)
		aiOutput["session_id"] = result.SessionID
	}
	var aiSpanErr error
	if aiStatus == tracing.StatusAbnormal {
		aiSpanErr = fmt.Errorf("%s", aiAbnormal)
	}
	aiSpan.End(aiOutput, aiSpanErr)
	if err != nil {
		logger.Ctx(ctx).Error().
			Err(err).
			Str("event_id", p.EventID).
			Msg("orchestrator handle failed after retries")
		return
	}
	if result == nil {
		return
	}

	// 转人工：不出站（座席已通过 autoAssignToAgent 通知）
	if result.Transferred {
		logger.Ctx(ctx).Info().
			Str("session_id", result.SessionID).
			Str("reason", result.TransferReason).
			Msg("transferred to human")
		return
	}
	// AI 自动回复：出站发送（文本 + 结构化富卡片）
	if result.AIReplied && (result.Reply != "" || len(result.Cards) > 0) {
		s.sendOutbound(ctx, channel, accountID, p, result.Reply, hubMsg, result.Cards)
		return
	}
	// 其他情况（座席接管 / 仅生成建议未自动回复）：不出站，等座席手动回复
	logger.Ctx(ctx).Info().
		Str("session_id", result.SessionID).
		Str("handler", string(result.HandlerType)).
		Msg("no outbound")
}

// ContentHashMsgID 基于「渠道 + 消息内容」生成稳定的消息ID（FNV-1a 32位 hex）。
// conversationID 不参与哈希 —— 同一文本在不同会话被 patrol 捕获时哈希一致，实现全局去重。
//
// 2026-08-05 根因修复（用户指定方案：消息ID用内容hash）：
//
//	核心问题：前端 sender_type 判定可能错误（把 AI 回复的 outbound 误判为 customer）。
//	正确方案：msg_id 只用稳定字段（channel + content），不含 sender_type/sender_id/conversationID/timestamp。
//	前端 contentHash() 用相同算法生成 event_id → 后端 outbound 的 msg_id 与之一致 → 前端 patrol 扫描 AI 回复时
//	生成的 event_id 与 DB msg_id 相同 → 钩子2 GetByMsgID 命中 → 跳过入库和 AI 触发，彻底解决回环。
//
// 2026-08-07 修正：去掉 conversationID。
//
//	同一 AI 回复可能被不同会话的 patrol 交叉捕获（DOM 切换残留），
//	若 contentHash 含 conversationID 则不同会话算不同消息 → GetByMsgID 漏检 → 回环未完全切断。
//
// 算法：FNV-1a 32位（与前端 types.js contentHash 完全一致，保证前后端结果相同）
//
//   - 输入：`channel|content`（content 去首尾空白）
//
//   - 输出：`mh:${hex}`（8位hex字符串，带 mh: 前缀便于日志识别）
//
//   - 锚点：ContentHashMsgID("douyin", "c1", "你好") == "mh:00550fed"（输入不含 conversationID；与前端 types.js::contentHash 逐字节一致）
//
//     ⚠️ 严禁在输入中加入 conversationID：message_hub.MsgID 采用 (msg_id, conversation_id) 复合唯一索引，
//     同一 AI 回复会被不同会话的 patrol 交叉捕获，含 conv 会让 GetByMsgID 漏检、回环去重失效（详见 commit 36509ab）。
func ContentHashMsgID(channel, conversationID, content string) string {
	// conversationID 参数保留以兼容调用方（与前端 types.js::contentHash 一致：接收但忽略 conv），不参与哈希。
	s := channel + "|" + strings.TrimSpace(content)
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("mh:%08x", h.Sum32())
}

// ContentHashWithSender 统一收件去重哈希（渠道 + 发送者名称 + 消息内容）。
//
// 这是「渠道+发送者+消息内容」唯一去重依据的权威实现，前端（types.js::sharedContentHash）
// 与后端必须逐字节一致：
//   - 算法：FNV-1a 32 位，输入 channel|senderName|TrimSpace(content)，UTF-8 字节
//   - 输出：mh:<8位hex>
//
// 设计要点（与 ContentHashMsgID 的区别）：
//   - ContentHashMsgID 仅含渠道+内容，无法区分「AI 自己发的」与「客户复述了 AI 的原话」，
//     会导致客户复述被误判为回显而丢失（回环去重误杀）。
//   - 本函数把发送者纳入哈希，使「平台自己发出的消息」与「客户发的消息」拥有不同的去重键，
//     从而真正基于 (渠道,发送者,内容) 三元组做去重/自他判定。
//
// 发送者名称以服务端权威判定为准（见 inbox_ingress.senderKeyForDedup）：前端 patrol 上报的
// sender_type/sender_name 不可信（无法可靠分辨自他），服务端在入库前通过 DB 回查 message_hub
// 出站(outbound)行判定「自己消息」，再以账号(platform 身份)回填发送者，保证自/他区分不依赖前端标签。
//
// 注意：content 仍做 TrimSpace（与 ContentHashMsgID 保持一致，兼容首尾空白差异），
// 但严禁加入 conversationID——跨会话同内容(不同发送者)必须可区分，且复合唯一索引
// (msg_id, conversation_id) 已为跨会话同内容留出空间。
func ContentHashWithSender(channel, senderName, content string) string {
	s := channel + "|" + senderName + "|" + strings.TrimSpace(content)
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("mh:%08x", h.Sum32())
}
