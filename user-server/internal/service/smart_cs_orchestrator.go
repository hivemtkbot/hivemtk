package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	ragcache "hivemtk-user/internal/aiagent/rag/cache"
	"hivemtk-user/internal/identity"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

// faqPromptVersion FAQ 语义缓存 prompt 版本（RT-2 缓存 key 维度之一；
// 答案生成 prompt 语义变更时必须递增，避免旧答案串新 prompt）
const faqPromptVersion = "v1"

// 全局 FAQ 语义缓存依赖（装配层经 SetGlobalFAQAnswerCache 注入，main.go 启动时）。
// 未注入（nil）时 SmartCSOrchestrator 零影响直通，向后兼容硬要求。
var (
	globalFAQCache    *ragcache.FAQAnswerCacheService
	globalFAQEmbedder llm.EmbeddingServiceInterface
)

// SetGlobalFAQAnswerCache 装配层注入 FAQ 答案语义缓存（M6 R-2 / RT-2 契约：仅 smart_cs FAQ 场景启用）
func SetGlobalFAQAnswerCache(svc *ragcache.FAQAnswerCacheService, embedder llm.EmbeddingServiceInterface) {
	globalFAQCache = svc
	globalFAQEmbedder = embedder
}

// SmartCSOrchestrator 智能体编排器
type SmartCSOrchestrator struct {
	engine         *SalesEngine
	sessionSvc     *CustomerSessionService
	assignmentSvc  *SessionAssignmentService
	suggestionRepo *repository.AISuggestionRepository
	sessionRepo    *repository.CustomerSessionRepository
	messageRepo    *repository.SessionMessageRepository
	agentRepo      *repository.AgentStatusRepository
	kbRepo         *repository.KnowledgeBaseRepository

	csAgentSvc  *CustomerServiceAgentService
	identitySvc *CustomerIdentityService // 可选：自动补建 customer 档案

	confidenceThreshold float64
	enableAutoReply     bool
	maxAIConsecutive    int

	// FAQ 语义缓存（M6 R-2）：构造时从全局装配读取；nil 时零影响直通
	faqCache    *ragcache.FAQAnswerCacheService
	faqEmbedder llm.EmbeddingServiceInterface
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
		MaxAIConsecutive:    10,
	}
}

// NewSmartCSOrchestrator 创建智能体编排器
func NewSmartCSOrchestrator(engine *SalesEngine, cfg *OrchestratorConfig, kbRepo *repository.KnowledgeBaseRepository) *SmartCSOrchestrator {
	if cfg == nil {
		cfg = DefaultOrchestratorConfig()
	}
	sessionSvc := NewCustomerSessionService()
	assignmentSvc := NewSessionAssignmentService()
	assignmentSvc.SetConfidenceThreshold(context.Background(), cfg.ConfidenceThreshold)
	return &SmartCSOrchestrator{
		engine:              engine,
		sessionSvc:          sessionSvc,
		assignmentSvc:       assignmentSvc,
		suggestionRepo:      repository.NewAISuggestionRepository(),
		sessionRepo:         repository.NewCustomerSessionRepository(),
		messageRepo:         repository.NewSessionMessageRepository(),
		agentRepo:           repository.NewAgentStatusRepository(),
		kbRepo:              kbRepo,
		confidenceThreshold: cfg.ConfidenceThreshold,
		enableAutoReply:     cfg.EnableAutoReply,
		maxAIConsecutive:    cfg.MaxAIConsecutive,
		faqCache:            globalFAQCache,
		faqEmbedder:         globalFAQEmbedder,
	}
}

// SetCustomerServiceAgentService 注入客服座席智能体挂载服务
// 注入后 HandleIncomingWithAgent 会按座席挂载的智能体覆盖渠道默认智能体
// 优先级：座席挂载 > 渠道绑定 > 默认配置
func (o *SmartCSOrchestrator) SetCustomerServiceAgentService(ctx context.Context, svc *CustomerServiceAgentService) {
	o.csAgentSvc = svc
}

// SetIdentityService 注入 CustomerIdentityService。
// 注入后 findOrCreateSession 在创建新会话时会自动 IdentifyOrCreate
// 补建 customer 档案（按平台 open_id 查找/创建），确保 session ↔ customer
// 关联不断裂；nil 时零影响（向后兼容）。
func (o *SmartCSOrchestrator) SetIdentityService(svc *CustomerIdentityService) {
	o.identitySvc = svc
}

