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
	parts := []string{token, p.Timestamp, p.Nonce}
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

	plain := s.parseWeComPlain(ctx, raw)
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
func (s *WebhookService) parseWeComPlain(ctx context.Context, raw []byte) map[string]any {
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

