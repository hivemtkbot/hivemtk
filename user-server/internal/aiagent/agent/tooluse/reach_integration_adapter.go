package tooluse

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"marketing/internal/email/service"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"marketing/internal/service"
)

// IntegrationReachAdapter 集成服务适配器
//
// 作用：把 reach_tools.go 中 ReachAdapter 接口的 SendTelegram/SendWhatsApp/SendFeishu
// 三个新方法路由到 service 包中已有的 IntegrationService（TelegramIntegrationService /
// WhatsAppCloudIntegrationService / FeishuIntegrationService）。
//
// 设计原则：
//   - 仅转发参数，不做业务逻辑（业务逻辑在 IntegrationService 中）
//   - 渠道特定错误透传（限流、token 过期等）
//   - AccountHealth/ListAccounts 走模型层直接查询（参考已有 account controller 模式）
//
// 其他方法（SMS/Email/Weixin/Douyin/Kuaishou/XHS/Card/Recall/AccountHealth/ListAccounts）保持 NoOp，
// 后续按需逐渠道补齐；DingTalk 已通过 DingTalkService 实现真实群机器人出站（补 #2）；
// WeCom 已委托 WeComIntegrationService 统一出站（收敛）。
type IntegrationReachAdapter struct {
	tg       *service.TelegramIntegrationService
	wa       *service.WhatsAppCloudIntegrationService
	feishu   *service.FeishuIntegrationService
	web      *service.CustomerSessionService  // 网页客服渠道（WebSocket 推送访客）
	wecom    *service.WeComIntegrationService // 企微渠道（收敛：统一企微出站入口，底层仍由 WeComIntegrationService 承载）
	dingtalk *service.DingTalkService         // 钉钉群机器人出站（补 todo.md #2 唯一未实现渠道）
	sms      service.SmsService               // 短信渠道（补 reach.sms.send 真实出站）
	email    *email.EmailSendService          // 邮件渠道（补 reach.email.send 真实出站）
}

// Sentinel errors
//
// 使用 sentinel error 替代字符串比较（MASTER_RULES 5.2）：
//   - 渠道未实现：ErrChannelNotImplemented
//   - 渠道 IntegrationService 未注入：ErrIntegrationServiceNotConfigured
//   - 参数解析失败：ErrInvalidAccountID / ErrInvalidInt64
//   - 工具调用参数错误：复用 tooluse 包内已有 sentinel
var (
	ErrChannelNotImplemented           = errors.New("channel not implemented in IntegrationReachAdapter")
	ErrIntegrationServiceNotConfigured = errors.New("integration service not configured for channel")
	ErrInvalidAccountID                = errors.New("invalid account_id")
	ErrInvalidInt64                    = errors.New("invalid int64 value")
)

// NewIntegrationReachAdapter 创建集成服务适配器
//
// 三个参数均可为 nil（nil 时对应渠道发送返回 ErrIntegrationServiceNotConfigured）
func NewIntegrationReachAdapter(tg *service.TelegramIntegrationService, wa *service.WhatsAppCloudIntegrationService, feishu *service.FeishuIntegrationService) *IntegrationReachAdapter {
	return &IntegrationReachAdapter{tg: tg, wa: wa, feishu: feishu}
}

// NewIntegrationReachAdapterFromDB 通过 db 一站式创建（推荐用法）
//
// 真正实例化 3 个 IntegrationService，让 智能体的 reach.telegram.send / reach.whatsapp.send /
// reach.feishu.send 工具在生产中可以真正发送消息。
func NewIntegrationReachAdapterFromDB(db *gorm.DB) *IntegrationReachAdapter {
	if db == nil {
		return &IntegrationReachAdapter{}
	}
	return &IntegrationReachAdapter{
		tg:       service.NewTelegramIntegrationService(db),
		wa:       service.NewWhatsAppCloudIntegrationService(db),
		feishu:   service.NewFeishuIntegrationService(db),
		web:      service.NewCustomerSessionServiceWithDB(db),
		wecom:    service.NewWeComIntegrationService(db),
		dingtalk: service.NewDingTalkService(),
		sms:      service.NewSmsService(repository.NewSmsRepository()),
		email:    email.NewEmailSendService(),
	}
}

// ===== Telegram =====

