package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"marketing/internal/channelbot/core"
	"marketing/internal/channelbot/telegram"
	"marketing/internal/channelbot/whatsapp"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/httpclient"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// ============================================================================
// 飞书服务（FeishuService + FeishuIntegrationService）
// ----------------------------------------------------------------------------
// 商业产品级场景：
//   1. 商户在 UI 配置 App ID / App Secret / Encrypt Key / Verification Token
//   2. 飞书回调（URL 验证 + 事件）→ WebhookService 验签 → FeishuService 解析
//   3. 入消息中台 + 收件箱 → 触发 智能体
//   4. 智能体回复 → 通过飞书 Open API 出站
// ============================================================================

// FeishuService 飞书账号管理
type FeishuService struct {
	db          *gorm.DB
	accountRepo *repository.FeishuAccountRepository
}

// NewFeishuService 创建飞书服务
func NewFeishuService(db *gorm.DB) *FeishuService {
	r := repository.NewFeishuAccountRepository()
	if db != nil {
		r.SetDB(context.Background(), db)
	}
	return &FeishuService{
		db:          db,
		accountRepo: r,
	}
}

// CreateAccount 创建飞书账号
func (s *FeishuService) CreateAccount(ctx context.Context, input *model.FeishuAccount) (*model.FeishuAccount, error) {
	if input.AccountName == "" || input.AppID == "" || input.AppSecret == "" {
		return nil, errors.New("account_name, app_id, app_secret are required")
	}
	if input.Status == 0 {
		input.Status = 1
	}
	if err := s.accountRepo.Create(ctx, input); err != nil {
		return nil, err
	}
	return input, nil
}

// UpdateAccount 更新账号
func (s *FeishuService) UpdateAccount(ctx context.Context, acc *model.FeishuAccount) error {
	return s.accountRepo.Update(ctx, acc)
}

// GetAccount 获取账号
func (s *FeishuService) GetAccount(ctx context.Context, id uint) (*model.FeishuAccount, error) {
	return s.accountRepo.GetByID(ctx, id)
}

// ListAccounts 列出所有账号
func (s *FeishuService) ListAccounts(ctx context.Context) ([]*model.FeishuAccount, error) {
	return s.accountRepo.GetAll(ctx)
}

// DeleteAccount 删除账号
func (s *FeishuService) DeleteAccount(ctx context.Context, id uint) error {
	return s.accountRepo.Delete(ctx, id)
}

// GetSecretsByAccountID 获取 app_id + verification_token + encrypt_key（按 ID）
func (s *FeishuService) GetSecretsByAccountID(ctx context.Context, accountID string) (appID, token, encryptKey string, err error) {
	if s.db == nil {
		return "", "", "", errors.New("db nil")
	}
	var acc model.FeishuAccount
	// 优先按 ID 解析
	var id uint
	if _, scanErr := fmt.Sscanf(accountID, "%d", &id); scanErr == nil && id > 0 {
		if err := s.db.WithContext(ctx).First(&acc, id).Error; err == nil {
			return acc.AppID, acc.VerificationToken, acc.EncryptKey, nil
		}
	}
	// 兜底：取第一个启用的账号
	if err := s.db.Where("webhook_enabled = ? AND status = ?", true, 1).First(&acc).Error; err == nil {
		return acc.AppID, acc.VerificationToken, acc.EncryptKey, nil
	}
	// 最后兜底：第一条
	if err := s.db.Order("id ASC").First(&acc).Error; err != nil {
		return "", "", "", err
	}
	return acc.AppID, acc.VerificationToken, acc.EncryptKey, nil
}

// FeishuIntegrationService 飞书消息分发 + 出站
type FeishuIntegrationService struct {
	db     *gorm.DB
	feishu *FeishuService
	hub    *MessageHubService
	inbox  *InboxService
}

// NewFeishuIntegrationService 创建集成服务
func NewFeishuIntegrationService(db *gorm.DB) *FeishuIntegrationService {
	return &FeishuIntegrationService{
		db:     db,
		feishu: NewFeishuService(db),
		hub:    NewMessageHubServiceWithDB(db, nil),
		inbox:  NewInboxServiceWithDB(db),
	}
}

