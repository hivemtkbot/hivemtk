package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// ============================================================================
// SmartCSOrchestrator 智能体统一编排器（LLM + 客服座席结合体）
// ----------------------------------------------------------------------------
// 这是商户端"智能体"支柱的核心枢纽，把 智能体引擎（SalesEngine 8 步链路）
// 与客服座席体系（SessionAssignment + CustomerSession + AISuggestion）统一编排。
//
// 设计理念（对应产品定位）：
//   - 智能体 = LLM 能力 + 客服座席 协作体
//   - 高置信度场景：AI 全自动回复（降本）
//   - 低置信度场景：转人工座席 + 推送 AI 建议参考（兜底）
//   - 座席可随时接管任意 AI 会话（人机协同）
//
// 入口：webhook 收到消息 → SmartCSOrchestrator.HandleIncoming
// 出口：① AI 自动回复  ② 转人工 + AI 建议推送  ③ 座席接管
//
// 五层架构：Service 层，依赖 Repository 层，被 Controller/Webhook 层调用。
// ============================================================================

// SmartCSOrchestrator 智能体编排器
type SmartCSOrchestrator struct {
	engine         *SalesEngine              // 智能体引擎（8 步链路）
	sessionSvc     *CustomerSessionService   // 客服会话服务
	assignmentSvc  *SessionAssignmentService // 会话分配服务（转人工）
	suggestionRepo *repository.AISuggestionRepository
	sessionRepo    *repository.CustomerSessionRepository
	messageRepo    *repository.SessionMessageRepository
	agentRepo      *repository.AgentStatusRepository

	// 多 AI 智能体：客服座席挂载的智能体服务
	// 注入后 HandleIncomingWithAgent 会按座席挂载的智能体覆盖渠道默认智能体
	// 未注入时仅使用渠道绑定智能体（agentCtxFromChannel）
	csAgentSvc *CustomerServiceAgentService

	// 配置
	confidenceThreshold float64 // AI 自动回复的置信度阈值
	enableAutoReply     bool    // 是否启用 AI 自动回复（false=仅生成建议，不自动发）
	maxAIConsecutive    int     // 单会话 AI 连续回复上限（防止 AI 死循环）
}

// OrchestratorConfig 编排器配置
type OrchestratorConfig struct {
	ConfidenceThreshold float64
	EnableAutoReply     bool
	MaxAIConsecutive    int
}

// DefaultOrchestratorConfig 默认配置
func DefaultOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		ConfidenceThreshold: 0.7,
		EnableAutoReply:     true,
		MaxAIConsecutive:    5,
	}
}

// NewSmartCSOrchestrator 创建智能体编排器
func NewSmartCSOrchestrator(engine *SalesEngine, cfg *OrchestratorConfig) *SmartCSOrchestrator {
	if cfg == nil {
		cfg = DefaultOrchestratorConfig()
	}
	sessionSvc := NewCustomerSessionService()
	assignmentSvc := NewSessionAssignmentService()
	assignmentSvc.SetConfidenceThreshold(cfg.ConfidenceThreshold)
	return &SmartCSOrchestrator{
		engine:              engine,
		sessionSvc:          sessionSvc,
		assignmentSvc:       assignmentSvc,
		suggestionRepo:      repository.NewAISuggestionRepository(),
		sessionRepo:         repository.NewCustomerSessionRepository(),
		messageRepo:         repository.NewSessionMessageRepository(),
		agentRepo:           repository.NewAgentStatusRepository(),
		confidenceThreshold: cfg.ConfidenceThreshold,
		enableAutoReply:     cfg.EnableAutoReply,
		maxAIConsecutive:    cfg.MaxAIConsecutive,
	}
}

// SetCustomerServiceAgentService 注入客服座席智能体挂载服务
// 注入后 HandleIncomingWithAgent 会按座席挂载的智能体覆盖渠道默认智能体
// 优先级：座席挂载 > 渠道绑定 > 默认配置
func (o *SmartCSOrchestrator) SetCustomerServiceAgentService(svc *CustomerServiceAgentService) {
	o.csAgentSvc = svc
}

