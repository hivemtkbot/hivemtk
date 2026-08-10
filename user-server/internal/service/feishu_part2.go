// 拆分自 feishu.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"hivemtk-user/internal/channelbot/core"
	"hivemtk-user/internal/channelbot/whatsapp"
	"hivemtk-user/internal/pkg/httpclient"
	"hivemtk-user/internal/pkg/utils/logger"
	"time"
)

func (s *WhatsAppCloudIntegrationService) SendMessage(ctx context.Context, accountID uint, toPhone, content string) error {
	if s.wa == nil {
		return errors.New("db nil")
	}
	acc, err := s.wa.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get wa account: %w", err)
	}
	// 主动发消息委托独立包 channelbot/whatsapp（官方 Cloud API，合规）
	// 注入统一出站 client（httpclient.Client，带超时+连接池），与 出站 client 收敛一致，并使测试可拦截
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
		if _, err := s.inbox.UpsertFromHubMessage(ctx, hubMsg); err != nil {
			logger.Warnf("[feishu] upsert outbound to inbox failed: %v", err)
		}
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
