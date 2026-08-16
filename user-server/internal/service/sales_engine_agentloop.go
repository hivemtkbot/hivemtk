package service

import (
	"context"

	"fmt"

	"time"

	"hivemtk-user/internal/aiagent/llm"

	"hivemtk-user/internal/dto"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"encoding/json"
	"sort"
	"strings"
)

// SetAgentLoopMaxIterations 注入 Agent Loop 最大迭代次数
// 由 main.go 启动时调用；≤0 时保持默认 5
func SetAgentLoopMaxIterations(n int) {
	if n <= 0 {
		return
	}
	agentLoopMaxIterations = n
}

// SetAgentLoopMaxTools 注入 Agent Loop 工具数量上限
func SetAgentLoopMaxTools(n int) {
	if n <= 0 {
		return
	}
	agentLoopMaxTools = n
}

// SetAgentLoopTimeout 注入 Agent Loop 总超时（秒）
// 由 main.go 启动时调用：service.SetAgentLoopTimeout(cfg.Inference.LLM.TimeoutSeconds)
// ≤0 时保持默认 180s
func SetAgentLoopTimeout(seconds int) {
	if seconds <= 0 {
		return
	}
	agentLoopTotalTimeout = time.Duration(seconds) * time.Second
}

// runAgentLoop 真正的智能体 Agent Loop（核心实现）
//
// 流程（ReAct 模式）：
//  1. 构造初始 messages（system + user）
//  2. 调用 LLM，携带 tools 数组
//  3. 若 LLM 返回 finish_reason=tool_calls：
//     a. 将 assistant 消息（含 tool_calls）追加到对话历史
//     b. 调用 AgentToolExecutor.DispatchToolCalls 并发执行所有 tool_call
//     c. 将每个工具执行结果作为 role=tool 消息追加到对话历史
//     d. 回到步骤 2，再次调用 LLM（携带更新后的 messages）
//  4. 若 LLM 返回 finish_reason=stop（或达到最大迭代次数）：
//     a. 取 LLM 最终文本作为候选回复
//     b. 返回 DispatchResult（含 tool_calls 历史、token 用量、finish_reason）
//
// 设计要点：
//   - 真正的智能体：LLM 自主决定调用哪些工具、何时停止；非硬编码的步骤序列
//   - 工具调用结果回灌：将工具返回的 JSON 作为 role=tool 消息，让 LLM 基于真实业务数据生成回复
//   - 失败降级：工具执行失败时仍把错误信息回灌给 LLM，让 LLM 决定是否重试或换工具
//   - 迭代次数限制：防止无限循环；达到上限时使用最后一次 LLM 输出
func (e *SalesEngine) runAgentLoop(
	ctx context.Context,
	scenario llm.DispatchScenario,
	prompt string,
	req *SalesRequest,
	intent *dto.RecognizeResult,
	mem *model.DialogueMemory,
	customer *model.Customer,
	availableTools []AgentToolDef,
	ragChunks []RAGChunk,
) (string, *llm.DispatchResult, []model.RichCard, error) {

	targetLang := e.resolveTargetLang(ctx, req.UserMessage)

	agentIDStr := fmt.Sprintf("%d", agentIDFromCtx(req))
	allowed := agentContextToolNames(req)

	if len(allowed) > 0 && e.permissionChecker != nil {
		e.permissionChecker.SetAgentWhitelist(agentIDStr, allowed)
	}

	maxTools, maxIter := resolveAgentSettings(ctx)
	filteredTools := limitToolsForAgent(availableTools, maxTools, allowed)
	toolDefs := make([]llm.ToolDefinition, 0, len(filteredTools))
	for _, fn := range filteredTools {
		params := fn.Parameters
		if params == nil {
			params = map[string]any{"type": "object"}
		}
		toolDefs = append(toolDefs, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        fn.Name,
				Description: fn.Description,
				Parameters:  params,
			},
		})
	}
	if len(toolDefs) == 0 {

		result, err := e.dispatcher.Dispatch(ctx, llm.DispatchRequest{
			Scenario:     scenario,
			Prompt:       prompt,
			SystemPrompt: e.personaWithLang(ctx, req.Config.Persona, targetLang),
			MaxTokens:    req.Config.MaxTokens,
			Temperature:  req.Config.Temperature,
			CacheKey:     llm.CacheKey(scenario, prompt),
			CacheTTL:     3600,
		})
		if err != nil {
			return "", nil, nil, err
		}
		content := strings.TrimSpace(result.Content)
		if content == "" {

			content = e.emptyReplyFallback()
		}
		return e.calibrate(ctx, content, targetLang), result, nil, nil
	}

	messages := make([]llm.ChatMessage, 0, 4+maxIter*2)
	messages = append(messages, llm.ChatMessage{
		Role:    "system",
		Content: buildAgentSystemPrompt(e.personaWithLang(ctx, req.Config.Persona, targetLang), intent, mem, customer, ragChunks),
	})
	messages = append(messages, llm.ChatMessage{
		Role:    "user",
		Content: prompt,
	})

	if e.db != nil && req.SessionID != "" {
		var hist []model.SessionMessage
		if err := e.db.Where("session_id = ?", req.SessionID).
			Order("id desc").Limit(20).Find(&hist).Error; err == nil && len(hist) > 0 {

			if hist[0].Content == req.UserMessage {
				hist = hist[1:]
			}
			if len(hist) > 0 {

				for i, j := 0, len(hist)-1; i < j; i, j = i+1, j-1 {
					hist[i], hist[j] = hist[j], hist[i]
				}
				historyMsgs := make([]llm.ChatMessage, 0, len(hist))
				for _, m := range hist {
					role := "user"
					if m.SenderType == "ai" || m.SenderType == "agent" {
						role = "assistant"
					}
					historyMsgs = append(historyMsgs, llm.ChatMessage{Role: role, Content: m.Content})
				}

				newMessages := make([]llm.ChatMessage, 0, len(messages)+len(historyMsgs))
				newMessages = append(newMessages, messages[0])
				newMessages = append(newMessages, historyMsgs...)
				newMessages = append(newMessages, messages[1:]...)
				messages = newMessages
			}
		}
	}

	agentLoopCtx, agentLoopCancel := context.WithTimeout(ctx, agentLoopTotalTimeout)
	defer agentLoopCancel()

	var lastResult *llm.DispatchResult
	totalToolCalls := 0
	var firstLLMError error 
	curMaxTokens := req.Config.MaxTokens
	if curMaxTokens <= 0 {

		curMaxTokens = 2048
	}
	curTools := toolDefs
	lengthRetryDone := false
	var collectedCards []model.RichCard 
	for iter := 1; iter <= maxIter; iter++ {

		if agentLoopCtx.Err() != nil {
			logger.Warnf("[AgentLoop] wall-clock timeout at iter=%d total_tool_calls=%d, fallback to last content",
				iter, totalToolCalls)
			break
		}

		logger.Infof("[AgentLoop] iter=%d messages=%d tools=%d prompt_len=%d max_tokens=%d", iter, len(messages), len(curTools), len(prompt), curMaxTokens)
		logger.Infof("[AgentLoop] prompt_preview=%s", truncate(prompt, 300))
		result, err := e.dispatcher.Dispatch(agentLoopCtx, llm.DispatchRequest{
			Scenario:     scenario,
			Prompt:       prompt,
			SystemPrompt: e.personaWithLang(ctx, req.Config.Persona, targetLang),
			MaxTokens:    curMaxTokens,
			Temperature:  req.Config.Temperature,
			Tools:        curTools,
			ToolChoice:   "auto",
			Messages:     messages,
		})
		if err != nil {

			if firstLLMError == nil {
				firstLLMError = err
			}
			logger.Warnf("[AgentLoop] iter=%d LLM dispatch failed: %v, fallback to text response", iter, err)

			break
		}
		lastResult = result

		if result.FinishReason != "tool_calls" || len(result.ToolCalls) == 0 {

			if result.FinishReason == "length" && strings.TrimSpace(result.Content) == "" && !lengthRetryDone {
				lengthRetryDone = true
				curMaxTokens = curMaxTokens * 2
				logger.Warnf("[AgentLoop] iter=%d 推理模型耗尽 token(content 为空)，重试: max_tokens=%d（保留工具）", iter, curMaxTokens)
				continue
			}

			content := strings.TrimSpace(result.Content)
			if content == "" {

				if firstLLMError == nil {
					firstLLMError = fmt.Errorf("LLM returned empty final content (finish_reason=%s)", result.FinishReason)
				}
				logger.Warnf("[AgentLoop] iter=%d 最终文本回复为空(finish_reason=%s)，降级处理", iter, result.FinishReason)
				break
			}
			logger.Infof("[AgentLoop] iter=%d finish_reason=%s content_len=%d tools_called=%d",
				iter, result.FinishReason, len(content), totalToolCalls)
			return e.calibrate(ctx, content, targetLang), result, collectedCards, nil
		}

		assistantMsg := llm.ChatMessage{
			Role:      "assistant",
			Content:   result.Content,
			ToolCalls: result.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		toolCtx := AgentToolContext{
			AgentID:    agentIDStr,
			SessionID:  req.SessionID,
			CustomerID: req.CustomerID,
			Source:     "agent",
		}

		agentCalls := make([]AgentToolCall, 0, len(result.ToolCalls))
		for _, tc := range result.ToolCalls {
			agentCalls = append(agentCalls, AgentToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		toolResults := e.toolExecutor.DispatchToolCalls(ctx, agentCalls, toolCtx)
		for _, tr := range toolResults {
			if tr.Card != nil {
				collectedCards = append(collectedCards, *tr.Card)
			}
		}
		totalToolCalls += len(toolResults)

		for i, tr := range toolResults {
			// 取对应的 tool_call 的 function name 用于 role=tool 的 name 字段
			var toolName string
			if i < len(result.ToolCalls) {
				toolName = result.ToolCalls[i].Function.Name
			}
			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				ToolCallID: tr.ToolCallID,
				Name:       toolName,
				Content:    tr.Content,
			})
		}
		logger.Infof("[AgentLoop] iter=%d finish_reason=%s tool_calls=%d total_calls=%d",
			iter, result.FinishReason, len(result.ToolCalls), totalToolCalls)
	}

	logger.Warnf("[AgentLoop] exited: total_tool_calls=%d, has_last_result=%v, llm_error=%v",
		totalToolCalls, lastResult != nil, firstLLMError)
	if lastResult != nil {
		content := strings.TrimSpace(lastResult.Content)
		if content == "" {

			content = e.emptyReplyFallback()
		}
		return content, lastResult, collectedCards, nil
	}
	if firstLLMError != nil {

		return "抱歉，AI 服务暂时不可用，请稍后再试或联系人工客服。", nil, nil, nil
	}
	return "", nil, nil, fmt.Errorf("agent loop exhausted with no final content")
}