// Mode 返回本编排器作为智能体生命周期的工作模式：被动（passive）。
// SmartCSOrchestrator 即 agent/lifecycle 体系下的「被动模式」实现——
// 消息/事件进入系统后由它调用智能体完成对话并返回（对话域主路径）。
// 主动模式（active）由后续主动触达引擎落地（详见 ADR-013）。
func (o *SmartCSOrchestrator) Mode() string { return string(model.AgentModePassive) }

// ----------------------------------------------------------------------------
// 入口：处理入站消息
// ----------------------------------------------------------------------------

// IncomingContext 入站消息上下文
type IncomingContext struct {
	Platform   model.Platform
	AccountID  string
	SenderID   string
	SenderName string
	Content    string
	MessageID  string
	MediaURL   string
	OneID      string // 客户 OneID（若已知）
}

// HandleResult 处理结果
type HandleResult struct {
	SessionID      string            `json:"session_id"`
	HandlerType    model.HandlerType `json:"handler_type"` // ai / human
	AIReplied      bool              `json:"ai_replied"`
	Reply          string            `json:"reply,omitempty"`
	Confidence     float64           `json:"confidence"`
	Transferred    bool              `json:"transferred"`
	TransferReason string            `json:"transfer_reason,omitempty"`
	SuggestionID   uint              `json:"suggestion_id,omitempty"`
	SalesResponse  *SalesResponse    `json:"sales_response,omitempty"`
}

// HandleIncoming 处理入站消息（智能体主入口，默认配置）
// 调用方：WebhookController 收到渠道消息后调用
// 等价于 HandleIncomingWithAgent(ctx, in, nil)
func (o *SmartCSOrchestrator) HandleIncoming(ctx context.Context, in *IncomingContext) (*HandleResult, error) {
	return o.HandleIncomingWithAgent(ctx, in, nil)
}

