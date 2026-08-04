package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	agent_runtime "marketing/internal/aiagent/agent/runtime"
	"marketing/internal/cache"
	"marketing/internal/channelbot/telegram"
	"marketing/internal/channelbot/whatsapp"
	"marketing/internal/model"
	"marketing/internal/pkg/trace"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// WebhookService 多渠道 Webhook 服务
// 对应 SYSTEM_AUDIT_REPORT_20260715_V3
// 支持渠道：抖音/快手/小红书/闲鱼/TikTok/微信/企业微信/WhatsApp/Telegram/自建
// 核心能力：
//  1. 验签（HMAC-SHA256 + 微信 SHA1 + 企微 SHA1 + 加解密）
//  2. 幂等（event_id 去重 + 5 分钟 TTL）
//  3. 异步分发（worker pool）
//  4. 重试（指数退避，最大 3 次）
//  5. 限流（per account token bucket）
//  6. 审计（记录 raw payload、来源 IP）
//  7. 业务分发（按 channel 路由到对应 Service，进入消息中台/收件箱/智能体）
//  8. TG 入群事件自动触发 智能体流程（new_chat_members / left_chat_member）
type WebhookService struct {
	db          *gorm.DB // 保留以维持与外部组件（如 WeComIntegrationService）的兼容
	eventRepo   *repository.WebhookEventRepository
	accountRepo *repository.IntegrationAccountRepository

	// Phase 1：企业微信完整入站链路
	wecomRepo   *repository.WeComAccountRepository
	integration *WeComIntegrationService

	// Phase 2-4：WhatsApp Cloud / Telegram / 飞书 集成服务
	feishuIntegration *FeishuIntegrationService
	tgIntegration     *TelegramIntegrationService
	waIntegration     *WhatsAppCloudIntegrationService

	// TG 账号仓库（用于查询 Bot Token + AIAgentEnabled）
	telegramRepo *repository.TelegramAccountRepository

	// 渠道账号仓库缓存：避免在每条消息的 shouldTriggerAI 中重复构造
	feishuRepo *repository.FeishuAccountRepository
	waRepo     *repository.WhatsAppCloudAccountRepository

	// 消息中台 / 收件箱会话 / 统一消息 仓库
	messageHubRepo *repository.MessageHubRepository
	inboxConvRepo  *repository.InboxConversationRepository
	unifiedMsgRepo repository.UnifiedMessageRepository

	// 线索仓库：Telegram 群发言自动挖掘为销售线索/商机（去重 + 意向分增量更新）
	clueRepo repository.ClueRepository

	// 渠道入站消息中台：各渠道适配器（telegram/whatsapp）经 core.IngressHandler 统一走
	// InboxIngressService.HandleIngressMessage（标准化 + 人工锁 + AI 串行锁 + 落库 + 触发 AgentRuntime）
	ingressSvc *InboxIngressService

	// Phase 1：智能体引擎（可选注入，nil 时仅入库不入 AI）
	salesEngine *SalesEngine
	// 智能体统一编排器（LLM + 客服座席结合体）
	// 注入后优先走 HandleIncoming 9 步编排（会话/消息/AI决策/转人工/建议保存）
	// 未注入时回退到 salesEngine.Handle 直接调用
	smartOrchestrator *SmartCSOrchestrator

	// 多 AI 智能体路由：按渠道账号查询绑定的智能体上下文
	// 注入后 triggerSalesEngine 会先加载智能体上下文，再调用 engine.HandleWithAgent
	// 未注入时回退到默认配置（DefaultSalesEngineConfig）
	agentBindingSvc *ChannelAgentBindingService

	mu        sync.Mutex // 仅用于 Stop 的 stopped 标志保护
	rlMu      sync.Mutex // 限流桶专用锁
	rlBuckets map[string]*tokenBucket

	workerCount int
	queue       chan *webhookJob
	wg          sync.WaitGroup
	stopCh      chan struct{}
	stopped     bool

	// 推理并发信号量：限制同时进行的本地 LLM 生成数，保护单节点推理栈。
	replySem chan struct{}

	// 注：TG 群「发现线索主动触达」冷却已迁至全局缓存（见 tgLeadOutreachAllowed），
	// 走 cache.GetGlobalCache()，REDIS_HOST 配置时为 Redis 共享后端（多实例防刷屏一致），
	// 否则为内存单例（单实例安全），不再需要进程内 map + 锁。
}

// tgDispatchExtra dispatchTelegram 的附加输出：携带群聊场景下车控/线索相关判定，
// 供上层（handleJob）决定「是否触达 / 是否主动回复」。非 TG 渠道该值为 nil。
type tgDispatchExtra struct {
	Mentioned      bool // 该消息是否 @提及了本机器人（群内「@bot 才回复」的触发条件）
	NewOpportunity bool // 该消息挖掘后是否让发言者「新晋为商机」
}

type webhookJob struct {
	event  *model.WebhookEvent
	raw    []byte
	header map[string]string
	source string
	// 解析后的入站消息（用于业务分发）
	channel WebhookChannel
	account string
	payload *ParsedPayload
}

type tokenBucket struct {
	mu         sync.Mutex
	capacity   int
	refillRate float64
	tokens     float64
	lastRefill time.Time
}

func (b *tokenBucket) allow(ctx context.Context) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > float64(b.capacity) {
		b.tokens = float64(b.capacity)
	}
	b.lastRefill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

const (
	WebhookDedupTTL    = 5 * time.Minute
	WebhookWorkerCount = 4 // 接入 worker 数（从 WEBHOOK_WORKER_COUNT 覆盖）
	WebhookQueueSize   = 512
	WebhookRateLimit   = 30
	WebhookRateBurst   = 60
	WebhookMaxRetries  = 3
	// WebhookReplyConcurrency 同时进行的 AI 生成（本地 LLM 推理）上限。
	// 与接入 worker 解耦：接入 worker 只做轻量入队，AI 生成交给本有界池，
	// 推理饱和时 AI 任务排队而非丢弃。可用 WEBHOOK_REPLY_CONCURRENCY 覆盖。
	WebhookReplyConcurrency = 32
)

// webhookEnvInt 读取整型环境变量，解析失败则使用默认值。
func webhookEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// WebhookChannel 渠道标识
type WebhookChannel string

const (
	ChannelDouyin      WebhookChannel = "douyin"
	ChannelKuaishou    WebhookChannel = "kuaishou"
	ChannelXiaohongshu WebhookChannel = "xiaohongshu"
	ChannelXianyu      WebhookChannel = "xianyu"
	ChannelTiktok      WebhookChannel = "tiktok"
	ChannelWechat      WebhookChannel = "wechat"
	ChannelWeCom       WebhookChannel = "wecom"
	ChannelWhatsapp    WebhookChannel = "whatsapp"
	ChannelTelegram    WebhookChannel = "telegram"
	ChannelFeishu      WebhookChannel = "feishu"
	ChannelCustom      WebhookChannel = "custom"
)

// NewWebhookService 构造 Webhook 服务
func NewWebhookService(db *gorm.DB) *WebhookService {
	wecomRepo := repository.NewWeComAccountRepository()
	if db != nil {
		wecomRepo.SetDB(context.Background(), db)
	}
	accountRepo := repository.NewIntegrationAccountRepository()
	if db != nil {
		accountRepo.SetDB(context.Background(), db)
	}
	eventRepo := repository.NewWebhookEventRepository()
	if db != nil {
		repository.SetWebhookEventRepoDB(eventRepo, db)
	}
	telegramRepo := repository.NewTelegramAccountRepository()
	if db != nil {
		telegramRepo.SetDB(context.Background(), db)
	}
	feishuRepo := repository.NewFeishuAccountRepository()
	if db != nil {
		feishuRepo.SetDB(context.Background(), db)
	}
	waRepo := repository.NewWhatsAppCloudAccountRepository()
	if db != nil {
		waRepo.SetDB(context.Background(), db)
	}
	// 仅在 db 非空时初始化新仓库，保持与原 s.db == nil 守卫等价的 nil-safe 语义
	var messageHubRepo *repository.MessageHubRepository
	if db != nil {
		messageHubRepo = repository.NewMessageHubRepository()
		repository.SetMessageHubRepoDB(messageHubRepo, db)
	}
	var inboxConvRepo *repository.InboxConversationRepository
	if db != nil {
		inboxConvRepo = repository.NewInboxConversationRepository()
		repository.SetInboxConversationRepoDB(inboxConvRepo, db)
	}
	var unifiedMsgRepo repository.UnifiedMessageRepository
	if db != nil {
		unifiedMsgRepo = repository.NewUnifiedMessageRepositoryWithDB(db)
	}
	s := &WebhookService{
		db:             db,
		eventRepo:      eventRepo,
		accountRepo:    accountRepo,
		wecomRepo:      wecomRepo,
		integration:    NewWeComIntegrationService(db),
		telegramRepo:   telegramRepo,
		feishuRepo:     feishuRepo,
		waRepo:         waRepo,
		messageHubRepo: messageHubRepo,
		inboxConvRepo:  inboxConvRepo,
		unifiedMsgRepo: unifiedMsgRepo,
		ingressSvc:     NewInboxIngressServiceWithDB(db, nil),
		rlBuckets:      make(map[string]*tokenBucket),
		workerCount:    webhookEnvInt("WEBHOOK_WORKER_COUNT", WebhookWorkerCount),
		queue:          make(chan *webhookJob, webhookEnvInt("WEBHOOK_QUEUE_SIZE", WebhookQueueSize)),
		stopCh:         make(chan struct{}),
		replySem:       make(chan struct{}, webhookEnvInt("WEBHOOK_REPLY_CONCURRENCY", WebhookReplyConcurrency)),
	}
	s.startWorkers(context.Background())
	return s
}