// emptyReplyFallback 当 LLM 返回空内容（被截断或未产出任何文本）时返回的友好降级话术。
// 铁律：LLM 返回空必须兜底，绝对禁止向用户展示空白回复。
// 集中定义便于回归测试守护，避免各处硬编码不一致。
func (e *SalesEngine) emptyReplyFallback() string {
	return "抱歉，我暂时无法处理您的请求，请稍后再试。"
}

// buildAgentSystemPrompt 构造 Agent 模式下的系统提示词
// 在原 Persona 基础上追加工具使用指引，让 LLM 知道何时调用哪些工具
func buildAgentSystemPrompt(persona string, intent *dto.RecognizeResult, mem *model.DialogueMemory, customer *model.Customer, ragChunks []RAGChunk) string {
	var sb strings.Builder
	sb.WriteString(persona)

	if customer != nil {
		name := customerNameOf(customer)
		if name != "" {
			sb.WriteString(fmt.Sprintf("\n[客户] %s", name))
		}
	}

	sb.WriteString("\n\n# 工具使用指引\n")
	sb.WriteString("- 当用户询问商品/产品推荐、清单、对比，或提到“有哪些产品/适合新手/推荐几款/给我看看”时，必须调用 card.show 工具返回结构化卡片，禁止只在文本里描述卡片。\n")

	sb.WriteString("- 系统已为你预检索相关知识库内容（见下方【知识库参考】），优先基于其回答，避免重复调用 rag.search；仅当用户问题明显超出已给范围、需要更多资料时，才调用 rag.search 补充检索。\n")

	if len(ragChunks) > 0 {
		sb.WriteString("\n【知识库参考】:\n")

		for i, chunk := range ragChunks {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, truncate(chunk.Content, 400)))
		}
	}
	return sb.String()
}