// HandleIncomingWithAgent 处理入站消息（按指定智能体编排）
// 多 AI 智能体路由核心入口：
//   - agentCtxFromChannel：渠道账号绑定的智能体上下文（由 WebhookService.loadAgentForChannel 加载）
//   - 若会话已分配座席，按座席挂载的智能体覆盖（座席挂载 > 渠道绑定 > 默认）
//   - agentCtx == nil 时回退到默认配置（engine.HandleWithAgent 内部回退到 Handle）
func (o *SmartCSOrchestrator) HandleIncomingWithAgent(ctx context.Context, in *IncomingContext, agentCtxFromChannel *AgentContext) (*HandleResult, error) {
	if o == nil || in == nil {
		return nil, errors.New("orchestrator or incoming context is nil")
	}
	if strings.TrimSpace(in.Content) == "" {
		return nil, errors.New("content is empty")
	}

	// 追踪：绑定 module=orchestrator，trace_id 由上游（webhook / websocket）经 ctx 传入
	ctx = logger.WithModule(ctx, "orchestrator")
	start := time.Now()
	result := &HandleResult{HandlerType: model.HandlerTypeAI}
	logger.Ctx(ctx).Info().
		Str("platform", string(in.Platform)).
		Str("account_id", in.AccountID).
		Str("sender_id", in.SenderID).
		Str("message_id", in.MessageID).
		Int("content_len", len(in.Content)).
		Msg("[1] orchestrator start")
	defer func() {
		logger.Ctx(ctx).Info().
			Dur("cost", time.Since(start)).
			Str("handler", string(result.HandlerType)).
			Bool("transferred", result.Transferred).
			Bool("ai_replied", result.AIReplied).
			Msg("[9] orchestrator done")
	}()

	// 1. 查找或创建客服会话
	session, err := o.findOrCreateSession(in)
	if err != nil {
		return nil, fmt.Errorf("find/create session failed: %w", err)
	}
	result.SessionID = session.SessionID

	// 2. 保存入站消息到会话
	if err := o.saveInboundMessage(session, in); err != nil {
		return nil, fmt.Errorf("save inbound message failed: %w", err)
	}

	// 3. 已分配给在线人工座席的会话：直接转人工，不触发 AI
	if session.HandlerType == model.HandlerTypeHuman && session.AgentID > 0 {
		if o.isAgentOnline(session.AgentID) {
			result.HandlerType = model.HandlerTypeHuman
			result.Transferred = true
			result.TransferReason = "会话已分配给在线座席"
			return result, nil
		}
		// 座席离线，继续走 AI 流程
	}

	// 4. 检查是否超过 AI 连续回复上限
	if session.AIReplyCount >= o.maxAIConsecutive {
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		result.TransferReason = fmt.Sprintf("AI 连续回复已达上限 (%d 次)，转人工跟进", o.maxAIConsecutive)
		_ = o.transferToHuman(session, result.TransferReason)
		return result, nil
	}

	// 5. 检查紧急/投诉内容：直接转人工
	if o.isUrgentOrComplaint(in.Content) {
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		result.TransferReason = "检测到紧急或投诉内容，转人工处理"
		_ = o.transferToHuman(session, result.TransferReason)
		return result, nil
	}

	// 6. 调用 SalesEngine 8 步链路（核心）
	if o.engine == nil {
		// 引擎未注入，降级转人工
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		result.TransferReason = "AI 引擎未就绪，转人工"
		_ = o.transferToHuman(session, result.TransferReason)
		return result, nil
	}

	// 6.1 智能体上下文优先级裁决：
	//   优先级：座席挂载 > 渠道绑定 > 默认配置
	//   若会话已分配座席（session.AgentID > 0），尝试按座席挂载加载智能体
	finalAgentCtx := agentCtxFromChannel
	if session.AgentID > 0 && o.csAgentSvc != nil {
		seatAgentCtx, err := o.csAgentSvc.LoadAgentForSeat(ctx, session.AgentID)
		if err != nil {
			// 加载失败不阻断主流程，降级使用渠道绑定智能体
			logger.Ctx(ctx).Warn().
				Err(err).
				Uint("agent_id", session.AgentID).
				Msg("[6.1] load seat agent failed, fallback to channel binding")
		} else if seatAgentCtx != nil {
			// 座席挂载优先：覆盖渠道绑定智能体
			finalAgentCtx = seatAgentCtx
		}
	}

	salesReq := &SalesRequest{
		SessionID:   session.SessionID,
		CustomerID:  session.UserID,
		OneID:       in.OneID,
		UserMessage: in.Content,
		Platform:    string(in.Platform),
		AutoExecute: o.enableAutoReply,
	}
	// 按智能体上下文执行（finalAgentCtx == nil 时 HandleWithAgent 内部回退到 Handle）
	salesResp, err := o.engine.HandleWithAgent(ctx, salesReq, finalAgentCtx)
	if err != nil || salesResp == nil {
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		result.TransferReason = "AI 引擎处理失败，转人工兜底"
		_ = o.transferToHuman(session, result.TransferReason)
		return result, nil
	}
	result.SalesResponse = salesResp
	result.Confidence = o.extractConfidence(salesResp)

	// 7. 保存 AI 建议（无论是否自动回复，都给座席参考）
	suggestionID := o.saveAISuggestion(session.SessionID, salesResp)
	result.SuggestionID = suggestionID

	// 8. 决策：AI 自动回复 or 转人工
	// 若智能体自定义了置信度阈值，优先使用智能体阈值
	threshold := o.confidenceThreshold
	if finalAgentCtx != nil && finalAgentCtx.ConfidenceThreshold > 0 {
		threshold = finalAgentCtx.ConfidenceThreshold
	}
	shouldTransfer := salesResp.TransferredToHuman || result.Confidence < threshold
	if shouldTransfer {
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		if salesResp.TransferReason != "" {
			result.TransferReason = salesResp.TransferReason
		} else {
			result.TransferReason = fmt.Sprintf("AI 置信度不足 (%.2f < %.2f)", result.Confidence, threshold)
		}
		_ = o.transferToHuman(session, result.TransferReason)
		return result, nil
	}

	// 9. AI 自动回复
	result.HandlerType = model.HandlerTypeAI
	result.AIReplied = true
	result.Reply = salesResp.Reply

	if o.enableAutoReply && salesResp.Reply != "" {
		if err := o.saveOutboundMessage(session, salesResp.Reply, true); err != nil {
			return nil, fmt.Errorf("save outbound message failed: %w", err)
		}
		_ = o.markSuggestionUsed(suggestionID)
		_ = o.incrementAIReplyCount(session)
	}

	return result, nil
}

// ----------------------------------------------------------------------------
// 内部方法
// ----------------------------------------------------------------------------

