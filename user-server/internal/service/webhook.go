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

	"os"

	"strconv"

	"strings"

	"sync"

	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/channelbot/telegram"

	"hivemtk-user/internal/channelbot/whatsapp"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"hivemtk-user/internal/repository"
)

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
	lastAccess time.Time // 用于 janitor 清理闲置桶
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
	WebhookDedupTTL = 5 * time.Minute

	WebhookWorkerCount = 4 // 接入 worker 数（从 WEBHOOK_WORKER_COUNT 覆盖）

	WebhookQueueSize = 512

	WebhookRateLimit = 30

	WebhookRateBurst = 60

	WebhookMaxRetries = 3

	WebhookReplyConcurrency = 32
)

func webhookEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

type WebhookChannel string

const (
	ChannelDouyin WebhookChannel = "douyin"

	ChannelKuaishou WebhookChannel = "kuaishou"

	ChannelXiaohongshu WebhookChannel = "xiaohongshu"

	ChannelXianyu WebhookChannel = "xianyu"

	ChannelTiktok WebhookChannel = "tiktok"

	ChannelWechat WebhookChannel = "wechat"

	ChannelWeCom WebhookChannel = "wecom"

	ChannelWhatsapp WebhookChannel = "whatsapp"

	ChannelTelegram WebhookChannel = "telegram"

	ChannelFeishu WebhookChannel = "feishu"

	ChannelCustom WebhookChannel = "custom"
)

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
	s.startRLJanitor(context.Background())
	return s
}

func (s *WebhookService) SetSalesEngine(ctx context.Context, e *SalesEngine) {
	s.salesEngine = e
}

func (s *WebhookService) SetIngressSvc(ingress *InboxIngressService) {
	if ingress != nil {
		s.ingressSvc = ingress
	}
}

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

func (s *WebhookService) SetSmartOrchestrator(ctx context.Context, o *SmartCSOrchestrator) {
	s.smartOrchestrator = o
}

func (s *WebhookService) SetAgentBindingService(ctx context.Context, svc *ChannelAgentBindingService) {
	s.agentBindingSvc = svc
}

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

func (s *WebhookService) ingressHandler(ctx context.Context) webhookIngressAdapter {
	return webhookIngressAdapter{svc: s.ingressSvc}
}

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
			// 修复：单条 job 处理（派发/解析/AI 编排）panic 不得杀死 worker goroutine，
			// 否则 worker 数静默下降，webhook 队列最终停摆。recover 后仅记日志，循环继续。
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[Webhook] worker-%d panic recovered, job dropped: %v", id, r)
					}
				}()
				s.handleJob(ctx, job)
			}()
		}
	}
}

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

type ReceiveRequest struct {
	Channel   WebhookChannel
	AccountID string
	Body      []byte
	Headers   map[string]string
	SourceIP  string
	// WeCom 加解密使用：加密的 body 解析时使用
	Query map[string]string
}

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

type ParsedPayload struct {
	EventID   string         `json:"event_id"`
	EventType string         `json:"event_type"`
	Sender    string         `json:"sender,omitempty"`
	Content   string         `json:"content,omitempty"`
	ChatID    string         `json:"chat_id,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}
