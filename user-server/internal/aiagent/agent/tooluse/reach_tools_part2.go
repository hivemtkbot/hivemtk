// 拆分自 reach_tools.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package tooluse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

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

	msgID, resp, err := sendViaPipeline(ctx, t.deps, &ReachSendRequest{
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

	// G4：通过 9 步 SendPipeline 发送（Channel="card"，子渠道存 Metadata["subchannel"]）
	req := &ReachSendRequest{
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
			req := &ReachJobRequest{
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

	req := &ReachJobRequest{
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

	req := &ReachSendRequest{
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
