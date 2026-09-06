package app

import (
	"context"
	"fmt"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/bridge"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"gorm.io/gorm"
)

type pipelineReachSender struct {
	inner  *IntegrationReachAdapter
	bridge *bridge.BridgeReachAdapter
}

var _ service.ReachSender = (*pipelineReachSender)(nil)

// newPipelineReachSender 构造调度器真实发送器；构造失败返回 nil（由调度器降级为占位发送）。
func NewPipelineReachSender(db *gorm.DB) *pipelineReachSender {
	inner := NewIntegrationReachAdapterFromDB(db)
	if inner == nil {
		logger.Warnf("[reach_pipeline] 构造触达发送器失败，触达调度将降级为占位发送")
		return nil
	}
	return &pipelineReachSender{
		inner:  inner,
		bridge: bridge.NewBridgeReachAdapter(inner, GetBridgeIngressSvc()),
	}
}

func (p *pipelineReachSender) SendReach(ctx context.Context, channel, accountID, to, content string) (string, error) {
	switch channel {
	case "telegram":
		return p.inner.SendTelegram(ctx, accountID, to, content)
	case "whatsapp":
		return p.inner.SendWhatsApp(ctx, accountID, to, content)
	case "feishu":
		return p.inner.SendFeishu(ctx, accountID, to, content)
	case "web":
		return p.inner.SendWeb(ctx, accountID, content)
	case "wecom":
		return p.inner.SendWeCom(ctx, accountID, to, "text", content)
	case "dingtalk":
		return p.inner.SendDingTalk(ctx, accountID, "text", content)
	case "sms":
		return p.inner.SendSMS(ctx, to, content, "", nil)
	case "email":
		return p.inner.SendEmail(ctx, to, "触达消息", content, nil)
	case "wechat":
		return p.inner.SendWeixin(ctx, to, "text", content)
	case "douyin", "kuaishou", "xiaohongshu", "tiktok", "xianyu":
		if p.bridge == nil {
			return "", fmt.Errorf("bridge 适配器未接线，无法触达渠道 %s", channel)
		}
		switch channel {
		case "douyin":
			return p.bridge.SendDouyin(ctx, accountID, to, "text", content)
		case "kuaishou":
			return p.bridge.SendKuaishou(ctx, accountID, to, "text", content)
		case "xiaohongshu":
			return p.bridge.SendXHS(ctx, accountID, to, "text", content)
		case "tiktok":
			return p.bridge.SendTikTok(ctx, accountID, to, "text", content)
		case "xianyu":
			return p.bridge.SendXianyu(ctx, accountID, to, "text", content)
		}
	}
	return "", fmt.Errorf("unsupported channel: %s", channel)
}

// RegisterAllReachServices 把全渠道 service 注册到 tooluse 全局注册中心
//
//	2026-08-16：之前 NewReachToolDeps 默认 NoOpReachAdapter，
//	AI Agent 触发 reach.*.send 永远失败。现在改为启动时一次性注册所有真实 service。
func RegisterAllReachServices(db *gorm.DB) {
	registry := tooluse.GlobalServiceRegistry()

	registry.RegisterSMS(&smsLikeAdapter{svc: service.NewSmsService(repository.NewSmsRepository())})

	registry.RegisterEmail(&emailLikeAdapter{svc: service.NewEmailService(db)})

	wecomSvc := service.NewWeComIntegrationService(db)
	registry.RegisterWeCom(&weComLikeAdapter{svc: wecomSvc})

	feishuSvc := service.NewFeishuIntegrationService(db)
	registry.RegisterFeishu(&feishuLikeAdapter{svc: feishuSvc})

	tgSvc := service.NewTelegramIntegrationService(db)
	registry.RegisterTelegram(&telegramLikeAdapter{svc: tgSvc})

	waSvc := service.NewWhatsAppCloudIntegrationService(db)
	registry.RegisterWhatsApp(&whatsAppLikeAdapter{svc: waSvc})

	dtSvc := service.NewDingTalkService()
	registry.RegisterDingTalk(&dingTalkLikeAdapter{svc: dtSvc})

	wechatSvc := service.NewWechatService(db)
	registry.RegisterWechat(&wechatLikeAdapter{svc: wechatSvc})

	tooluse.RegisterBridgeOutboundDeliver(func(ctx context.Context, channel, accountID, conversationID, msgType, content, mediaURL string) error {
		return service.DeliverBridgeOutbound(ctx, channel, accountID, conversationID, msgType, content, mediaURL)
	})

	logger.Infof("[ReachAdapter] 全渠道 service 已注册到 tooluse.GlobalServiceRegistry")
}

type smsLikeAdapter struct {
	svc service.SmsService
}

func (a *smsLikeAdapter) Send(ctx context.Context, phone, content, templateID string, params map[string]string) (string, error) {
	if err := a.svc.SendSms(ctx, &dto.SmsSendRequest{
		Phone: phone, Content: content,
	}); err != nil {
		return "", err
	}
	return "sms_out", nil
}

type emailLikeAdapter struct {
	svc *service.EmailService
}

func (a *emailLikeAdapter) Send(ctx context.Context, accountID uint, to, subject, content string, attachments []string) (string, error) {
	return a.svc.Send(ctx, accountID, to, subject, content, attachments)
}

type weComLikeAdapter struct {
	svc *service.WeComIntegrationService
}

func (a *weComLikeAdapter) SendMessage(ctx context.Context, accountID uint, externalUserID, msgType, content string, isAIReply bool, agent string) (string, error) {
	_, err := a.svc.SendMessage(ctx, &service.WeComSendRequest{
		AccountID:      accountID,
		ExternalUserID: externalUserID,
		MsgType:        msgType,
		Content:        content,
		IsAIReply:      isAIReply,
		AIAgent:        agent,
	})
	if err != nil {
		return "", err
	}
	return "wecom_out", nil
}

type feishuLikeAdapter struct {
	svc *service.FeishuIntegrationService
}

func (a *feishuLikeAdapter) SendMessage(ctx context.Context, accountID uint, openID, content, receiveIDType string) error {
	return a.svc.SendMessage(ctx, accountID, openID, content, receiveIDType, "")
}

type telegramLikeAdapter struct {
	svc *service.TelegramIntegrationService
}

func (a *telegramLikeAdapter) SendMessage(ctx context.Context, accountID uint, chatID int64, content string) error {
	return a.svc.SendMessage(ctx, accountID, chatID, content)
}

type whatsAppLikeAdapter struct {
	svc *service.WhatsAppCloudIntegrationService
}

func (a *whatsAppLikeAdapter) SendMessage(ctx context.Context, accountID uint, toPhone, content string) error {
	return a.svc.SendMessage(ctx, accountID, toPhone, content)
}

type dingTalkLikeAdapter struct {
	svc *service.DingTalkService
}

func (a *dingTalkLikeAdapter) SendRobot(ctx context.Context, webhookOrToken, secret, msgType, content string) (string, error) {
	return a.svc.SendRobot(ctx, webhookOrToken, secret, msgType, content)
}

type wechatLikeAdapter struct {
	svc *service.WechatService
}

func (a *wechatLikeAdapter) SendCustomMessage(ctx context.Context, accountID uint, openID, msgType, content string) (string, error) {
	return a.svc.SendCustomMessage(ctx, accountID, openID, msgType, content)
}
