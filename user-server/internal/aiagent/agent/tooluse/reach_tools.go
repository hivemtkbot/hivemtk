package tooluse

import (
	"context"

	"errors"

	"fmt"

	"time"

	"gorm.io/gorm"
)

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
	// SendXianyu 闲鱼私信（仅网页桥接支持，无官方 API）
	SendXianyu(ctx context.Context, accountID, openID, msgType, content string) (msgID string, err error)
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

type AccountInfo struct {
	AccountID string `json:"account_id"`
	Channel   string `json:"channel"`
	Nickname  string `json:"nickname"`
	Status    string `json:"status"` // online / offline / banned
	IsHealthy bool   `json:"is_healthy"`
}

var ErrAdapterNotConfigured = errors.New("reach adapter not configured")

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

func (NoOpReachAdapter) SendXianyu(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
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

type ReachToolDeps struct {
	Adapter      ReachAdapter
	Pipeline     ReachBatchPipelinePort // 用于 batch / schedule / history
	DB           *gorm.DB               // 用于 history 查询（如未提供 Pipeline）
	SendPipeline ReachSendPipelinePort  // G4：9 步消息发送 Pipeline
}

func NewReachToolDeps() ReachToolDeps {
	return ReachToolDeps{
		Adapter: NoOpReachAdapter{},
	}
}

func dispatchToAdapter(ctx context.Context, adapter ReachAdapter, req *ReachSendRequest) (string, error) {
	if adapter == nil {
		return "", ErrAdapterNotConfigured
	}
	switch req.Channel {
	case "sms":
		return adapter.SendSMS(ctx, req.RecipientID, req.Content, req.TemplateID, req.Params)
	case "email":
		return adapter.SendEmail(ctx, req.RecipientID, req.Subject, req.Content, req.Attachments)
	case "wecom":
		return adapter.SendWeCom(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "weixin":
		return adapter.SendWeixin(ctx, req.RecipientID, req.MsgType, req.Content)
	case "douyin", "douyin_web":
		return adapter.SendDouyin(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "kuaishou", "kuaishou_web":
		return adapter.SendKuaishou(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "xiaohongshu", "xhs", "xhs_web":
		return adapter.SendXHS(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "tiktok", "tiktok_web":
		return adapter.SendTikTok(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "xianyu", "xianyu_web":
		return adapter.SendXianyu(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "dingtalk":
		return adapter.SendDingTalk(ctx, req.RecipientID, req.MsgType, req.Content)
	case "telegram":
		return adapter.SendTelegram(ctx, req.AccountID, req.RecipientID, req.Content)
	case "whatsapp":
		return adapter.SendWhatsApp(ctx, req.AccountID, req.RecipientID, req.Content)
	case "feishu":
		return adapter.SendFeishu(ctx, req.AccountID, req.RecipientID, req.Content)
	case "web":
		return adapter.SendWeb(ctx, req.RecipientID, req.Content)
	case "card":
		// 卡片渠道：实际子渠道通过 Metadata["subchannel"] 传递（douyin/kuaishou/wecom/weixin）
		subchannel := "douyin"
		if req.Metadata != nil {
			if sc, ok := req.Metadata["subchannel"]; ok && sc != "" {
				subchannel = sc
			}
		}
		return adapter.SendCard(ctx, subchannel, req.AccountID, req.RecipientID, req.CardID)
	default:
		return "", fmt.Errorf("unknown channel: %s", req.Channel)
	}
}

func sendViaPipeline(ctx context.Context, deps ReachToolDeps, req *ReachSendRequest) (string, *ReachSendResponse, error) {
	if deps.SendPipeline != nil {
		resp := deps.SendPipeline.Send(ctx, req)
		if !resp.Success {
			return "", resp, errors.New(resp.Error)
		}
		return resp.MessageID, resp, nil
	}
	// 回退：直接调用 Adapter（未配置 9 步 SendPipeline 时）
	// 核心敏感接口：此处不经过 defaultSendPipeline.Send，仍需强制输出合规提示
	complianceReminderHook(req.Channel, req.RecipientID)
	msgID, err := dispatchToAdapter(ctx, deps.Adapter, req)
	return msgID, &ReachSendResponse{
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

func RegisterReachTools(registry *ToolRegistry, deps ReachToolDeps) error {
	tools := BuildReachTools(deps)
	for _, t := range tools {
		if err := registry.Register(t); err != nil {
			return fmt.Errorf("注册触达工具 %s 失败：%w", t.Name(), err)
		}
	}
	return nil
}

func MustRegisterReachTools(registry *ToolRegistry, deps ReachToolDeps) {
	if err := RegisterReachTools(registry, deps); err != nil {
		panic(err)
	}
}

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

	// G4：通过 9 步 SendPipeline 发送
	req := &ReachSendRequest{
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

	req := &ReachSendRequest{
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

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &ReachSendRequest{
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

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &ReachSendRequest{
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

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &ReachSendRequest{
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

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &ReachSendRequest{
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

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &ReachSendRequest{
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

type ReachDingTalkSendTool struct {
	BaseTool
	deps ReachToolDeps
}
