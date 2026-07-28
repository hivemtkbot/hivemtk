package tooluse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/service"
)

// reach_tools.go 触达工具实现（PRD §5.2 P0-3 G3）
//
// 20 个触达工具：
//   1.  reach.sms.send        - 短信发送
//   2.  reach.email.send      - 邮件发送
//   3.  reach.wecom.send      - 企微发送
//   4.  reach.weixin.send     - 微信公众号发送
//   5.  reach.douyin.send     - 抖音私信
//   6.  reach.kuaishou.send   - 快手私信
//   7.  reach.xhs.send        - 小红书私信
//   8.  reach.dingtalk.send   - 钉钉发送
//   9.  reach.telegram.send   - Telegram Bot API 发送（境外 IM）
//   10. reach.whatsapp.send   - WhatsApp Cloud API 发送（Meta 商业）
//   11. reach.feishu.send     - 飞书 Open API 发送（协作）
//   12. reach.card.send       - 客户卡片（多渠道）
//   13. reach.web.send        - 网页客服（WebSocket 实时推送访客会话，完整业务闭环）
//   14. reach.batch           - 批量触达
//   15. reach.schedule        - 定时触达
//   16. reach.recall          - 撤回消息
//   17. reach.health          - 账号健康度
//   18. reach.history         - 触达历史
//   19. reach.template.apply  - 模板应用
//   20. reach.account.list    - 账号列表

// ===== ReachAdapter 接口 =====

// ReachAdapter 触达适配器接口
// 由调用方注入具体实现；默认 NoOpReachAdapter 返回未配置错误
type ReachAdapter interface {
	// SendSMS 发送短信
	SendSMS(ctx context.Context, phone, content, templateID string, params map[string]string) (msgID string, err error)
	// SendEmail 发送邮件
	SendEmail(ctx context.Context, to, subject, content string, attachments []string) (msgID string, err error)
	// SendWeCom 企微发送
	SendWeCom(ctx context.Context, accountID, externalUserID, msgType, content string) (msgID string, err error)
	// SendWeixin 微信公众号发送
	SendWeixin(ctx context.Context, openID, msgType, content string) (msgID string, err error)
	// SendDouyin 抖音私信
	SendDouyin(ctx context.Context, accountID, openID, msgType, content string) (msgID string, err error)
	// SendKuaishou 快手私信
	SendKuaishou(ctx context.Context, accountID, openID, msgType, content string) (msgID string, err error)
	// SendXHS 小红书私信
	SendXHS(ctx context.Context, accountID, openID, msgType, content string) (msgID string, err error)
	// SendTikTok TikTok 私信（仅网页桥接支持，无官方 API）
	SendTikTok(ctx context.Context, accountID, openID, msgType, content string) (msgID string, err error)
	// SendDingTalk 钉钉发送
	SendDingTalk(ctx context.Context, chatID, msgType, content string) (msgID string, err error)
	// SendTelegram 通过 Telegram Bot API 发送消息
	// chatID 私聊为正、群组为负（如 -1001234567890）
	SendTelegram(ctx context.Context, accountID, chatID, content string) (msgID string, err error)
	// SendWhatsApp 通过 WhatsApp Cloud API 发送消息
	// toPhone E.164 国际格式（如 +8613800138000）
	SendWhatsApp(ctx context.Context, accountID, toPhone, content string) (msgID string, err error)
	// SendFeishu 通过飞书 Open API 发送消息
	// openID 飞书用户 open_id（ou_xxx）或群 chat_id（oc_xxx）
	SendFeishu(ctx context.Context, accountID, openID, content string) (msgID string, err error)
	// SendWeb 通过网页客服渠道（WebSocket）向访客会话推送消息
	// sessionID 访客会话 ID（对应 customer_sessions.session_id）
	// 实时推送给在线访客；访客离线时返回离线提示由调用方决定补发策略
	SendWeb(ctx context.Context, sessionID, content string) (msgID string, err error)
	// SendCard 客户卡片
	SendCard(ctx context.Context, channel, accountID, externalUserID, cardID string) (msgID string, err error)
	// Recall 撤回
	Recall(ctx context.Context, channel, msgID string) error
	// AccountHealth 账号健康度
	AccountHealth(ctx context.Context, channel, accountID string) (*AccountHealthInfo, error)
	// ListAccounts 账号列表
	ListAccounts(ctx context.Context, channel string) ([]AccountInfo, error)
}

// AccountHealthInfo 账号健康度信息
type AccountHealthInfo struct {
	AccountID   string `json:"account_id"`
	Channel     string `json:"channel"`
	Status      string `json:"status"` // healthy / warning / risk / banned
	DailyQuota  int    `json:"daily_quota"`
	DailyUsed   int    `json:"daily_used"`
	DailyRemain int    `json:"daily_remain"`
	RiskLevel   string `json:"risk_level"` // low / medium / high
	LastCheckAt string `json:"last_check_at"`
}

// AccountInfo 账号信息
type AccountInfo struct {
	AccountID string `json:"account_id"`
	Channel   string `json:"channel"`
	Nickname  string `json:"nickname"`
	Status    string `json:"status"` // online / offline / banned
	IsHealthy bool   `json:"is_healthy"`
}

// ErrAdapterNotConfigured 适配器未配置
var ErrAdapterNotConfigured = errors.New("reach adapter not configured")

// NoOpReachAdapter 默认空实现（所有方法返回未配置错误）
type NoOpReachAdapter struct{}

