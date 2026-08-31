package service

import (
	"context"

	"crypto/aes"

	"crypto/cipher"

	"crypto/subtle"

	"encoding/base64"

	"encoding/binary"

	"encoding/json"

	"errors"

	"fmt"

	"strconv"

	"strings"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"
)

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
	// 官方四元组口径：
	//   POST 加密事件: msg_signature = sha1(sort(token, timestamp, nonce, encrypt))
	//   GET  URL验证 : msg_signature = sha1(sort(token, timestamp, nonce, echostr))
	// v3 审计 P1-6 修复：原三元组不绑定报文内容，捕获一次合法签名即可永久重放任意伪造 payload。
	fourth := p.Encrypt
	if fourth == "" {
		fourth = query["echostr"]
	}
	parts := []string{token, p.Timestamp, p.Nonce, fourth}
	sortStrings(parts)
	h := sha1Hex([]byte(strings.Join(parts, "")))
	if subtle.ConstantTimeCompare([]byte(h), []byte(p.MsgSignature)) != 1 {
		return false, errors.New("signature mismatch")
	}
	return true, nil
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

	parts := []string{token, ts, nonce}
	sortStrings(parts)
	h := sha1Hex([]byte(strings.Join(parts, "")))
	return subtle.ConstantTimeCompare([]byte(h), []byte(sig)) == 1
}

func DecryptWeComMessage(aesKey, encrypted string) ([]byte, error) {
	if len(aesKey) != 43 {
		return nil, fmt.Errorf("invalid EncodingAESKey length: %d", len(aesKey))
	}

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

	plen := int(plain[len(plain)-1])
	if plen < 1 || plen > 32 {
		return nil, fmt.Errorf("invalid padding: %d", plen)
	}
	plain = plain[:len(plain)-plen]

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

	plain, err := DecryptWeComMessage(aesKey, echostr)
	if err != nil {
		return "", fmt.Errorf("decrypt echostr: %w", err)
	}

	plainStr := strings.TrimRight(string(plain), "\x00")
	if len(plainStr) > 20 {
		msgLen := int(binary.BigEndian.Uint32([]byte(plainStr)[16:20]))
		if 20+msgLen <= len(plainStr) {
			plainStr = plainStr[20 : 20+msgLen]
		}
	}

	parts := []string{token, timestamp, nonce, plainStr}
	sortStrings(parts)
	h := sha1Hex([]byte(strings.Join(parts, "")))
	if subtle.ConstantTimeCompare([]byte(h), []byte(msgSignature)) != 1 {
		return "", errors.New("signature mismatch")
	}

	return plainStr, nil
}

