package service

import (

	"context"

	"crypto/subtle"

	"encoding/json"

	"errors"

	"fmt"

	"strconv"

	"strings"

	"time"

	"hivemtk-user/internal/model"
)

// getFeishuEncryptKey 获取飞书账号 EncryptKey。
// 数据源断链修复（2026-08-25）：管理端只往 feishu_accounts.encrypt_key 写值，
// 而 integration_accounts.api_secret 通常无记录，导致 POST 事件验签/加密事件解密恒失败。
// 统一走本函数取键；账号不存在或 ID 非法返回空串（调用方自行决定 fail 策略）。
func (s *WebhookService) getFeishuEncryptKey(ctx context.Context, accountID string) string {
	if s.db == nil || s.feishuRepo == nil {
		return ""
	}
	id, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || id == 0 {
		return ""
	}
	acc, err := s.feishuRepo.GetByID(ctx, uint(id))
	if err != nil || acc == nil {
		return ""
	}
	return acc.EncryptKey
}

// HandleFeishuURLVerification 处理飞书事件订阅的 POST url_verification 挑战。
// 官方流程：配置回调 URL 时飞书 POST {"challenge":"...","token":"...","type":"url_verification"}
// （开启 Encrypt Key 时为 {"encrypt":"..."} 信封），服务端必须原样回显 {"challenge": "..."}。
// 返回 (challenge, true, nil) 表示已处理；(…, false, nil) 表示非验证请求（调用方继续常规处理）；
// 校验失败返回 err（token 与账号 VerificationToken 不匹配等，防任意第三方伪造绑定）。
func (s *WebhookService) HandleFeishuURLVerification(ctx context.Context, accountID string, raw []byte) (string, bool, error) {
	payload := raw
	var probe struct {
		Encrypt   string `json:"encrypt"`
		Challenge string `json:"challenge"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", false, nil // 非 JSON，交给常规链路报错
	}
	isVerification := probe.Type == "url_verification"
	if !isVerification && probe.Encrypt == "" {
		return "", false, nil
	}
	if probe.Encrypt != "" {
		key := s.getFeishuEncryptKey(ctx, accountID)
		if key == "" {
			return "", true, errors.New("feishu encrypt_key not configured")
		}
		plain, derr := DecryptFeishuEvent(key, probe.Encrypt)
		if derr != nil {
			return "", true, fmt.Errorf("decrypt url_verification: %w", derr)
		}
		payload = plain
	}
	var req struct {
		Challenge string `json:"challenge"`
		Token     string `json:"token"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", true, fmt.Errorf("parse url_verification: %w", err)
	}
	if req.Type != "url_verification" || req.Challenge == "" {
		if isVerification {
			// 声明为验证请求但缺少 challenge：显式报错而非静默放行，
			// 避免飞书控制台拿到 200 空响应后无法定位配置失败原因
			return "", true, fmt.Errorf("url_verification missing challenge")
		}
		return "", false, nil
	}
	id, perr := strconv.ParseUint(accountID, 10, 64)
	if perr != nil || id == 0 || s.feishuRepo == nil {
		return "", true, errors.New("invalid feishu account_id")
	}
	acc, gerr := s.feishuRepo.GetByID(ctx, uint(id))
	if gerr != nil || acc == nil {
		return "", true, errors.New("feishu account not found")
	}
	// 空 token 一律拒绝（与 GET FeishuVerify 的 v3 审计口径一致）
	if acc.VerificationToken == "" || subtle.ConstantTimeCompare([]byte(req.Token), []byte(acc.VerificationToken)) != 1 {
		return "", true, errors.New("feishu verification token mismatch")
	}
	return req.Challenge, true, nil
}

func (s *WebhookService) dispatchFeishu(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB(ctx)

	// 2026-08-25 修复（交付阻断）：租户开启 Encrypt Key 时事件体为 {"encrypt":"..."} 信封，
	// 原实现直接按明文 JSON 解析 → Header/Event 为 nil 被静默丢弃（消息收不到且无任何日志）。
	// 现检测到信封先用账号 EncryptKey 解密再继续。
	var envProbe struct {
		Encrypt string `json:"encrypt"`
	}
	if err := json.Unmarshal(raw, &envProbe); err == nil && envProbe.Encrypt != "" {
		key := s.getFeishuEncryptKey(ctx, accountID)
		if key == "" {
			return nil, fmt.Errorf("feishu encrypted event but encrypt_key not configured (account=%s)", accountID)
		}
		plain, derr := DecryptFeishuEvent(key, envProbe.Encrypt)
		if derr != nil {
			return nil, fmt.Errorf("feishu decrypt event: %w", derr)
		}
		raw = plain
	}

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
				Content     string `json:"content"` 
				CreateTime  int64  `json:"create_time"`
			} `json:"message"`
		} `json:"event,omitempty"`
	}
	if err := json.Unmarshal(raw, &fsPayload); err != nil {
		return nil, fmt.Errorf("feishu parse: %w", err)
	}

	if fsPayload.Challenge != "" && (fsPayload.Type == "url_verification" || fsPayload.Header == nil) {

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

	// 2026-08-19：接入通用线索发现（所有渠道复用）
	MineUnifiedLead(ctx, s, hub, FeishuLeadAdapter{}, accountID, m.ChatID, "", senderID, "", "", content)

	p.Content = content
	p.Sender = senderID
	p.ChatID = m.ChatID
	return hub, nil
}