// SetSalesEngine 注入 智能体引擎（解耦依赖，可选）
func (s *WebhookService) SetSalesEngine(ctx context.Context, e *SalesEngine) {
	s.salesEngine = e
}

// ensureReposFromDB 在 struct 直接构造（如测试中 &WebhookService{db: db}）时，
// 按需从 s.db 派生新的仓库实例，保持与原 s.db 直用等价语义。
func (s *WebhookService) ensureReposFromDB(ctx context.Context) {
	if s.db == nil {
		return
	}
	if s.messageHubRepo == nil {
		s.messageHubRepo = repository.NewMessageHubRepository()
		repository.SetMessageHubRepoDB(s.messageHubRepo, s.db)
	}
	if s.inboxConvRepo == nil {
		s.inboxConvRepo = repository.NewInboxConversationRepository()
		repository.SetInboxConversationRepoDB(s.inboxConvRepo, s.db)
	}
	if s.unifiedMsgRepo == nil {
		s.unifiedMsgRepo = repository.NewUnifiedMessageRepositoryWithDB(s.db)
	}
	if s.clueRepo == nil {
		s.clueRepo = repository.NewClueRepositoryWithDB(s.db)
	}
}

// SetSmartOrchestrator 注入智能体统一编排器
// 注入后 Webhook 入站消息会走 SmartCSOrchestrator.HandleIncoming 9 步编排：
// 会话查找/创建 → 入站消息保存 → 座席接管检查 → AI 连续上限检查 → 紧急投诉检查
// → SalesEngine.Handle 8 步链路 → AI 建议保存 → 置信度决策(AI 自动回复/转人工)
func (s *WebhookService) SetSmartOrchestrator(ctx context.Context, o *SmartCSOrchestrator) {
	s.smartOrchestrator = o
}

// SetAgentBindingService 注入渠道智能体绑定服务
// 注入后 triggerSalesEngine 会先按 (channel_type, account_id) 查询绑定的智能体上下文，
// 再调用 SalesEngine.HandleWithAgent 按智能体配置执行 9 步链路
// 未注入时回退到 DefaultSalesEngineConfig 默认行为
func (s *WebhookService) SetAgentBindingService(ctx context.Context, svc *ChannelAgentBindingService) {
	s.agentBindingSvc = svc
}

// webhookIngressAdapter 把 InboxIngressService 适配为 channelbot/core.IngressHandler。
// 收敛返回值（*InboxIngressResult 丢弃）并对 message_hub 唯一约束冲突做幂等容忍，
// 与原 dispatch 直接 messageHubRepo.Create 时吞掉 UNIQUE/duplicate 的行为等价。
type webhookIngressAdapter struct{ svc *InboxIngressService }

// HandleIngressMessage 实现 core.IngressHandler
func (a webhookIngressAdapter) HandleIngressMessage(ctx context.Context, event *model.MessageEvent) error {
	if a.svc == nil {
		return nil
	}
	_, err := a.svc.HandleIngressMessage(ctx, event)
	if err != nil && (strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate")) {
		// 渠道重复投递视为成功（中台已落库过同 EventID 的消息）
		return nil
	}
	return err
}

// ingressHandler 返回供渠道适配器（telegram/whatsapp）调用的入站中台处理器。
// 未注入 ingressSvc 时返回零值适配器（HandleIngressMessage 无副作用降级）。
func (s *WebhookService) ingressHandler(ctx context.Context) webhookIngressAdapter {
	return webhookIngressAdapter{svc: s.ingressSvc}
}

// 轻量级 repository 包装，支持 db 注入（不破坏既有全局单例）
type webhookEventRepo struct {
	db *gorm.DB
}

func newWebhookEventRepoWithDB(db *gorm.DB) *repository.WebhookEventRepository {
	// 注入到全局 repository 的 db 字段
	r := repository.NewWebhookEventRepository()
	if db != nil {
		// 通过 SetDB 注入
		repository.SetWebhookEventRepoDB(r, db)
	}
	return r
}

func newIntegrationAccountRepoWithDB(db *gorm.DB) *repository.IntegrationAccountRepository {
	r := repository.NewIntegrationAccountRepository()
	if db != nil {
		repository.SetIntegrationAccountRepoDB(r, db)
	}
	return r
}

// Stop 关闭 worker
func (s *WebhookService) Stop(ctx context.Context) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.stopCh)
	s.mu.Unlock()
	// 在锁外等待，避免与 worker 内部可能的回调产生死锁
	// 通过 context 超时防止 worker 处于 retryWithBackoff 长延迟时无限阻塞
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// 全部 worker 正常退出
	case <-time.After(2 * time.Second):
		// worker 卡在 retryWithBackoff 等长延迟，强制返回
		// （测试环境允许；生产环境应保证 worker 不在长延迟中）
	}
}

func (s *WebhookService) startWorkers(ctx context.Context) {
	for i := 0; i < s.workerCount; i++ {
		s.wg.Add(1)
		go s.worker(ctx, i)
	}
}

func (s *WebhookService) worker(ctx context.Context, id int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		case job, ok := <-s.queue:
			if !ok {
				return
			}
			s.handleJob(ctx, job)
		}
	}
}

// groupNameFromHub 从 MessageHub 提取群名：Extra 冗余为第一来源（群名不在 Hub 顶层列）。
func groupNameFromHub(hub *model.MessageHub) string {
	if hub == nil || hub.Extra == nil {
		return ""
	}
	if v, ok := hub.Extra["group_name"]; ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return ""
}

// =================== 入口 ===================

// ReceiveRequest 渠道回调请求通用结构
type ReceiveRequest struct {
	Channel   WebhookChannel
	AccountID string
	Body      []byte
	Headers   map[string]string
	SourceIP  string
	// WeCom 加解密使用：加密的 body 解析时使用
	Query map[string]string
}