// IngestFeishuMessage 入站消息
type FeishuIngestRequest struct {
	AccountID uint
	OpenID    string
	UnionID   string
	UserID    string
	Name      string
	MsgType   string
	Content   string
	MsgID     string
	ChatID    string
	ChatType  string // p2p / group
}

// IngestMessage 飞书入站消息入消息中台 + 收件箱
func (s *FeishuIntegrationService) IngestMessage(ctx context.Context, req *FeishuIngestRequest) (*model.MessageHub, *model.InboxConversation, error) {
	if s.db == nil {
		return nil, nil, errors.New("db nil")
	}
	if req.MsgType == "" {
		req.MsgType = "text"
	}
	if req.ChatType == "" {
		req.ChatType = "p2p"
	}
	if req.MsgID == "" {
		req.MsgID = fmt.Sprintf("feishu-%d", time.Now().UnixNano())
	}
	convID := req.ChatID
	if convID == "" {
		convID = fmt.Sprintf("feishu-%d-%s", req.AccountID, req.OpenID)
	}
	hubMsg, err := s.hub.Push(ctx, &PushMessageRequest{
		Platform:       "feishu",
		AccountID:      fmt.Sprintf("%d", req.AccountID),
		MsgID:          req.MsgID,
		Direction:      "inbound",
		MsgType:        req.MsgType,
		SenderID:       req.OpenID,
		SenderName:     req.Name,
		Content:        req.Content,
		ConversationID: convID,
		IsGroup:        req.ChatType == "group",
		GroupID:        req.ChatID,
		SentAt:         timePtr(time.Now()),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("hub push: %w", err)
	}
	var conv *model.InboxConversation
	if hubMsg != nil {
		conv, err = s.inbox.UpsertFromHubMessage(ctx, hubMsg)
		if err != nil {
			return hubMsg, nil, fmt.Errorf("inbox upsert: %w", err)
		}
	}
	return hubMsg, conv, nil
}

// SendMessage 通过飞书 Open API 发送文本消息
// 参考：https://open.feishu.cn/document/server-docs/im-v1/message/create
func (s *FeishuIntegrationService) SendMessage(ctx context.Context, accountID uint, openID, content string) error {
	if s.db == nil {
		return errors.New("db nil")
	}
	acc, err := s.feishu.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get feishu account: %w", err)
	}
	// 拿 access_token（缓存复用 / 过期刷新）
	tk, err := s.getAccessToken(ctx, acc)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}
	// 构造消息体
	body := map[string]any{
		"receive_id": openID,
		"msg_type":   "text",
		"content":    map[string]string{"text": content},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id",
		bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+tk)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return fmt.Errorf("send feishu msg: %w", err)
	}
	defer resp.Body.Close()
	respB, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// 记录错误
		now := time.Now()
		acc.LastErrorAt = &now
		acc.LastErrorMsg = string(respB)
		_ = s.feishu.UpdateAccount(ctx, acc)
		return fmt.Errorf("feishu api status %d: %s", resp.StatusCode, string(respB))
	}
	// 写出站消息记录
	outMsg := &model.FeishuMessage{
		AccountID: accountID,
		MsgID:     fmt.Sprintf("feishu-out-%d", time.Now().UnixNano()),
		ChatID:    openID,
		ChatType:  "p2p",
		SenderID:  openID,
		MsgType:   "text",
		Content:   content,
		Direction: "outbound",
	}
	_ = s.db.Create(outMsg).Error
	// 写消息中台出站
	hubMsg, _ := s.hub.Push(ctx, &PushMessageRequest{
		Platform:       "feishu",
		AccountID:      fmt.Sprintf("%d", accountID),
		MsgID:          outMsg.MsgID,
		Direction:      "outbound",
		MsgType:        "text",
		SenderID:       fmt.Sprintf("%d", accountID),
		ReceiverID:     openID,
		Content:        content,
		ConversationID: fmt.Sprintf("feishu-%d-%s", accountID, openID),
		IsAIReply:      true,
		AIAgent:        "sales_engine",
		SentAt:         timePtr(time.Now()),
	})
	if hubMsg != nil {
		_, _ = s.inbox.UpsertFromHubMessage(ctx, hubMsg)
	}
	return nil
}