func (NoOpReachAdapter) SendSMS(ctx context.Context, phone, content, templateID string, params map[string]string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendEmail(ctx context.Context, to, subject, content string, attachments []string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendWeCom(ctx context.Context, accountID, externalUserID, msgType, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendWeixin(ctx context.Context, openID, msgType, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendDouyin(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendKuaishou(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendXHS(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendTikTok(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendDingTalk(ctx context.Context, chatID, msgType, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendTelegram(ctx context.Context, accountID, chatID, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendWhatsApp(ctx context.Context, accountID, toPhone, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendFeishu(ctx context.Context, accountID, openID, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendWeb(ctx context.Context, sessionID, content string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) SendCard(ctx context.Context, channel, accountID, externalUserID, cardID string) (string, error) {
	return "", ErrAdapterNotConfigured
}
func (NoOpReachAdapter) Recall(ctx context.Context, channel, msgID string) error {
	return ErrAdapterNotConfigured
}
func (NoOpReachAdapter) AccountHealth(ctx context.Context, channel, accountID string) (*AccountHealthInfo, error) {
	return nil, ErrAdapterNotConfigured
}
func (NoOpReachAdapter) ListAccounts(ctx context.Context, channel string) ([]AccountInfo, error) {
	return nil, ErrAdapterNotConfigured
}

// ===== 触达工具依赖 =====

// ReachToolDeps 触达工具依赖
type ReachToolDeps struct {
	Adapter      ReachAdapter
	Pipeline     *service.ReachPipelineService // 用于 batch / schedule / history
	DB           *gorm.DB                      // 用于 history 查询（如未提供 Pipeline）
	SendPipeline service.SendPipeline          // P0-4 G4：9 步消息发送 Pipeline
}

// NewReachToolDeps 创建触达工具依赖（默认 NoOp Adapter）
func NewReachToolDeps() ReachToolDeps {
	return ReachToolDeps{
		Adapter: NoOpReachAdapter{},
	}
}

// NewReachToolDepsWithDB 创建触达工具依赖（带 DB，自动初始化 Pipeline + SendPipeline）
func NewReachToolDepsWithDB(db *gorm.DB) ReachToolDeps {
	deps := ReachToolDeps{
		Adapter:  NoOpReachAdapter{},
		Pipeline: service.NewReachPipelineService(db),
		DB:       db,
	}
	// P0-4 G4：自动初始化 9 步 SendPipeline（包装 Adapter）
	deps.SendPipeline = service.NewSendPipeline(service.DefaultSendPipelineConfig(&reachChannelAdapterBridge{adapter: NoOpReachAdapter{}}))
	return deps
}

// WithSendPipeline 注入 SendPipeline（用于自定义配置）
func (d ReachToolDeps) WithSendPipeline(sp service.SendPipeline) ReachToolDeps {
	d.SendPipeline = sp
	return d
}

// NewReachToolDepsWithAdapter 创建触达工具依赖（带真实 Adapter，打通全部业务渠道）
//
// 与 NewReachToolDepsWithDB 的区别：SendPipeline 的桥接器使用【真实 adapter】
// 而非 NoOpReachAdapter，确保 reach.web.send 等工具在生产/集成测试中真正调用
// IntegrationReachAdapter.SendWeb（落库 + 实时推访客），而非空壳 NoOp。
//
// 调用方：router.Setup 生产接线、真实端到端集成测试。
func NewReachToolDepsWithAdapter(db *gorm.DB, adapter ReachAdapter) ReachToolDeps {
	deps := ReachToolDeps{
		Adapter:  adapter,
		Pipeline: service.NewReachPipelineService(db),
		DB:       db,
	}
	deps.SendPipeline = service.NewSendPipeline(
		service.DefaultSendPipelineConfig(&reachChannelAdapterBridge{adapter: adapter}),
	)
	return deps
}

// reachChannelAdapterBridge 把 ReachAdapter 桥接为 service.ChannelAdapter
// 根据 req.Channel 分发到对应的 ReachAdapter 方法
type reachChannelAdapterBridge struct {
	adapter ReachAdapter
}

// Send 实现 service.ChannelAdapter
func (b *reachChannelAdapterBridge) Send(ctx context.Context, req *service.ReachSendRequest) (string, error) {
	if b.adapter == nil {
		return "", service.ErrSendChannelNotConfig
	}
	switch req.Channel {
	case "sms":
		return b.adapter.SendSMS(ctx, req.RecipientID, req.Content, req.TemplateID, req.Params)
	case "email":
		return b.adapter.SendEmail(ctx, req.RecipientID, req.Subject, req.Content, req.Attachments)
	case "wecom":
		return b.adapter.SendWeCom(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "weixin":
		return b.adapter.SendWeixin(ctx, req.RecipientID, req.MsgType, req.Content)
	case "douyin", "douyin_web":
		return b.adapter.SendDouyin(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "kuaishou":
		return b.adapter.SendKuaishou(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "xhs", "xhs_web":
		return b.adapter.SendXHS(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "tiktok", "tiktok_web":
		return b.adapter.SendTikTok(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "dingtalk":
		return b.adapter.SendDingTalk(ctx, req.RecipientID, req.MsgType, req.Content)
	case "telegram":
		return b.adapter.SendTelegram(ctx, req.AccountID, req.RecipientID, req.Content)
	case "whatsapp":
		return b.adapter.SendWhatsApp(ctx, req.AccountID, req.RecipientID, req.Content)
	case "feishu":
		return b.adapter.SendFeishu(ctx, req.AccountID, req.RecipientID, req.Content)
	case "web":
		return b.adapter.SendWeb(ctx, req.RecipientID, req.Content)
	case "card":
		// 卡片渠道：实际子渠道通过 Metadata["subchannel"] 传递（douyin/kuaishou/wecom/weixin）
		subchannel := "douyin"
		if req.Metadata != nil {
			if sc, ok := req.Metadata["subchannel"]; ok && sc != "" {
				subchannel = sc
			}
		}
		return b.adapter.SendCard(ctx, subchannel, req.AccountID, req.RecipientID, req.CardID)
	default:
		return "", fmt.Errorf("unknown channel: %s", req.Channel)
	}
}

// sendViaPipeline 通过 SendPipeline 发送消息（如果未配置则回退到直接调用 Adapter）
// 返回：messageID, error
func sendViaPipeline(ctx context.Context, deps ReachToolDeps, req *service.ReachSendRequest) (string, *service.SendResponse, error) {
	if deps.SendPipeline != nil {
		resp := deps.SendPipeline.Send(ctx, req)
		if !resp.Success {
			return "", resp, errors.New(resp.Error)
		}
		return resp.MessageID, resp, nil
	}
	// 回退：直接调用 Adapter（未配置 9 步 SendPipeline 时）
	// 核心敏感接口：此处不经过 defaultSendPipeline.Send，仍需强制输出合规提示
	service.LogComplianceReminder(req.Channel, req.RecipientID)
	bridge := &reachChannelAdapterBridge{adapter: deps.Adapter}
	msgID, err := bridge.Send(ctx, req)
	return msgID, &service.SendResponse{
		Success:   err == nil,
		MessageID: msgID,
		Channel:   req.Channel,
		Error: func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}(),
		SentAt: time.Now(),
	}, err
}

// BuildReachTools 构造全部 20 个触达工具（不注册到 Registry）
//
// 用于 ToolProvider.Provide() 返回工具列表，由 ProviderRegistry 统一注册。
// 调用方：ReachToolProvider.Provide()
func BuildReachTools(deps ReachToolDeps) []Tool {
	return []Tool{
		NewReachSMSSendTool(deps),
		NewReachEmailSendTool(deps),
		NewReachWeComSendTool(deps),
		NewReachWeixinSendTool(deps),
		NewReachDouyinSendTool(deps),
		NewReachKuaishouSendTool(deps),
		NewReachXHSSendTool(deps),
		NewReachDingTalkSendTool(deps),
		NewReachTelegramSendTool(deps),
		NewReachWhatsAppSendTool(deps),
		NewReachFeishuSendTool(deps),
		NewReachWebSendTool(deps),
		NewReachCardSendTool(deps),
		NewReachBatchTool(deps),
		NewReachScheduleTool(deps),
		NewReachRecallTool(deps),
		NewReachHealthTool(deps),
		NewReachHistoryTool(deps),
		NewReachTemplateApplyTool(deps),
		NewReachAccountListTool(deps),
	}
}

// RegisterReachTools 注册所有 20 个触达工具
func RegisterReachTools(registry *ToolRegistry, deps ReachToolDeps) error {
	tools := BuildReachTools(deps)
	for _, t := range tools {
		if err := registry.Register(t); err != nil {
			return fmt.Errorf("注册触达工具 %s 失败：%w", t.Name(), err)
		}
	}
	return nil
}

// MustRegisterReachTools 注册所有触达工具，出错 panic
func MustRegisterReachTools(registry *ToolRegistry, deps ReachToolDeps) {
	if err := RegisterReachTools(registry, deps); err != nil {
		panic(err)
	}
}

// ===== 工具 1：reach.sms.send =====

// ReachSMSSendTool 短信发送
type ReachSMSSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachSMSSendTool(deps ReachToolDeps) *ReachSMSSendTool {
	return &ReachSMSSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.sms.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "发送短信。支持模板短信（传 template_id + params）和直发（传 content）。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"phone":       {Type: "string", Description: "手机号"},
					"content":     {Type: "string", Description: "短信内容（直发模式）"},
					"template_id": {Type: "string", Description: "短信模板 ID（模板模式）"},
					"params": {
						Type:        "object",
						Description: "模板参数（key-value）",
						Properties:  map[string]ToolParam{},
					},
				},
				Required: []string{"phone"},
			},
		},
		deps: deps,
	}
}

func (t *ReachSMSSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"phone"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	phone, _ := GetStringArg(args, "phone")
	content := getArgString(args, "content")
	templateID := getArgString(args, "template_id")
	params := getArgStringMap(args, "params")

	if content == "" && templateID == "" {
		return ErrorResult(t.Name(), errors.New("content 和 template_id 至少需要一个")), errors.New("content 和 template_id 至少需要一个")
	}

	// P0-4 G4：通过 9 步 SendPipeline 发送
	req := &service.ReachSendRequest{
		Channel:     "sms",
		RecipientID: phone,
		Content:     content,
		TemplateID:  templateID,
		Params:      params,
	}
	if custID, ok := args["customer_id"]; ok {
		req.CustomerID, _ = custID.(string)
	}
	if opID, ok := args["operator_id"]; ok {
		req.OperatorID, _ = opID.(string)
	}

	msgID, resp, err := sendViaPipeline(ctx, t.deps, req)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"phone":         phone,
		"message_id":    msgID,
		"channel":       "sms",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 2：reach.email.send =====

// ReachEmailSendTool 邮件发送
type ReachEmailSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachEmailSendTool(deps ReachToolDeps) *ReachEmailSendTool {
	return &ReachEmailSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.email.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "发送邮件。支持附件。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"to":      {Type: "string", Description: "收件人邮箱（多个用逗号分隔）"},
					"subject": {Type: "string", Description: "邮件主题"},
					"content": {Type: "string", Description: "邮件内容（HTML 或纯文本）"},
					"attachments": {
						Type:        "array",
						Description: "附件 URL 列表",
						Items:       &ToolParam{Type: "string"},
					},
				},
				Required: []string{"to", "subject", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachEmailSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"to", "subject", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	to, _ := GetStringArg(args, "to")
	subject, _ := GetStringArg(args, "subject")
	content, _ := GetStringArg(args, "content")
	attachments := getArgStringSlice(args, "attachments")

	req := &service.ReachSendRequest{
		Channel:     "email",
		RecipientID: to,
		Subject:     subject,
		Content:     content,
		Attachments: attachments,
	}
	if custID, ok := args["customer_id"]; ok {
		req.CustomerID, _ = custID.(string)
	}

	msgID, resp, err := sendViaPipeline(ctx, t.deps, req)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"to":            to,
		"subject":       subject,
		"message_id":    msgID,
		"channel":       "email",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 3：reach.wecom.send =====

// ReachWeComSendTool 企微发送
type ReachWeComSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachWeComSendTool(deps ReachToolDeps) *ReachWeComSendTool {
	return &ReachWeComSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.wecom.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过企业微信向客户发送消息。支持 text/image/link/textcard 等消息类型。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id":       {Type: "string", Description: "企微账号 ID"},
					"external_user_id": {Type: "string", Description: "外部联系人 ID"},
					"msg_type": {
						Type:        "string",
						Description: "消息类型",
						Enum:        []string{"text", "image", "link", "textcard", "miniprogram", "file"},
						Default:     "text",
					},
					"content": {Type: "string", Description: "消息内容（text 类型为文本，其他类型为 JSON 配置）"},
				},
				Required: []string{"account_id", "external_user_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachWeComSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "external_user_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	accountID, _ := GetStringArg(args, "account_id")
	externalUserID, _ := GetStringArg(args, "external_user_id")
	msgType := getArgString(args, "msg_type")
	if msgType == "" {
		msgType = "text"
	}
	content, _ := GetStringArg(args, "content")

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &service.ReachSendRequest{
		Channel:     "wecom",
		AccountID:   accountID,
		RecipientID: externalUserID,
		MsgType:     msgType,
		Content:     content,
	})
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"account_id":       accountID,
		"external_user_id": externalUserID,
		"msg_type":         msgType,
		"message_id":       msgID,
		"channel":          "wecom",
		"sent_at":          time.Now().Format(time.RFC3339),
		"step_results":     resp.StepResults,
		"retry_count":      resp.RetryCount,
		"fallback_used":    resp.FallbackUsed,
	}), nil
}

// ===== 工具 4：reach.weixin.send =====

// ReachWeixinSendTool 微信公众号发送
type ReachWeixinSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachWeixinSendTool(deps ReachToolDeps) *ReachWeixinSendTool {
	return &ReachWeixinSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.weixin.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过微信公众号向用户发送消息（客服消息）。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"open_id": {Type: "string", Description: "用户 OpenID"},
					"msg_type": {
						Type:        "string",
						Description: "消息类型",
						Enum:        []string{"text", "image", "news", "template"},
						Default:     "text",
					},
					"content": {Type: "string", Description: "消息内容"},
				},
				Required: []string{"open_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachWeixinSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"open_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	openID, _ := GetStringArg(args, "open_id")
	msgType := getArgString(args, "msg_type")
	if msgType == "" {
		msgType = "text"
	}
	content, _ := GetStringArg(args, "content")

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &service.ReachSendRequest{
		Channel:     "weixin",
		RecipientID: openID,
		MsgType:     msgType,
		Content:     content,
	})
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"open_id":       openID,
		"msg_type":      msgType,
		"message_id":    msgID,
		"channel":       "weixin",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 5：reach.douyin.send =====

// ReachDouyinSendTool 抖音私信
type ReachDouyinSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachDouyinSendTool(deps ReachToolDeps) *ReachDouyinSendTool {
	return &ReachDouyinSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.douyin.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过抖音私信向用户发送消息。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id": {Type: "string", Description: "抖音账号 ID"},
					"open_id":    {Type: "string", Description: "用户 OpenID"},
					"msg_type": {
						Type:        "string",
						Description: "消息类型",
						Enum:        []string{"text", "image", "card", "video"},
						Default:     "text",
					},
					"content": {Type: "string", Description: "消息内容"},
				},
				Required: []string{"account_id", "open_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachDouyinSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "open_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	accountID, _ := GetStringArg(args, "account_id")
	openID, _ := GetStringArg(args, "open_id")
	msgType := getArgString(args, "msg_type")
	if msgType == "" {
		msgType = "text"
	}
	content, _ := GetStringArg(args, "content")

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &service.ReachSendRequest{
		Channel:     "douyin",
		AccountID:   accountID,
		RecipientID: openID,
		MsgType:     msgType,
		Content:     content,
	})
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"account_id":    accountID,
		"open_id":       openID,
		"msg_type":      msgType,
		"message_id":    msgID,
		"channel":       "douyin",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 6：reach.kuaishou.send =====

// ReachKuaishouSendTool 快手私信
type ReachKuaishouSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachKuaishouSendTool(deps ReachToolDeps) *ReachKuaishouSendTool {
	return &ReachKuaishouSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.kuaishou.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过快手私信向用户发送消息。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id": {Type: "string", Description: "快手账号 ID"},
					"open_id":    {Type: "string", Description: "用户 OpenID"},
					"msg_type": {
						Type:        "string",
						Description: "消息类型",
						Enum:        []string{"text", "image", "card", "video"},
						Default:     "text",
					},
					"content": {Type: "string", Description: "消息内容"},
				},
				Required: []string{"account_id", "open_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachKuaishouSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "open_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	accountID, _ := GetStringArg(args, "account_id")
	openID, _ := GetStringArg(args, "open_id")
	msgType := getArgString(args, "msg_type")
	if msgType == "" {
		msgType = "text"
	}
	content, _ := GetStringArg(args, "content")

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &service.ReachSendRequest{
		Channel:     "kuaishou",
		AccountID:   accountID,
		RecipientID: openID,
		MsgType:     msgType,
		Content:     content,
	})
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"account_id":    accountID,
		"open_id":       openID,
		"msg_type":      msgType,
		"message_id":    msgID,
		"channel":       "kuaishou",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 7：reach.xhs.send =====

// ReachXHSSendTool 小红书私信
type ReachXHSSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachXHSSendTool(deps ReachToolDeps) *ReachXHSSendTool {
	return &ReachXHSSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.xhs.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过小红书私信向用户发送消息。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id": {Type: "string", Description: "小红书账号 ID"},
					"open_id":    {Type: "string", Description: "用户 OpenID"},
					"msg_type": {
						Type:        "string",
						Description: "消息类型",
						Enum:        []string{"text", "image", "card"},
						Default:     "text",
					},
					"content": {Type: "string", Description: "消息内容"},
				},
				Required: []string{"account_id", "open_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachXHSSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "open_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	accountID, _ := GetStringArg(args, "account_id")
	openID, _ := GetStringArg(args, "open_id")
	msgType := getArgString(args, "msg_type")
	if msgType == "" {
		msgType = "text"
	}
	content, _ := GetStringArg(args, "content")

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &service.ReachSendRequest{
		Channel:     "xhs",
		AccountID:   accountID,
		RecipientID: openID,
		MsgType:     msgType,
		Content:     content,
	})
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"account_id":    accountID,
		"open_id":       openID,
		"msg_type":      msgType,
		"message_id":    msgID,
		"channel":       "xiaohongshu",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 8：reach.dingtalk.send =====

// ReachDingTalkSendTool 钉钉发送
type ReachDingTalkSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachDingTalkSendTool(deps ReachToolDeps) *ReachDingTalkSendTool {
	return &ReachDingTalkSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.dingtalk.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过钉钉机器人或群消息发送消息。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"chat_id": {Type: "string", Description: "钉钉会话 ID（群 ID 或机器人 webhook）"},
					"msg_type": {
						Type:        "string",
						Description: "消息类型",
						Enum:        []string{"text", "markdown", "action_card", "link"},
						Default:     "text",
					},
					"content": {Type: "string", Description: "消息内容"},
				},
				Required: []string{"chat_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachDingTalkSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"chat_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	chatID, _ := GetStringArg(args, "chat_id")
	msgType := getArgString(args, "msg_type")
	if msgType == "" {
		msgType = "text"
	}
	content, _ := GetStringArg(args, "content")

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &service.ReachSendRequest{
		Channel:     "dingtalk",
		RecipientID: chatID,
		MsgType:     msgType,
		Content:     content,
	})
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"chat_id":       chatID,
		"msg_type":      msgType,
		"message_id":    msgID,
		"channel":       "dingtalk",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 9：reach.card.send =====

// ReachCardSendTool 客户卡片（多渠道）
type ReachCardSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachCardSendTool(deps ReachToolDeps) *ReachCardSendTool {
	return &ReachCardSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.card.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "发送客户卡片（如抖音卡片、快手卡片）。支持指定渠道和卡片模板。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"channel": {
						Type:        "string",
						Description: "目标渠道",
						Enum:        []string{"douyin", "kuaishou", "wecom", "weixin"},
					},
					"account_id":       {Type: "string", Description: "发送账号 ID"},
					"external_user_id": {Type: "string", Description: "接收方 ID"},
					"card_id":          {Type: "string", Description: "卡片模板 ID"},
				},
				Required: []string{"channel", "account_id", "external_user_id", "card_id"},
			},
		},
		deps: deps,
	}
}

func (t *ReachCardSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"channel", "account_id", "external_user_id", "card_id"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	channel, _ := GetStringArg(args, "channel")
	accountID, _ := GetStringArg(args, "account_id")
	externalUserID, _ := GetStringArg(args, "external_user_id")
	cardID, _ := GetStringArg(args, "card_id")

	// P0-4 G4：通过 9 步 SendPipeline 发送（Channel="card"，子渠道存 Metadata["subchannel"]）
	req := &service.ReachSendRequest{
		Channel:     "card",
		AccountID:   accountID,
		RecipientID: externalUserID,
		CardID:      cardID,
		Metadata:    map[string]string{"subchannel": channel},
	}
	if custID, ok := args["customer_id"]; ok {
		req.CustomerID, _ = custID.(string)
	}
	if opID, ok := args["operator_id"]; ok {
		req.OperatorID, _ = opID.(string)
	}

	msgID, resp, err := sendViaPipeline(ctx, t.deps, req)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"channel":          channel,
		"account_id":       accountID,
		"external_user_id": externalUserID,
		"card_id":          cardID,
		"message_id":       msgID,
		"sent_at":          time.Now().Format(time.RFC3339),
		"step_results":     resp.StepResults,
		"retry_count":      resp.RetryCount,
		"fallback_used":    resp.FallbackUsed,
	}), nil
}

// ===== 工具 10：reach.batch =====

// BatchSendItem 批量触达条目
type BatchSendItem struct {
	CustomerID string         `json:"customer_id"`
	AccountID  string         `json:"account_id,omitempty"`
	Payload    map[string]any `json:"payload"`
}

// ReachBatchTool 批量触达
type ReachBatchTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachBatchTool(deps ReachToolDeps) *ReachBatchTool {
	return &ReachBatchTool{
		BaseTool: BaseTool{
			NameVal:        "reach.batch",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过触达 Pipeline 批量发送消息。需要指定 pipeline_id，会为每个 customer 创建一个 job。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"pipeline_id": {Type: "string", Description: "触达 Pipeline ID"},
					"channel": {
						Type:        "string",
						Description: "渠道（不填则用 pipeline 配置）",
						Enum:        []string{"sms", "email", "wecom", "weixin", "douyin", "kuaishou", "xiaohongshu", "dingtalk", "telegram", "whatsapp", "feishu", "card"},
					},
					"items": {
						Type:        "array",
						Description: "批量触达条目列表",
						Items: &ToolParam{
							Type: "object",
							Properties: map[string]ToolParam{
								"customer_id": {Type: "string", Description: "客户 ID"},
								"account_id":  {Type: "string", Description: "账号 ID（可选）"},
								"payload":     {Type: "object", Description: "消息载荷"},
							},
						},
					},
				},
				Required: []string{"pipeline_id", "items"},
			},
		},
		deps: deps,
	}
}

func (t *ReachBatchTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if t.deps.Pipeline == nil {
		return ErrorResult(t.Name(), errors.New("batch 工具需要 Pipeline 依赖")), errors.New("batch 工具需要 Pipeline 依赖")
	}
	if err := ValidateRequired(args, []string{"pipeline_id", "items"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	pipelineIDStr, _ := GetStringArg(args, "pipeline_id")
	pipelineID := parseUint(pipelineIDStr)
	channel := getArgString(args, "channel")

	items := getArgInterfaceSlice(args, "items")
	if len(items) == 0 {
		return ErrorResult(t.Name(), errors.New("items 不能为空")), errors.New("items 不能为空")
	}

	jobIDs := make([]string, 0, len(items))
	failed := make([]string, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			failed = append(failed, "invalid item")
			continue
		}
		customerID := getArgString(itemMap, "customer_id")
		accountID := getArgString(itemMap, "account_id")
		// Payload 保留原始类型（数字/布尔/对象），避免被 fmt.Sprintf 强转
		payload := getArgMap(itemMap, "payload")
		if customerID == "" {
			failed = append(failed, "missing customer_id")
			continue
		}
		wg.Add(1)
		go func(cid, aid string, p map[string]any) {
			defer wg.Done()
			req := &service.EnqueueJobRequest{
				PipelineID: pipelineID,
				Channel:    channel,
				CustomerID: cid,
				AccountID:  aid,
				Payload:    p,
			}
			job, err := t.deps.Pipeline.EnqueueJob(ctx, req)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", cid, err))
				return
			}
			jobIDs = append(jobIDs, fmt.Sprintf("%d", job.ID))
		}(customerID, accountID, payload)
	}
	wg.Wait()

	success := len(jobIDs)
	return SuccessResult(t.Name(), map[string]any{
		"pipeline_id":    pipelineID,
		"total":          len(items),
		"success_count":  success,
		"failed_count":   len(failed),
		"job_ids":        jobIDs,
		"failed_details": failed,
	}), nil
}

// ===== 工具 11：reach.schedule =====

// ReachScheduleTool 定时触达
type ReachScheduleTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachScheduleTool(deps ReachToolDeps) *ReachScheduleTool {
	return &ReachScheduleTool{
		BaseTool: BaseTool{
			NameVal:        "reach.schedule",
			CategoryVal:    CategoryReach,
			DescriptionVal: "定时触发触达任务。通过 Pipeline 在指定时间执行。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"pipeline_id": {Type: "string", Description: "触达 Pipeline ID"},
					"channel":     {Type: "string", Description: "渠道"},
					"customer_id": {Type: "string", Description: "客户 ID"},
					"account_id":  {Type: "string", Description: "账号 ID"},
					"run_at":      {Type: "string", Description: "执行时间（RFC3339 格式）"},
					"payload":     {Type: "object", Description: "消息载荷"},
				},
				Required: []string{"pipeline_id", "customer_id", "run_at", "payload"},
			},
		},
		deps: deps,
	}
}