// ReceiveResult 处理结果
type ReceiveResult struct {
	Accepted   bool   `json:"accepted"`
	EventID    string `json:"event_id"`
	Duplicate  bool   `json:"duplicate"`
	RateLimit  bool   `json:"rate_limit"`
	VerifyFail bool   `json:"verify_failed"`
	Reason     string `json:"reason,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	// 业务分发结果（用于 智能体异步触发）
	Dispatched   bool   `json:"dispatched,omitempty"`
	HubMessageID string `json:"hub_message_id,omitempty"`
}

// Receive 接收 webhook
func (s *WebhookService) Receive(ctx context.Context, req *ReceiveRequest) (*ReceiveResult, error) {
	if req == nil || len(req.Body) == 0 {
		return &ReceiveResult{Accepted: false, Reason: "empty body"}, nil
	}
	if req.Channel == "" {
		req.Channel = ChannelCustom
	}
	if req.AccountID == "" {
		return &ReceiveResult{Accepted: false, Reason: "missing account_id"}, nil
	}

	// 1) 验签
	verified, err := s.Verify(ctx, req.Channel, req.AccountID, req.Body, req.Headers, req.Query)
	if err != nil {
		logger.Errorf("[Webhook] 验签异常 channel=%s account=%s: %v", req.Channel, req.AccountID, err)
		return &ReceiveResult{Accepted: false, VerifyFail: true, Reason: "verify error: " + err.Error()}, nil
	}
	if !verified {
		return &ReceiveResult{Accepted: false, VerifyFail: true, Reason: "signature mismatch"}, nil
	}

	// 2) 解析基础字段
	payload, err := s.ParsePayload(ctx, req.Channel, req.Body)
	if err != nil {
		return &ReceiveResult{Accepted: false, Reason: "parse error: " + err.Error()}, nil
	}
	if payload.EventID == "" {
		payload.EventID = s.generateEventID(ctx, req.Channel, req.AccountID, req.Body)
	}
	if payload.EventType == "" {
		payload.EventType = "unknown"
	}

	// 3) 幂等去重
	if s.isDuplicate(ctx, payload.EventID) {
		return &ReceiveResult{
			Accepted:  true,
			EventID:   payload.EventID,
			Duplicate: true,
		}, nil
	}

	// 4) 限流
	key := string(req.Channel) + ":" + req.AccountID
	if !s.allowRate(ctx, key) {
		return &ReceiveResult{Accepted: false, RateLimit: true, Reason: "rate limited"}, nil
	}

	// 5) 持久化事件记录
	evt := &model.WebhookEvent{
		Platform:  string(req.Channel),
		EventID:   payload.EventID,
		EventType: payload.EventType,
		RawData:   s.TruncateForStore(ctx, req.Body),
		Processed: false,
	}
	if s.eventRepo != nil {
		if err := s.eventRepo.Create(ctx, evt); err != nil {
			return &ReceiveResult{Accepted: false, Reason: "persist error: " + err.Error()}, nil
		}
	}

	// 6) 入异步队列（携带解析后的 payload 减少 worker 重复解析）
	job := &webhookJob{
		event:   evt,
		raw:     req.Body,
		header:  req.Headers,
		source:  req.SourceIP,
		channel: req.Channel,
		account: req.AccountID,
		payload: payload,
	}
	select {
	case s.queue <- job:
	default:
		s.handleJob(ctx, job)
	}

	return &ReceiveResult{
		Accepted:  true,
		EventID:   payload.EventID,
		EventType: payload.EventType,
	}, nil
}

// =================== 验签 ===================

// Verify 验签
// query 用于 WeCom URL 验证（msg_signature/timestamp/nonce/echoest）
func (s *WebhookService) Verify(ctx context.Context, channel WebhookChannel, accountID string, body []byte, headers map[string]string, query map[string]string) (bool, error) {
	switch channel {
	case ChannelWeCom:
		token, aesKey, err := s.getWeComSecrets(ctx, accountID)
		if err != nil || token == "" {
			return false, fmt.Errorf("wecom token missing: %v", err)
		}
		return verifyWeCom(token, aesKey, body, query)
	case ChannelWechat:
		token, _ := s.getWechatSecrets(ctx, accountID)
		return verifyWechat(token, body, headers), nil
	case ChannelDouyin, ChannelTiktok:
		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		return verifyHMAC(secret, body, headers, "X-Lark-Signature", "X-Douyin-Signature", "Signature"), nil
	case ChannelTelegram:
		// TG 验签：Telegram Bot API 通过 X-Telegram-Bot-Api-Secret-Token 头传递 webhook secret
		// secret 来自 TelegramAccount.WebhookSecret（在 setWebhook 时配置）；验签委托独立包 channelbot/telegram
		secret := s.getTelegramWebhookSecret(ctx, accountID)
		if secret == "" {
			// 未配置 secret：开发/测试环境放行；生产环境必须配置，否则拒绝伪造 webhook
			if os.Getenv("GIN_MODE") == "release" {
				return false, errors.New("telegram webhook secret 未配置，生产环境拒绝放行")
			}
			return true, nil
		}
		headerSecret := headers["X-Telegram-Bot-Api-Secret-Token"]
		if headerSecret == "" {
			headerSecret = headers["x-telegram-bot-api-secret-token"]
		}
		if headerSecret == "" {
			return false, errors.New("missing X-Telegram-Bot-Api-Secret-Token header")
		}
		return telegram.VerifyWebhook(secret, headerSecret), nil
	case ChannelKuaishou, ChannelXiaohongshu, ChannelXianyu, ChannelFeishu:
		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		return verifyHMAC(secret, body, headers, "X-Signature", "Signature", "X-Hub-Signature-256", "Encrypt"), nil
	case ChannelWhatsapp:
		// WA 验签：X-Hub-Signature-256（HMAC-SHA256，app secret 对 raw body 签名）；委托独立包 channelbot/whatsapp
		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		if secret == "" {
			if os.Getenv("GIN_MODE") == "release" {
				return false, errors.New("whatsapp app secret 未配置，生产环境拒绝放行")
			}
			return true, nil
		}
		return whatsapp.VerifyWebhook(secret, body, headers["X-Hub-Signature-256"]), nil
	default:
		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		if secret == "" {
			// 未配置 secret：开发/测试环境放行；生产环境必须配置，否则拒绝伪造 webhook
			// （此前 default 分支无条件放行，攻击者可在 secret 未配置时伪造任意入站消息，P1 缺陷）
			if os.Getenv("GIN_MODE") == "release" {
				return false, errors.New("webhook secret 未配置，生产环境拒绝放行(channel=" + string(channel) + ")")
			}
			return true, nil
		}
		return verifyHMAC(secret, body, headers, "X-Signature", "Signature", "X-Hub-Signature-256"), nil
	}
}

// verifyWeCom 企微验签 + 可选解密
// body 形如 {"encrypt":"...", "msg_signature":"...", "timestamp":"...", "nonce":"..."}
// 验证：sha1(sort([token, timestamp, nonce])) 与 msg_signature 比对
// 解密：用 EncodingAESKey 解出明文（含 ToUserName/FromUserName/MsgType/Content 等）
func verifyWeCom(token, aesKey string, body []byte, query map[string]string) (bool, error) {
	if token == "" {
		return false, errors.New("missing token")
	}
	// body 中取 msg_signature/timestamp/nonce
	var p struct {
		MsgSignature string `json:"msg_signature"`
		Timestamp    string `json:"timestamp"`
		Nonce        string `json:"nonce"`
		Encrypt      string `json:"encrypt"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		// 也允许 query 携带
		if query != nil {
			p.MsgSignature = query["msg_signature"]
			p.Timestamp = query["timestamp"]
			p.Nonce = query["nonce"]
		}
	}
	if p.MsgSignature == "" && query != nil {
		p.MsgSignature = query["msg_signature"]
		p.Timestamp = query["timestamp"]
		p.Nonce = query["nonce"]
	}
	if p.Timestamp == "" {
		p.Timestamp = query["timestamp"]
	}
	if p.Nonce == "" {
		p.Nonce = query["nonce"]
	}
	if p.MsgSignature == "" || p.Timestamp == "" || p.Nonce == "" {
		return false, errors.New("missing msg_signature/timestamp/nonce")
	}
	parts := []string{token, p.Timestamp, p.Nonce}
	sortStrings(parts)
	h := sha1Hex([]byte(strings.Join(parts, "")))
	if subtle.ConstantTimeCompare([]byte(h), []byte(p.MsgSignature)) != 1 {
		return false, errors.New("signature mismatch")
	}
	return true, nil
}

// sortStrings 字符串原地排序（避免引入 sort 包依赖）
func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

func verifyWechat(token string, body []byte, headers map[string]string) bool {
	if token == "" {
		return false
	}
	ts := headers["X-Wechat-Timestamp"]
	nonce := headers["X-Wechat-Nonce"]
	sig := headers["X-Wechat-Signature"]
	if sig == "" {
		sig = headers["signature"]
	}
	if ts == "" || nonce == "" || sig == "" {
		return false
	}
	// 微信公众号官方：sha1(sort([token, timestamp, nonce]))
	parts := []string{token, ts, nonce}
	sortStrings(parts)
	h := sha1Hex([]byte(strings.Join(parts, "")))
	return subtle.ConstantTimeCompare([]byte(h), []byte(sig)) == 1
}

func verifyHMAC(secret string, body []byte, headers map[string]string, headerNames ...string) bool {
	if secret == "" {
		return false
	}
	var sig string
	for _, h := range headerNames {
		if v := headers[h]; v != "" {
			sig = v
			break
		}
	}
	if sig == "" {
		return false
	}
	sig = strings.TrimPrefix(sig, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) == 1
}

func sha1Hex(b []byte) string {
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}

// =================== 企微消息解密（标准 AES-CBC + PKCS#7） ===================

