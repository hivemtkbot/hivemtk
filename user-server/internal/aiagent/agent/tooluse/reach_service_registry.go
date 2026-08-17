package tooluse

import (
	"context"
	"errors"
	"sync"
)

// ReachChannelService 渠道 service 抽象接口
//
//	2026-08-16：tooluse 包不能直接 import service（循环依赖），
//	改为通过适配器把 service 包装成这套接口，由 service 包在启动时注册。
type ReachChannelService interface {
	ChannelName() string
}

// SMSServiceLike SMS 发送
type SMSServiceLike interface {
	Send(ctx context.Context, phone, content, templateID string, params map[string]string) (string, error)
}

// EmailServiceLike 邮件发送
type EmailServiceLike interface {
	Send(ctx context.Context, accountID uint, to, subject, content string, attachments []string) (string, error)
}

// WeComServiceLike 企微发送
type WeComServiceLike interface {
	SendMessage(ctx context.Context, accountID uint, externalUserID, msgType, content string, isAIReply bool, agent string) (string, error)
}

// TelegramServiceLike Telegram 发送
type TelegramServiceLike interface {
	SendMessage(ctx context.Context, accountID uint, chatID int64, content string) error
}

// WhatsAppServiceLike WhatsApp 发送
type WhatsAppServiceLike interface {
	SendMessage(ctx context.Context, accountID uint, toPhone, content string) error
}

// FeishuServiceLike 飞书发送
type FeishuServiceLike interface {
	SendMessage(ctx context.Context, accountID uint, openID, content, receiveIDType string) error
}

// DingTalkServiceLike 钉钉发送
type DingTalkServiceLike interface {
	SendRobot(ctx context.Context, webhookOrToken, secret, msgType, content string) (string, error)
}

// WechatServiceLike 微信公众号发送
type WechatServiceLike interface {
	SendCustomMessage(ctx context.Context, accountID uint, openID, msgType, content string) (string, error)
}

// ServiceRegistry 全局渠道 service 注册中心
type ServiceRegistry struct {
	mu sync.RWMutex

	sms      SMSServiceLike
	email    EmailServiceLike
	wecom    WeComServiceLike
	telegram TelegramServiceLike
	whatsapp WhatsAppServiceLike
	feishu   FeishuServiceLike
	dingtalk DingTalkServiceLike
	wechat   WechatServiceLike
}

var globalReachServiceRegistry = &ServiceRegistry{}

// GlobalServiceRegistry 获取全局注册中心
func GlobalServiceRegistry() *ServiceRegistry { return globalReachServiceRegistry }

// RegisterSMS 注册 SMS service
func (r *ServiceRegistry) RegisterSMS(s SMSServiceLike) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sms = s
}

// RegisterEmail 注册 Email service
func (r *ServiceRegistry) RegisterEmail(s EmailServiceLike) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.email = s
}

// RegisterWeCom 注册企微 service
func (r *ServiceRegistry) RegisterWeCom(s WeComServiceLike) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wecom = s
}

// RegisterTelegram 注册 Telegram service
func (r *ServiceRegistry) RegisterTelegram(s TelegramServiceLike) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.telegram = s
}

// RegisterWhatsApp 注册 WhatsApp service
func (r *ServiceRegistry) RegisterWhatsApp(s WhatsAppServiceLike) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.whatsapp = s
}

// RegisterFeishu 注册飞书 service
func (r *ServiceRegistry) RegisterFeishu(s FeishuServiceLike) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feishu = s
}

// RegisterDingTalk 注册钉钉 service
func (r *ServiceRegistry) RegisterDingTalk(s DingTalkServiceLike) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dingtalk = s
}

// RegisterWechat 注册微信公众号 service
func (r *ServiceRegistry) RegisterWechat(s WechatServiceLike) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wechat = s
}

// SMS 取 SMS
func (r *ServiceRegistry) SMS() (SMSServiceLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.sms == nil {
		return nil, errors.New("sms service not registered")
	}
	return r.sms, nil
}

// Email 取 Email
func (r *ServiceRegistry) Email() (EmailServiceLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.email == nil {
		return nil, errors.New("email service not registered")
	}
	return r.email, nil
}

// WeCom 取企微
func (r *ServiceRegistry) WeCom() (WeComServiceLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.wecom == nil {
		return nil, errors.New("wecom service not registered")
	}
	return r.wecom, nil
}

// Telegram 取 Telegram
func (r *ServiceRegistry) Telegram() (TelegramServiceLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.telegram == nil {
		return nil, errors.New("telegram service not registered")
	}
	return r.telegram, nil
}

// WhatsApp 取 WhatsApp
func (r *ServiceRegistry) WhatsApp() (WhatsAppServiceLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.whatsapp == nil {
		return nil, errors.New("whatsapp service not registered")
	}
	return r.whatsapp, nil
}

// Feishu 取飞书
func (r *ServiceRegistry) Feishu() (FeishuServiceLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.feishu == nil {
		return nil, errors.New("feishu service not registered")
	}
	return r.feishu, nil
}

// DingTalk 取钉钉
func (r *ServiceRegistry) DingTalk() (DingTalkServiceLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.dingtalk == nil {
		return nil, errors.New("dingtalk service not registered")
	}
	return r.dingtalk, nil
}

// Wechat 取微信公众号
func (r *ServiceRegistry) Wechat() (WechatServiceLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.wechat == nil {
		return nil, errors.New("wechat service not registered")
	}
	return r.wechat, nil
}

// deliverBridgeOutbound 是 Bridge 下发的占位接口，由 service 包在 Init 时提供真实实现。
// 如果未注册，则回退到 NoOpBridgeDeliver（返回 nil 不报错，方便本地 mock）。
var bridgeOutboundDeliver func(ctx context.Context, channel, accountID, conversationID, msgType, content, mediaURL string) error

// RegisterBridgeOutboundDeliver 注册 Bridge 下发实现（service 包在 Init 时调用）
func RegisterBridgeOutboundDeliver(fn func(ctx context.Context, channel, accountID, conversationID, msgType, content, mediaURL string) error) {
	bridgeOutboundDeliver = fn
}

// deliverBridgeOutbound 内部调用（找不到时降级为 noop）
func deliverBridgeOutbound(ctx context.Context, channel, accountID, conversationID, msgType, content, mediaURL string) error {
	if bridgeOutboundDeliver == nil {
		return errors.New("bridge outbound not registered (need to call RegisterBridgeOutboundDeliver at startup)")
	}
	return bridgeOutboundDeliver(ctx, channel, accountID, conversationID, msgType, content, mediaURL)
}