func (t *ReachScheduleTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if t.deps.Pipeline == nil {
		return ErrorResult(t.Name(), errors.New("schedule 工具需要 Pipeline 依赖")), errors.New("schedule 工具需要 Pipeline 依赖")
	}
	if err := ValidateRequired(args, []string{"pipeline_id", "customer_id", "run_at", "payload"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	pipelineIDStr, _ := GetStringArg(args, "pipeline_id")
	pipelineID := parseUint(pipelineIDStr)
	channel := getArgString(args, "channel")
	customerID, _ := GetStringArg(args, "customer_id")
	accountID := getArgString(args, "account_id")
	runAtStr, _ := GetStringArg(args, "run_at")
	payload := getArgStringMap(args, "payload")

	runAt, err := time.Parse(time.RFC3339, runAtStr)
	if err != nil {
		return ErrorResult(t.Name(), fmt.Errorf("run_at 格式错误：%w", err)), fmt.Errorf("run_at 格式错误：%w", err)
	}
	if runAt.Before(time.Now()) {
		return ErrorResult(t.Name(), errors.New("run_at 不能早于当前时间")), errors.New("run_at 不能早于当前时间")
	}

	payloadMap := map[string]any{}
	for k, v := range payload {
		payloadMap[k] = v
	}

	req := &service.EnqueueJobRequest{
		PipelineID: pipelineID,
		Channel:    channel,
		CustomerID: customerID,
		AccountID:  accountID,
		Payload:    payloadMap,
		RunAt:      &runAt,
	}
	job, err := t.deps.Pipeline.EnqueueJob(ctx, req)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"job_id":      job.ID,
		"pipeline_id": pipelineID,
		"customer_id": customerID,
		"channel":     channel,
		"run_at":      runAt.Format(time.RFC3339),
		"state":       job.State,
	}), nil
}

