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

	"marketing/internal/aiagent/agent/runtime"
	"marketing/internal/channelbot/telegram"
	"marketing/internal/channelbot/whatsapp"
	"marketing/internal/model"
	"marketing/internal/pkg/metrics"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// WebhookService 多渠道 Webhook 服务
// 对应 SYSTEM_AUDIT_REPORT_20260715_V3 P0-14
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

	// 渠道账号仓库缓存（性能审计 P3-3：避免在每条消息的 shouldTriggerAI 中重复构造）
	feishuRepo *repository.FeishuAccountRepository
	waRepo     *repository.WhatsAppCloudAccountRepository

	// 消息中台 / 收件箱会话 / 统一消息 仓库
	messageHubRepo *repository.MessageHubRepository
	inboxConvRepo  *repository.InboxConversationRepository
	unifiedMsgRepo repository.UnifiedMessageRepository

	// 线索仓库：Telegram 群发言自动挖掘为销售线索/商机（去重 + 意向分增量更新）
	clueRepo repository.ClueRepository

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
	dedup     sync.Map   // eventID -> expireTime(time.Time)，O(1) 幂等（性能审计 P1-2）
	rlMu      sync.Mutex // 限流桶专用锁，与 dedup 分离避免相互阻塞
	rlBuckets map[string]*tokenBucket

	workerCount int
	queue       chan *webhookJob
	wg          sync.WaitGroup
	stopCh      chan struct{}
	stopped     bool

	// 推理并发信号量：限制同时进行的本地 LLM 生成数，保护单节点推理栈（性能审计 P1-1）。
	replySem chan struct{}
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

func (b *tokenBucket) allow() bool {
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
	// 推理饱和时 AI 任务排队而非丢弃（性能审计 P1-1）。可用 WEBHOOK_REPLY_CONCURRENCY 覆盖。
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
		wecomRepo.SetDB(db)
	}
	accountRepo := repository.NewIntegrationAccountRepository()
	if db != nil {
		accountRepo.SetDB(db)
	}
	eventRepo := repository.NewWebhookEventRepository()
	if db != nil {
		repository.SetWebhookEventRepoDB(eventRepo, db)
	}
	telegramRepo := repository.NewTelegramAccountRepository()
	if db != nil {
		telegramRepo.SetDB(db)
	}
	feishuRepo := repository.NewFeishuAccountRepository()
	if db != nil {
		feishuRepo.SetDB(db)
	}
	waRepo := repository.NewWhatsAppCloudAccountRepository()
	if db != nil {
		waRepo.SetDB(db)
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
		rlBuckets:      make(map[string]*tokenBucket),
		workerCount:    webhookEnvInt("WEBHOOK_WORKER_COUNT", WebhookWorkerCount),
		queue:          make(chan *webhookJob, webhookEnvInt("WEBHOOK_QUEUE_SIZE", WebhookQueueSize)),
		stopCh:         make(chan struct{}),
		replySem:       make(chan struct{}, webhookEnvInt("WEBHOOK_REPLY_CONCURRENCY", WebhookReplyConcurrency)),
	}
	s.startWorkers()
	s.startDedupJanitor()
	return s
}

// SetSalesEngine 注入 智能体引擎（解耦依赖，可选）
func (s *WebhookService) SetSalesEngine(e *SalesEngine) {
	s.salesEngine = e
}