// limitToolsForAgent 限制注入到 LLM 的工具数量
// 40 个工具全注入会超出上下文窗口，只保留最相关的 maxTools 个
// agentContextToolNames 返回本 agent 配置的工具白名单（来自 AgentContext.Tools）。
// 非空时 runAgentLoop 仅注入并放行名单内工具；空时走 limitToolsForAgent 默认优先级集。
func agentContextToolNames(req *SalesRequest) []string {
	if req == nil || req.AgentContext == nil {
		return nil
	}
	return req.AgentContext.Tools
}

// limitToolsForAgent 计算注入给 LLM 的工具子集（双层防护之「注入期过滤」）。
//
// 行为：
//   - allowed 非空（来自 AgentContext.Tools 工具白名单）：仅保留名单内工具，按白名单顺序返回，
//     上限 30（保护 LLM 上下文）。未被 agent 授权的工具 LLM 根本看不到 → 无法发起调用。
//   - allowed 为空：按默认优先级取前 maxTools 个工具。默认优先级已覆盖电商客服关键路径
//     （rag/customer/order/pm/reach.card.send/reach.sms.send/aftersale.*/logistics.track 等），
//     不再像旧版硬编码 top-10 那样把发卡片、售后、物流工具砍掉。
func limitToolsForAgent(tools []AgentToolDef, maxTools int, allowed []string) []AgentToolDef {
	if maxTools <= 0 {
		maxTools = 18
	}

	priority := map[string]int{

		"rag.search": 1, "card.show": 2, "knowledge.feedback": 3, "knowledge.add_doc": 4, "knowledge.list_kb": 5,

		"customer.search": 6, "customer.get": 7, "customer.create": 8, "customer.update": 9,
		"customer.merge": 10, "customer.add_tag": 11, "customer.remove_tag": 12, "customer.segment": 13,

		"order.lookup": 14, "aftersale.create": 15, "aftersale.query": 16, "logistics.track": 17,

		"pm.session.open": 18, "pm.session.read": 19, "pm.message.send": 20,

		"follow_task.create": 21, "follow_task.update": 22,

		"reach.card.send": 23, "reach.sms.send": 24, "reach.web.send": 25,

		"reach.weixin.send": 26, "reach.wecom.send": 27, "reach.feishu.send": 28, "reach.dingtalk.send": 29,
		"reach.telegram.send": 30, "reach.whatsapp.send": 31, "reach.douyin.send": 32, "reach.kuaishou.send": 33,
		"reach.xhs.send": 34,

		"reach.batch": 35, "reach.schedule": 36, "reach.recall": 37, "reach.health": 38,
		"reach.history": 39, "reach.template.apply": 40, "reach.account.list": 41,
	}

	if len(allowed) > 0 {
		allowedWithCard := ensureToolInList(allowed, cardShowToolName)
		ordered := make([]AgentToolDef, 0, len(allowedWithCard))
		for _, n := range allowedWithCard {
			for _, t := range tools {
				if t.Name == n {
					ordered = append(ordered, t)
					break
				}
			}
		}
		if len(ordered) > 30 {
			return ordered[:30]
		}
		return ordered
	}

	if len(tools) <= maxTools {
		return ensureCardShowPresent(tools, tools, maxTools)
	}
	type scored struct {
		tool  AgentToolDef
		score int
	}
	scoredTools := make([]scored, 0, len(tools))
	for i, t := range tools {
		s, ok := priority[t.Name]
		if !ok {

			s = 100 + i
		}
		scoredTools = append(scoredTools, scored{tool: t, score: s})
	}
	sort.Slice(scoredTools, func(i, j int) bool {
		return scoredTools[i].score < scoredTools[j].score
	})
	result := make([]AgentToolDef, 0, maxTools)
	for i := 0; i < maxTools && i < len(scoredTools); i++ {
		result = append(result, scoredTools[i].tool)
	}
	return ensureCardShowPresent(tools, result, maxTools)
}