// ===== 工具 12：reach.recall =====

// ReachRecallTool 撤回消息
type ReachRecallTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachRecallTool(deps ReachToolDeps) *ReachRecallTool {
	return &ReachRecallTool{
		BaseTool: BaseTool{
			NameVal:        "reach.recall",
			CategoryVal:    CategoryReach,
			DescriptionVal: "撤回已发送的消息。并非所有渠道都支持撤回。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"channel": {
						Type:        "string",
						Description: "渠道",
						Enum:        []string{"sms", "email", "wecom", "weixin", "douyin", "kuaishou", "xiaohongshu", "dingtalk", "telegram", "whatsapp", "feishu", "card"},
					},
					"message_id": {Type: "string", Description: "消息 ID"},
				},
				Required: []string{"channel", "message_id"},
			},
		},
		deps: deps,
	}
}

func (t *ReachRecallTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"channel", "message_id"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	channel, _ := GetStringArg(args, "channel")
	msgID, _ := GetStringArg(args, "message_id")

	if err := t.deps.Adapter.Recall(ctx, channel, msgID); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"channel":     channel,
		"message_id":  msgID,
		"recalled":    true,
		"recalled_at": time.Now().Format(time.RFC3339),
	}), nil
}

// ===== 工具 13：reach.health =====