// ensureReposFromDB 在 struct 直接构造（如测试中 &WebhookService{db: db}）时，
// 按需从 s.db 派生新的仓库实例，保持与原 s.db 直用等价语义。
func (s *WebhookService) ensureReposFromDB() {
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
func (s *WebhookService) SetSmartOrchestrator(o *SmartCSOrchestrator) {
	s.smartOrchestrator = o
}

// SetAgentBindingService 注入渠道智能体绑定服务
// 注入后 triggerSalesEngine 会先按 (channel_type, account_id) 查询绑定的智能体上下文，
// 再调用 SalesEngine.HandleWithAgent 按智能体配置执行 9 步链路
// 未注入时回退到 DefaultSalesEngineConfig 默认行为
func (s *WebhookService) SetAgentBindingService(svc *ChannelAgentBindingService) {
	s.agentBindingSvc = svc
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
func (s *WebhookService) Stop() {
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

func (s *WebhookService) startWorkers() {
	for i := 0; i < s.workerCount; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
}

func (s *WebhookService) worker(id int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		case job, ok := <-s.queue:
			if !ok {
				return
			}
			s.handleJob(job)
		}
	}
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
	verified, err := s.Verify(req.Channel, req.AccountID, req.Body, req.Headers, req.Query)
	if err != nil {
		logger.Errorf("[Webhook] 验签异常 channel=%s account=%s: %v", req.Channel, req.AccountID, err)
		return &ReceiveResult{Accepted: false, VerifyFail: true, Reason: "verify error: " + err.Error()}, nil
	}
	if !verified {
		return &ReceiveResult{Accepted: false, VerifyFail: true, Reason: "signature mismatch"}, nil
	}

	// 2) 解析基础字段
	payload, err := s.ParsePayload(req.Channel, req.Body)
	if err != nil {
		return &ReceiveResult{Accepted: false, Reason: "parse error: " + err.Error()}, nil
	}
	if payload.EventID == "" {
		payload.EventID = s.generateEventID(req.Channel, req.AccountID, req.Body)
	}
	if payload.EventType == "" {
		payload.EventType = "unknown"
	}

	// 3) 幂等去重
	if s.isDuplicate(payload.EventID) {
		return &ReceiveResult{
			Accepted:  true,
			EventID:   payload.EventID,
			Duplicate: true,
		}, nil
	}

	// 4) 限流
	key := string(req.Channel) + ":" + req.AccountID
	if !s.allowRate(key) {
		return &ReceiveResult{Accepted: false, RateLimit: true, Reason: "rate limited"}, nil
	}

	// 5) 持久化事件记录
	evt := &model.WebhookEvent{
		Platform:  string(req.Channel),
		EventID:   payload.EventID,
		EventType: payload.EventType,
		RawData:   s.TruncateForStore(req.Body),
		Processed: false,
	}
	if s.eventRepo != nil {
		if err := s.eventRepo.Create(evt); err != nil {
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
		s.handleJob(job)
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
func (s *WebhookService) Verify(channel WebhookChannel, accountID string, body []byte, headers map[string]string, query map[string]string) (bool, error) {
	switch channel {
	case ChannelWeCom:
		token, aesKey, err := s.getWeComSecrets(accountID)
		if err != nil || token == "" {
			return false, fmt.Errorf("wecom token missing: %v", err)
		}
		return verifyWeCom(token, aesKey, body, query)
	case ChannelWechat:
		token, _ := s.getWechatSecrets(accountID)
		return verifyWechat(token, body, headers), nil
	case ChannelDouyin, ChannelTiktok:
		secret, _ := s.getAccountSecret(string(channel), accountID)
		return verifyHMAC(secret, body, headers, "X-Lark-Signature", "X-Douyin-Signature", "Signature"), nil
	case ChannelTelegram:
		// TG 验签：Telegram Bot API 通过 X-Telegram-Bot-Api-Secret-Token 头传递 webhook secret
		// secret 来自 TelegramAccount.WebhookSecret（在 setWebhook 时配置）；验签委托独立包 channelbot/telegram
		secret := s.getTelegramWebhookSecret(accountID)
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
		secret, _ := s.getAccountSecret(string(channel), accountID)
		return verifyHMAC(secret, body, headers, "X-Signature", "Signature", "X-Hub-Signature-256", "Encrypt"), nil
	case ChannelWhatsapp:
		// WA 验签：X-Hub-Signature-256（HMAC-SHA256，app secret 对 raw body 签名）；委托独立包 channelbot/whatsapp
		secret, _ := s.getAccountSecret(string(channel), accountID)
		if secret == "" {
			if os.Getenv("GIN_MODE") == "release" {
				return false, errors.New("whatsapp app secret 未配置，生产环境拒绝放行")
			}
			return true, nil
		}
		return whatsapp.VerifyWebhook(secret, body, headers["X-Hub-Signature-256"]), nil
	default:
		secret, _ := s.getAccountSecret(string(channel), accountID)
		if secret == "" {
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
func (s *WebhookService) ParsePayload(channel WebhookChannel, body []byte) (*ParsedPayload, error) {
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
func (s *WebhookService) ToUnifiedMessage(channel WebhookChannel, accountID string, p *ParsedPayload) *model.UnifiedMessage {
	return &model.UnifiedMessage{
		MessageID:   s.genMessageID(channel, accountID, p),
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

func (s *WebhookService) handleJob(job *webhookJob) {
	channel := job.channel
	if channel == "" {
		channel = WebhookChannel(job.event.Platform)
	}
	payload := job.payload
	if payload == nil {
		var err error
		payload, err = s.ParsePayload(channel, job.raw)
		if err != nil {
			logger.Errorf("[Webhook] 处理失败 event=%s: %v", job.event.EventID, err)
			return
		}
	}

	// 1) 基础入库（统一消息）
	um := s.ToUnifiedMessage(channel, job.account, payload)
	if um.AccountID == "" {
		um.AccountID = job.event.EventID
	}
	if err := s.dispatchToUnified(um); err != nil {
		s.retryWithBackoff(job, payload, err)
		return
	}

	// 2) 按渠道业务分发
	hubMsg, _ := s.dispatchToChannel(channel, job.account, payload, job.raw, job.header)

	// 3) 触发 智能体（如已注入 + 启用了 智能体开关）
	if hubMsg != nil && s.shouldTriggerAI(channel, job.account) {
		s.triggerSalesEngine(channel, job.account, payload, hubMsg)
	}

	s.markProcessed(job.event)
}

// dispatchToChannel 按渠道路由业务逻辑
func (s *WebhookService) dispatchToChannel(channel WebhookChannel, accountID string, p *ParsedPayload, raw []byte, headers map[string]string) (*model.MessageHub, error) {
	switch channel {
	case ChannelWeCom:
		return s.dispatchWeCom(accountID, p, raw, headers)
	case ChannelWhatsapp:
		return s.dispatchWhatsApp(accountID, p, raw)
	case ChannelTelegram:
		return s.dispatchTelegram(accountID, p, raw)
	case ChannelFeishu:
		return s.dispatchFeishu(accountID, p, raw)
	default:
		// 自定义渠道仅入统一消息即可
		return nil, nil
	}
}

// dispatchWeCom 企业微信业务分发
func (s *WebhookService) dispatchWeCom(accountID string, p *ParsedPayload, raw []byte, headers map[string]string) (*model.MessageHub, error) {
	if s.integration == nil {
		return nil, nil
	}
	// 解析企微解密后的明文
	plain := s.parseWeComPlain(raw)
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
		acc, gerr := s.wecomRepo.GetByMerchant()
		if gerr != nil || len(acc) == 0 {
			return nil, fmt.Errorf("invalid account_id")
		}
		accID = uint64(acc[0].ID)
	}

	hubMsg, _, err := s.integration.ReceiveCallback(context.Background(), &ReceiveCallbackRequest{
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
	return hubMsg, err
}

// parseWeComPlain 如果 body 包含 encrypt 字段，尝试解密（需要从 wecomRepo 拉 EncodingAESKey）
func (s *WebhookService) parseWeComPlain(raw []byte) map[string]any {
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
	accs, err := s.wecomRepo.GetByMerchant()
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
func (s *WebhookService) dispatchWhatsApp(accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB()
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
			if err := s.messageHubRepo.Create(hub); err != nil {
				if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
					return nil, err
				}
			}
			// 写收件箱会话
			s.upsertInboxFromHub(hub, name)
			return hub, nil
		}
	}
	return nil, nil
}

func (s *WebhookService) dispatchTelegram(accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB()
	// 解析 TG 消息（被动入站解析委托独立包 channelbot/telegram，支持消息/编辑/回调/入群事件）
	tgPayload, err := telegram.ParseUpdate(raw)
	if err != nil {
		return nil, fmt.Errorf("telegram parse: %w", err)
	}

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
			if err := s.messageHubRepo.Create(hub); err != nil {
				if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
					return nil, err
				}
			}
			s.upsertInboxFromHub(hub, fromName)

			// 触发 智能体流程：入群即销售起点
			// SalesRequest.UserMessage 用自然语言描述入群事件，便于 LLM 理解上下文
			triggerMsg := fmt.Sprintf("新用户 %s (@%s) 刚加入群组「%s」。请以销售助手身份主动发起欢迎+销售开场白，引导用户了解我们的产品。",
				newMember.FirstName, newMember.Username, groupLabel)
			s.triggerTelegramJoinSales(accountID, chatIDStr, senderIDStr, triggerMsg)
			return hub, nil
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
		if err := s.messageHubRepo.Create(hub); err != nil {
			if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
				return nil, err
			}
		}
		s.upsertInboxFromHub(hub, fromName)
		return hub, nil
	}

	// 提取消息
	type tgMsg struct {
		msgID    int64
		chatID   int64
		chatType string
		fromID   int64
		fromName string
		username string
		text     string
	}
	var picked *tgMsg
	if tgPayload.Message != nil && tgPayload.Message.From != nil && tgPayload.Message.Chat != nil {
		// 如果消息只有 new_chat_title 等系统通知（无 text、无 new_chat_members），跳过
		if tgPayload.Message.Text == "" && len(tgPayload.Message.NewChatMembers) == 0 && tgPayload.Message.LeftChatMember == nil && tgPayload.Message.NewChatTitle != "" {
			return nil, nil
		}
		tm := &tgMsg{
			msgID:    tgPayload.Message.MessageID,
			chatID:   tgPayload.Message.Chat.ID,
			chatType: tgPayload.Message.Chat.Type,
			fromID:   tgPayload.Message.From.ID,
			fromName: tgPayload.Message.From.FirstName,
			username: tgPayload.Message.From.Username,
			text:     tgPayload.Message.Text,
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
			text:     tgPayload.EditedMessage.Text,
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
			msgID:    0,
			chatID:   chatID,
			chatType: chatType,
			fromID:   tgPayload.CallbackQuery.From.ID,
			fromName: tgPayload.CallbackQuery.From.FirstName,
			text:     "/callback " + tgPayload.CallbackQuery.Data,
		}
	}
	if picked == nil {
		return nil, nil
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
	if err := s.messageHubRepo.Create(hub); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
			return nil, err
		}
	}
	s.upsertInboxFromHub(hub, picked.fromName)

	// 群发言 → 销售线索/商机 自动挖掘：
	// 群里每个真实发言者都是潜在客户，发言即线索。静默写入线索库（去重 + 意向分增量更新），
	// 与 AI 自动回复解耦、不向群里发消息，因此绝不刷屏；best-effort，不影响入站主链路。
	if hub.IsGroup && picked.fromID != 0 {
		groupTitle := ""
		if tgPayload.Message != nil && tgPayload.Message.Chat != nil {
			groupTitle = tgPayload.Message.Chat.Title
		}
		s.mineTelegramGroupLead(chatIDStr, groupTitle, senderIDStr, picked.username, picked.fromName, picked.text)
	}
	return hub, nil
}

// triggerTelegramJoinSales TG 入群事件触发 智能体流程
// 与 triggerSalesEngine 类似，但 UserMessage 是入群事件描述，让 LLM 主动发起销售对话
func (s *WebhookService) triggerTelegramJoinSales(accountID, chatID, senderID, triggerMsg string) {
	if s.salesEngine == nil {
		return
	}
	if !s.shouldTriggerAI(ChannelTelegram, accountID) {
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

func (s *WebhookService) dispatchFeishu(accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB()
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
	if err := s.messageHubRepo.Create(hub); err != nil {
		if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
			return nil, err
		}
	}
	s.upsertInboxFromHub(hub, "")
	return hub, nil
}

// upsertInboxFromHub 写入收件箱会话
func (s *WebhookService) upsertInboxFromHub(hub *model.MessageHub, customerName string) {
	if s.inboxConvRepo == nil || hub == nil {
		return
	}
	conv, err := s.inboxConvRepo.FindByPlatformAccountCustomer(hub.Platform, hub.AccountID, hub.SenderID)
	if err == nil && conv != nil {
		// 更新最后消息（atomic 自增 unread_count）
		_ = s.inboxConvRepo.UpdateLastMessage(conv.ID, hub.Content, hub.SentAt, 1)
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
	_ = s.inboxConvRepo.Create(newConv)
}

func (s *WebhookService) dispatchToUnified(um *model.UnifiedMessage) error {
	s.ensureReposFromDB()
	if s.unifiedMsgRepo == nil {
		return errors.New("unified message repo nil")
	}
	// 先查是否已存在（幂等）
	if _, err := s.unifiedMsgRepo.GetByMessageID(um.MessageID); err == nil {
		return nil
	}
	if err := s.unifiedMsgRepo.Create(um); err != nil {
		// 唯一冲突也视为成功
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			return nil
		}
		return err
	}
	return nil
}

func (s *WebhookService) retryWithBackoff(job *webhookJob, payload *ParsedPayload, origErr error) {
	delays := []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}
	for i := 0; i < WebhookMaxRetries; i++ {
		time.Sleep(delays[i])
		um := s.ToUnifiedMessage(job.channel, job.account, payload)
		if err := s.dispatchToUnified(um); err == nil {
			s.markProcessed(job.event)
			return
		}
	}
	logger.Errorf("[Webhook] 多次重试失败 event=%s err=%v", job.event.EventID, origErr)
}

func (s *WebhookService) markProcessed(evt *model.WebhookEvent) {
	now := time.Now()
	evt.Processed = true
	evt.ProcessedAt = &now
	if s.eventRepo != nil {
		_ = s.eventRepo.Update(evt)
	}
}

// shouldTriggerAI 是否触发 智能体
func (s *WebhookService) shouldTriggerAI(channel WebhookChannel, accountID string) bool {
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
		acc, err := s.wecomRepo.GetByID(uint(accID))
		if err != nil {
			return false
		}
		return acc.AIAgentEnabled
	case ChannelFeishu:
		// 性能审计 P3-3：复用构造期缓存的仓库，不再每条消息重复 New
		if s.feishuRepo == nil {
			return false
		}
		acc, err := s.feishuRepo.GetByID(uint(accID))
		if err != nil {
			return false
		}
		return acc.AIAgentEnabled
	case ChannelTelegram:
		// TG 机器人收到消息后，如果账号开启了 智能体开关，则自动触发 智能体流程
		if s.telegramRepo == nil {
			return false
		}
		acc, err := s.telegramRepo.GetByID(uint(accID))
		if err != nil {
			return false
		}
		return acc.AIAgentEnabled && acc.Status == 1
	case ChannelWhatsapp:
		if s.waRepo == nil {
			return false
		}
		acc, err := s.waRepo.GetByID(uint(accID))
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
//
// 2026-07-17 扩展（ADR-008 §2.2）：
//   - 在主流程之前 publish customer.message.received 事件
//   - AgentRuntime.EventSubscriber 异步消费
//   - 双写期：旧路径（直接调 SalesEngine）保留 1 个月,确保平滑迁移
func (s *WebhookService) triggerSalesEngine(channel WebhookChannel, accountID string, p *ParsedPayload, hubMsg *model.MessageHub) {
	// 0. ADR-008:发布事件到 Event Bus（异步,失败不影响主流程）
	//    AgentRuntime 订阅此事件,实现 L1 入口层与 L4 AI 引擎层解耦
	//    即使 AgentRuntime 未启动,事件会被 event.Publish 静默丢弃
	{
		customerID := ""
		if hubMsg != nil {
			customerID = hubMsg.SenderID
		}
		agent_runtime.PublishCustomerMessage(string(channel), accountID, customerID, p.Content, p.EventID)
	}

	// 分支 A：智能体统一编排器（推荐路径，9 步编排完整闭环）
	if s.smartOrchestrator != nil {
		s.triggerSmartOrchestrator(channel, accountID, p, hubMsg)
		return
	}
	// 分支 B：回退路径（仅 SalesEngine 8 步链路，无会话/座席联动）
	if s.salesEngine == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// 分配追踪 ID，使回退链路日志可串联
	ctx = logger.WithTraceID(ctx, "")
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
	// 出站：按 channel 调用对应 Service
	s.sendOutbound(ctx, channel, accountID, p, resp.Reply, hubMsg)
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
func (s *WebhookService) triggerSmartOrchestrator(channel WebhookChannel, accountID string, p *ParsedPayload, hubMsg *model.MessageHub) {
	if s.smartOrchestrator == nil {
		return
	}
	// 轻量同步准备：加载智能体路由上下文 + 构造 IncomingContext（快速，不阻塞接入 worker）
	routeCtx := context.Background()
	routeCtx = logger.WithTraceID(routeCtx, "")
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

	// 性能审计 P1-1：将重 LLM 生成从接入 worker 解耦。
	// 接入 worker 完成轻量准备后立即返回，AI 生成由独立有界池（replySem）执行，
	// 推理饱和时 AI 任务排队等待而非丢弃，避免 4 worker 同步跑 LLM 成为被动回复吞吐天花板。
	go s.runAIGeneration(channel, accountID, p, hubMsg, in, agentCtx)
}

// runAIGeneration 在独立有界池中执行 AI 生成与出站（性能审计 P1-1）。
func (s *WebhookService) runAIGeneration(channel WebhookChannel, accountID string, p *ParsedPayload, hubMsg *model.MessageHub, in *IncomingContext, agentCtx *AgentContext) {
	// 有界信号量：限制并发本地推理数，保护单节点推理栈不被压垮
	s.replySem <- struct{}{}
	defer func() { <-s.replySem }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = logger.WithTraceID(ctx, "")
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
	// AI 自动回复：出站发送
	if result.AIReplied && result.Reply != "" {
		s.sendOutbound(ctx, channel, accountID, p, result.Reply, hubMsg)
		return
	}
	// 其他情况（座席接管 / 仅生成建议未自动回复）：不出站，等座席手动回复
	logger.Ctx(ctx).Info().
		Str("session_id", result.SessionID).
		Str("handler", string(result.HandlerType)).
		Msg("no outbound")
}

// sendOutbound 出站发送（按 channel）；ctx 用于透传 trace_id（来自 triggerSmartOrchestrator / 回退链路）
func (s *WebhookService) sendOutbound(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, content string, hubMsg *model.MessageHub) {
	// R3/V3 幂等守卫：与 AgentRuntime 事件总线订阅共享同一 EventID 守卫。
	// 同一 EventID 仅首条链路出站，杜绝重复消息；同时防御 webhook 平台重复投递
	// （同 EventID 二次到达）导致的重复出站。
	if !agent_runtime.ClaimReply(p.EventID) {
		logger.Ctx(ctx).Info().Str("event_id", p.EventID).Msg("skip duplicate outbound (event already replied)")
		// R9 可观测性：记录被幂等守卫拦截的重复出站
		metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|duplicate")
		return
	}
	// 出站结果追踪：仅当真正成功出站时才保留认领（防重复）；
	// 若全线出站失败则释放认领，允许平台重投在本实例内重试（R3 二次论证修复漏回边角）。
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
		// 企微出站底层 = WeComIntegrationService（与 IntegrationReachAdapter.SendWeCom 共享同一底层，R3 已收敛为单一出站入口）
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
			metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|failed")
		} else {
			sent = true
			metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|success")
		}
	case ChannelFeishu:
		if s.feishuIntegration == nil {
			s.feishuIntegration = NewFeishuIntegrationService(s.db)
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		// Feishu 用 open_id 作为发送目标（p.Sender 已是 open_id）
		if err := s.feishuIntegration.SendMessage(ctx, uint(accID), p.Sender, content); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "feishu").Str("account_id", accountID).Msg("outbound failed")
			metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|failed")
		} else {
			sent = true
			metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|success")
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
			metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|failed")
		} else {
			sent = true
			metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|success")
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
			metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|failed")
		} else {
			sent = true
			metrics.GlobalMetrics.OutboundTotal.Inc(string(channel) + "|success")
		}
	default:
		// 该渠道暂未实现主动出站：记录日志并跳过，避免静默吞掉消息难以排查
		logger.Ctx(ctx).Warn().Str("channel", string(channel)).Str("account_id", accountID).Msg("unsupported outbound channel, skipped")
	}
}

// =================== 工具 ===================

// isDuplicate 基于 eventID 的 TTL 幂等（性能审计 P1-2）。
// 改用 sync.Map + 惰性过期，O(1) 且无全局扫描；过期条目由 startDedupJanitor 后台清理。
func (s *WebhookService) isDuplicate(eventID string) bool {
	if eventID == "" {
		return false
	}
	now := time.Now()
	if v, ok := s.dedup.Load(eventID); ok {
		if exp, ok := v.(time.Time); ok && now.Before(exp) {
			// R9 可观测性：命中去重的重复投递
			metrics.GlobalMetrics.WebhookDedupTotal.Inc("duplicate")
			return true
		}
		s.dedup.Delete(eventID)
	}
	s.dedup.Store(eventID, now.Add(WebhookDedupTTL))
	// R9 可观测性：新事件接受（进入幂等窗口）
	metrics.GlobalMetrics.WebhookDedupTotal.Inc("accepted")
	return false
}

func (s *WebhookService) allowRate(key string) bool {
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
	return b.allow()
}

// startDedupJanitor 后台周期性清理过期的 dedup 条目，避免 sync.Map 无限增长（性能审计 P1-2）。
func (s *WebhookService) startDedupJanitor() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				now := time.Now()
				s.dedup.Range(func(k, v any) bool {
					if exp, ok := v.(time.Time); ok && now.After(exp) {
						s.dedup.Delete(k)
					}
					return true
				})
			}
		}
	}()
}