// getAccessToken 内部拿 access_token（含缓存过期刷新）
func (s *FeishuIntegrationService) getAccessToken(ctx context.Context, acc *model.FeishuAccount) (string, error) {
	if acc.AccessToken != "" && acc.TokenExpires != nil && time.Now().Before(*acc.TokenExpires) {
		return acc.AccessToken, nil
	}
	// 调接口：POST https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal
	body := map[string]string{
		"app_id":     acc.AppID,
		"app_secret": acc.AppSecret,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Code != 0 {
		return "", errors.New(out.Msg)
	}
	expires := time.Now().Add(time.Duration(out.Expire-300) * time.Second)
	acc.AccessToken = out.TenantAccessToken
	acc.TokenExpires = &expires
	_ = s.feishu.UpdateAccount(ctx, acc)
	return out.TenantAccessToken, nil
}

// RefreshAccessToken 公开方法：强制刷新 access_token（用于 controller 主动刷新）
func (s *FeishuIntegrationService) RefreshAccessToken(ctx context.Context, acc *model.FeishuAccount) error {
	if acc == nil {
		return errors.New("account nil")
	}
	acc.AccessToken = ""
	acc.TokenExpires = nil
	_, err := s.getAccessToken(ctx, acc)
	return err
}

// GetAccessTokenForTest 公开方法：测试用拿 access_token（不清空）
func (s *FeishuIntegrationService) GetAccessTokenForTest(ctx context.Context, acc *model.FeishuAccount) (string, error) {
	return s.getAccessToken(ctx, acc)
}

// ============================================================================
// Telegram 集成服务
// ============================================================================

// TelegramService Telegram 账号管理
type TelegramService struct {
	db      *gorm.DB
	accRepo *repository.TelegramAccountRepository
}

func NewTelegramService(db *gorm.DB) *TelegramService {
	r := repository.NewTelegramAccountRepository()
	if db != nil {
		r.SetDB(context.Background(), db)
	}
	return &TelegramService{db: db, accRepo: r}
}

func (s *TelegramService) CreateAccount(ctx context.Context, acc *model.TelegramAccount) (*model.TelegramAccount, error) {
	if acc.AccountName == "" || acc.BotToken == "" {
		return nil, errors.New("account_name and bot_token are required")
	}
	if acc.Status == 0 {
		acc.Status = 1
	}
	if err := s.accRepo.Create(ctx, acc); err != nil {
		return nil, err
	}
	return acc, nil
}

func (s *TelegramService) GetAccount(ctx context.Context, id uint) (*model.TelegramAccount, error) {
	return s.accRepo.GetByID(ctx, id)
}

func (s *TelegramService) ListAccounts(ctx context.Context) ([]*model.TelegramAccount, error) {
	return s.accRepo.GetAll(ctx)
}

func (s *TelegramService) UpdateAccount(ctx context.Context, acc *model.TelegramAccount) error {
	return s.accRepo.Update(ctx, acc)
}

func (s *TelegramService) DeleteAccount(ctx context.Context, id uint) error {
	return s.accRepo.Delete(ctx, id)
}

// GetSecretsByAccountID 获取 bot_token + webhook_secret
func (s *TelegramService) GetSecretsByAccountID(ctx context.Context, accountID string) (botToken, webhookSecret string, err error) {
	if s.db == nil {
		return "", "", errors.New("db nil")
	}
	var acc model.TelegramAccount
	var id uint
	if _, scanErr := fmt.Sscanf(accountID, "%d", &id); scanErr == nil && id > 0 {
		if err := s.db.WithContext(ctx).First(&acc, id).Error; err == nil {
			return acc.BotToken, acc.WebhookSecret, nil
		}
	}
	if err := s.db.Where("webhook_enabled = ? AND status = ?", true, 1).First(&acc).Error; err == nil {
		return acc.BotToken, acc.WebhookSecret, nil
	}
	if err := s.db.Order("id ASC").First(&acc).Error; err != nil {
		return "", "", err
	}
	return acc.BotToken, acc.WebhookSecret, nil
}

// TelegramIntegrationService Telegram 消息分发 + 出站
type TelegramIntegrationService struct {
	db    *gorm.DB
	tg    *TelegramService
	hub   *MessageHubService
	inbox *InboxService
}

func NewTelegramIntegrationService(db *gorm.DB) *TelegramIntegrationService {
	return &TelegramIntegrationService{
		db:    db,
		tg:    NewTelegramService(db),
		hub:   NewMessageHubServiceWithDB(db, nil),
		inbox: NewInboxServiceWithDB(db),
	}
}

type TelegramIngestRequest struct {
	AccountID  uint
	ChatID     int64
	FromID     int64
	FromName   string
	Username   string
	MsgType    string
	Content    string
	MsgID      int64
	IsGroup    bool
	GroupTitle string
}

// IngestMessage Telegram 入站
func (s *TelegramIntegrationService) IngestMessage(ctx context.Context, req *TelegramIngestRequest) (*model.MessageHub, *model.InboxConversation, error) {
	if s.db == nil {
		return nil, nil, errors.New("db nil")
	}
	if req.MsgType == "" {
		req.MsgType = "text"
	}
	chatIDStr := fmt.Sprintf("%d", req.ChatID)
	fromIDStr := fmt.Sprintf("%d", req.FromID)
	msgIDStr := fmt.Sprintf("tg_%d", req.MsgID)
	convID := chatIDStr
	hubMsg, err := s.hub.Push(ctx, &PushMessageRequest{
		Platform:       "telegram",
		AccountID:      fmt.Sprintf("%d", req.AccountID),
		MsgID:          msgIDStr,
		Direction:      "inbound",
		MsgType:        req.MsgType,
		SenderID:       fromIDStr,
		SenderName:     req.FromName,
		Content:        req.Content,
		ConversationID: convID,
		IsGroup:        req.IsGroup,
		GroupID:        chatIDStr,
		SentAt:         timePtr(time.Now()),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("hub push: %w", err)
	}
	var conv *model.InboxConversation
	if hubMsg != nil {
		conv, err = s.inbox.UpsertFromHubMessage(ctx, hubMsg)
		if err != nil {
			return hubMsg, nil, fmt.Errorf("inbox upsert: %w", err)
		}
	}
	return hubMsg, conv, nil
}

// SendMessage 通过 Telegram Bot API 发送消息
// POST https://api.telegram.org/bot<token>/sendMessage
func (s *TelegramIntegrationService) SendMessage(ctx context.Context, accountID uint, chatID int64, content string) error {
	if s.db == nil {
		return errors.New("db nil")
	}
	acc, err := s.tg.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get tg account: %w", err)
	}
	// 主动发消息委托独立包 channelbot/telegram（纯协议层）
	// 注入统一出站 client（httpclient.Client，带超时+连接池），与 R4 出站 client 收敛一致，并使测试可拦截
	cli := telegram.NewClient(acc.BotToken, core.WithHTTPClient(httpclient.Client))
	if _, err := cli.SendMessage(ctx, chatID, content); err != nil {
		now := time.Now()
		acc.LastErrorAt = &now
		acc.LastErrorMsg = err.Error()
		_ = s.tg.UpdateAccount(ctx, acc)
		return fmt.Errorf("send tg msg: %w", err)
	}
	// 写消息中台
	chatIDStr := fmt.Sprintf("%d", chatID)
	hubMsg, _ := s.hub.Push(ctx, &PushMessageRequest{
		Platform:       "telegram",
		AccountID:      fmt.Sprintf("%d", accountID),
		MsgID:          fmt.Sprintf("tg-out-%d", time.Now().UnixNano()),
		Direction:      "outbound",
		MsgType:        "text",
		SenderID:       fmt.Sprintf("%d", accountID),
		ReceiverID:     chatIDStr,
		Content:        content,
		ConversationID: chatIDStr,
		IsAIReply:      true,
		AIAgent:        "sales_engine",
		SentAt:         timePtr(time.Now()),
	})
	if hubMsg != nil {
		_, _ = s.inbox.UpsertFromHubMessage(ctx, hubMsg)
	}
	return nil
}

func subtleConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	res := 0
	for i := 0; i < len(a); i++ {
		res |= int(a[i]) ^ int(b[i])
	}
	return res == 0
}

