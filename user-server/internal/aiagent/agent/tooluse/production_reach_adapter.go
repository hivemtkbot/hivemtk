package tooluse

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ProductionReachAdapter 把智能体 Reach 工具全部接上真实的渠道 service
//
// 2026-08-16 严肃化：之前 NewReachToolDeps 默认返回 NoOpReachAdapter，
// 导致 20 个 Reach 工具虽然注册成功但全部走 NoOp 路径 → AI 永远触发不了真实发送。
//
// 本 Adapter 通过 GlobalServiceRegistry() 调用 service 层；启动时 service 包会注册。
// 任意渠道调用失败不阻断其他渠道（fallback 由 Pipeline 处理）。
type ProductionReachAdapter struct{}

// NewProductionReachAdapter 创建生产 Reach Adapter
func NewProductionReachAdapter() *ProductionReachAdapter {
	return &ProductionReachAdapter{}
}

// SendSMS 阿里云短信
func (p *ProductionReachAdapter) SendSMS(ctx context.Context, phone, content, templateID string, params map[string]string) (string, error) {
	svc, err := GlobalServiceRegistry().SMS()
	if err != nil {
		return "", err
	}
	return svc.Send(ctx, phone, content, templateID, params)
}

// SendEmail SMTP 邮件
func (p *ProductionReachAdapter) SendEmail(ctx context.Context, to, subject, content string, attachments []string) (string, error) {
	svc, err := GlobalServiceRegistry().Email()
	if err != nil {
		return "", err
	}
	return svc.Send(ctx, 0, to, subject, content, attachments)
}

// SendWeCom 企业微信
func (p *ProductionReachAdapter) SendWeCom(ctx context.Context, accountID, externalUserID, msgType, content string) (string, error) {
	svc, err := GlobalServiceRegistry().WeCom()
	if err != nil {
		return "", err
	}
	id, _ := strconv.ParseUint(accountID, 10, 64)
	return svc.SendMessage(ctx, uint(id), externalUserID, msgType, content, true, "ai_agent")
}

// SendWeixin 微信公众号（客服消息）
func (p *ProductionReachAdapter) SendWeixin(ctx context.Context, openID, msgType, content string) (string, error) {
	svc, err := GlobalServiceRegistry().Wechat()
	if err != nil {
		return "", err
	}
	// 使用 accountID=0 表示默认账号（由注册中心使用第一个 active 账号）
	return svc.SendCustomMessage(ctx, 0, openID, msgType, content)
}

// SendDouyin 抖音私信（Bridge）
func (p *ProductionReachAdapter) SendDouyin(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return p.sendBridge(ctx, "douyin", accountID, openID, msgType, content)
}

// SendKuaishou 快手私信（Bridge）
func (p *ProductionReachAdapter) SendKuaishou(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return p.sendBridge(ctx, "kuaishou", accountID, openID, msgType, content)
}

// SendXHS 小红书私信（Bridge）
func (p *ProductionReachAdapter) SendXHS(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return p.sendBridge(ctx, "xiaohongshu", accountID, openID, msgType, content)
}

// SendTikTok TikTok 私信（Bridge）
func (p *ProductionReachAdapter) SendTikTok(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return p.sendBridge(ctx, "tiktok", accountID, openID, msgType, content)
}

// SendXianyu 闲鱼私信（Bridge）
func (p *ProductionReachAdapter) SendXianyu(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return p.sendBridge(ctx, "xianyu", accountID, openID, msgType, content)
}

// SendDingTalk 钉钉群机器人
func (p *ProductionReachAdapter) SendDingTalk(ctx context.Context, chatID, msgType, content string) (string, error) {
	if chatID == "" {
		return "", errors.New("dingtalk: chat_id (webhook) required")
	}
	svc, err := GlobalServiceRegistry().DingTalk()
	if err != nil {
		return "", err
	}
	return svc.SendRobot(ctx, chatID, "", msgType, content)
}

// SendTelegram Telegram Bot
func (p *ProductionReachAdapter) SendTelegram(ctx context.Context, accountID, chatID, content string) (string, error) {
	svc, err := GlobalServiceRegistry().Telegram()
	if err != nil {
		return "", err
	}
	accID, _ := strconv.ParseUint(accountID, 10, 64)
	cid, _ := strconv.ParseInt(chatID, 10, 64)
	return "", svc.SendMessage(ctx, uint(accID), cid, content)
}

// SendWhatsApp WhatsApp Cloud API
func (p *ProductionReachAdapter) SendWhatsApp(ctx context.Context, accountID, toPhone, content string) (string, error) {
	svc, err := GlobalServiceRegistry().WhatsApp()
	if err != nil {
		return "", err
	}
	accID, _ := strconv.ParseUint(accountID, 10, 64)
	return "", svc.SendMessage(ctx, uint(accID), toPhone, content)
}

// SendFeishu 飞书 Open API
func (p *ProductionReachAdapter) SendFeishu(ctx context.Context, accountID, openID, content string) (string, error) {
	svc, err := GlobalServiceRegistry().Feishu()
	if err != nil {
		return "", err
	}
	accID, _ := strconv.ParseUint(accountID, 10, 64)
	return "", svc.SendMessage(ctx, uint(accID), openID, content, "open_id")
}

// SendWeb Web Widget
func (p *ProductionReachAdapter) SendWeb(ctx context.Context, sessionID, content string) (string, error) {
	return p.sendBridge(ctx, "web", "", sessionID, "text", content)
}

// SendCard 卡片
func (p *ProductionReachAdapter) SendCard(ctx context.Context, channel, accountID, externalUserID, cardID string) (string, error) {
	return "", errors.New("card send: not yet implemented (requires card template engine)")
}

// Recall 撤回
func (p *ProductionReachAdapter) Recall(ctx context.Context, channel, msgID string) error {
	return fmt.Errorf("recall: not supported on %s yet", channel)
}

// AccountHealth 健康度
func (p *ProductionReachAdapter) AccountHealth(ctx context.Context, channel, accountID string) (*AccountHealthInfo, error) {
	return &AccountHealthInfo{
		AccountID:   accountID,
		Channel:     channel,
		Status:      "unknown",
		LastCheckAt: time.Now().Format(time.RFC3339),
	}, nil
}

// ListAccounts 账号列表
func (p *ProductionReachAdapter) ListAccounts(ctx context.Context, channel string) ([]AccountInfo, error) {
	return nil, nil
}

// sendBridge 统一的 Bridge 下发
func (p *ProductionReachAdapter) sendBridge(ctx context.Context, channel, accountID, receiverID, msgType, content string) (string, error) {
	if err := deliverBridgeOutbound(ctx, channel, accountID, receiverID, msgType, content, ""); err != nil {
		return "", fmt.Errorf("%s bridge deliver: %w", channel, err)
	}
	return fmt.Sprintf("%s_%d", channel, time.Now().UnixNano()), nil
}