// ReachHealthTool 账号健康度查询
type ReachHealthTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachHealthTool(deps ReachToolDeps) *ReachHealthTool {
	return &ReachHealthTool{
		BaseTool: BaseTool{
			NameVal:        "reach.health",
			CategoryVal:    CategoryReach,
			DescriptionVal: "查询账号健康度。返回账号状态、剩余配额、风险等级等信息。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"channel": {
						Type:        "string",
						Description: "渠道",
						Enum:        []string{"sms", "email", "wecom", "weixin", "douyin", "kuaishou", "xiaohongshu", "dingtalk", "telegram", "whatsapp", "feishu", "card"},
					},
					"account_id": {Type: "string", Description: "账号 ID（不填则查询渠道下所有账号摘要）"},
				},
				Required: []string{"channel"},
			},
		},
		deps: deps,
	}
}

func (t *ReachHealthTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"channel"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	channel, _ := GetStringArg(args, "channel")
	accountID := getArgString(args, "account_id")

	info, err := t.deps.Adapter.AccountHealth(ctx, channel, accountID)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), info), nil
}

// ===== 工具 14：reach.history =====

// ReachHistoryTool 触达历史查询
type ReachHistoryTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachHistoryTool(deps ReachToolDeps) *ReachHistoryTool {
	return &ReachHistoryTool{
		BaseTool: BaseTool{
			NameVal:        "reach.history",
			CategoryVal:    CategoryReach,
			DescriptionVal: "查询触达历史。可按渠道、客户 ID、状态筛选。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"channel":     {Type: "string", Description: "渠道（可选）"},
					"customer_id": {Type: "string", Description: "客户 ID（可选）"},
					"state": {
						Type:        "string",
						Description: "任务状态",
						Enum:        []string{"pending", "running", "success", "failed", "canceled", "retrying", "rate_limited"},
					},
					"page":      {Type: "integer", Description: "页码（默认 1）", Default: 1},
					"page_size": {Type: "integer", Description: "每页数量（默认 20）", Default: 20},
				},
			},
		},
		deps: deps,
	}
}

