// 拆分自 reach_tools.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package tooluse

import (
	"context"
	"errors"
	"time"
)

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

	req := &ReachSendRequest{
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

	req := &ReachSendRequest{
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

	req := &ReachSendRequest{
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