func (s *WebhookService) generateEventID(channel WebhookChannel, accountID string, body []byte) string {
	h := sha256.Sum256([]byte(string(channel) + ":" + accountID + ":" + string(body)))
	return fmt.Sprintf("evt_%s", hex.EncodeToString(h[:8]))
}

func (s *WebhookService) genMessageID(channel WebhookChannel, accountID string, p *ParsedPayload) string {
	h := sha256.Sum256([]byte(string(channel) + ":" + accountID + ":" + p.Sender + ":" + p.Content + ":" + p.EventID))
	// UnifiedMessage.MessageID 列宽为 varchar(50)，"msg_" 前缀 + 完整 64 hex 会超长。
	// 截断为前 22 hex 字符（共 26 字符，留足唯一空间：2^88）。
	return fmt.Sprintf("msg_%s", hex.EncodeToString(h[:])[:22])
}

// TruncateForStore 截断防止 raw_data 过大
func (s *WebhookService) TruncateForStore(body []byte) string {
	const max = 64 * 1024
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...[truncated]"
}

func (s *WebhookService) getAccountSecret(platform, accountID string) (string, error) {
	if s.accountRepo == nil {
		return "", nil
	}
	// 防止全局 nil db 触发 panic
	if s.db == nil {
		return "", nil
	}
	acc, err := s.accountRepo.GetByPlatform(platform)
	if err != nil {
		return "", nil
	}
	return acc.APISecret, nil
}