// SendTelegram 通过 TelegramIntegrationService 发送消息
//
// 参数：accountID 数字字符串，chatID 数字字符串（私聊为正、群组为负），content 消息文本
// 返回：msgID 为 "tg-{accountID}-{nanos}" 占位（真实 message_id 已写入 MessageHub）
// 错误：IntegrationService 透传（网络错误、Bot Token 无效、chat 限流等）
func (a *IntegrationReachAdapter) SendTelegram(ctx context.Context, accountID, chatID, content string) (string, error) {
	ctx = logger.WithModule(ctx, "reach")
	logger.Ctx(ctx).Debug().Str("channel", "telegram").Str("account_id", accountID).Str("chat_id", chatID).Int("content_len", len(content)).Msg("reach send start")
	if a.tg == nil {
		return "", fmt.Errorf("telegram: %w", ErrIntegrationServiceNotConfigured)
	}
	accID, err := parseAccountID(accountID)
	if err != nil {
		return "", fmt.Errorf("telegram: %w", err)
	}
	cid, err := parseInt64(chatID)
	if err != nil {
		return "", fmt.Errorf("telegram: %w", err)
	}
	if err := a.tg.SendMessage(ctx, accID, cid, content); err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("channel", "telegram").Str("account_id", accountID).Msg("reach send failed")
		return "", fmt.Errorf("telegram send: %w", err)
	}
	return fmt.Sprintf("tg-%d-%d", accID, time.Now().UnixNano()), nil
}

// ===== WhatsApp Cloud =====

// SendWhatsApp 通过 WhatsAppCloudIntegrationService 发送消息
//
// 参数：accountID 数字字符串，toPhone E.164 格式，content 消息文本
// 返回：msgID 为 "wa-{accountID}-{nanos}" 占位
// 错误：透传 IntegrationService（401 token 失效、403 模板未审批、429 限流等）
func (a *IntegrationReachAdapter) SendWhatsApp(ctx context.Context, accountID, toPhone, content string) (string, error) {
	ctx = logger.WithModule(ctx, "reach")
	logger.Ctx(ctx).Debug().Str("channel", "whatsapp").Str("account_id", accountID).Int("content_len", len(content)).Msg("reach send start")
	if a.wa == nil {
		return "", fmt.Errorf("whatsapp: %w", ErrIntegrationServiceNotConfigured)
	}
	accID, err := parseAccountID(accountID)
	if err != nil {
		return "", fmt.Errorf("whatsapp: %w", err)
	}
	if err := a.wa.SendMessage(ctx, accID, toPhone, content); err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("channel", "whatsapp").Str("account_id", accountID).Msg("reach send failed")
		return "", fmt.Errorf("whatsapp send: %w", err)
	}
	return fmt.Sprintf("wa-%d-%d", accID, time.Now().UnixNano()), nil
}

// ===== Feishu =====

// SendFeishu 通过 FeishuIntegrationService 发送消息
//
// 参数：accountID 数字字符串，openID 飞书 open_id，content 消息文本
// 返回：msgID 为 "feishu-{accountID}-{nanos}" 占位
// 错误：透传 IntegrationService（token 过期、app_id 无效、用户不在可见范围等）
func (a *IntegrationReachAdapter) SendFeishu(ctx context.Context, accountID, openID, content string) (string, error) {
	ctx = logger.WithModule(ctx, "reach")
	logger.Ctx(ctx).Debug().Str("channel", "feishu").Str("account_id", accountID).Int("content_len", len(content)).Msg("reach send start")
	if a.feishu == nil {
		return "", fmt.Errorf("feishu: %w", ErrIntegrationServiceNotConfigured)
	}
	accID, err := parseAccountID(accountID)
	if err != nil {
		return "", fmt.Errorf("feishu: %w", err)
	}
	if err := a.feishu.SendMessage(ctx, accID, openID, content); err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("channel", "feishu").Str("account_id", accountID).Msg("reach send failed")
		return "", fmt.Errorf("feishu send: %w", err)
	}
	return fmt.Sprintf("feishu-%d-%d", accID, time.Now().UnixNano()), nil
}

// ===== Web（网页客服）=====