// cardShowToolName 是会话内结构化卡片工具的固定名称，作为通用能力始终注入 Agent Loop。
const cardShowToolName = "card.show"

// ensureToolInList 确保 name 存在于列表；若不存在则追加到末尾。
func ensureToolInList(list []string, name string) []string {
	for _, l := range list {
		if l == name {
			return list
		}
	}
	return append(append([]string{}, list...), name)
}

// ensureCardShowPresent 保证 card.show 工具（若已注册）出现在最终注入列表中：
// 未满则追加；已满则替换最低优先级（末尾）工具，确保通用卡片能力不被限额挤掉。
func ensureCardShowPresent(all, selected []AgentToolDef, maxTools int) []AgentToolDef {
	var cardTool *AgentToolDef
	for i := range all {
		if all[i].Name == cardShowToolName {
			cardTool = &all[i]
			break
		}
	}
	if cardTool == nil {
		return selected
	}
	for _, t := range selected {
		if t.Name == cardShowToolName {
			return selected
		}
	}
	if len(selected) < maxTools {
		return append(selected, *cardTool)
	}
	res := make([]AgentToolDef, len(selected))
	copy(res, selected)
	res[len(res)-1] = *cardTool
	return res
}

// buildPrompt 构造 LLM prompt
func (e *SalesEngine) buildPrompt(
	req *SalesRequest,
	intent *dto.RecognizeResult,
	mem *model.DialogueMemory,
	sop *model.SOPAgent,
	stage string,
	ragChunks []RAGChunk,
	script *ScriptTemplate,
	customer *model.Customer,
) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("【客户消息】: %s\n\n", req.UserMessage))

	if customer != nil {
		name := customerNameOf(customer)
		if name != "" {
			sb.WriteString(fmt.Sprintf("【客户昵称】: %s\n", name))
		}
		if customer.Phone != "" {
			sb.WriteString(fmt.Sprintf("【客户手机】: %s\n", customer.Phone))
		}
	}

	if intent != nil {
		sb.WriteString(fmt.Sprintf("\n【识别意图】: %s (置信度 %.0f%%)\n",
			intent.IntentName, intent.Confidence*100))
	}

	if sop != nil {
		sb.WriteString(fmt.Sprintf("【适用 SOP】: %s\n", sop.Name))
	}
	if stage != "" && stage != "default" {
		sb.WriteString(fmt.Sprintf("【客户阶段】: %s\n", stage))
	}

	if len(ragChunks) > 0 {
		sb.WriteString("\n【知识库参考】:\n")
		for i, chunk := range ragChunks {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, truncate(chunk.Content, 200)))
		}
	}

	if script != nil {
		sb.WriteString(fmt.Sprintf("\n【话术参考】: %s\n", truncate(script.Content, 150)))
	}

	if mem != nil && len(mem.KeyFacts) > 0 {
		sb.WriteString("\n【关键事实】:\n")
		factsJSON, _ := json.Marshal(mem.KeyFacts)
		sb.WriteString(string(factsJSON))
		sb.WriteString("\n")
	}

	if e.db != nil && req.SessionID != "" {
		var hist []model.SessionMessage
		if err := e.db.Where("session_id = ?", req.SessionID).
			Order("id desc").Limit(20).Find(&hist).Error; err == nil && len(hist) > 0 {

			if hist[0].Content == req.UserMessage {
				hist = hist[1:]
			}
			if len(hist) > 0 {
				for i, j := 0, len(hist)-1; i < j; i, j = i+1, j-1 {
					hist[i], hist[j] = hist[j], hist[i]
				}
				var hb strings.Builder
				hb.WriteString(fmt.Sprintf("\n【对话历史（按时间顺序，共 %d 条）】\n", len(hist)))
				for _, m := range hist {
					role := "客户"
					if m.SenderType == "ai" || m.SenderType == "agent" {
						role = "AI"
					}
					hb.WriteString(fmt.Sprintf("%s：%s\n", role, m.Content))
				}
				sb.WriteString(hb.String())
			}
		}
	}

	sb.WriteString("\n【回复要求】:\n")
	sb.WriteString("1. 基于上述信息（含对话历史）生成回复，不要编造事实\n")
	sb.WriteString("2. 简洁（≤80 字），分自然段\n")
	sb.WriteString("3. 语气亲切、像真人对话\n")
	sb.WriteString("4. 若客户异议，按话术/SOP 引导\n")

	return sb.String()
}

// safeID 安全返回客户 ID
func safeID(c *model.Customer) string {
	if c == nil {
		return ""
	}
	return c.ID
}