// ============================================================================
// WhatsApp Cloud 集成服务
// ============================================================================

// WhatsAppCloudService WhatsApp Cloud API 账号管理
type WhatsAppCloudService struct {
	db      *gorm.DB
	accRepo *repository.WhatsAppCloudAccountRepository
}

func NewWhatsAppCloudService(db *gorm.DB) *WhatsAppCloudService {
	r := repository.NewWhatsAppCloudAccountRepository()
	if db != nil {
		r.SetDB(context.Background(), db)
	}
	return &WhatsAppCloudService{db: db, accRepo: r}
}

func (s *WhatsAppCloudService) CreateAccount(ctx context.Context, acc *model.WhatsAppCloudAccount) (*model.WhatsAppCloudAccount, error) {
	if acc.AccountName == "" || acc.PhoneNumberID == "" || acc.AccessToken == "" {
		return nil, errors.New("account_name, phone_number_id, access_token are required")
	}
	if acc.Status == 0 {
		acc.Status = 1
	}
	if err := s.accRepo.Create(ctx, acc); err != nil {
		return nil, err
	}
	return acc, nil
}

func (s *WhatsAppCloudService) GetAccount(ctx context.Context, id uint) (*model.WhatsAppCloudAccount, error) {
	return s.accRepo.GetByID(ctx, id)
}