// ensureCustomerForSession best-effort 补建 customer 档案。
// 当 identitySvc 已注入且 sender 非空时，从 platform+open_id 构造 Identifiers
// 调 IdentifyOrCreate，确保 session 创建后一定存在可关联的 customer。
// 失败只记 warn 日志，不阻断 session 创建主流程（session 是核心，customer 是增强）。
func (o *SmartCSOrchestrator) ensureCustomerForSession(ctx context.Context, platform model.Platform, senderID, userName string) {
	if o.identitySvc == nil || senderID == "" {
		return
	}
	var identifiers identity.Identifiers
	switch platform {
	case model.PlatformWeChat:
		identifiers.WechatOpenID = senderID
	case model.PlatformDouyin:
		identifiers.DouyinOpenID = senderID
	case model.PlatformXiaohongshu:
		identifiers.XiaohongshuID = senderID
	}
	if !HasAnyIdentifier(identifiers) {
		return
	}
	if _, err := o.identitySvc.IdentifyOrCreate(ctx, identifiers); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("platform", string(platform)).
			Str("sender", senderID).Msg("[Orchestrator] IdentifyOrCreate best-effort 失败，不阻断 session")
	}
}

// Mode 返回本编排器作为智能体生命周期的工作模式：被动（passive）。
// SmartCSOrchestrator 即 agent/lifecycle 体系下的「被动模式」实现——
// 消息/事件进入系统后由它调用智能体完成对话并返回（对话域主路径）。
// 主动模式（active）由后续主动触达引擎落地（详见）。
func (o *SmartCSOrchestrator) Mode(ctx context.Context) string { return string(model.AgentModePassive) }

// IncomingContext 入站消息上下文
type IncomingContext struct {
	Platform   model.Platform
	AccountID  string
	SenderID   string
	SenderName string
	Content    string
	MessageID  string
	MediaURL   string
	OneID      string
	IsGroup    bool
	GroupID    string
	GroupName  string
}

