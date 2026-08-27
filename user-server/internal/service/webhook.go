package service

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"hivemtk-user/internal/channelbot/telegram"
	"hivemtk-user/internal/channelbot/whatsapp"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

type WebhookService struct {
	db          *gorm.DB 
	eventRepo   *repository.WebhookEventRepository
	accountRepo *repository.IntegrationAccountRepository

	wecomRepo   *repository.WeComAccountRepository
	integration *WeComIntegrationService

	feishuIntegration *FeishuIntegrationService
	tgIntegration     *TelegramIntegrationService
	waIntegration     *WhatsAppCloudIntegrationService

	// wechatIntegration 微信公众号客服消息发送（AI 回复出站）
	// 2026-08-25 修复：sendOutbound 原先无 ChannelWechat 分支，公众号 AI 回复被
	// default 分支静默丢弃（仅 Warn 日志），导致"机器人不回复"。
	wechatIntegration *WechatService

	telegramRepo *repository.TelegramAccountRepository

	feishuRepo *repository.FeishuAccountRepository
	waRepo     *repository.WhatsAppCloudAccountRepository

	messageHubRepo *repository.MessageHubRepository
	inboxConvRepo  *repository.InboxConversationRepository
	unifiedMsgRepo repository.UnifiedMessageRepository

	clueRepo repository.ClueRepository

	ingressSvc *InboxIngressService

	salesEngine *SalesEngine
	smartOrchestrator *SmartCSOrchestrator

	agentBindingSvc *ChannelAgentBindingService

	mu        sync.Mutex 
	rlMu      sync.Mutex 
	rlBuckets map[string]*tokenBucket

	workerCount int
	queue       chan *webhookJob
	wg          sync.WaitGroup
	stopCh      chan struct{}
	stopped     bool

	replySem chan struct{}
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

type WebhookChannel string

func NewWebhookService(db *gorm.DB) *WebhookService {
	guardInsecureWebhookAtStartup()
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

	// P1-7: 注册全局 WhatsApp 消息重排序缓冲 FlushHandler
	globalReorderBuffer.FlushHandler = func(accountID, sessionID string, ordered [][]byte) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, raw := range ordered {
			parsed, err := s.ParsePayload(ctx, ChannelWhatsapp, raw)
			if err != nil {
				logger.Errorf("[ReorderBuffer] flush parse payload failed account=%s session=%s: %v", accountID, sessionID, err)
				continue
			}
			hub, err := s.dispatchWhatsApp(ctx, accountID, parsed, raw)
			if err != nil {
				logger.Errorf("[ReorderBuffer] flush dispatchWhatsApp failed account=%s session=%s: %v", accountID, sessionID, err)
				continue
			}
			um := s.ToUnifiedMessage(ctx, ChannelWhatsapp, accountID, parsed)
			s.dispatchToUnified(ctx, um)
			if hub != nil && s.shouldTriggerAI(ctx, ChannelWhatsapp, accountID) {
				s.triggerSalesEngine(ctx, ChannelWhatsapp, accountID, parsed, hub)
			}
		}
	}

	return s
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

		return nil
	}
	return err
}

func (s *WebhookService) ingressHandler(ctx context.Context) webhookIngressAdapter {
	return webhookIngressAdapter{svc: s.ingressSvc}
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

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:

	case <-time.After(2 * time.Second):

	}
}

