package service

import (
	"context"

	"fmt"

	"strconv"

	"time"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/pkg/trace"
	"hivemtk-user/internal/pkg/tracing"
)

func (s *WebhookService) SetSalesEngine(ctx context.Context, e *SalesEngine) {
	s.salesEngine = e
}

func (s *WebhookService) SetSmartOrchestrator(ctx context.Context, o *SmartCSOrchestrator) {
	s.smartOrchestrator = o
}

func (s *WebhookService) retryWithBackoff(ctx context.Context, job *webhookJob, payload *ParsedPayload, origErr error) {
	delays := []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}
	for i := 0; i < WebhookMaxRetries; i++ {

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

	if s.smartOrchestrator != nil {
		s.triggerSmartOrchestrator(ctx, channel, accountID, p, hubMsg)
		return
	}

	if s.salesEngine == nil {
		return
	}

	parentCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(ctx); parentTraceID != "" {
		parentCtx = trace.NewContextWithTraceID(parentCtx, parentTraceID)
	}

	aiTimeout := webhookEnvInt("WEBHOOK_AI_TIMEOUT_SECONDS", 180)
	if aiTimeout < 60 {
		aiTimeout = 60
	}
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(aiTimeout)*time.Second)
	defer cancel()
	ctx = logger.WithModule(ctx, "webhook")

	agentCtx, _ := s.loadAgentForChannel(ctx, channel, accountID)

	req := &SalesRequest{
		SessionID:   p.ChatID,
		UserMessage: p.Content,
		Platform:    string(channel),
		AutoExecute: true,
		Config:      DefaultSalesEngineConfig(),
	}

	if hubMsg != nil {
		req.CustomerID = hubMsg.SenderID
		req.OneID = hubMsg.SenderID
	}

	resp, err := s.salesEngine.HandleWithAgent(ctx, req, agentCtx)
	if err != nil {
		logger.Errorf("[Webhook] sales engine error: %v", err)
		return
	}
	if resp == nil || resp.Reply == "" {
		return
	}
	if resp.TransferredToHuman {

		logger.Infof("[Webhook] transferred to human: %s", resp.TransferReason)
		return
	}

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

		in.IsGroup = hubMsg.IsGroup
		in.GroupID = hubMsg.GroupID
		in.GroupName = groupNameFromHub(hubMsg)
	}

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

	parentCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(ctx); parentTraceID != "" {
		parentCtx = trace.NewContextWithTraceID(parentCtx, parentTraceID)
	}

	aiTimeout := webhookEnvInt("WEBHOOK_AI_TIMEOUT_SECONDS", 180)
	if aiTimeout < 60 {
		aiTimeout = 60
	}
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(aiTimeout)*time.Second)
	defer cancel()
	ctx = logger.WithModule(ctx, "webhook")

	traceCarrier := &tracing.Carrier{
		TraceID:        hubMsg.TraceID,
		ConversationID: hubMsg.ConversationID,
		AccountID:      hubMsg.AccountID,
		Channel:        string(channel),
	}
	ctx = tracing.WithCarrier(ctx, traceCarrier)

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

	aiAbnormal := ""
	aiStatus := tracing.StatusOk
	if err != nil {
		aiStatus = tracing.StatusAbnormal
		aiAbnormal = "AI 编排器在重试后仍失败：" + err.Error()
	} else if result == nil {
		aiStatus = tracing.StatusAbnormal
		aiAbnormal = "AI 编排器返回 nil 结果（无回复决策）"
	}

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

	if result.Transferred {
		logger.Ctx(ctx).Info().
			Str("session_id", result.SessionID).
			Str("reason", result.TransferReason).
			Msg("transferred to human")
		return
	}

	if result.AIReplied && (result.Reply != "" || len(result.Cards) > 0) {
		s.sendOutbound(ctx, channel, accountID, p, result.Reply, hubMsg, result.Cards)
		return
	}

	logger.Ctx(ctx).Info().
		Str("session_id", result.SessionID).
		Str("handler", string(result.HandlerType)).
		Msg("no outbound")
}