// HandleResult 处理结果
type HandleResult struct {
	SessionID      string            `json:"session_id"`
	HandlerType    model.HandlerType `json:"handler_type"`
	AIReplied      bool              `json:"ai_replied"`
	Reply          string            `json:"reply,omitempty"`
	Confidence     float64           `json:"confidence"`
	Transferred    bool              `json:"transferred"`
	TransferReason string            `json:"transfer_reason,omitempty"`
	Cards          []model.RichCard  `json:"cards,omitempty"`
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

	session, err := o.findOrCreateSession(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("find/create session failed: %w", err)
	}
	result.SessionID = session.SessionID

	// 全链路审查发现：ensureCustomerForSession 仅有定义无调用（d8d3cdf 半成品
	// 接线），导致桥接渠道客户档案永不创建（api_verify_full.py S3.1 长期 FAIL）。
	// 在 session 创建后补建 customer 档案，失败不阻断主流程（best-effort）。
	o.ensureCustomerForSession(ctx, in.Platform, in.SenderID, in.SenderName)

	if err := o.saveInboundMessage(ctx, session, in); err != nil {
		return nil, fmt.Errorf("save inbound message failed: %w", err)
	}

	if session.HandlerType == model.HandlerTypeHuman && session.AgentID > 0 {
		if o.isAgentOnline(ctx, session.AgentID) {
			result.HandlerType = model.HandlerTypeHuman
			result.Transferred = true
			result.TransferReason = "会话已分配给在线座席"
			return result, nil
		}
	}

	if o.maxAIConsecutive > 0 && session.AIReplyCount >= o.maxAIConsecutive {
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		result.TransferReason = fmt.Sprintf("AI 连续回复已达上限 (%d 次)，转人工跟进", o.maxAIConsecutive)
		utils.WarnErrKV("smartcs.transferToHuman.upperLimit", o.transferToHuman(ctx, session, result.TransferReason), "session_id", session.SessionID, "agent_id", strconv.FormatUint(uint64(session.AgentID), 10))
		return result, nil
	}

	// P1g 情感分层：愤怒→补偿+高级客服；焦虑→进度可视化继续走 AI（不盲转）
	emotionHint := ""
	if o.isUrgentOrComplaint(ctx, in.Content) {
		emoStrat := StrategyForEmotion(ClassifyEmotion(in.Content))
		if emoStrat.TransferToHuman {
			result.HandlerType = model.HandlerTypeHuman
			result.Transferred = true
			result.TransferReason = emoStrat.TransferReason
			utils.WarnErrKV("smartcs.transferToHuman.emotion", o.transferToHuman(ctx, session, result.TransferReason), "session_id", session.SessionID, "reason", emoStrat.TransferReason)
			return result, nil
		}
		emotionHint = emoStrat.ReplyHint
	}

	if o.engine == nil {
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		result.TransferReason = "AI 引擎未就绪，转人工"
		_ = o.transferToHuman(ctx, session, result.TransferReason)
		return result, nil
	}

	finalAgentCtx := agentCtxFromChannel
	if session.AgentID > 0 && o.csAgentSvc != nil {
		seatAgentCtx, err := o.csAgentSvc.LoadAgentForSeat(ctx, session.AgentID)
		if err != nil {
			logger.Ctx(ctx).Warn().
				Err(err).
				Uint("agent_id", session.AgentID).
				Msg("[6.1] load seat agent failed, fallback to channel binding")
		} else if seatAgentCtx != nil {
			finalAgentCtx = seatAgentCtx
		}
	}

	// M6 R-2 FAQ 语义缓存 Lookup（RT-2 契约：仅知识库可答的 FAQ 场景）。
	// 任一前提不满足（缓存未装配 / 座席未挂 KB / 向量化失败）即直通回源，零影响。
	faqKBID, faqVec := "", []float32(nil)
	if o.faqCache != nil && o.faqEmbedder != nil {
		if faqKBID = o.resolveFAQKBID(ctx, finalAgentCtx); faqKBID != "" {
			faqVec = o.embedFAQQuery(ctx, in.Content)
		}
	}
	if len(faqVec) > 0 {
		if res, hit := o.lookupFAQAnswerCache(ctx, faqKBID, faqVec, result); hit {
			return res, nil
		}
	}

	salesReq := &SalesRequest{
		SessionID:   session.SessionID,
		CustomerID:  session.UserID,
		OneID:       in.OneID,
		UserMessage: in.Content,
		Platform:    string(in.Platform),
		AutoExecute: o.enableAutoReply,
		EmotionHint: emotionHint,
	}
	salesResp, err := o.engine.HandleWithAgent(ctx, salesReq, finalAgentCtx)
	if err != nil || salesResp == nil {
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		result.TransferReason = "AI 引擎处理失败，转人工兜底"
		_ = o.transferToHuman(ctx, session, result.TransferReason)
		return result, nil
	}
	result.SalesResponse = salesResp
	result.Confidence = o.extractConfidence(ctx, salesResp)
	result.Cards = RichCardsFromDTO(salesResp.Cards)

	suggestionID := o.saveAISuggestion(ctx, session.SessionID, salesResp)
	result.SuggestionID = suggestionID

	threshold := o.confidenceThreshold
	if finalAgentCtx != nil && finalAgentCtx.ConfidenceThreshold > 0 {
		threshold = finalAgentCtx.ConfidenceThreshold
	}
	effectiveConf := result.Confidence
	if len(salesResp.Cards) > 0 && effectiveConf < threshold {
		effectiveConf = threshold
	}
	result.Confidence = effectiveConf
	knownIntent := salesResp.Intent != nil && salesResp.Intent.IntentType != IntentUnknown
	safeIntent := salesResp.Intent != nil &&
		(salesResp.Intent.IntentType == IntentGreeting || salesResp.Intent.IntentType == IntentSocial)
	shouldTransfer := salesResp.TransferredToHuman ||
		(knownIntent && !safeIntent && effectiveConf < threshold)
	if shouldTransfer {
		result.HandlerType = model.HandlerTypeHuman
		result.Transferred = true
		if salesResp.TransferReason != "" {
			result.TransferReason = salesResp.TransferReason
		} else {
			result.TransferReason = fmt.Sprintf("AI 置信度不足 (%.2f < %.2f)", result.Confidence, threshold)
		}
		utils.WarnErrKV("smartcs.transferToHuman.lowConfidence", o.transferToHuman(ctx, session, result.TransferReason), "session_id", session.SessionID, "confidence", strconv.FormatFloat(result.Confidence, 'f', 4, 64), "threshold", strconv.FormatFloat(threshold, 'f', 4, 64))
		return result, nil
	}

	result.HandlerType = model.HandlerTypeAI
	result.AIReplied = true
	result.Reply = salesResp.Reply

	// M6 R-2 FAQ 语义缓存 Store：答案来自知识库召回（RAGChunks 非空 = FromKnowledgeBase
	// 等效标志）时异步入缓存。四道门（CanCache）由缓存服务内部把关；失败仅告警不影响主链路。
	if faqKBID != "" && len(faqVec) > 0 && len(salesResp.RAGChunks) > 0 && salesResp.Reply != "" {
		go func(answer string, vec []float32) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[panic-recover] %T: %v\n%s", r, r, string(debug.Stack()))
				}
			}()
			if err := o.faqCache.Store(context.Background(), ragcache.StoreRequest{
				KBID:              faqKBID,
				PromptVersion:     faqPromptVersion,
				QueryVector:       vec,
				Answer:            answer,
				FromKnowledgeBase: true,
			}); err != nil {
				logger.Warnf("[ragcache] store answer failed (kb_id=%s): %v", faqKBID, err)
			}
		}(salesResp.Reply, faqVec)
	}

	if o.enableAutoReply && salesResp.Reply != "" {
		if err := o.saveOutboundMessage(ctx, session, salesResp.Reply, true); err != nil {
			return nil, fmt.Errorf("save outbound message failed: %w", err)
		}
		utils.WarnErrKV("smartcs.markSuggestionUsed", o.markSuggestionUsed(ctx, suggestionID), "session_id", session.SessionID, "suggestion_id", strconv.FormatUint(uint64(suggestionID), 10))
		utils.WarnErrKV("smartcs.incrementAIReplyCount", o.incrementAIReplyCount(ctx, session), "session_id", session.SessionID, "ai_reply_count", strconv.Itoa(session.AIReplyCount+1))
	}

	return result, nil
}