func (s *WebhookService) startWorkers(ctx context.Context) {
	for i := 0; i < s.workerCount; i++ {
		s.wg.Add(1)
		// 最高标准审计 P1-3 修复：webhook 消费 worker 改走 SafeGo，panic 不再击穿进程
		id := i
		utils.SafeGo(ctx, "webhook.worker", func(ctx context.Context) {
			s.worker(ctx, id)
		})
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

type ReceiveRequest struct {
	Channel   WebhookChannel
	AccountID string
	Body      []byte
	Headers   map[string]string
	SourceIP  string
	Query map[string]string
}

type ReceiveResult struct {
	Accepted    bool   `json:"accepted"`
	EventID     string `json:"event_id"`
	Duplicate   bool   `json:"duplicate"`
	RateLimit   bool   `json:"rate_limit"`
	VerifyFail  bool   `json:"verify_failed"`
	QueueFull   bool   `json:"queue_full,omitempty"`
	Reason      string `json:"reason,omitempty"`
	EventType   string `json:"event_type,omitempty"`
	Dispatched  bool   `json:"dispatched,omitempty"`
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

	verified, err := s.Verify(ctx, req.Channel, req.AccountID, req.Body, req.Headers, req.Query)
	if err != nil {
		logger.Errorf("[Webhook] 验签异常 channel=%s account=%s: %v", req.Channel, req.AccountID, err)
		return &ReceiveResult{Accepted: false, VerifyFail: true, Reason: "verify error: " + err.Error()}, nil
	}
	if !verified {
		return &ReceiveResult{Accepted: false, VerifyFail: true, Reason: "signature mismatch"}, nil
	}

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

	if s.isDuplicate(ctx, payload.EventID) {
		return &ReceiveResult{
			Accepted:  true,
			EventID:   payload.EventID,
			Duplicate: true,
		}, nil
	}

	key := string(req.Channel) + ":" + req.AccountID
	if !s.allowRate(ctx, key) {
		return &ReceiveResult{Accepted: false, RateLimit: true, Reason: "rate limited"}, nil
	}

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
		return &ReceiveResult{
			Accepted:  false,
			QueueFull: true,
			Reason:    "queue full, please retry after a short delay",
			EventID:   payload.EventID,
			EventType: payload.EventType,
		}, nil
	}

	return &ReceiveResult{
		Accepted:  true,
		EventID:   payload.EventID,
		EventType: payload.EventType,
	}, nil
}

// insecureWebhookStartupError 判定 ALLOW_INSECURE_WEBHOOK=true 是否禁止启动。
//
// W-1 验签绕过防护：该开关会跳过所有渠道验签，生产环境一旦误配等于关闭回调鉴权。
// 项目现有环境判断惯例为 APP_ENV（见 internal/pkg/db/db.go），此处兼容 MODE 别名。
// 返回 nil 表示允许启动；返回非 nil 时调用方应 log.Fatal 拒绝启动。
func insecureWebhookStartupError(appEnv, mode, allowInsecure string) error {
	if allowInsecure != "true" {
		return nil
	}
	env := strings.ToLower(strings.TrimSpace(appEnv))
	if env == "" {
		env = strings.ToLower(strings.TrimSpace(mode))
	}
	switch env {
	case "", "dev", "development", "debug":
		return nil
	default:
		return errors.New("ALLOW_INSECURE_WEBHOOK=true 禁止在非开发环境(APP_ENV/MODE=" + env + ")使用：" +
			"该开关会跳过全部渠道 webhook 验签。请移除该环境变量，并为企业微信/飞书/Telegram 等" +
			"各渠道账号配置正确的 CallbackToken/AppSecret 后重启")
	}
}

var insecureWebhookGuardOnce sync.Once

// guardInsecureWebhookAtStartup 应用启动配置加载守卫：production 环境下
// ALLOW_INSECURE_WEBHOOK=true 直接拒绝启动（dev/test 保持现状）。
func guardInsecureWebhookAtStartup() {
	insecureWebhookGuardOnce.Do(func() {
		if err := insecureWebhookStartupError(
			os.Getenv("APP_ENV"), os.Getenv("MODE"), os.Getenv("ALLOW_INSECURE_WEBHOOK"),
		); err != nil {
			log.Fatalf("[SECURITY] %v", err)
		}
	})
}

func (s *WebhookService) Verify(ctx context.Context, channel WebhookChannel, accountID string, body []byte, headers map[string]string, query map[string]string) (bool, error) {
	// 显式开发模式总开关：跳过全部渠道验签（仅限联调；生产严禁设置）
	if os.Getenv("ALLOW_INSECURE_WEBHOOK") == "true" {
		return true, nil
	}
	switch channel {
	case ChannelWeCom:
		token, aesKey, err := s.getWeComSecrets(ctx, accountID)
		if err != nil || token == "" {
			return false, fmt.Errorf("wecom token missing: %v", err)
		}
		return verifyWeCom(token, aesKey, body, query)
	case ChannelWechat:
		// W-6：secret 从公众号账号配置读取；未配置时明确 WARN 并跳过该渠道验签
		// （原 getWechatSecrets 恒空串导致该渠道验签永远失败、回调全部被拒）。
		token, _ := s.getWechatSecrets(ctx, accountID)
		if token == "" {
			logger.Warnf("[Webhook] wechat 验签 secret 未配置 account=%s，跳过该渠道验签（W-6）", accountID)
			return true, nil
		}
		return verifyWechat(token, body, headers), nil
	case ChannelDouyin, ChannelTiktok:
		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		return verifyHMAC(secret, body, headers, "X-Lark-Signature", "X-Douyin-Signature", "Signature"), nil
	case ChannelTelegram:

		secret := s.getTelegramWebhookSecret(ctx, accountID)
		if secret == "" {

			if os.Getenv("ALLOW_INSECURE_WEBHOOK") != "true" {
				return false, errors.New("telegram webhook secret 未配置；开发放行需显式 ALLOW_INSECURE_WEBHOOK=true")
			}
			return true, nil // 显式开发模式：跳过验签
		}
		headerSecret := headers["X-Telegram-Bot-Api-Secret-Token"]
		if headerSecret == "" {
			headerSecret = headers["x-telegram-bot-api-secret-token"]
		}
		if headerSecret == "" {
			return false, errors.New("missing X-Telegram-Bot-Api-Secret-Token header")
		}
		return telegram.VerifyWebhook(secret, headerSecret), nil
	case ChannelFeishu:
		// 飞书使用专属 EncryptKey 与官方签名口径
		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		if secret == "" {
			// 2026-08-25 修复（交付阻断）：验签数据源断链——管理端只写 feishu_accounts.encrypt_key，
			// integration_accounts 无记录 → secret 恒空 → 所有 POST 事件 401。回退读飞书账号表。
			secret = s.getFeishuEncryptKey(ctx, accountID)
		}
		return verifyFeishu(secret, body, headers), nil
	case ChannelKuaishou, ChannelXiaohongshu, ChannelXianyu:
		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		return verifyHMAC(secret, body, headers, "X-Signature", "Signature", "X-Hub-Signature-256"), nil
	case ChannelWhatsapp:

		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		if secret == "" {
			if os.Getenv("ALLOW_INSECURE_WEBHOOK") != "true" {
			return false, errors.New("whatsapp app secret 未配置；开发放行需显式 ALLOW_INSECURE_WEBHOOK=true")
			}
		return true, nil // 显式开发模式：跳过验签
		}
		return whatsapp.VerifyWebhook(secret, body, headers["X-Hub-Signature-256"]), nil
	default:
		secret, _ := s.getAccountSecret(ctx, string(channel), accountID)
		if secret == "" {

			if os.Getenv("ALLOW_INSECURE_WEBHOOK") != "true" {
			return false, errors.New("webhook secret 未配置(channel=" + string(channel) + ")；开发环境请显式设置 ALLOW_INSECURE_WEBHOOK=true")
			}
		}
		return verifyHMAC(secret, body, headers, "X-Signature", "Signature", "X-Hub-Signature-256"), nil
	}
}

// verifyFeishu 飞书官方事件订阅签名校验：
// X-Lark-Signature = base64(sha256(encrypt_key + X-Lark-Timestamp + X-Lark-Nonce + body))。
// v3 审计 P1-6 修复：原走泛化 verifyHMAC(hex 口径、header 名拼凑)，真实飞书事件必然验签失败。
func verifyFeishu(encryptKey string, body []byte, headers map[string]string) bool {
	if encryptKey == "" {
		return false
	}
	sig := headers["X-Lark-Signature"]
	if sig == "" {
		sig = headers["x-lark-signature"]
	}
	ts := headers["X-Lark-Timestamp"]
	if ts == "" {
		ts = headers["x-lark-timestamp"]
	}
	nonce := headers["X-Lark-Nonce"]
	if nonce == "" {
		nonce = headers["x-lark-nonce"]
	}
	if sig == "" || ts == "" || nonce == "" {
		return false
	}
	h := sha256.New()
	h.Write([]byte(encryptKey + ts + nonce))
	h.Write(body)
	expected := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) == 1
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

type ParsedPayload struct {
	EventID   string         `json:"event_id"`
	EventType string         `json:"event_type"`
	Sender    string         `json:"sender,omitempty"`
	Content   string         `json:"content,omitempty"`
	ChatID    string         `json:"chat_id,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

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

	hubMsg, tgExtra, dispatchErr := s.dispatchToChannel(ctx, channel, job.account, payload, job.raw, job.header)
	if dispatchErr != nil {
		logger.Errorf("[Webhook] dispatch %s failed event=%s: %v", channel, job.event.EventID, dispatchErr)
	}

	if hubMsg == nil && dispatchErr == nil {
		known := channel == ChannelWeCom || channel == ChannelWhatsapp ||
			channel == ChannelTelegram || channel == ChannelFeishu
		if known {
			logger.Infof("[Webhook] skip non-message event channel=%s event=%s", channel, job.event.EventID)
			s.markProcessed(ctx, job.event)
			return
		}
	}

	um := s.ToUnifiedMessage(ctx, channel, job.account, payload)
	if um.AccountID == "" {
		um.AccountID = job.event.EventID
	}
	if err := s.dispatchToUnified(ctx, um); err != nil {
		s.retryWithBackoff(ctx, job, payload, err)
		return
	}

	triggerAI := hubMsg != nil && s.shouldTriggerAI(ctx, channel, job.account)
	if triggerAI {
		if channel != ChannelTelegram || !hubMsg.IsGroup {
			s.triggerSalesEngine(ctx, channel, job.account, payload, hubMsg)
		} else {
			mentioned := tgExtra != nil && tgExtra.Mentioned
			newOpp := tgExtra != nil && tgExtra.NewOpportunity
			switch {
			case mentioned:

				s.triggerSalesEngine(ctx, channel, job.account, payload, hubMsg)
			case newOpp && s.tgLeadOutreachAllowed(ctx, job.account, payload.ChatID, payload.Sender):

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
	case ChannelDouyin, ChannelTiktok:
		return s.dispatchDouyin(ctx, accountID, p, raw)
	default:

		return nil, nil, nil
	}
}