// SendWeb 通过网页客服渠道（WebSocket）向访客会话推送消息。
//
// 完整业务闭环（区别于其他渠道的纯 API 转发）：
//  1. 校验会话存在（CustomerSessionService.SendMessage 内部校验，不存在返回错误）
//  2. 落库 SessionMessage（sender_type=agent, sender_name=客服）并更新会话最后消息与回复计数
//  3. 实时经 WebSocket 推送给在线访客（SendMessage 内部 pushToVisitor 已实现）
//  4. 访客在线则标记 delivered_at，避免 WebSocket 重连后离线补发重复展示
//
// 为遵守「客服页面不对用户显示 AI」，网页客服主动触达统一以「客服」身份下发，不暴露 AI 标识。
// 错误：服务未注入 / session_id 为空 / content 为空 / 会话不存在。
func (a *IntegrationReachAdapter) SendWeb(ctx context.Context, sessionID, content string) (string, error) {
	ctx = logger.WithModule(ctx, "reach")
	logger.Ctx(ctx).Debug().Str("channel", "web").Str("session_id", sessionID).Int("content_len", len(content)).Msg("reach send start")
	if a.web == nil {
		return "", fmt.Errorf("web: %w", ErrIntegrationServiceNotConfigured)
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", errors.New("web: session_id required")
	}
	if strings.TrimSpace(content) == "" {
		return "", errors.New("web: content required")
	}
	msg, err := a.web.SendMessage(ctx, &service.SendMessageRequest{
		SessionID:   sessionID,
		Content:     content,
		ContentType: model.MessageTypeText,
		SenderType:  "agent",
		SenderName:  "客服",
		SenderID:    "reach_web",
	})
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("channel", "web").Str("session_id", sessionID).Msg("reach send failed")
		return "", fmt.Errorf("web send: %w", err)
	}
	return fmt.Sprintf("web-%s-%d", sessionID, msg.ID), nil
}

// WithWeb 注入网页客服会话服务（实现 reach.web.send 完整业务）
func (a *IntegrationReachAdapter) WithWeb(svc *service.CustomerSessionService) *IntegrationReachAdapter {
	a.web = svc
	return a
}

// WithWeCom 注入企微集成服务（实现 reach.wecom.send 完整业务， 收敛统一企微出站入口）
func (a *IntegrationReachAdapter) WithWeCom(svc *service.WeComIntegrationService) *IntegrationReachAdapter {
	a.wecom = svc
	return a
}

// WebReachAdapter 网页客服触达适配器类型别名。
// 通过注入 CustomerSessionService 实现 reach.web.send 的完整业务（落库 + 实时推访客），
// 可直接作为 ReachToolDeps.Adapter 注入，独立承载网页客服渠道。
type WebReachAdapter = IntegrationReachAdapter

// NewWebReachAdapter 创建仅聚焦网页客服渠道的触达适配器
func NewWebReachAdapter(svc *service.CustomerSessionService) *WebReachAdapter {
	return &WebReachAdapter{web: svc}
}

// ===== 其他渠道：暂未实现（保持 NoOp 等价行为）=====
// 注：WeCom 已委托 WeComIntegrationService 实现（见上方 SendWeCom），不在此 NoOp 区块。

// SendSMS 未实现（沿用 NoOp 语义）
func (a *IntegrationReachAdapter) SendSMS(ctx context.Context, phone, content, templateID string, params map[string]string) (string, error) {
	return "", fmt.Errorf("sms: %w", ErrChannelNotImplemented)
}

// SendEmail 未实现
func (a *IntegrationReachAdapter) SendEmail(ctx context.Context, to, subject, content string, attachments []string) (string, error) {
	return "", fmt.Errorf("email: %w", ErrChannelNotImplemented)
}

// SendWeCom 通过 WeComIntegrationService 发送企微消息（收敛：统一企微出站入口）
//
// 此前为 NoOp（ErrChannelNotImplemented），企微出站独立在 WeComIntegrationService，
// 与 ReachAdapter 接口重叠（缺陷）。现委托 WeComIntegrationService.SendMessage，
// 使 IntegrationReachAdapter 成为覆盖 TG/WA/Feishu/Web/WeCom 的单一出站入口。
//
// 底层语义与 WeComIntegrationService.SendMessage 既有行为一致：
//   - 账号健康度/配额检查后推消息中台 + 收件箱
// 配置了真实企微凭证（CorpID/CorpSecret）时真实调企微 API；无凭证安全跳过
//   - SelectHealthyAccount 自动选健康账号（忽略传入 AccountID，与既有出站一致）
func (a *IntegrationReachAdapter) SendWeCom(ctx context.Context, accountID, externalUserID, msgType, content string) (string, error) {
	ctx = logger.WithModule(ctx, "reach")
	logger.Ctx(ctx).Debug().Str("channel", "wecom").Str("account_id", accountID).Str("external_user_id", externalUserID).Int("content_len", len(content)).Msg("reach send start")
	if a.wecom == nil {
		return "", fmt.Errorf("wecom: %w", ErrIntegrationServiceNotConfigured)
	}
	accID, err := parseAccountID(accountID)
	if err != nil {
		return "", fmt.Errorf("wecom: %w", err)
	}
	if externalUserID == "" {
		return "", errors.New("wecom: external_user_id required")
	}
	mt := msgType
	if mt == "" {
		mt = "text"
	}
	hubMsg, err := a.wecom.SendMessage(ctx, &service.WeComSendRequest{
		AccountID:      accID,
		ExternalUserID: externalUserID,
		MsgType:        mt,
		Content:        content,
	})
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("channel", "wecom").Str("account_id", accountID).Msg("reach send failed")
		return "", fmt.Errorf("wecom send: %w", err)
	}
	if hubMsg == nil {
		return "", nil
	}
	return hubMsg.MsgID, nil
}