func (s *WhatsAppCloudService) GetAccountByPhone(ctx context.Context, phoneID string) (*model.WhatsAppCloudAccount, error) {
	return s.accRepo.GetByPhoneNumberID(ctx, phoneID)
}

func (s *WhatsAppCloudService) ListAccounts(ctx context.Context) ([]*model.WhatsAppCloudAccount, error) {
	return s.accRepo.GetAll(ctx)
}

func (s *WhatsAppCloudService) UpdateAccount(ctx context.Context, acc *model.WhatsAppCloudAccount) error {
	return s.accRepo.Update(ctx, acc)
}

func (s *WhatsAppCloudService) DeleteAccount(ctx context.Context, id uint) error {
	return s.accRepo.Delete(ctx, id)
}

// GetSecretsByAccountID 获取 access_token + app_secret
func (s *WhatsAppCloudService) GetSecretsByAccountID(ctx context.Context, accountID string) (token, appSecret string, err error) {
	if s.db == nil {
		return "", "", errors.New("db nil")
	}
	var acc model.WhatsAppCloudAccount
	var id uint
	if _, scanErr := fmt.Sscanf(accountID, "%d", &id); scanErr == nil && id > 0 {
		if err := s.db.WithContext(ctx).First(&acc, id).Error; err == nil {
			return acc.AccessToken, acc.AppSecret, nil
		}
	}
	if err := s.db.Where("webhook_enabled = ? AND status = ?", true, 1).First(&acc).Error; err == nil {
		return acc.AccessToken, acc.AppSecret, nil
	}
	if err := s.db.Order("id ASC").First(&acc).Error; err != nil {
		return "", "", err
	}
	return acc.AccessToken, acc.AppSecret, nil
}

// VerifyWhatsAppSignature 校验 X-Hub-Signature-256（sha256=hex）
func VerifyWhatsAppSignature(appSecret string, body []byte, signature string) bool {
	if appSecret == "" {
		return true
	}
	signature = strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtleConstantTimeEqual(expected, signature)
}

// WhatsAppCloudIntegrationService WhatsApp Cloud 消息分发 + 出站
type WhatsAppCloudIntegrationService struct {
	db    *gorm.DB
	wa    *WhatsAppCloudService
	hub   *MessageHubService
	inbox *InboxService
}

func NewWhatsAppCloudIntegrationService(db *gorm.DB) *WhatsAppCloudIntegrationService {
	return &WhatsAppCloudIntegrationService{
		db:    db,
		wa:    NewWhatsAppCloudService(db),
		hub:   NewMessageHubServiceWithDB(db, nil),
		inbox: NewInboxServiceWithDB(db),
	}
}

type WhatsAppIngestRequest struct {
	AccountID    uint
	PhoneFrom    string // 客户手机号（E.164）
	CustomerName string
	MsgType      string
	Content      string
	MsgID        string
	ChatID       string
}