// resolveFAQKBID 解析座席挂载的主 FAQ/RAG 知识库 ID（RT-2 缓存 key 的 kb_id 维度）。
// 读不到（座席为空 / 无绑定 / DB 异常）返回空 = 本轮跳过缓存，零影响直通。
func (o *SmartCSOrchestrator) resolveFAQKBID(ctx context.Context, agentCtx *AgentContext) string {
	if agentCtx == nil || agentCtx.AgentID == 0 {
		return ""
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[panic-recover] %T: %v\n%s", r, r, string(debug.Stack()))
		}
	}()
	if o.kbRepo == nil {
		return ""
	}
	kbs, err := o.kbRepo.ListByAgent(ctx, agentCtx.AgentID)
	if err != nil {
		return ""
	}
	fallback := ""
	for _, kb := range kbs {
		switch kb.Type {
		case model.KnowledgeBaseTypeFAQ:
			return strconv.FormatUint(uint64(kb.ID), 10)
		case model.KnowledgeBaseTypeRAG:
			if fallback == "" {
				fallback = strconv.FormatUint(uint64(kb.ID), 10)
			}
		}
	}
	return fallback
}

// embedFAQQuery 把用户消息向量化（与知识库 chunk 同一 embedding 服务，保证同向量空间）。
// 失败返回 nil = 跳过缓存直通。
func (o *SmartCSOrchestrator) embedFAQQuery(ctx context.Context, text string) []float32 {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[panic-recover] %T: %v\n%s", r, r, string(debug.Stack()))
		}
	}()
	vec, err := o.faqEmbedder.EmbedOne(ctx, o.faqEmbedder.DefaultConfig(), text)
	if err != nil || len(vec) == 0 {
		return nil
	}
	return vec
}