// DecryptWeComMessage 用 EncodingAESKey 解密企微 encrypt 字段
// 参考官方文档：https://developer.work.weixin.qq.com/document/path/90968
func DecryptWeComMessage(aesKey, encrypted string) ([]byte, error) {
	if len(aesKey) != 43 {
		return nil, fmt.Errorf("invalid EncodingAESKey length: %d", len(aesKey))
	}
	// 补 '='
	key := aesKey + "="
	keyB, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	cipherB, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decode cipher: %w", err)
	}
	if len(cipherB) < 16 || len(cipherB)%16 != 0 {
		return nil, fmt.Errorf("invalid cipher length: %d", len(cipherB))
	}
	block, err := aes.NewCipher(keyB)
	if err != nil {
		return nil, err
	}
	iv := cipherB[:16]
	mode := cipher.NewCBCDecrypter(block, iv)
	plain := make([]byte, len(cipherB)-16)
	mode.CryptBlocks(plain, cipherB[16:])
	// PKCS#7 去填充
	plen := int(plain[len(plain)-1])
	if plen < 1 || plen > 32 {
		return nil, fmt.Errorf("invalid padding: %d", plen)
	}
	plain = plain[:len(plain)-plen]
	// 结构：rand(16) + msg_len(4) + msg + receiveid
	if len(plain) < 20 {
		return nil, fmt.Errorf("plain too short: %d", len(plain))
	}
	msgLen := int(binary.BigEndian.Uint32(plain[16:20]))
	if 20+msgLen > len(plain) {
		return nil, fmt.Errorf("msg_len overflow: %d", msgLen)
	}
	return plain[20 : 20+msgLen], nil
}

// VerifyURL 企微 URL 验证（首次启用回调配置时）
// echostr 加密后回传，需要用 EncodingAESKey 解密
// 验证：sha1(sort([token, timestamp, nonce, echostr_decrypted])) 与 signature 比对
// 参考官方文档：https://developer.work.weixin.qq.com/document/path/90968
func VerifyURL(token, aesKey, msgSignature, timestamp, nonce, echostr string) (string, error) {
	if len(aesKey) != 43 {
		return "", errors.New("invalid EncodingAESKey")
	}
	// 1) 解密 echostr（先解密才能拿到原文用于验签）
	plain, err := DecryptWeComMessage(aesKey, echostr)
	if err != nil {
		return "", fmt.Errorf("decrypt echostr: %w", err)
	}
	// 兼容结构体：rand(16)+len(4)+echostr
	plainStr := strings.TrimRight(string(plain), "\x00")
	if len(plainStr) > 20 {
		msgLen := int(binary.BigEndian.Uint32([]byte(plainStr)[16:20]))
		if 20+msgLen <= len(plainStr) {
			plainStr = plainStr[20 : 20+msgLen]
		}
	}
	// 2) 验签：用解密后的明文 + token + timestamp + nonce
	parts := []string{token, timestamp, nonce, plainStr}
	sortStrings(parts)
	h := sha1Hex([]byte(strings.Join(parts, "")))
	if subtle.ConstantTimeCompare([]byte(h), []byte(msgSignature)) != 1 {
		return "", errors.New("signature mismatch")
	}
	// 3) 返回明文
	return plainStr, nil
}

// =================== 解析 ===================

