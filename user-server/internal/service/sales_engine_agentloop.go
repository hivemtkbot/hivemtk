package service

import (
	"context"

	"fmt"

	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/aiagent/llm"

	"hivemtk-user/internal/dto"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/pkg/tracing"
	textutil "hivemtk-user/internal/pkg/utils/text"

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
	// TL-3：按场景白名单裁剪工具（intent_recognize 只暴露 knowledge/customer 类；
	// 其余场景全量）。见 tooluse.ScenarioAllowedCategories。
	availableTools = filterToolsForScenario(scenario, availableTools)
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

	guard := newAgentLoopGuard(agentLoopTotalTimeout, agentLoopMaxTotalTokens, agentLoopMaxTotalCostUSD)

	messages := make([]llm.ChatMessage, 0, 4+maxIter*2)
	messages = append(messages, llm.ChatMessage{
		Role:    "system",
		Content: buildAgentSystemPrompt(e.personaWithLang(ctx, req.Config.Persona, targetLang), intent, mem, customer, ragChunks, guard),
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

	stopReason := stopReasonNone
	totalToolCalls := 0

	// A-2/A-3 LoopGuard 接线：跨轮次累计 LLM 成本追踪 + 结构化停止原因写 span status。
	// 与本地 agentLoopGuard 双层互补：本地管单次运行预算熔断，LoopGuard 管累计成本
	// 记账（RecordCost）与收尾清理（FinishTrace 防长驻进程内存累积）。
	loopGuard := tooluse.NewLoopGuard(tooluse.DefaultLoopGuardConfig())
	loopTraceID := req.SessionID
	if loopTraceID == "" {
		loopTraceID = "agentloop:" + agentIDStr
	}
	defer loopGuard.FinishTrace(loopTraceID)

	// writeLoopSpan 收敛处写 span status：StopReasonOf 将错误归类为结构化 StopReason，
	// 落 message_trace（NodeAgentTurn），监控侧按 stop_reason 维度聚合
	writeLoopSpan := func(output any, err error) {
		sr := tooluse.StopReasonOf(err)
		sp := tracing.Start(ctx, tracing.NodeAgentTurn).
			TraceID(loopTraceID).
			Kind("agent_turn").
			Agent(agentIDStr)
		if sr != tooluse.StopReasonCompleted {
			sp = sp.Abnormal("stop_reason=" + string(sr))
		}
		sp.End(output, err)
	}

	defer func() {
		logger.Infof("[AgentLoop] stop_reason=%s used_tokens=%d used_cost_usd=%.4f tool_calls=%d",
			stopReason, guard.usedTokens, guard.usedCost, totalToolCalls)
	}()

	defer func() {
		agentLoopStops.WithLabel(string(stopReason)).Inc()
	}()

	var lastResult *llm.DispatchResult
	totalTokensUsed := 0
	var firstLLMError error
	curMaxTokens := req.Config.MaxTokens
	if curMaxTokens <= 0 {

		curMaxTokens = 2048
	}
	curTools := toolDefs
	lengthRetryDone := false
	var collectedCards []model.RichCard
	for iter := 1; iter <= maxIter; iter++ {

		// P1-5: 预估先行 —— 在 check() 之前先扣减一个预估量，防止同轮内预算穿透
		// 预估值 = 当前已用 / 当前迭代号（粗略估算单轮平均消耗）
		if iter > 1 && guard != nil {
			avgCost := guard.usedCost / float64(iter-1)
			if avgCost > 0 {
				guard.ChargeEstimated(0, avgCost)
			}
		}

		// 统一护栏：先检查后消费（check before spend），任一维度触达即停止
		if r := guard.check(); r != stopReasonNone {
			stopReason = r
			logger.Warnf("[AgentLoop] guard tripped at iter=%d reason=%s tokens=%d cost_usd=%.4f, fallback to last content",
				iter, r, guard.usedTokens, guard.usedCost)
			break
		}

		// v3 审计 P3-1 修复：per-iteration 超时（业界共识：单次 LLM 调用超时）
		// 用派生 ctx 限定单次 LLM 调用时长，不影响总 agentLoopTotalTimeout
		iterCtx, iterCancel := context.WithTimeout(agentLoopCtx, agentLoopMaxPerIterTimeout)
		logger.Infof("[AgentLoop] iter=%d messages=%d tools=%d prompt_len=%d max_tokens=%d used_tokens=%d", iter, len(messages), len(curTools), len(prompt), curMaxTokens, totalTokensUsed)
		logger.Infof("[AgentLoop] prompt_preview=%s", truncate(prompt, 300))
		result, err := e.dispatcher.Dispatch(iterCtx, llm.DispatchRequest{
			Scenario:     scenario,
			Prompt:       prompt,
			SystemPrompt: e.personaWithLang(ctx, req.Config.Persona, targetLang),
			MaxTokens:    curMaxTokens,
			Temperature:  req.Config.Temperature,
			Tools:        curTools,
			ToolChoice:   "auto",
			Messages:     messages,
		})
		iterCancel() // 立即释放 per-iter ctx
		if err != nil {
			// 区分"总超时"vs"单次超时"
			if iterCtx.Err() == context.DeadlineExceeded && agentLoopCtx.Err() == nil {
				logger.Warnf("[AgentLoop] iter=%d per-iter timeout (budget=%s), continue with next iter", iter, agentLoopMaxPerIterTimeout)
				continue
			}

			if firstLLMError == nil {
				firstLLMError = err
			}
			stopReason = stopReasonLLMError
			logger.Warnf("[AgentLoop] iter=%d LLM dispatch failed: %v, fallback to text response", iter, err)

			break
		}
		lastResult = result

		// 累计消耗（token + 美元成本，供统一护栏判定）
		// token 优先 LLM 真实 Usage.Usage.TotalTokens（业界最佳），否则退回 result.TotalTokens；
		// 成本取 dispatcher 计算的 result.Cost（按 Provider CostPer1k 计价）
		iterTokens := 0
		if result.Usage.TotalTokens > 0 {
			iterTokens = result.Usage.TotalTokens
		} else if result.TotalTokens > 0 {
			iterTokens = result.TotalTokens
		}
		totalTokensUsed += iterTokens
		guard.charge(iterTokens, result.Cost)

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
				stopReason = stopReasonEmptyFinal
				logger.Warnf("[AgentLoop] iter=%d 最终文本回复为空(finish_reason=%s)，降级处理", iter, result.FinishReason)
				break
			}
			logger.Infof("[AgentLoop] iter=%d finish_reason=%s content_len=%d tools_called=%d",
				iter, result.FinishReason, len(content), totalToolCalls)
			stopReason = stopReasonCompleted
			finalContent := e.calibrate(ctx, content, targetLang)
			writeLoopSpan(finalContent, nil)
			return finalContent, result, collectedCards, nil
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
	if stopReason == stopReasonNone {
		stopReason = "max_iterations_exhausted"
	}
	logger.Warnf("[AgentLoop] exited: stop_reason=%s total_tool_calls=%d, has_last_result=%v, llm_error=%v",
		stopReason, totalToolCalls, lastResult != nil, firstLLMError)
	if lastResult != nil {
		content := strings.TrimSpace(lastResult.Content)
		if content == "" {

			content = e.emptyReplyFallback()
		}
		writeLoopSpan(content, firstLLMError)
		return content, lastResult, collectedCards, nil
	}
	if firstLLMError != nil {

		fallback := "抱歉，AI 服务暂时不可用，请稍后再试或联系人工客服。"
		writeLoopSpan(fallback, firstLLMError)
		return fallback, nil, nil, nil
	}
	exhaustedErr := fmt.Errorf("agent loop exhausted with no final content")
	writeLoopSpan("", exhaustedErr)
	return "", nil, nil, exhaustedErr
}

// emptyReplyFallback 当 LLM 返回空内容（被截断或未产出任何文本）时返回的友好降级话术。
// 铁律：LLM 返回空必须兜底，绝对禁止向用户展示空白回复。
// 集中定义便于回归测试守护，避免各处硬编码不一致。
func (e *SalesEngine) emptyReplyFallback() string {
	return "抱歉，我暂时无法处理您的请求，请稍后再试。"
}

// buildAgentSystemPrompt 构造 Agent 模式下的系统提示词
// 在原 Persona 基础上追加工具使用指引，让 LLM 知道何时调用哪些工具
func buildAgentSystemPrompt(persona string, intent *dto.RecognizeResult, mem *model.DialogueMemory, customer *model.Customer, ragChunks []RAGChunk, guard *agentLoopGuard) string {
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

	if guard != nil {
		sb.WriteString("\n\n# 系统资源预算\n")
		remainTokens := guard.RemainingTokens()
		remainCost := guard.RemainingCostUSD()
		if remainTokens >= 0 {
			sb.WriteString(fmt.Sprintf("- 剩余 token 预算: ~%d\n", remainTokens))
		}
		if remainCost >= 0 {
			sb.WriteString(fmt.Sprintf("- 剩余成本预算: $%.2f\n", remainCost))
			if remainCost < 0.10 && remainCost >= 0 {
				sb.WriteString("- ⚠️ 成本即将耗尽，请尽量用已有工具数据回答，不要再调新的 LLM\n")
			}
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

// filterToolsForScenario TL-3 场景裁剪：intent_recognize 类场景只保留
// knowledge/customer 类工具（映射表见 tooluse.scenarioToolWhitelist）；
// 场景未配置白名单时原样返回（全量，向后兼容）。
func filterToolsForScenario(scenario llm.DispatchScenario, defs []AgentToolDef) []AgentToolDef {
	cats, restricted := tooluse.ScenarioAllowedCategories(string(scenario))
	if !restricted {
		return defs
	}
	allowed := make(map[tooluse.ToolCategory]bool, len(cats))
	for _, c := range cats {
		allowed[c] = true
	}
	reg := tooluse.GetGlobalRegistry()
	out := make([]AgentToolDef, 0, len(defs))
	for _, d := range defs {
		if t, err := reg.Get(d.Name); err == nil && allowed[t.Category()] {
			out = append(out, d)
		}
	}
	return out
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

	// P1g 情感分层策略提示（焦虑=进度可视化 / 满意=裂变引导），空=不注入
	if req.EmotionHint != "" {
		sb.WriteString(fmt.Sprintf("【情绪应对策略】: %s\n\n", req.EmotionHint))
	}

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
		hist := e.fetchHistoryWithinTokenBudget(req.SessionID, req.UserMessage)
		if len(hist) > 0 {
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

// —— A-4 TokenBudget 轻量版：历史消息按 token 预算截断（替代固定取 20 条）——

// agentLoopHistoryTokenBudget 历史消息上下文 token 预算（含输出预留）
const agentLoopHistoryTokenBudget = 4096

// agentLoopHistoryOutputReservePct 为 LLM 输出预留的预算百分比（30%）
const agentLoopHistoryOutputReservePct = 30

// agentLoopHistoryMaxCandidates 单次查询的历史候选条数上限（截断前预取，防长会话全量拉取）
const agentLoopHistoryMaxCandidates = 200

// historyMsgTokenOverhead 每条历史消息的格式化开销估算 token（"客户：" / "AI：" + 换行）
const historyMsgTokenOverhead = 6

// fetchHistoryWithinTokenBudget 拉取会话历史并按 token 预算从新到旧保留：
//   - 可用预算 = 总预算 × (1 - 输出预留 30%)
//   - 从最新一条向前累加估算 token，超出预算即停止（保留最近上下文优先）
//   - 保持消息对完整性：截断边界若落在 AI 回复上（其配对的客户消息已被裁掉），
//     则丢弃该孤儿 AI 消息，保证最旧保留项一定是「客户」发言（一对的开头）
func (e *SalesEngine) fetchHistoryWithinTokenBudget(sessionID, userMessage string) []model.SessionMessage {
	var hist []model.SessionMessage
	if err := e.db.Where("session_id = ?", sessionID).
		Order("id desc").Limit(agentLoopHistoryMaxCandidates).Find(&hist).Error; err != nil || len(hist) == 0 {
		return nil
	}

	// 与既有行为一致：最新一条若是当前消息本身则剔除
	if hist[0].Content == userMessage {
		hist = hist[1:]
	}
	if len(hist) == 0 {
		return nil
	}

	budget := agentLoopHistoryTokenBudget * (100 - agentLoopHistoryOutputReservePct) / 100
	used := 0
	keep := 0
	for i, m := range hist {
		cost := textutil.EstimateTokens(m.Content) + historyMsgTokenOverhead
		if used+cost > budget {
			keep = i
			break
		}
		used += cost
		keep = i + 1
	}
	truncated := keep < len(hist)
	hist = hist[:keep]

	// 消息对完整性（仅预算截断发生时）：此时 hist 从新到旧，最旧保留项在末尾；
	// 若它是 AI 发言（其配对的客户消息已被裁掉），从末尾丢弃该孤儿回复，
	// 保证最旧保留项一定是「客户」发言（一对的开头）
	if truncated {
		for len(hist) > 0 {
			last := hist[len(hist)-1]
			if last.SenderType != "ai" && last.SenderType != "agent" {
				break
			}
			hist = hist[:len(hist)-1]
		}
	}

	// 恢复时间正序
	for i, j := 0, len(hist)-1; i < j; i, j = i+1, j-1 {
		hist[i], hist[j] = hist[j], hist[i]
	}
	return hist
}