// lookupFAQAnswerCache 查询语义缓存；TierExact/TierSemantic 命中时直接以缓存答案回复。
// 未命中/异常一律返回 hit=false 回源生成（零影响直通）。
func (o *SmartCSOrchestrator) lookupFAQAnswerCache(ctx context.Context, kbID string, vec []float32, result *HandleResult) (*HandleResult, bool) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warnf("[ragcache] lookup panic (kb_id=%s): %v", kbID, r)
		}
	}()
	lr, err := o.faqCache.Lookup(ctx, ragcache.LookupRequest{
		KBID:          kbID,
		PromptVersion: faqPromptVersion,
		QueryVector:   vec,
	})
	if err != nil || lr == nil || lr.Tier == ragcache.TierMiss || strings.TrimSpace(lr.Answer) == "" {
		return nil, false
	}
	logger.Ctx(ctx).Info().
		Str("kb_id", kbID).
		Str("tier", string(lr.Tier)).
		Float64("similarity", lr.Similarity).
		Msg("[ragcache] FAQ answer cache HIT, skip LLM generation")
	result.HandlerType = model.HandlerTypeAI
	result.AIReplied = true
	result.Reply = lr.Answer
	result.Confidence = 1.0
	if o.enableAutoReply {
		if session := o.sessionOfResult(result); session != nil {
			utils.WarnErrKV("smartcs.saveOutboundMessage.hit", o.saveOutboundMessage(ctx, session, lr.Answer, true), "session_id", session.SessionID, "source", "ragcache")
			utils.WarnErrKV("smartcs.incrementAIReplyCount.hit", o.incrementAIReplyCount(ctx, session), "session_id", session.SessionID, "source", "ragcache")
		}
	}
	return result, true
}

// sessionOfResult 按 result.SessionID 取会话（缓存命中路径的落库/计数用）；
// 取不到返回 nil，调用方跳过后续会话级写操作（缓存命中回复本身不受影响）。
func (o *SmartCSOrchestrator) sessionOfResult(result *HandleResult) *model.CustomerSession {
	if result == nil || result.SessionID == "" {
		return nil
	}
	session, err := o.sessionRepo.GetByIDString(context.Background(), result.SessionID)
	if err != nil || session == nil {
		return nil
	}
	return session
}

// findOrCreateSession 查找或创建会话
//
// 匹配优先级（S3-1 OneID 跨渠道合并 + 兜底）：
//  1. OneID（跨渠道合并辅助键）—— 同 OneID 视为同一人，跨平台 user_id 不同但 OneID
//     相同则合并会话，避免冷启动
//  2. user_id（单渠道内）—— 命中 user_id 索引，单点查
//  3. 创建新会话时，若 OneID 为空，自动以 Platform:SenderID 拼接临时 OneID
//
// （兜底），保证同 Platform + SenderID 的用户在 TTL 内可被同会话合并
//
// 注释：S3-1 之前的实现只按 user_id 匹配，会导致 web → TG 切换时冷启动。
func (o *SmartCSOrchestrator) findOrCreateSession(ctx context.Context, in *IncomingContext) (*model.CustomerSession, error) {
	if in.IsGroup {
		groupKey := in.GroupID
		if groupKey == "" {
			groupKey = in.SenderID
		}
		derivedOneID := "group:" + groupKey
		now := time.Now()
		stableSessionID := fmt.Sprintf("sess_%s_%s_%s", string(in.Platform), in.AccountID, derivedOneID)
		if id, err := o.sessionRepo.UpsertByOneID(ctx, string(in.Platform), in.AccountID, derivedOneID, derivedOneID, in.GroupName, in.Content, &now); err != nil {
			staleID := fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), safeMessageID(in.MessageID))
			logger.Ctx(ctx).Warn().Err(err).Str("stable_id", stableSessionID).Str("stale_id", staleID).
				Msg("[Orchestrator] 群聊 UpsertByOneID 失败，降级用 stale session_id")
			session := &model.CustomerSession{
				SessionID:     staleID,
				Platform:      in.Platform,
				AccountID:     in.AccountID,
				UserID:        derivedOneID,
				OneID:         derivedOneID,
				UserName:      in.GroupName,
				Status:        model.SessionStatusPending,
				Priority:      0,
				LastMessage:   in.Content,
				LastMessageAt: &now,
				LastMessageBy: "user",
				HandlerType:   model.HandlerTypeAI,
			}
			if err := o.sessionRepo.Create(ctx, session); err != nil {
				return nil, err
			}
			return session, nil
		} else if id != "" {
			return o.sessionRepo.GetByIDString(ctx, id)
		}
		staleID := fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), safeMessageID(in.MessageID))
		session := &model.CustomerSession{
			SessionID:     staleID,
			Platform:      in.Platform,
			AccountID:     in.AccountID,
			UserID:        derivedOneID,
			OneID:         derivedOneID,
			UserName:      in.GroupName,
			Status:        model.SessionStatusPending,
			Priority:      0,
			LastMessage:   in.Content,
			LastMessageAt: &now,
			LastMessageBy: "user",
			HandlerType:   model.HandlerTypeAI,
		}
		if err := o.sessionRepo.Create(ctx, session); err != nil {
			return nil, err
		}
		return session, nil
	}

	if in.OneID != "" {
		if existing, err := o.sessionRepo.GetActiveByOneID(ctx, in.OneID); err == nil && existing != nil {
			return existing, nil
		}
	}

	if existing, err := o.sessionRepo.GetActiveByUserID(ctx, in.SenderID); err == nil && existing != nil {
		return existing, nil
	}

	derivedOneID := in.OneID
	if derivedOneID == "" {
		derivedOneID = fmt.Sprintf("%s:%s", in.Platform, in.SenderID)
	}
	now := time.Now()
	stableSessionID := fmt.Sprintf("sess_%s_%s_%s", string(in.Platform), in.AccountID, derivedOneID)
	if id, err := o.sessionRepo.UpsertByOneID(ctx, string(in.Platform), in.AccountID, derivedOneID, in.SenderID, in.SenderName, in.Content, &now); err != nil {
		staleID := fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), safeMessageID(in.MessageID))
		logger.Ctx(ctx).Warn().Err(err).Str("stable_id", stableSessionID).Str("stale_id", staleID).Msg("[Orchestrator] UpsertByOneID 失败，降级用 stale session_id")
		session := &model.CustomerSession{
			SessionID:     staleID,
			Platform:      in.Platform,
			AccountID:     in.AccountID,
			UserID:        in.SenderID,
			OneID:         derivedOneID,
			UserName:      in.SenderName,
			Status:        model.SessionStatusPending,
			Priority:      0,
			LastMessage:   in.Content,
			LastMessageAt: &now,
			LastMessageBy: "user",
			HandlerType:   model.HandlerTypeAI,
		}
		if err := o.sessionRepo.Create(ctx, session); err != nil {
			return nil, err
		}
		return session, nil
	} else if id != "" {
		return o.sessionRepo.GetByIDString(ctx, id)
	}
	staleID := fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), safeMessageID(in.MessageID))
	session := &model.CustomerSession{
		SessionID:     staleID,
		Platform:      in.Platform,
		AccountID:     in.AccountID,
		UserID:        in.SenderID,
		OneID:         derivedOneID,
		UserName:      in.SenderName,
		Status:        model.SessionStatusPending,
		Priority:      0,
		LastMessage:   in.Content,
		LastMessageAt: &now,
		LastMessageBy: "user",
		HandlerType:   model.HandlerTypeAI,
	}
	if err := o.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// saveInboundMessage 保存入站消息