// SendWeixin 未实现
func (a *IntegrationReachAdapter) SendWeixin(ctx context.Context, openID, msgType, content string) (string, error) {
	return "", fmt.Errorf("weixin: %w", ErrChannelNotImplemented)
}

// SendDouyin 未实现
func (a *IntegrationReachAdapter) SendDouyin(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return "", fmt.Errorf("douyin: %w", ErrChannelNotImplemented)
}

// SendKuaishou 未实现
func (a *IntegrationReachAdapter) SendKuaishou(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return "", fmt.Errorf("kuaishou: %w", ErrChannelNotImplemented)
}

// SendXHS 未实现
func (a *IntegrationReachAdapter) SendXHS(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return "", fmt.Errorf("xhs: %w", ErrChannelNotImplemented)
}

// SendTikTok 未实现（TikTok 无官方私信 API，仅通过 bridge 网页桥接出站）
func (a *IntegrationReachAdapter) SendTikTok(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return "", fmt.Errorf("tiktok: %w", ErrChannelNotImplemented)
}

// SendDingTalk 通过钉钉群机器人 webhook 出站（补齐 todo.md #2 唯一未实现渠道）
//
// chatID 为 `reach.dingtalk.send` 工具传入的 recipient_id，语义：
//   - 完整 webhook URL（含 access_token），或仅 access_token（自动拼接 base）
//   - 若机器人开启「加签」，用 `webhook|secret` 形式携带签名密钥
//
// 支持 text / markdown / link / action_card 消息类型（msgType 默认 text）。
func (a *IntegrationReachAdapter) SendDingTalk(ctx context.Context, chatID, msgType, content string) (string, error) {
	ctx = logger.WithModule(ctx, "reach")
	logger.Ctx(ctx).Debug().Str("channel", "dingtalk").Str("chat_id", chatID).Int("content_len", len(content)).Msg("reach send start")
	if a.dingtalk == nil {
		return "", fmt.Errorf("dingtalk: %w", ErrIntegrationServiceNotConfigured)
	}
	if chatID == "" {
		return "", errors.New("dingtalk: chat_id (webhook or access_token) required")
	}
	mt := msgType
	if mt == "" {
		mt = "text"
	}
	msgID, err := a.dingtalk.SendRobot(ctx, chatID, "", mt, content)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("channel", "dingtalk").Str("chat_id", chatID).Msg("reach send failed")
		return "", fmt.Errorf("dingtalk send: %w", err)
	}
	return msgID, nil
}

// SendCard 未实现（卡片渠道需要先查询卡片元数据再分发到子渠道）
func (a *IntegrationReachAdapter) SendCard(ctx context.Context, channel, accountID, externalUserID, cardID string) (string, error) {
	return "", fmt.Errorf("card: %w", ErrChannelNotImplemented)
}

// Recall 未实现
func (a *IntegrationReachAdapter) Recall(ctx context.Context, channel, msgID string) error {
	return fmt.Errorf("recall(%s): %w", channel, ErrChannelNotImplemented)
}

// AccountHealth 未实现
func (a *IntegrationReachAdapter) AccountHealth(ctx context.Context, channel, accountID string) (*AccountHealthInfo, error) {
	return nil, fmt.Errorf("account_health(%s): %w", channel, ErrChannelNotImplemented)
}

// ListAccounts 未实现
func (a *IntegrationReachAdapter) ListAccounts(ctx context.Context, channel string) ([]AccountInfo, error) {
	return nil, fmt.Errorf("list_accounts(%s): %w", channel, ErrChannelNotImplemented)
}

// ===== 辅助函数 =====

// parseAccountID 解析账号 ID 字符串为 uint
func parseAccountID(s string) (uint, error) {
	if s == "" {
		return 0, fmt.Errorf("empty: %w", ErrInvalidAccountID)
	}
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", s, ErrInvalidAccountID)
	}
	if id == 0 {
		return 0, fmt.Errorf("zero: %w", ErrInvalidAccountID)
	}
	return uint(id), nil
}

// parseInt64 解析 int64 字符串（支持负数）
func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty: %w", ErrInvalidInt64)
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", s, ErrInvalidInt64)
	}
	if v == 0 {
		return 0, fmt.Errorf("zero: %w", ErrInvalidInt64)
	}
	return v, nil
}