func (t *ReachHistoryTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if t.deps.Pipeline == nil {
		return ErrorResult(t.Name(), errors.New("history 工具需要 Pipeline 依赖")), errors.New("history 工具需要 Pipeline 依赖")
	}
	channel := getArgString(args, "channel")
	state := getArgString(args, "state")
	page, _ := GetIntArg(args, "page")
	if page <= 0 {
		page = 1
	}
	pageSize, _ := GetIntArg(args, "page_size")
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	jobs, total, err := t.deps.Pipeline.ListJobs(ctx, channel, state, page, pageSize)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	totalPages := int64(0)
	if pageSize > 0 {
		totalPages = (total + int64(pageSize) - 1) / int64(pageSize)
	}
	return SuccessResult(t.Name(), map[string]any{
		"jobs":        jobs,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	}), nil
}

// ===== 工具 15：reach.template.apply =====

// ReachTemplateApplyTool 模板应用
type ReachTemplateApplyTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachTemplateApplyTool(deps ReachToolDeps) *ReachTemplateApplyTool {
	return &ReachTemplateApplyTool{
		BaseTool: BaseTool{
			NameVal:        "reach.template.apply",
			CategoryVal:    CategoryReach,
			DescriptionVal: "应用模板参数生成最终消息内容。支持 {{var}} 占位符替换。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"template": {Type: "string", Description: "模板内容（含 {{var}} 占位符）"},
					"params": {
						Type:        "object",
						Description: "替换参数（key-value）",
						Properties:  map[string]ToolParam{},
					},
				},
				Required: []string{"template", "params"},
			},
		},
		deps: deps,
	}
}