//
// 去重逻辑（修复 chat 访客端双保存 bug）：
//
//	chat_visitor_service.SendMessage 也保存了 user 消息，再调 HandleIncomingWithAgent
//	会导致同一条用户消息被保存两次（数据库中 2 条 row，前端列表重复）。
//	解决：在保存前查最近 5 秒内是否已存在同 (session, content, sender) 的消息，
//	若有则跳过保存，返回已存在消息的引用。
func (o *SmartCSOrchestrator) saveInboundMessage(ctx context.Context, session *model.CustomerSession, in *IncomingContext) error {
	if existing, _ := o.messageRepo.FindRecentDuplicate(ctx, session.SessionID, "user", in.SenderID, in.Content, 5*time.Second); existing != nil {
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
	return o.messageRepo.Create(ctx, msg)
}

// saveOutboundMessage 保存出站消息（去重：避免与 visitor 端双保存）
func (o *SmartCSOrchestrator) saveOutboundMessage(ctx context.Context, session *model.CustomerSession, content string, aiGenerated bool) error {
	senderType := "agent"
	if aiGenerated {
		senderType = "ai"
	}
	if existing, _ := o.messageRepo.FindRecentDuplicate(ctx, session.SessionID, senderType, "ai_assistant", content, 5*time.Second); existing != nil {
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
	return o.messageRepo.Create(ctx, msg)
}

// saveAISuggestion 保存 AI 建议供座席参考
func (o *SmartCSOrchestrator) saveAISuggestion(ctx context.Context, sessionID string, resp *SalesResponse) uint {
	if o.suggestionRepo == nil || resp == nil || resp.Reply == "" {
		return 0
	}
	confidence := o.extractConfidence(context.Background(), resp)
	suggestion := &model.AISuggestion{
		SessionID:  sessionID,
		Suggestion: resp.Reply,
		Confidence: confidence,
		Source:     "sales_engine",
	}
	if err := o.suggestionRepo.Create(ctx, suggestion); err != nil {
		return 0
	}
	return suggestion.ID
}

// markSuggestionUsed 标记建议被采用
func (o *SmartCSOrchestrator) markSuggestionUsed(ctx context.Context, id uint) error {
	if id == 0 || o.suggestionRepo == nil {
		return nil
	}
	return o.suggestionRepo.MarkAsUsed(ctx, id, 0)
}

// transferToHuman 转人工（联动 SessionAssignmentService 真正分配在线座席）
func (o *SmartCSOrchestrator) transferToHuman(ctx context.Context, session *model.CustomerSession, reason string) error {
	session.Status = model.SessionStatusWaiting
	session.HandlerType = model.HandlerTypeHuman
	session.LastMessage = reason
	now := time.Now()
	session.LastMessageAt = &now
	session.AIReplyCount = 0
	if err := o.sessionRepo.Update(ctx, session); err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[transferToHuman] update session status failed")
		return err
	}

	sysMsg := &model.SessionMessage{
		SessionID:   session.SessionID,
		Content:     "【系统】" + reason,
		ContentType: model.MessageTypeText,
		SenderType:  "system",
		SenderID:    "system",
		SenderName:  "系统",
	}
	if err := o.messageRepo.Create(ctx, sysMsg); err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("[transferToHuman] create system message failed")
	}

	if o.assignmentSvc != nil {
		if err := o.assignmentSvc.autoAssignToAgent(ctx, session, reason); err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[transferToHuman] autoAssignToAgent failed")
		}
	}
	return nil
}