// ParsedPayload 解析后的基础结构
type ParsedPayload struct {
	EventID   string         `json:"event_id"`
	EventType string         `json:"event_type"`
	Sender    string         `json:"sender,omitempty"`
	Content   string         `json:"content,omitempty"`
	ChatID    string         `json:"chat_id,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// ParsePayload 解析渠道原始 payload 为通用结构
func (s *WebhookService) ParsePayload(ctx context.Context, channel WebhookChannel, body []byte) (*ParsedPayload, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	p := &ParsedPayload{Extra: raw}
	p.EventID = getString(raw, "event_id", "EventID", "msg_id", "MsgId")
	p.EventType = getString(raw, "event_type", "EventType", "event", "Event", "type", "Type", "msg_type", "MsgType")
	p.Sender = getString(raw, "from_user", "FromUserName", "sender", "sender_id", "from")
	p.Content = getString(raw, "content", "text", "Text", "Content", "message")
	p.ChatID = getString(raw, "chat_id", "ChatID", "conversation_id", "to_user", "ToUserName")
	return p, nil
}

// ToUnifiedMessage 转成统一消息
func (s *WebhookService) ToUnifiedMessage(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload) *model.UnifiedMessage {
	return &model.UnifiedMessage{
		MessageID:   s.genMessageID(ctx, channel, accountID, p),
		Platform:    model.Platform(channel),
		AccountID:   accountID,
		ChatID:      p.ChatID,
		SenderID:    p.Sender,
		Content:     p.Content,
		ContentType: model.MessageTypeText,
		RawData:     "",
		Status:      model.MessageStatusPending,
	}
}

// =================== 内部处理 ===================

func (s *WebhookService) handleJob(ctx context.Context, job *webhookJob) {
	channel := job.channel
	if channel == "" {
		channel = WebhookChannel(job.event.Platform)
	}
	payload := job.payload
	if payload == nil {
		var err error
		payload, err = s.ParsePayload(ctx, channel, job.raw)
		if err != nil {
			logger.Errorf("[Webhook] 处理失败 event=%s: %v", job.event.EventID, err)
			return
		}
	}

	// 1) 按渠道业务分发（先分发，回填 payload.Content/payload.Sender 等标准化字段，
	//    供后续统一消息入库与 AI 编排复用，避免嵌套/密文结构导致 content/sender 为空）。
	hubMsg, tgExtra, _ := s.dispatchToChannel(ctx, channel, job.account, payload, job.raw, job.header)

	// 2) 基础入库（统一消息）：依赖上一步回填的 payload.Content / payload.Sender
	um := s.ToUnifiedMessage(ctx, channel, job.account, payload)
	if um.AccountID == "" {
		um.AccountID = job.event.EventID
	}
	if err := s.dispatchToUnified(ctx, um); err != nil {
		s.retryWithBackoff(ctx, job, payload, err)
		return
	}

	// 3) 触发 智能体（如已注入 + 启用了 智能体开关）
	// TG 群聊的触达策略（避免无脑刷屏）：
	//   · 私聊 / 非 TG：AI 直接回复；
	//   · TG 群聊「@机器人」：响应模式，直接回复（需求：群里 @bot 时才回复）；
	//   · TG 群聊「发现新晋商机线索」：主动触达（需求：群聊发现符合线索可以机器人发消息），
	//     并受冷却限制（每个发言者限频），避免同一客户反复触达；
	//   · 入群欢迎仍由 dispatchTelegram 内的 triggerTelegramJoinSales 单独处理，此处跳过。
	triggerAI := hubMsg != nil && s.shouldTriggerAI(ctx, channel, job.account)
	if triggerAI {
		if channel != ChannelTelegram || !hubMsg.IsGroup {
			s.triggerSalesEngine(ctx, channel, job.account, payload, hubMsg)
		} else {
			mentioned := tgExtra != nil && tgExtra.Mentioned
			newOpp := tgExtra != nil && tgExtra.NewOpportunity
			switch {
			case mentioned:
				// 响应：群内 @机器人 才回复
				s.triggerSalesEngine(ctx, channel, job.account, payload, hubMsg)
			case newOpp && s.tgLeadOutreachAllowed(ctx, job.account, payload.ChatID, payload.Sender):
				// 主动触达：发现新晋商机线索时，机器人主动在群里发消息（带冷却）
				s.triggerSalesEngine(ctx, channel, job.account, payload, hubMsg)
			}
		}
	}

	s.markProcessed(ctx, job.event)
}

// dispatchToChannel 按渠道路由业务逻辑
func (s *WebhookService) dispatchToChannel(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, raw []byte, headers map[string]string) (*model.MessageHub, *tgDispatchExtra, error) {
	switch channel {
	case ChannelWeCom:
		hub, err := s.dispatchWeCom(ctx, accountID, p, raw, headers)
		return hub, nil, err
	case ChannelWhatsapp:
		hub, err := s.dispatchWhatsApp(ctx, accountID, p, raw)
		return hub, nil, err
	case ChannelTelegram:
		return s.dispatchTelegram(ctx, accountID, p, raw)
	case ChannelFeishu:
		hub, err := s.dispatchFeishu(ctx, accountID, p, raw)
		return hub, nil, err
	default:
		// 自定义渠道仅入统一消息即可
		return nil, nil, nil
	}
}

// dispatchWeCom 企业微信业务分发
func (s *WebhookService) dispatchWeCom(ctx context.Context, accountID string, p *ParsedPayload, raw []byte, headers map[string]string) (*model.MessageHub, error) {
	if s.integration == nil {
		return nil, nil
	}
	// 解析企微解密后的明文
	plain := s.parseWeComPlain(ctx, raw)
	if plain == nil {
		// 已是明文结构
		plain = p.Extra
	}
	if plain == nil {
		return nil, fmt.Errorf("wecom plain nil")
	}
	// 解析字段
	fromUser := getString(plain, "FromUserName", "from")
	fromName := getString(plain, "FromUserName")
	msgType := strings.ToLower(getString(plain, "MsgType", "msg_type"))
	content := getString(plain, "Content", "content", "Text", "text")
	if content == "" {
		// 不同类型消息内容字段不同
		switch msgType {
		case "image":
			content = "[图片]"
		case "voice":
			content = "[语音]"
		case "video":
			content = "[视频]"
		case "file":
			content = "[文件]"
		case "location":
			content = "[位置]"
		case "link":
			content = getString(plain, "Title", "title") + " " + getString(plain, "Url", "url")
		default:
			content = getString(plain, "Content", "content", "Text", "text")
		}
	}
	mediaID := getString(plain, "MediaId", "media_id")
	chatID := getString(plain, "ChatId", "chat_id")
	chatType := getString(plain, "ChatType", "chat_type") // single/group
	event := getString(plain, "Event", "event")
	msgID := getString(plain, "MsgId", "msg_id")

	// 兼容事件型消息（如进入应用、菜单点击等）
	if msgType == "event" {
		logger.Infof("[Webhook] wecom event=%s from=%s", event, fromUser)
		return nil, nil
	}

	// accountID 转 uint
	var accID uint64
	if v, err := strconv.ParseUint(accountID, 10, 64); err == nil && v > 0 {
		accID = v
	} else {
		// 兜底：取第一个启用的企微账号
		acc, gerr := s.wecomRepo.GetByMerchant(ctx)
		if gerr != nil || len(acc) == 0 {
			return nil, fmt.Errorf("invalid account_id")
		}
		accID = uint64(acc[0].ID)
	}

	hubMsg, _, err := s.integration.ReceiveCallback(ctx, &ReceiveCallbackRequest{
		AccountID: uint(accID),
		FromUser:  fromUser,
		FromName:  fromName,
		MsgType:   msgType,
		Content:   content,
		MsgID:     msgID,
		MediaID:   mediaID,
		ChatID:    chatID,
		ChatType:  chatType,
	})
	// 回填标准化字段，供下游 AI 编排（triggerSalesEngine）与出站（sendOutbound）复用：
	// ParsePayload 无法解析企微嵌套/密文结构，否则 content/sender 为空导致 AI 拿到空输入、出站目标为空。
	if err == nil {
		p.Content = content
		p.Sender = fromUser
		p.ChatID = chatID
	}
	return hubMsg, err
}

// parseWeComPlain 如果 body 包含 encrypt 字段，尝试解密（需要从 wecomRepo 拉 EncodingAESKey）
func (s *WebhookService) parseWeComPlain(ctx context.Context, raw []byte) map[string]any {
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	enc, _ := p["encrypt"].(string)
	if enc == "" {
		return p
	}
	// 需要获取 aesKey
	if s.wecomRepo == nil {
		return nil
	}
	// 这里使用 accountID 解析，由调用方注入；走 dispatchWeCom 的 raw
	// 注：因为 dispatchWeCom 中是按 accountID 路由，所以这里再次从 wecomRepo 拉
	accs, err := s.wecomRepo.GetByMerchant(ctx)
	if err != nil || len(accs) == 0 {
		return nil
	}
	var out map[string]any
	for _, a := range accs {
		if a.EncodingAESKey == "" {
			continue
		}
		plain, err := DecryptWeComMessage(a.EncodingAESKey, enc)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(plain, &out); err != nil {
			continue
		}
		return out
	}
	return nil
}

// dispatchWhatsApp 解析 WhatsApp Cloud API 推送并入消息中台
// payload 形如：
//
//	{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"messages":[...]}]}]}]}
func (s *WebhookService) dispatchWhatsApp(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB(ctx)
	// 解析 WA webhook（被动入站解析委托独立包 channelbot/whatsapp）
	waPayload, err := whatsapp.ParseWebhook(raw)
	if err != nil {
		return nil, fmt.Errorf("whatsapp parse: %w", err)
	}
	// 提取第一条有效消息
	for _, e := range waPayload.Entry {
		for _, c := range e.Changes {
			if len(c.Value.Messages) == 0 {
				continue
			}
			msg := c.Value.Messages[0]
			var content string
			msgType := msg.Type
			switch msg.Type {
			case "text":
				content = msg.Text.Body
			case "image":
				content = "[图片]"
			case "audio":
				content = "[语音]"
			case "video":
				content = "[视频]"
			case "document":
				content = "[文件]"
			default:
				content = "[" + msg.Type + "]"
			}
			// 查找联系人姓名
			name := msg.From
			for _, ct := range c.Value.Contacts {
				if ct.WAID == msg.From {
					name = ct.Profile.Name
					break
				}
			}
			// 写入消息中台
			hub := &model.MessageHub{
				Platform:       "whatsapp",
				AccountID:      accountID,
				MsgID:          msg.ID,
				Direction:      "inbound",
				SenderID:       msg.From,
				ConversationID: msg.From,
				MsgType:        msgType,
				Content:        content,
				SentAt:         time.Now(),
			}
			// 入站消息统一经消息中台处理（标准化 + 人工锁 + AI 串行锁 + 落库 + 触发 AgentRuntime）。
			// hub 仅作为收件箱会话 upsert 与上层 AI 触发的字段载体，不再单独 messageHubRepo.Create。
			if err := waPayload.Ingress(ctx, s.ingressHandler(ctx), accountID); err != nil {
				return nil, err
			}
			// 写收件箱会话
			s.upsertInboxFromHub(ctx, hub, name)
			// 回填标准化字段，供下游 AI 编排与出站复用：
			// ParsePayload 无法解析 WhatsApp 嵌套结构，否则 content/sender 为空导致 AI 空输入、出站目标（手机号）为空。
			p.Content = content
			p.Sender = msg.From
			p.ChatID = msg.From
			return hub, nil
		}
	}
	return nil, nil
}

func (s *WebhookService) dispatchTelegram(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, *tgDispatchExtra, error) {
	if s.db == nil {
		return nil, nil, nil
	}
	s.ensureReposFromDB(ctx)
	// 解析 TG 消息（被动入站解析委托独立包 channelbot/telegram，支持消息/编辑/回调/入群事件）
	tgPayload, err := telegram.ParseUpdate(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("telegram parse: %w", err)
	}
	// 从 Telegram 消息中提取文本内容，设置到 ParsedPayload.Content
	// 这样 triggerSmartOrchestrator 可以正确获取消息内容
	if tgPayload.Message != nil {
		text := tgPayload.Message.Text
		if text == "" {
			text = tgPayload.Message.Caption
		}
		if text != "" {
			p.Content = text
		}
		if tgPayload.Message.From != nil {
			p.Sender = strconv.FormatInt(tgPayload.Message.From.ID, 10)
		}
		if tgPayload.Message.Chat != nil {
			p.ChatID = strconv.FormatInt(tgPayload.Message.Chat.ID, 10)
		}
	}
	// 群内「@机器人 才回复」所需的 @username（注册 webhook 时经 getMe 自动回填；为空则降级为仅回复被@机器人消息）
	botUsername := s.getTelegramBotUsername(ctx, accountID)

	// ====================================================================
	// TG 入群事件：自动触发 智能体流程
	// ====================================================================
	// 当 new_chat_members 不为空时，为每个新成员构造一条入群事件消息，
	// 写入消息中台 + 触发 智能体流程（销售引擎主动发起欢迎+销售话术）。
	// 设计原则：入群即销售触点，不依赖用户主动 /start 或发消息。
	// ====================================================================
	if tgPayload.Message != nil && len(tgPayload.Message.NewChatMembers) > 0 && tgPayload.Message.Chat != nil {
		chatID := tgPayload.Message.Chat.ID
		chatType := tgPayload.Message.Chat.Type
		chatTitle := tgPayload.Message.Chat.Title
		if chatType == "" {
			chatType = "group"
		}
		chatIDStr := fmt.Sprintf("%d", chatID)
		isGroup := chatType == "group" || chatType == "supergroup"

		// 仅取第一个非 bot 新成员（TG 群通常一次入群一人；批量入群时取首位，其余会被 dedup）
		var newMember *telegram.TGUser
		for i := range tgPayload.Message.NewChatMembers {
			if !tgPayload.Message.NewChatMembers[i].IsBot {
				// tgPayload.Message.NewChatMembers 元素类型是匿名 struct，需先转换为 telegram.TGUser
				member := tgPayload.Message.NewChatMembers[i]
				newMember = &telegram.TGUser{
					ID:        member.ID,
					FirstName: member.FirstName,
					Username:  member.Username,
					IsBot:     member.IsBot,
				}
				break
			}
		}
		if newMember != nil {
			senderIDStr := fmt.Sprintf("%d", newMember.ID)
			fromName := newMember.FirstName
			if newMember.Username != "" {
				fromName = newMember.Username
			}
			// 构造入群事件消息（写入消息中台，便于审计/复盘）
			groupLabel := chatTitle
			if groupLabel == "" {
				groupLabel = chatIDStr
			}
			eventContent := fmt.Sprintf("[入群事件] 用户 %s (@%s) 加入群组 %s", newMember.FirstName, newMember.Username, groupLabel)
			hub := &model.MessageHub{
				Platform:       "telegram",
				AccountID:      accountID,
				MsgID:          fmt.Sprintf("tg_join_%d_%d", chatID, newMember.ID),
				Direction:      "inbound",
				SenderID:       senderIDStr,
				ConversationID: chatIDStr,
				MsgType:        "event",
				Content:        eventContent,
				SentAt:         time.Now(),
				IsGroup:        isGroup,
				GroupID:        chatIDStr,
			}
			if err := s.messageHubRepo.Create(ctx, hub); err != nil {
				if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
					return nil, nil, err
				}
			}
			s.upsertInboxFromHub(ctx, hub, fromName)

			// 触发 智能体流程：入群即销售起点
			// SalesRequest.UserMessage 用自然语言描述入群事件，便于 LLM 理解上下文
			triggerMsg := fmt.Sprintf("新用户 %s (@%s) 刚加入群组「%s」。请以销售助手身份主动发起欢迎+销售开场白，引导用户了解我们的产品。",
				newMember.FirstName, newMember.Username, groupLabel)
			s.triggerTelegramJoinSales(ctx, accountID, chatIDStr, senderIDStr, triggerMsg)
			return hub, nil, nil
		}
	}

	// 退群事件：仅记录到消息中台，不触发 AI
	if tgPayload.Message != nil && tgPayload.Message.LeftChatMember != nil && tgPayload.Message.Chat != nil {
		chatID := tgPayload.Message.Chat.ID
		chatIDStr := fmt.Sprintf("%d", chatID)
		left := tgPayload.Message.LeftChatMember
		senderIDStr := fmt.Sprintf("%d", left.ID)
		fromName := left.FirstName
		if left.Username != "" {
			fromName = left.Username
		}
		eventContent := fmt.Sprintf("[退群事件] 用户 %s (@%s) 离开群组", left.FirstName, left.Username)
		hub := &model.MessageHub{
			Platform:       "telegram",
			AccountID:      accountID,
			MsgID:          fmt.Sprintf("tg_left_%d_%d", chatID, left.ID),
			Direction:      "inbound",
			SenderID:       senderIDStr,
			ConversationID: chatIDStr,
			MsgType:        "event",
			Content:        eventContent,
			SentAt:         time.Now(),
			IsGroup:        true,
			GroupID:        chatIDStr,
		}
		if err := s.messageHubRepo.Create(ctx, hub); err != nil {
			if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
				return nil, nil, err
			}
		}
		s.upsertInboxFromHub(ctx, hub, fromName)
		return hub, nil, nil
	}

	// 提取消息
	type tgMsg struct {
		msgID     int64
		chatID    int64
		chatType  string
		fromID    int64
		fromName  string
		username  string
		fromIsBot bool
		text      string
	}
	var picked *tgMsg
	if tgPayload.Message != nil && tgPayload.Message.From != nil && tgPayload.Message.Chat != nil {
		// 如果消息只有 new_chat_title 等系统通知（无 text、无 new_chat_members），跳过
		if tgPayload.Message.Text == "" && len(tgPayload.Message.NewChatMembers) == 0 && tgPayload.Message.LeftChatMember == nil && tgPayload.Message.NewChatTitle != "" {
			return nil, nil, nil
		}
		tm := &tgMsg{
			msgID:     tgPayload.Message.MessageID,
			chatID:    tgPayload.Message.Chat.ID,
			chatType:  tgPayload.Message.Chat.Type,
			fromID:    tgPayload.Message.From.ID,
			fromName:  tgPayload.Message.From.FirstName,
			username:  tgPayload.Message.From.Username,
			fromIsBot: tgPayload.Message.From.IsBot,
			text:      tgPayload.Message.Text,
		}
		if tm.chatType == "" {
			tm.chatType = "private"
		}
		picked = tm
	} else if tgPayload.EditedMessage != nil && tgPayload.EditedMessage.Chat != nil {
		tm := &tgMsg{
			msgID:    tgPayload.EditedMessage.MessageID,
			chatID:   tgPayload.EditedMessage.Chat.ID,
			chatType: tgPayload.EditedMessage.Chat.Type,
			fromID: func() int64 {
				if tgPayload.EditedMessage.From != nil {
					return tgPayload.EditedMessage.From.ID
				}
				return 0
			}(),
			fromName: func() string {
				if tgPayload.EditedMessage.From != nil {
					return tgPayload.EditedMessage.From.FirstName
				}
				return ""
			}(),
			username: func() string {
				if tgPayload.EditedMessage.From != nil {
					return tgPayload.EditedMessage.From.Username
				}
				return ""
			}(),
			fromIsBot: func() bool {
				if tgPayload.EditedMessage.From != nil {
					return tgPayload.EditedMessage.From.IsBot
				}
				return false
			}(),
			text: tgPayload.EditedMessage.Text,
		}
		if tm.chatType == "" {
			tm.chatType = "private"
		}
		picked = tm
	} else if tgPayload.CallbackQuery != nil && tgPayload.CallbackQuery.From != nil {
		chatID := int64(0)
		chatType := "private"
		if tgPayload.CallbackQuery.Message != nil {
			chatID = tgPayload.CallbackQuery.Message.Chat.ID
			chatType = tgPayload.CallbackQuery.Message.Chat.Type
		}
		picked = &tgMsg{
			msgID:     0,
			chatID:    chatID,
			chatType:  chatType,
			fromID:    tgPayload.CallbackQuery.From.ID,
			fromName:  tgPayload.CallbackQuery.From.FirstName,
			fromIsBot: tgPayload.CallbackQuery.From.IsBot,
			text:      "/callback " + tgPayload.CallbackQuery.Data,
		}
	}
	if picked == nil {
		return nil, nil, nil
	}
	chatIDStr := fmt.Sprintf("%d", picked.chatID)
	senderIDStr := fmt.Sprintf("%d", picked.fromID)
	hub := &model.MessageHub{
		Platform:       "telegram",
		AccountID:      accountID,
		MsgID:          fmt.Sprintf("tg_%d", picked.msgID),
		Direction:      "inbound",
		SenderID:       senderIDStr,
		ConversationID: chatIDStr,
		MsgType:        "text",
		Content:        picked.text,
		SentAt:         time.Now(),
		IsGroup:        picked.chatType == "group" || picked.chatType == "supergroup",
		GroupID:        chatIDStr,
	}
	if hub.Content == "" {
		hub.Content = "[" + picked.chatType + "]"
	}
	// 入站消息统一经消息中台处理（标准化 + 人工锁 + AI 串行锁 + 落库 + 触发 AgentRuntime）。
	// TG 渠道特有逻辑（群入退群事件/线索挖掘/@提及判定）保留在 dispatch 内作为中台调用前后的预处理；
	// hub 仅作为收件箱会话 upsert 与上层 AI 触发的字段载体，不再单独 messageHubRepo.Create。
	if err := tgPayload.Ingress(ctx, s.ingressHandler(ctx), accountID); err != nil {
		return nil, nil, err
	}
	s.upsertInboxFromHub(ctx, hub, picked.fromName)

	// 销售线索/商机 自动挖掘：群发言与私聊中「真人」的发言都是潜在客户（发言即线索）。
	// 静默写入线索库（去重 + 意向分增量更新），与 AI 自动回复解耦、不向会话发任何消息，绝不刷屏；
	// 排除机器人自身回复(fromIsBot)与系统事件(fromID==0)；best-effort，不影响入站主链路。
	// 返回 newOpportunity：本次是否让发言者「新晋为商机」，用于群内「发现线索主动触达」。
	newOpportunity := false
	if picked.fromID != 0 && !picked.fromIsBot {
		groupTitle := ""
		if tgPayload.Message != nil && tgPayload.Message.Chat != nil {
			groupTitle = tgPayload.Message.Chat.Title
		}
		newOpportunity = s.mineTelegramGroupLead(context.Background(), hub, chatIDStr, groupTitle, senderIDStr, picked.username, picked.fromName, picked.text)
	}

	// 群内「@机器人 才回复」的提及判定：文本含 @username，或本条是「回复了某条机器人消息」
	mentioned := isTelegramBotMentioned(picked.text, botUsername)
	if !mentioned && tgPayload.Message != nil && tgPayload.Message.ReplyToMessage != nil &&
		tgPayload.Message.ReplyToMessage.From != nil && tgPayload.Message.ReplyToMessage.From.IsBot {
		mentioned = true
	}

	return hub, &tgDispatchExtra{Mentioned: mentioned, NewOpportunity: newOpportunity}, nil
}

// tgLeadOutreachCooldown 「发现线索主动触达」对同一发言者的冷却时长，避免群内刷屏。
const tgLeadOutreachCooldown = 30 * time.Minute

// getTelegramBotUsername 取账号绑定的机器人 @username（用于群内 @提及 识别）。
// 该值通常在注册 webhook 时经 getMe 自动回填；缺失则返回空（上层降级为仅回复被@机器人消息）。
func (s *WebhookService) getTelegramBotUsername(ctx context.Context, accountID string) string {
	if s.telegramRepo == nil {
		return ""
	}
	accID, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || accID == 0 {
		return ""
	}
	acc, err := s.telegramRepo.GetByID(ctx, uint(accID))
	if err != nil || acc == nil {
		return ""
	}
	return strings.TrimSpace(acc.BotUsername)
}

// isTelegramBotMentioned 判断一条消息文本是否 @提及了本机器人。
// botUsername 为空（未配置/未回填）时返回 false。匹配大小写不敏感（TG 用户名统一小写）。
// 采用词边界匹配：@username 之后必须紧跟「非用户名字符」（字母/数字/下划线之外）或结尾，
// 避免把 @mybotX、@mybotfoo 误判为 @mybot。
func isTelegramBotMentioned(text, botUsername string) bool {
	uname := strings.TrimSpace(botUsername)
	if uname == "" {
		return false
	}
	needle := "@" + strings.ToLower(uname)
	lower := strings.ToLower(text)
	idx := strings.Index(lower, needle)
	if idx < 0 {
		return false
	}
	end := idx + len(needle)
	if end < len(lower) {
		next := lower[end]
		if (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') || next == '_' {
			return false
		}
	}
	return true
}

// tgLeadOutreachAllowed 判断该（账号, 群组, 发言者）是否允许本次「发现线索主动触达」。
// 业务需要：绝不能骚扰用户，多实例下若各持进程内冷却 map，可能仍被不同实例短时重复触达刷屏。
// 实现：基于全局缓存 SetNX + 冷却 TTL——首次设置返回 true(允许)，冷却窗口内已存在返回 false(拦截)，
// 超时后自动释放。REDIS_HOST 配置时为 Redis 共享后端（跨实例一致），否则为内存单例。
func (s *WebhookService) tgLeadOutreachAllowed(ctx context.Context, accountID, chatID, senderID string) bool {
	key := "mtk:tg:outreach:" + accountID + ":" + chatID + ":" + senderID
	set, err := cache.GetGlobalCache().SetNX(ctx, key, "1", tgLeadOutreachCooldown)
	if err != nil {
		// 后端异常时放行（可用性优先，仅损失防刷屏），不阻断正常触达
		return true
	}
	return set
}

// triggerTelegramJoinSales TG 入群事件触发 智能体流程
// 与 triggerSalesEngine 类似，但 UserMessage 是入群事件描述，让 LLM 主动发起销售对话
func (s *WebhookService) triggerTelegramJoinSales(ctx context.Context, accountID, chatID, senderID, triggerMsg string) {
	if s.salesEngine == nil {
		return
	}
	if !s.shouldTriggerAI(ctx, ChannelTelegram, accountID) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := &SalesRequest{
		SessionID:   "telegram:" + chatID,
		CustomerID:  senderID,
		OneID:       "telegram:" + senderID,
		UserMessage: triggerMsg,
		Platform:    "telegram",
		AutoExecute: true,
		Config:      DefaultSalesEngineConfig(),
	}
	// 入群场景的人设：销售助手主动开场
	req.Config.Persona = "你是 Telegram 群组里的销售助手。新用户加入群组时，主动发起一段简洁、亲切的欢迎+销售开场白，引导用户了解产品。回复不超过 80 字。"

	resp, err := s.salesEngine.Handle(ctx, req)
	if err != nil {
		logger.Errorf("[Webhook] TG 入群触发 智能体失败 account=%s chat=%s: %v", accountID, chatID, err)
		return
	}
	if resp == nil || resp.Reply == "" {
		return
	}
	if resp.TransferredToHuman {
		logger.Infof("[Webhook] TG 入群触发转人工: %s", resp.TransferReason)
		return
	}
	// 出站：发送到 TG 群组
	chatIDInt, _ := strconv.ParseInt(chatID, 10, 64)
	if chatIDInt == 0 {
		return
	}
	if s.tgIntegration == nil {
		s.tgIntegration = NewTelegramIntegrationService(s.db)
	}
	accID, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || accID == 0 {
		return
	}
	if err := s.tgIntegration.SendMessage(ctx, uint(accID), chatIDInt, resp.Reply); err != nil {
		logger.Errorf("[Webhook] TG 入群欢迎消息发送失败 account=%s chat=%s: %v", accountID, chatID, err)
	}
}

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
	if s.eventRepo != nil {
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
	s.sendOutbound(ctx, channel, accountID, p, resp.Reply, hubMsg, resp.Cards)
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
func (s *WebhookService) triggerSmartOrchestrator(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, hubMsg *model.MessageHub) {
	if s.smartOrchestrator == nil {
		return
	}
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
func (s *WebhookService) TriggerInboundAI(ctx context.Context, channel, accountID, conversationID, customerID, content, eventID string, opts ...TriggerInboundOption) {
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
	// 有界信号量：限制并发本地推理数，保护单节点推理栈不被压垮
	s.replySem <- struct{}{}
	defer func() { <-s.replySem }()

	// 继承上游 trace_id（如有），保证全链路日志可串联；
	// 上游未注入时，WithTraceID 自动生成新 ID（不丢可观测性）。
	parentCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(ctx); parentTraceID != "" {
		parentCtx = trace.NewContextWithTraceID(parentCtx, parentTraceID)
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	ctx = logger.WithModule(ctx, "webhook")

	// 本地推理偶发超时，最多重试 WebhookMaxRetries 次
	var result *HandleResult
	var err error
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

// sendOutbound 出站发送（按 channel）；ctx 用于透传 trace_id（来自 triggerSmartOrchestrator / 回退链路）
func (s *WebhookService) sendOutbound(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, content string, hubMsg *model.MessageHub, cards []model.RichCard) {
	// 幂等守卫：与 AgentRuntime 事件总线订阅共享同一 EventID 守卫。
	// 同一 EventID 仅首条链路出站，杜绝重复消息；同时防御 webhook 平台重复投递
	// （同 EventID 二次到达）导致的重复出站。
	if !agent_runtime.ClaimReply(p.EventID) {
		logger.Ctx(ctx).Info().Str("event_id", p.EventID).Msg("skip duplicate outbound (event already replied)")
		// 可观测性：记录被幂等守卫拦截的重复出站 (私域: 无 Prometheus, 仅日志)
		return
	}
	// 出站结果追踪：仅当真正成功出站时才保留认领（防重复）；
	// 若全线出站失败则释放认领，允许平台重投在本实例内重试。
	sent := false
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer func() {
		if !sent {
			agent_runtime.ReleaseReply(p.EventID)
		}
		cancel()
	}()
	switch channel {
	case ChannelWeCom:
		// 企微出站底层 = WeComIntegrationService（与 IntegrationReachAdapter.SendWeCom 共享同一底层， 已收敛为单一出站入口）
		if s.integration == nil {
			return
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		if _, err := s.integration.SendMessage(ctx, &WeComSendRequest{
			AccountID:      uint(accID),
			ExternalUserID: p.Sender,
			MsgType:        "text",
			Content:        content,
			IsAIReply:      true,
			AIAgent:        "sales_engine",
		}); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "wecom").Str("account_id", accountID).Msg("outbound failed")
		} else {
			sent = true
		}
	case ChannelFeishu:
		if s.feishuIntegration == nil {
			s.feishuIntegration = NewFeishuIntegrationService(s.db)
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		// Feishu：个人消息用 open_id 作为发送目标；群消息必须用群 chat_id（open_chat_id），
		// 否则会被当成私信发给用户而非在群里回复。
		target := p.Sender
		idType := "open_id"
		if hubMsg != nil && hubMsg.IsGroup && hubMsg.GroupID != "" {
			target = hubMsg.GroupID
			idType = "open_chat_id"
		}
		if err := s.feishuIntegration.SendMessage(ctx, uint(accID), target, content, idType); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "feishu").Str("account_id", accountID).Msg("outbound failed")
		} else {
			sent = true
		}
	case ChannelTelegram:
		if s.tgIntegration == nil {
			s.tgIntegration = NewTelegramIntegrationService(s.db)
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		// 解析 chat_id
		var chatID int64
		if hubMsg != nil && hubMsg.ConversationID != "" {
			chatID, _ = strconv.ParseInt(hubMsg.ConversationID, 10, 64)
		}
		if chatID == 0 {
			return
		}
		if err := s.tgIntegration.SendMessage(ctx, uint(accID), chatID, content); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "telegram").Str("account_id", accountID).Int64("chat_id", chatID).Msg("outbound failed")
		} else {
			sent = true
		}
		// 结构化富卡片：随文本一并下发（Telegram 以 inline keyboard 按钮呈现）
		for _, card := range cards {
			if err := s.tgIntegration.SendCard(ctx, uint(accID), chatID, &card); err != nil {
				logger.Ctx(ctx).Error().Err(err).Str("channel", "telegram").Int64("chat_id", chatID).Msg("outbound card failed")
			} else {
				sent = true
			}
		}
	case ChannelWhatsapp:
		if s.waIntegration == nil {
			s.waIntegration = NewWhatsAppCloudIntegrationService(s.db)
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		// 收件人是手机号 (E.164)
		if err := s.waIntegration.SendMessage(ctx, uint(accID), p.Sender, content); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "whatsapp").Str("account_id", accountID).Msg("outbound failed")
		} else {
			sent = true
		}
	case "douyin_web", "xhs_web", "tiktok_web":
		// 网页桥接渠道：AI 回复经 WebSocket 投递到 Chrome 扩展（由 bridge 包注册的回调完成），
		// 不走官方 API（避免把私信误发到平台开放接口）。
		// p.EventID 传给 bridge 用于 ClaimReply 幂等守卫。
		if err := DeliverBridgeOutbound(ctx, string(channel), accountID, hubMsg.ConversationID, "text", content, p.EventID); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", string(channel)).Str("account_id", accountID).Msg("bridge outbound failed")
		} else {
			sent = true
		}
	default:
		// 该渠道暂未实现主动出站：记录日志并跳过，避免静默吞掉消息难以排查
		logger.Ctx(ctx).Warn().Str("channel", string(channel)).Str("account_id", accountID).Msg("unsupported outbound channel, skipped")
	}
}

// =================== 工具 ===================

// isDuplicate 基于 eventID 的 TTL 幂等。
// 业务需要：外部渠道事件必须「恰好一次」处理。多实例下若各持进程内去重表，
// 重复投递会被不同实例各自放过 → 双处理。故改走全局缓存 SetNX：
//   - REDIS_HOST 配置时为 Redis 共享后端（跨实例去重）
//   - 否则为内存单例（单实例安全）
//
// TTL 内重复 key 已存在即命中返回 true；SetNX 异常时放行并告警（可用性优先）。
func (s *WebhookService) isDuplicate(ctx context.Context, eventID string) bool {
	if eventID == "" {
		return false
	}
	key := "mtk:webhook:dedup:" + eventID
	set, err := cache.GetGlobalCache().SetNX(ctx, key, "1", WebhookDedupTTL)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("event_id", eventID).Msg("[webhook] dedup 后端异常，放行")
		return false
	}
	if !set {
		// 可观测性：命中去重的重复投递 (私域: 无 Prometheus, 仅日志)
		logger.Ctx(ctx).Debug().Str("event_id", eventID).Msg("[webhook] dedup hit")
		return true
	}
	return false
}

func (s *WebhookService) allowRate(ctx context.Context, key string) bool {
	s.rlMu.Lock()
	b, ok := s.rlBuckets[key]
	if !ok {
		b = &tokenBucket{
			capacity:   WebhookRateBurst,
			refillRate: float64(WebhookRateLimit),
			tokens:     float64(WebhookRateBurst),
			lastRefill: time.Now(),
		}
		s.rlBuckets[key] = b
	}
	s.rlMu.Unlock()
	return b.allow(context.Background())
}

func (s *WebhookService) generateEventID(ctx context.Context, channel WebhookChannel, accountID string, body []byte) string {
	h := sha256.Sum256([]byte(string(channel) + ":" + accountID + ":" + string(body)))
	return fmt.Sprintf("evt_%s", hex.EncodeToString(h[:8]))
}

func (s *WebhookService) genMessageID(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload) string {
	h := sha256.Sum256([]byte(string(channel) + ":" + accountID + ":" + p.Sender + ":" + p.Content + ":" + p.EventID))
	// UnifiedMessage.MessageID 列宽为 varchar(50)，"msg_" 前缀 + 完整 64 hex 会超长。
	// 截断为前 22 hex 字符（共 26 字符，留足唯一空间：2^88）。
	return fmt.Sprintf("msg_%s", hex.EncodeToString(h[:])[:22])
}

// TruncateForStore 截断防止 raw_data 过大
func (s *WebhookService) TruncateForStore(ctx context.Context, body []byte) string {
	const max = 64 * 1024
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...[truncated]"
}

func (s *WebhookService) getAccountSecret(ctx context.Context, platform, accountID string) (string, error) {
	if s.accountRepo == nil {
		return "", nil
	}
	// 防止全局 nil db 触发 panic
	if s.db == nil {
		return "", nil
	}
	acc, err := s.accountRepo.GetByPlatform(ctx, platform)
	if err != nil {
		return "", nil
	}
	return acc.APISecret, nil
}

// getTelegramWebhookSecret 获取 Telegram Bot 的 webhook secret
// secret 来自 TelegramAccount.WebhookSecret（在 setWebhook 时由商户配置）
// 未配置时返回空字符串，调用方应跳过验签
func (s *WebhookService) getTelegramWebhookSecret(ctx context.Context, accountID string) string {
	if s.telegramRepo == nil || s.db == nil {
		return ""
	}
	accID, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || accID == 0 {
		return ""
	}
	acc, err := s.telegramRepo.GetByID(ctx, uint(accID))
	if err != nil {
		return ""
	}
	return acc.WebhookSecret
}

func (s *WebhookService) getWechatSecrets(ctx context.Context, accountID string) (string, string) {
	return "", ""
}

// GetWeComSecrets 公开方法：供 controller 层 URL 验证使用
func (s *WebhookService) GetWeComSecrets(ctx context.Context, accountID string) (string, string, error) {
	return s.getWeComSecrets(ctx, accountID)
}

// getWeComSecrets 从 wecom_accounts 读取 token + EncodingAESKey
// accountID 优先按数字 ID 解析；解析失败则取第一条启用 webhook 的账号
func (s *WebhookService) getWeComSecrets(ctx context.Context, accountID string) (string, string, error) {
	if s.wecomRepo == nil {
		return "", "", errors.New("wecomRepo nil")
	}
	if s.db == nil {
		return "", "", nil
	}
	// 1) 按 ID 解析
	if id, err := strconv.ParseUint(accountID, 10, 64); err == nil && id > 0 {
		acc, err := s.wecomRepo.GetByID(ctx, uint(id))
		if err == nil && acc != nil {
			return acc.CallbackToken, acc.EncodingAESKey, nil
		}
	}
	// 2) 兜底：取第一个启用 webhook 的账号
	accs, err := s.wecomRepo.GetByMerchant(ctx)
	if err != nil {
		return "", "", err
	}
	for _, a := range accs {
		if a.WebhookEnabled {
			return a.CallbackToken, a.EncodingAESKey, nil
		}
	}
	if len(accs) > 0 {
		return accs[0].CallbackToken, accs[0].EncodingAESKey, nil
	}
	return "", "", errors.New("wecom account not found")
}

// PendingCount 待处理事件数
func (s *WebhookService) PendingCount(ctx context.Context) int64 {
	if s.eventRepo == nil {
		return 0
	}
	c, _ := s.eventRepo.CountUnprocessed(ctx)
	return c
}

// QueueLen 队列长度
func (s *WebhookService) QueueLen(ctx context.Context) int { return len(s.queue) }

// ReadAll 读取请求体
func ReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

// helpers
func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