// IngestMessage WhatsApp Cloud 入站
func (s *WhatsAppCloudIntegrationService) IngestMessage(ctx context.Context, req *WhatsAppIngestRequest) (*model.MessageHub, *model.InboxConversation, error) {
	if s.db == nil {
		return nil, nil, errors.New("db nil")
	}
	if req.MsgType == "" {
		req.MsgType = "text"
	}
	if req.ChatID == "" {
		req.ChatID = req.PhoneFrom
	}
	hubMsg, err := s.hub.Push(ctx, &PushMessageRequest{
		Platform:       "whatsapp",
		AccountID:      fmt.Sprintf("%d", req.AccountID),
		MsgID:          req.MsgID,
		Direction:      "inbound",
		MsgType:        req.MsgType,
		SenderID:       req.PhoneFrom,
		SenderName:     req.CustomerName,
		Content:        req.Content,
		ConversationID: req.ChatID,
		SentAt:         timePtr(time.Now()),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("hub push: %w", err)
	}
	var conv *model.InboxConversation
	if hubMsg != nil {
		conv, err = s.inbox.UpsertFromHubMessage(ctx, hubMsg)
		if err != nil {
			return hubMsg, nil, fmt.Errorf("inbox upsert: %w", err)
		}
	}
	return hubMsg, conv, nil
}

// SendMessage 通过 WhatsApp Cloud API 发送文本
// POST https://graph.facebook.com/v18.0/<phone_number_id>/messages
func (s *WhatsAppCloudIntegrationService) SendMessage(ctx context.Context, accountID uint, toPhone, content string) error {
	if s.db == nil {
		return errors.New("db nil")
	}
	acc, err := s.wa.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get wa account: %w", err)
	}
	// 主动发消息委托独立包 channelbot/whatsapp（官方 Cloud API，合规）
	// 注入统一出站 client（httpclient.Client，带超时+连接池），与 R4 出站 client 收敛一致，并使测试可拦截
	cli := whatsapp.NewCloudClient(acc.PhoneNumberID, acc.AccessToken, core.WithHTTPClient(httpclient.Client))
	if _, err := cli.SendText(ctx, toPhone, content); err != nil {
		now := time.Now()
		acc.LastErrorAt = &now
		acc.LastErrorMsg = err.Error()
		_ = s.wa.UpdateAccount(ctx, acc)
		return fmt.Errorf("send wa msg: %w", err)
	}
	// 写消息中台
	hubMsg, _ := s.hub.Push(ctx, &PushMessageRequest{
		Platform:       "whatsapp",
		AccountID:      fmt.Sprintf("%d", accountID),
		MsgID:          fmt.Sprintf("wa-out-%d", time.Now().UnixNano()),
		Direction:      "outbound",
		MsgType:        "text",
		SenderID:       fmt.Sprintf("%d", accountID),
		ReceiverID:     toPhone,
		Content:        content,
		ConversationID: toPhone,
		IsAIReply:      true,
		AIAgent:        "sales_engine",
		SentAt:         timePtr(time.Now()),
	})
	if hubMsg != nil {
		_, _ = s.inbox.UpsertFromHubMessage(ctx, hubMsg)
	}
	return nil
}

// ============================================================================
// 飞书事件加密/解密（用于卡片回调等可选加密场景）
// ============================================================================

// DecryptFeishuEvent 用 EncryptKey 解密飞书事件 payload
func DecryptFeishuEvent(encryptKey, encrypted string) ([]byte, error) {
	if encryptKey == "" {
		return nil, errors.New("encrypt_key empty")
	}
	// encryptKey 是 32 字节字符串作为 AES-256 key（截断/补足）
	key := []byte(encryptKey)
	if len(key) > 32 {
		key = key[:32]
	} else {
		pad := make([]byte, 32-len(key))
		key = append(key, pad...)
	}
	enc, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(enc)%aes.BlockSize != 0 || len(enc) < aes.BlockSize {
		return nil, errors.New("invalid encrypted length")
	}
	iv := enc[:aes.BlockSize]
	ciphertext := enc[aes.BlockSize:]
	mode := cipher.NewCBCDecrypter(block, iv)
	plain := make([]byte, len(ciphertext))
	mode.CryptBlocks(plain, ciphertext)
	// PKCS#7 去填充
	padLen := int(plain[len(plain)-1])
	if padLen < 1 || padLen > aes.BlockSize {
		return nil, errors.New("invalid padding")
	}
	plain = plain[:len(plain)-padLen]
	return plain, nil
}

// 随机 IV（PKCS#7 填充，标准做法）
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		logger.Errorf("rand failed: %v", err)
	}
	return b
}

// timePtr 工具
func timePtr(t time.Time) *time.Time { return &t }