// findOrCreateSession 查找或创建会话
func (o *SmartCSOrchestrator) findOrCreateSession(in *IncomingContext) (*model.CustomerSession, error) {
	// 查找活跃会话：直接按 user_id + 活跃状态点查（命中 user_id 索引）。
	// 性能审计 P1-3：原实现 GetByMerchant("",1,20) 会对全量会话做 COUNT + 取最近 20 条再线性匹配，
	// 在 1000 万/日被动回复下每条消息都触发一次全表 COUNT，且只扫 20 条会漏掉用户真实会话。
	if existing, err := o.sessionRepo.GetActiveByUserID(in.SenderID); err == nil && existing != nil {
		return existing, nil
	}

	// 创建新会话
	sessionID := fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), safeMessageID(in.MessageID))
	now := time.Now()
	session := &model.CustomerSession{
		SessionID:     sessionID,
		Platform:      in.Platform,
		AccountID:     in.AccountID,
		UserID:        in.SenderID,
		UserName:      in.SenderName,
		Status:        model.SessionStatusPending,
		Priority:      0,
		LastMessage:   in.Content,
		LastMessageAt: &now,
		LastMessageBy: "user",
		HandlerType:   model.HandlerTypeAI,
	}
	if err := o.sessionRepo.Create(session); err != nil {
		return nil, err
	}
	return session, nil
}

// saveInboundMessage 保存入站消息
//
// 去重逻辑（修复 chat 访客端双保存 bug，2026-07-17）：
//
//	chat_visitor_service.SendMessage 也保存了 user 消息，再调 HandleIncomingWithAgent
//	会导致同一条用户消息被保存两次（数据库中 2 条 row，前端列表重复）。
//	解决：在保存前查最近 5 秒内是否已存在同 (session, content, sender) 的消息，
//	若有则跳过保存，返回已存在消息的引用。
func (o *SmartCSOrchestrator) saveInboundMessage(session *model.CustomerSession, in *IncomingContext) error {
	if existing, _ := o.messageRepo.FindRecentDuplicate(session.SessionID, "user", in.SenderID, in.Content, 5*time.Second); existing != nil {
		return nil
	}
	msg := &model.SessionMessage{
		SessionID:   session.SessionID,
		Content:     in.Content,
		ContentType: model.MessageTypeText,
		MediaURL:    in.MediaURL,
		SenderType:  "user",
		SenderID:    in.SenderID,
		SenderName:  in.SenderName,
	}
	return o.messageRepo.Create(msg)
}

// saveOutboundMessage 保存出站消息（去重：避免与 visitor 端双保存）
func (o *SmartCSOrchestrator) saveOutboundMessage(session *model.CustomerSession, content string, aiGenerated bool) error {
	senderType := "agent"
	if aiGenerated {
		senderType = "ai"
	}
	if existing, _ := o.messageRepo.FindRecentDuplicate(session.SessionID, senderType, "ai_assistant", content, 5*time.Second); existing != nil {
		return nil
	}
	msg := &model.SessionMessage{
		SessionID:   session.SessionID,
		Content:     content,
		ContentType: model.MessageTypeText,
		SenderType:  senderType,
		SenderID:    "ai_assistant",
		SenderName:  "AI 助手",
	}
	return o.messageRepo.Create(msg)
}

// saveAISuggestion 保存 AI 建议供座席参考
func (o *SmartCSOrchestrator) saveAISuggestion(sessionID string, resp *SalesResponse) uint {
	if o.suggestionRepo == nil || resp == nil || resp.Reply == "" {
		return 0
	}
	confidence := o.extractConfidence(resp)
	suggestion := &model.AISuggestion{
		SessionID:  sessionID,
		Suggestion: resp.Reply,
		Confidence: confidence,
		Source:     "sales_engine",
	}
	if err := o.suggestionRepo.Create(suggestion); err != nil {
		return 0
	}
	return suggestion.ID
}

// markSuggestionUsed 标记建议被采用
func (o *SmartCSOrchestrator) markSuggestionUsed(id uint) error {
	if id == 0 || o.suggestionRepo == nil {
		return nil
	}
	return o.suggestionRepo.MarkAsUsed(id, 0)
}