// incrementAIReplyCount 增加 AI 回复计数
func (o *SmartCSOrchestrator) incrementAIReplyCount(ctx context.Context, session *model.CustomerSession) error {
	session.AIReplyCount++
	session.Status = model.SessionStatusAIHandling
	now := time.Now()
	session.LastMessageAt = &now
	return o.sessionRepo.Update(ctx, session)
}

// isAgentOnline 座席是否在线
func (o *SmartCSOrchestrator) isAgentOnline(ctx context.Context, agentID uint) (online bool) {
	defer func() {
		if r := recover(); r != nil {
			online = false
		}
	}()
	if o.agentRepo == nil {
		return false
	}
	agent, err := o.agentRepo.GetByAgentID(ctx, agentID)
	if err != nil || agent == nil {
		return false
	}
	return agent.Status == "online" || agent.Status == "busy"
}

// isUrgentOrComplaint 是否紧急/投诉
func (o *SmartCSOrchestrator) isUrgentOrComplaint(ctx context.Context, content string) bool {
	return MatchUrgentKeywords(content)
}

// extractConfidence 从 SalesResponse 提取置信度
func (o *SmartCSOrchestrator) extractConfidence(ctx context.Context, resp *SalesResponse) float64 {
	if resp == nil {
		return 0
	}
	if resp.Intent != nil && resp.Intent.Confidence > 0 {
		return resp.Intent.Confidence
	}
	score := 0.5
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

// AgentTakeover 座席接管 AI 会话
// 当座席认为 AI 回复不合适时，可主动接管会话
func (o *SmartCSOrchestrator) AgentTakeover(ctx context.Context, sessionID string, agentID uint) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("session not found (repo panic): %s, %v", sessionID, r)
		}
	}()
	session, err := o.sessionRepo.GetBySessionID(ctx, sessionID)
	if err != nil || session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	session.HandlerType = model.HandlerTypeHuman
	session.AgentID = agentID
	session.Status = model.SessionStatusHumanHandling
	now := time.Now()
	session.LastMessageAt = &now
	return o.sessionRepo.Update(ctx, session)
}

// AgentReply 座席手动回复（覆盖 AI 建议）
func (o *SmartCSOrchestrator) AgentReply(ctx context.Context, sessionID string, agentID uint, content string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("session not found (repo panic): %s, %v", sessionID, r)
		}
	}()
	session, err := o.sessionRepo.GetBySessionID(ctx, sessionID)
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
	if err := o.messageRepo.Create(ctx, msg); err != nil {
		return err
	}
	session.HumanReplyCount++
	session.LastMessage = content
	now := time.Now()
	session.LastMessageAt = &now
	session.LastMessageBy = "agent"
	return o.sessionRepo.Update(ctx, session)
}