func (t *ReachTemplateApplyTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"template", "params"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	tmpl, _ := GetStringArg(args, "template")
	params := getArgStringMap(args, "params")

	// 简单 {{var}} 替换
	result := tmpl
	for k, v := range params {
		placeholder := fmt.Sprintf("{{%s}}", k)
		result = strings.ReplaceAll(result, placeholder, v)
		// 兼容 {{ var }} 带空格
		placeholder2 := fmt.Sprintf("{{ %s }}", k)
		result = strings.ReplaceAll(result, placeholder2, v)
	}

	// 检查未替换的占位符
	remaining := findUnreplacedPlaceholders(result)
	return SuccessResult(t.Name(), map[string]any{
		"template":                tmpl,
		"params":                  params,
		"content":                 result,
		"unreplaced_placeholders": remaining,
	}), nil
}

// findUnreplacedPlaceholders 查找未替换的 {{...}} 占位符
func findUnreplacedPlaceholders(s string) []string {
	var placeholders []string
	i := 0
	for i < len(s) {
		if s[i] == '{' && i+1 < len(s) && s[i+1] == '{' {
			// 找到 }}
			end := strings.Index(s[i+2:], "}}")
			if end < 0 {
				break
			}
			placeholder := strings.TrimSpace(s[i+2 : i+2+end])
			placeholders = append(placeholders, placeholder)
			i = i + 2 + end + 2
		} else {
			i++
		}
	}
	return placeholders
}

// ===== 工具 16：reach.account.list =====

// ReachAccountListTool 账号列表查询
type ReachAccountListTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachAccountListTool(deps ReachToolDeps) *ReachAccountListTool {
	return &ReachAccountListTool{
		BaseTool: BaseTool{
			NameVal:        "reach.account.list",
			CategoryVal:    CategoryReach,
			DescriptionVal: "查询指定渠道下的可用账号列表（含健康状态）。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"channel": {
						Type:        "string",
						Description: "渠道（不填则返回所有渠道）",
						Enum:        []string{"sms", "email", "wecom", "weixin", "douyin", "kuaishou", "xiaohongshu", "dingtalk", "telegram", "whatsapp", "feishu", "card"},
					},
				},
			},
		},
		deps: deps,
	}
}

func (t *ReachAccountListTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	channel := getArgString(args, "channel")

	accounts, err := t.deps.Adapter.ListAccounts(ctx, channel)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	// 统计健康/不健康数量
	healthy := 0
	for _, a := range accounts {
		if a.IsHealthy {
			healthy++
		}
	}
	return SuccessResult(t.Name(), map[string]any{
		"channel":       channel,
		"accounts":      accounts,
		"total":         len(accounts),
		"healthy_count": healthy,
	}), nil
}

// ===== 辅助函数 =====

// getArgStringMap 安全获取 map[string]string 参数
// 支持 map[string]interface{}（JSON 反序列化结果）
// getArgStringMap 从 args 取 map[string]string（向旧路径兼容：非 string 值会被 fmt.Sprintf 转字符串）
//
// 保留此函数供 SMS 等需要 map[string]string 的场景
// 新代码请优先使用 getArgMap（保留原始类型）
func getArgStringMap(args map[string]any, key string) map[string]string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		out := make(map[string]string, len(m))
		for k, val := range m {
			if s, ok := val.(string); ok {
				out[k] = s
			} else {
				out[k] = fmt.Sprintf("%v", val)
			}
		}
		return out
	}
	// 直接是 map[string]string
	if m, ok := v.(map[string]string); ok {
		return m
	}
	return nil
}

// getArgInterfaceSlice 安全获取 []interface{} 参数
func getArgInterfaceSlice(args map[string]any, key string) []any {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}

// getArgMap 安全获取 map[string]interface{} 参数（保留原始类型，不像 getArgStringMap 那样把数字/布尔强转字符串）
//
// 用于 BatchSendItem.Payload / EnqueueJobRequest.Payload 等需要保留类型的场景
func getArgMap(args map[string]any, key string) map[string]any {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// parseUint 字符串转 uint（解析失败返回 0）
func parseUint(s string) uint {
	if s == "" {
		return 0
	}
	var n uint
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0
	}
	return n
}

// ===== 工具 9：reach.telegram.send =====
// 境外 IM 渠道，智能体可通过本工具向 Telegram 用户/群组发送消息
// 入站已有 webhook 入口（webhook_service.go）→ 触发 智能体 → 本工具回包形成闭环
// 群组主动入群欢迎、bot 私聊、付费 broadcast 详见 docs/research/messaging/01-03