// getTelegramWebhookSecret 获取 Telegram Bot 的 webhook secret
// secret 来自 TelegramAccount.WebhookSecret（在 setWebhook 时由商户配置）
// 未配置时返回空字符串，调用方应跳过验签
func (s *WebhookService) getTelegramWebhookSecret(accountID string) string {
	if s.telegramRepo == nil || s.db == nil {
		return ""
	}
	accID, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || accID == 0 {
		return ""
	}
	acc, err := s.telegramRepo.GetByID(uint(accID))
	if err != nil {
		return ""
	}
	return acc.WebhookSecret
}

func (s *WebhookService) getWechatSecrets(accountID string) (string, string) {
	return "", ""
}

// GetWeComSecrets 公开方法：供 controller 层 URL 验证使用
func (s *WebhookService) GetWeComSecrets(accountID string) (string, string, error) {
	return s.getWeComSecrets(accountID)
}

// getWeComSecrets 从 wecom_accounts 读取 token + EncodingAESKey
// accountID 优先按数字 ID 解析；解析失败则取第一条启用 webhook 的账号
func (s *WebhookService) getWeComSecrets(accountID string) (string, string, error) {
	if s.wecomRepo == nil {
		return "", "", errors.New("wecomRepo nil")
	}
	if s.db == nil {
		return "", "", nil
	}
	// 1) 按 ID 解析
	if id, err := strconv.ParseUint(accountID, 10, 64); err == nil && id > 0 {
		acc, err := s.wecomRepo.GetByID(uint(id))
		if err == nil && acc != nil {
			return acc.CallbackToken, acc.EncodingAESKey, nil
		}
	}
	// 2) 兜底：取第一个启用 webhook 的账号
	accs, err := s.wecomRepo.GetByMerchant()
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
func (s *WebhookService) PendingCount() int64 {
	if s.eventRepo == nil {
		return 0
	}
	c, _ := s.eventRepo.CountUnprocessed()
	return c
}

// QueueLen 队列长度
func (s *WebhookService) QueueLen() int { return len(s.queue) }

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