// dispatchWeCom 企业微信业务分发
func (s *WebhookService) dispatchWeCom(ctx context.Context, accountID string, p *ParsedPayload, raw []byte, headers map[string]string) (*model.MessageHub, error) {
	if s.integration == nil {
		return nil, nil
	}

	plain := s.parseWeComPlain(ctx, accountID, raw)
	if plain == nil {

		plain = p.Extra
	}
	if plain == nil {
		return nil, fmt.Errorf("wecom plain nil")
	}

	fromUser := getString(plain, "FromUserName", "from")
	fromName := getString(plain, "FromUserName")
	msgType := strings.ToLower(getString(plain, "MsgType", "msg_type"))
	content := getString(plain, "Content", "content", "Text", "text")
	if content == "" {

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
	chatType := getString(plain, "ChatType", "chat_type")
	event := getString(plain, "Event", "event")
	msgID := getString(plain, "MsgId", "msg_id")

	if msgType == "event" {
		logger.Infof("[Webhook] wecom event=%s from=%s", event, fromUser)
		return nil, nil
	}

	// accountID 转 uint
	var accID uint64
	if v, err := strconv.ParseUint(accountID, 10, 64); err == nil && v > 0 {
		accID = v
	} else {

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

	if err == nil {
		p.Content = content
		p.Sender = fromUser
		p.ChatID = chatID

		// 2026-08-19：接入通用线索发现（所有渠道复用）
		if hubMsg != nil && content != "" && fromUser != "" && msgType != "event" {
			MineUnifiedLead(ctx, s, hubMsg, WeComLeadAdapter{}, accountID, chatID, "", fromUser, fromName, "", content)
		}
	}
	return hubMsg, err
}

// parseWeComPlain 如果 body 包含 encrypt 字段，尝试解密（需要从 wecomRepo 拉 EncodingAESKey）
//
// W-2 解密精确路由：
//  1. 优先按 account_id（路径参数）精确定位账号，用该账号 EncodingAESKey 解密；
//     解密成功后校验 payload.AgentID 是否匹配账号配置的 AgentID（防 key 误配错解）。
//  2. account_id 无法定位 / 解密失败 / AgentID 不匹配 → 回退全量遍历，
//     遍历中同样校验 AgentID，首个匹配成功即返回；全量失败打 WARN。
//  3. body 无 encrypt 字段（明文）直接返回 p。
func (s *WebhookService) parseWeComPlain(ctx context.Context, accountID string, raw []byte) map[string]any {
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	enc, _ := p["encrypt"].(string)
	if enc == "" {
		return p
	}

	if s.wecomRepo == nil {
		return nil
	}

	// 1) 精确路由：按 account_id 取唯一账号的 EncodingAESKey + AgentID
	if id, err := strconv.ParseUint(accountID, 10, 64); err == nil && id > 0 {
		if acc, gerr := s.wecomRepo.GetByID(ctx, uint(id)); gerr == nil && acc != nil && acc.EncodingAESKey != "" {
			if out := decryptWeComPayload(acc.EncodingAESKey, enc); out != nil {
				if validateWeComAgentID(out, acc.AgentID) {
					return out
				}
				logger.Warnf("[Webhook] wecom AgentID 校验失败(精确路由) account=%d payload_agent=%v expected=%d，回退全量遍历",
					id, out["AgentID"], acc.AgentID)
			} else {
				logger.Warnf("[Webhook] wecom 解密失败(精确路由) account=%d，回退全量遍历", id)
			}
		}
	} else {
		logger.Warnf("[Webhook] wecom 解密缺少有效 account 标识 account=%q，回退全量遍历", accountID)
	}

	// 2) 回退：遍历所有账号逐个试 key，用 AgentID 精确匹配
	accs, err := s.wecomRepo.GetByMerchant(ctx)
	if err != nil || len(accs) == 0 {
		return nil
	}
	for _, a := range accs {
		if a.EncodingAESKey == "" {
			continue
		}
		if out := decryptWeComPayload(a.EncodingAESKey, enc); out != nil {
			if validateWeComAgentID(out, a.AgentID) {
				return out
			}
		}
	}
	logger.Warnf("[Webhook] wecom 全量遍历未能匹配到有效账号 account_id=%s", accountID)
	return nil
}

// decryptWeComPayload 用指定 aesKey 解密并反序列化；任一步失败返回 nil
func decryptWeComPayload(aesKey, enc string) map[string]any {
	if aesKey == "" {
		return nil
	}
	plain, err := DecryptWeComMessage(aesKey, enc)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil
	}
	return out
}

// validateWeComAgentID 从解密后的 payload 提取 AgentID 与账号配置比对。
// payload 中 AgentID 缺失或类型不明时报 true（兼容 XML 明文消息无 AgentID 的场景）。
func validateWeComAgentID(payload map[string]any, expected int) bool {
	if payload == nil || expected == 0 {
		return true
	}
	raw, ok := payload["AgentID"]
	if !ok {
		return true
	}
	var got int
	switch v := raw.(type) {
	case float64:
		got = int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			got = n
		} else {
			return true
		}
	default:
		return true
	}
	return got == expected
}

// getWechatSecrets 读取 wechat 渠道 webhook 验签 token。
//
// W-6 修复：原实现恒返回空串，导致 verifyWechat 对任何请求都验签失败。
// 现从公众号账号配置（wechat_accounts.token，即「服务器配置」里的验证 Token）读取：
//   - accountID 可解析时优先精确匹配该账号；
//   - 否则回退到第一个 active 账号；
//   - 均未配置时返回空串，由调用方(webhook.go::Verify)记 WARN 并跳过该渠道验签。
func (s *WebhookService) getWechatSecrets(ctx context.Context, accountID string) (string, string) {
	if s.wechatIntegration == nil {
		if s.db == nil {
			return "", ""
		}
		s.wechatIntegration = NewWechatService(s.db)
	}
	var acc *model.WechatAccount
	if id, err := strconv.ParseUint(accountID, 10, 64); err == nil && id > 0 {
		if a, gerr := s.wechatIntegration.GetAccount(ctx, uint(id)); gerr == nil {
			acc = a
		}
	}
	if acc == nil || acc.Token == "" {
		if a, gerr := s.wechatIntegration.GetFirstActiveAccount(ctx); gerr == nil {
			acc = a
		}
	}
	if acc == nil {
		return "", ""
	}
	return acc.Token, acc.EncodingAESKey
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

	if id, err := strconv.ParseUint(accountID, 10, 64); err == nil && id > 0 {
		acc, err := s.wecomRepo.GetByID(ctx, uint(id))
		if err == nil && acc != nil {
			return acc.CallbackToken, acc.EncodingAESKey, nil
		}
	}

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