// transferToHuman 转人工（联动 SessionAssignmentService 真正分配在线座席）
func (o *SmartCSOrchestrator) transferToHuman(session *model.CustomerSession, reason string) error {
	// 1. 更新会话状态为待人工
	session.Status = model.SessionStatusWaiting
	session.HandlerType = model.HandlerTypeHuman
	session.LastMessage = reason
	now := time.Now()
	session.LastMessageAt = &now
	if err := o.sessionRepo.Update(session); err != nil {
		return err
	}
	// 2. 联动 SessionAssignmentService 真正分配在线座席（避免会话只标 waiting 却无人接）
	if o.assignmentSvc != nil {
		// autoAssignToAgent 内部：选活跃会话最少的在线客服 → AssignAgent → 通知
		// 失败时（无在线客服）保持 waiting 状态等待后续分配，不阻断主流程
		_ = o.assignmentSvc.autoAssignToAgent(session, reason)
	}
	return nil
}

// incrementAIReplyCount 增加 AI 回复计数
func (o *SmartCSOrchestrator) incrementAIReplyCount(session *model.CustomerSession) error {
	session.AIReplyCount++
	session.Status = model.SessionStatusAIHandling
	now := time.Now()
	session.LastMessageAt = &now
	return o.sessionRepo.Update(session)
}

// isAgentOnline 座席是否在线
func (o *SmartCSOrchestrator) isAgentOnline(agentID uint) (online bool) {
	defer func() {
		if r := recover(); r != nil {
			online = false
		}
	}()
	if o.agentRepo == nil {
		return false
	}
	agent, err := o.agentRepo.GetByAgentID(agentID)
	if err != nil || agent == nil {
		return false
	}
	return agent.Status == "online" || agent.Status == "busy"
}

// isUrgentOrComplaint 是否紧急/投诉
func (o *SmartCSOrchestrator) isUrgentOrComplaint(content string) bool {
	urgentKeywords := []string{
		"投诉", "举报", "曝光", "315", "消协", "工商局",
		"紧急", "着急", "马上", "立刻", "赶紧", "快点",
		"骗子", "假货", "垃圾", "再也不买",
		"退钱", "退款", "赔钱", "赔偿",
	}
	lower := strings.ToLower(content)
	for _, kw := range urgentKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// extractConfidence 从 SalesResponse 提取置信度
func (o *SmartCSOrchestrator) extractConfidence(resp *SalesResponse) float64 {
	if resp == nil {
		return 0
	}
	if resp.Intent != nil && resp.Intent.Confidence > 0 {
		return resp.Intent.Confidence
	}
	// 无意图置信度时，根据链路完整度评估
	score := 0.5 // 基础分
	if resp.Reply != "" {
		score += 0.1
	}
	if resp.Polished {
		score += 0.05
	}
	if resp.Audited && len(resp.AuditIssues) == 0 {
		score += 0.1
	}
	if len(resp.RAGChunks) > 0 {
		score += 0.05
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// safeMessageID 安全截取 MessageID
func safeMessageID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	if id == "" {
		return "nomsgid"
	}
	return id
}

// ----------------------------------------------------------------------------
// 座席接管相关
// ----------------------------------------------------------------------------

// AgentTakeover 座席接管 AI 会话
// 当座席认为 AI 回复不合适时，可主动接管会话
func (o *SmartCSOrchestrator) AgentTakeover(sessionID string, agentID uint) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("session not found (repo panic): %s, %v", sessionID, r)
		}
	}()
	session, err := o.sessionRepo.GetBySessionID(sessionID)
	if err != nil || session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	session.HandlerType = model.HandlerTypeHuman
	session.AgentID = agentID
	session.Status = model.SessionStatusHumanHandling
	now := time.Now()
	session.LastMessageAt = &now
	return o.sessionRepo.Update(session)
}

// AgentReply 座席手动回复（覆盖 AI 建议）
func (o *SmartCSOrchestrator) AgentReply(sessionID string, agentID uint, content string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("session not found (repo panic): %s, %v", sessionID, r)
		}
	}()
	session, err := o.sessionRepo.GetBySessionID(sessionID)
	if err != nil || session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	msg := &model.SessionMessage{
		SessionID:   sessionID,
		Content:     content,
		ContentType: model.MessageTypeText,
		SenderType:  "agent",
		SenderID:    fmt.Sprintf("%d", agentID),
	}
	if err := o.messageRepo.Create(msg); err != nil {
		return err
	}
	session.HumanReplyCount++
	session.LastMessage = content
	now := time.Now()
	session.LastMessageAt = &now
	session.LastMessageBy = "agent"
	return o.sessionRepo.Update(session)
}