// ReachTelegramSendTool Telegram Bot API 发送
type ReachTelegramSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachTelegramSendTool(deps ReachToolDeps) *ReachTelegramSendTool {
	return &ReachTelegramSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.telegram.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过 Telegram Bot API 发送消息。支持私聊（chat_id 为正）和群组（chat_id 为负）。限流 1 QPS/chat + 30 msg/s 全局。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id": {Type: "string", Description: "TG 机器人账号 ID（数字字符串）"},
					"chat_id": {
						Type:        "string",
						Description: "目标 chat_id：私聊为正（如 123456789），群组为负（如 -1001234567890）",
					},
					"content": {Type: "string", Description: "消息文本（最长 4096 字符，超过会被 Telegram API 拒绝）"},
					// LLM Function Calling 参数（用于客户轨迹/限流维度/审计）
					"customer_id": {Type: "string", Description: "客户 ID（用于客户轨迹和限流维度）"},
					"operator_id": {Type: "string", Description: "操作员 ID（用于权限校验）"},
				},
				Required: []string{"account_id", "chat_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachTelegramSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "chat_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	accountID, _ := GetStringArg(args, "account_id")
	chatID, _ := GetStringArg(args, "chat_id")
	content, _ := GetStringArg(args, "content")
	if content == "" {
		return ErrorResult(t.Name(), errors.New("content 不能为空")), errors.New("content 不能为空")
	}

	req := &service.ReachSendRequest{
		Channel:     "telegram",
		AccountID:   accountID,
		RecipientID: chatID,
		Content:     content,
	}
	if custID, ok := args["customer_id"]; ok {
		req.CustomerID, _ = custID.(string)
	}
	if opID, ok := args["operator_id"]; ok {
		req.OperatorID, _ = opID.(string)
	}

	msgID, resp, err := sendViaPipeline(ctx, t.deps, req)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"account_id":    accountID,
		"chat_id":       chatID,
		"message_id":    msgID,
		"channel":       "telegram",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 10：reach.whatsapp.send =====
// Meta 商业渠道，智能体可通过本工具发送 WhatsApp 消息
// 必须使用 Meta 审批通过的 marketing/utility 模板（首次主动触达）
// 24h 客服窗口内可发送自由文本，详见 docs/research/messaging/04

// ReachWhatsAppSendTool WhatsApp Cloud API 发送
type ReachWhatsAppSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachWhatsAppSendTool(deps ReachToolDeps) *ReachWhatsAppSendTool {
	return &ReachWhatsAppSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.whatsapp.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过 WhatsApp Cloud API 发送消息。主动触达需使用 Meta 审批模板；24h 客服窗口内可发自由文本。限流 5 msg/s/号。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id": {Type: "string", Description: "WhatsApp Cloud 账号 ID（数字字符串）"},
					"to_phone":   {Type: "string", Description: "目标手机号（E.164 国际格式，如 +8613800138000）"},
					"content":    {Type: "string", Description: "消息文本"},
					"template_id": {
						Type:        "string",
						Description: "模板消息名（marketing/utility 模板需 Meta 审批；非必填，24h 窗口内可发自由文本）",
					},
					"params": {
						Type:        "object",
						Description: "模板参数（key-value，对应模板 {{1}} {{2}} 等占位符）",
						Properties:  map[string]ToolParam{},
					},
					// LLM Function Calling 参数
					"customer_id": {Type: "string", Description: "客户 ID（用于客户轨迹和限流维度）"},
					"operator_id": {Type: "string", Description: "操作员 ID（用于权限校验）"},
				},
				Required: []string{"account_id", "to_phone", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachWhatsAppSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "to_phone", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	accountID, _ := GetStringArg(args, "account_id")
	toPhone, _ := GetStringArg(args, "to_phone")
	content, _ := GetStringArg(args, "content")
	templateID := getArgString(args, "template_id")
	params := getArgStringMap(args, "params")
	if content == "" && templateID == "" {
		return ErrorResult(t.Name(), errors.New("content 和 template_id 至少需要一个")), errors.New("content 和 template_id 至少需要一个")
	}

	req := &service.ReachSendRequest{
		Channel:     "whatsapp",
		AccountID:   accountID,
		RecipientID: toPhone,
		Content:     content,
		TemplateID:  templateID,
		Params:      params,
	}
	if custID, ok := args["customer_id"]; ok {
		req.CustomerID, _ = custID.(string)
	}
	if opID, ok := args["operator_id"]; ok {
		req.OperatorID, _ = opID.(string)
	}

	msgID, resp, err := sendViaPipeline(ctx, t.deps, req)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"account_id":    accountID,
		"to_phone":      toPhone,
		"template_id":   templateID,
		"message_id":    msgID,
		"channel":       "whatsapp",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 11：reach.feishu.send =====
// 协作平台，智能体可通过本工具向飞书用户/群发送消息
// 飞书不能 cold DM，需用户先发起对话；详见 docs/research/messaging/06

// ReachFeishuSendTool 飞书 Open API 发送
type ReachFeishuSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachFeishuSendTool(deps ReachToolDeps) *ReachFeishuSendTool {
	return &ReachFeishuSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.feishu.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过飞书 Open API 发送消息。receive_id 需为 open_id（ou_xxx）或 chat_id（oc_xxx）。限流 50 QPS 全局 + 5 QPS/用户。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id": {Type: "string", Description: "飞书应用账号 ID（数字字符串）"},
					"open_id": {
						Type:        "string",
						Description: "接收者 ID：用户 open_id（ou_xxx）或群 chat_id（oc_xxx）或 email",
					},
					"content": {Type: "string", Description: "消息文本"},
					"msg_type": {
						Type:        "string",
						Description: "消息类型（默认 text，可选 text/post/image/interactive）",
						Enum:        []string{"text", "post", "image", "interactive"},
						Default:     "text",
					},
					// LLM Function Calling 参数
					"customer_id": {Type: "string", Description: "客户 ID（用于客户轨迹和限流维度）"},
					"operator_id": {Type: "string", Description: "操作员 ID（用于权限校验）"},
				},
				Required: []string{"account_id", "open_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachFeishuSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "open_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	accountID, _ := GetStringArg(args, "account_id")
	openID, _ := GetStringArg(args, "open_id")
	content, _ := GetStringArg(args, "content")
	msgType := getArgString(args, "msg_type")
	if msgType == "" {
		msgType = "text"
	}
	if content == "" {
		return ErrorResult(t.Name(), errors.New("content 不能为空")), errors.New("content 不能为空")
	}

	req := &service.ReachSendRequest{
		Channel:     "feishu",
		AccountID:   accountID,
		RecipientID: openID,
		MsgType:     msgType,
		Content:     content,
	}
	if custID, ok := args["customer_id"]; ok {
		req.CustomerID, _ = custID.(string)
	}
	if opID, ok := args["operator_id"]; ok {
		req.OperatorID, _ = opID.(string)
	}

	msgID, resp, err := sendViaPipeline(ctx, t.deps, req)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"account_id":    accountID,
		"open_id":       openID,
		"msg_type":      msgType,
		"message_id":    msgID,
		"channel":       "feishu",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}

// ===== 工具 13：reach.web.send =====

// ReachWebSendTool 网页客服发送（WebSocket 实时推送访客会话）
type ReachWebSendTool struct {
	BaseTool
	deps ReachToolDeps
}

func NewReachWebSendTool(deps ReachToolDeps) *ReachWebSendTool {
	return &ReachWebSendTool{
		BaseTool: BaseTool{
			NameVal:        "reach.web.send",
			CategoryVal:    CategoryReach,
			DescriptionVal: "通过网页客服渠道（WebSocket）向指定访客会话实时推送消息。消息落库并以「客服」身份展示给访客，不暴露 AI 标识。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"session_id": {Type: "string", Description: "访客会话 ID（对应 customer_sessions.session_id）"},
					"content":    {Type: "string", Description: "推送消息内容"},
				},
				Required: []string{"session_id", "content"},
			},
		},
		deps: deps,
	}
}

func (t *ReachWebSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"session_id", "content"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	sessionID, _ := GetStringArg(args, "session_id")
	content, _ := GetStringArg(args, "content")

	req := &service.ReachSendRequest{
		Channel:     "web",
		RecipientID: sessionID,
		Content:     content,
	}
	if custID, ok := args["customer_id"]; ok {
		req.CustomerID, _ = custID.(string)
	}

	msgID, resp, err := sendViaPipeline(ctx, t.deps, req)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}
	return SuccessResult(t.Name(), map[string]any{
		"session_id":    sessionID,
		"message_id":    msgID,
		"channel":       "web",
		"sent_at":       time.Now().Format(time.RFC3339),
		"step_results":  resp.StepResults,
		"retry_count":   resp.RetryCount,
		"fallback_used": resp.FallbackUsed,
	}), nil
}
