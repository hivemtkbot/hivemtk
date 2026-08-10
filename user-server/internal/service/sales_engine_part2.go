// 拆分自 sales_engine.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"sort"
	"strings"
	"time"
)

func (e *SalesEngine) matchSOP(ctx context.Context, intent *dto.RecognizeResult, customer *model.Customer) (*model.SOPAgent, string, error) {
	if e.sop == nil || intent == nil {
		return nil, "", nil
	}
	if intent.IntentType == IntentUnknown {
		return nil, "", nil
	}
	stage := "default"
	if customer != nil && customer.ChurnRisk != "" {
		switch customer.ChurnRisk {
		case "high":
			stage = "churn_risk"
		case "low":
			stage = "active"
		}
	}
	sops, err := e.sop.MatchByIntent(ctx, intent.IntentType)
	if err != nil || len(sops) == 0 {
		return nil, stage, nil
	}
	// 返回第一个匹配的 SOP（生产环境可按评分选择）
	return &sops[0], stage, nil
}

// recallRAG RAG 召回
func (e *SalesEngine) recallRAG(ctx context.Context, req *SalesRequest, intent *dto.RecognizeResult) ([]RAGChunk, error) {
	if !req.Config.EnableRAG || e.ragSearcher == nil {
		return nil, nil
	}
	return e.ragSearcher.Search(ctx, req.UserMessage, req.Config.RAGTopK)
}

// matchScript 话术库匹配
func (e *SalesEngine) matchScript(ctx context.Context, intent *dto.RecognizeResult) (*ScriptTemplate, error) {
	if e.scriptLookup == nil || intent == nil {
		return nil, nil
	}
	script, err := e.scriptLookup.MatchScript(ctx, intent.IntentType, intent.IntentName)
	if err != nil {
		return nil, err
	}
	if script != nil {
		return script, nil
	}
	// M2 运行时覆盖默认：未匹配到配置话术时，回退到「生效中」的已购 sales_script 资产。
	if r := GetAssetResolver(); r != nil {
		if s, ok := r.GetActiveScript(ctx); ok && s != nil {
			return &ScriptTemplate{
				ID:       s.ID,
				Title:    s.Name,
				Scenario: s.Scenario,
				Content:  renderSalesScriptSteps(s.Scripts),
				Tags:     []string{"asset-market", s.ID},
			}, nil
		}
	}
	return nil, nil
}

// renderSalesScriptSteps 将资产话术步骤渲染为可直接作为话术参考的纯文本。
func renderSalesScriptSteps(scripts []map[string]interface{}) string {
	if len(scripts) == 0 {
		return ""
	}
	var b strings.Builder
	for i, st := range scripts {
		if i > 0 {
			b.WriteString("\n")
		}
		if name, ok := st["name"].(string); ok && name != "" {
			b.WriteString("【" + name + "】\n")
		}
		if content, ok := st["content"].(string); ok {
			b.WriteString(content)
		}
	}
	return b.String()
}

// SetPlaybook 注入销冠话术库（可选）
// 重构：参数改为 PlaybookRecommenderInterface，可注入测试替身实现进行单元测试
func (e *SalesEngine) SetPlaybook(ctx context.Context, p PlaybookRecommenderInterface) {
	e.playbook = p
}

// RecommendPlaybook 推荐话术（销售辅助模式专用入口）
// 商业产品级场景：销售收到客户消息后，点"获取建议"按钮调用
//   - industry: 推断或由前端传入（从客户档案/产品配置获取）
//   - productID: 产品ID
//   - stage: 客户当前旅程阶段
//   - intent: 当前意图（用于推断异议类型）
//
// 返回 3-5 条按成功率排序的话术建议
func (e *SalesEngine) RecommendPlaybook(ctx context.Context, industry Industry, productID string, stage JourneyStage, intent string) []*PlaybookEntry {
	if e.playbook == nil {
		return nil
	}
	return e.playbook.RecommendForResponse(ctx, industry, productID, stage, intent)
}

// fetchPlaybookSuggestions 在 Handle 流程中根据客户阶段+意图自动拉取话术建议
func (e *SalesEngine) fetchPlaybookSuggestions(ctx context.Context, industry Industry, productID string, stage JourneyStage, intent string) []*PlaybookEntry {
	if e.playbook == nil {
		return nil
	}
	return e.playbook.RecommendForResponse(ctx, industry, productID, stage, intent)
}

// generateCandidate LLM 生成候选回复
//
// 智能体升级：当注入了 toolExecutor 且存在可用工具时，调用 runAgentLoop
// 走真正的 Agent Loop（LLM ↔ 工具 循环）；否则走原始一次性 LLM 调用（向后兼容）。
//
// 集成双层架构 - 顶部先调 LayerRouter.Route
//   - Layer1 命中 (SkipLLM) -> 直接返回 FAQ/SOP 模板回复, 不调 LLM
//   - Layer1 未命中/置信度低 -> 走原 LLM 路径
func (e *SalesEngine) generateCandidate(
	ctx context.Context,
	req *SalesRequest,
	intent *dto.RecognizeResult,
	mem *model.DialogueMemory,
	sop *model.SOPAgent,
	stage string,
	ragChunks []RAGChunk,
	script *ScriptTemplate,
	customer *model.Customer,
) (string, *llm.DispatchResult, []model.RichCard, error) {
	// 回复语言链路：解析目标语种（配置优先 + 客户消息自动检测）。
	// 后续跨语言路径据此追加语种指令与术语表块，并对 LLM 输出做后置校准。
	targetLang := e.resolveTargetLang(ctx, req.UserMessage)

	// Layer1 双层路由 - 命中即跳过 LLM
	if e.layerRouter != nil {
		decision := e.layerRouter.Route(ctx, &RouteRequest{
			SessionID:   req.SessionID,
			CustomerID:  req.CustomerID,
			UserMessage: req.UserMessage,
			Intent:      intent,
			RAGChunks:   ragChunks,
			Stage:       stage,
			// 传 agentID 实现按智能体隔离的 FAQ/SOP 匹配
			AgentID: agentIDFromCtx(req),
		})
		if decision != nil && decision.SkipLLM && decision.Reply != "" {
			// Layer1 命中: 直接返回模板回复, dispatchResult=nil 表示未调 LLM
			return e.calibrate(ctx, decision.Reply, targetLang), nil, nil, nil
		}
	}

	if e.dispatcher == nil {
		return "", nil, nil, fmt.Errorf("dispatcher is nil")
	}

	// Agent Loop 场景：用户消息只传原始内容，让模型自己决定是否调用 rag.search；
	// 但 RAG 预检索结果已通过 ragChunks 注入系统提示词，避免无谓的额外检索往返降低延迟。
	agentPrompt := req.UserMessage
	prompt := e.buildPrompt(req, intent, mem, sop, stage, ragChunks, script, customer)
	scenario := llm.ScenarioSOPReply
	if intent != nil && intent.IntentType == IntentObjectionPrice {
		scenario = llm.ScenarioObjection
	} else if intent != nil && (intent.IntentType == IntentSocial || intent.IntentType == IntentGreeting) {
		scenario = llm.ScenarioFriendlyChat
	}

	// 智能体 Agent Loop 路径（真正的智能体，不做流程编排）
	if e.toolExecutor != nil {
		availableTools := e.toolExecutor.ListTools()
		if len(availableTools) > 0 {
			return e.runAgentLoop(ctx, scenario, agentPrompt, req, intent, mem, customer, availableTools, ragChunks)
		}
	}

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
	return e.calibrate(ctx, strings.TrimSpace(result.Content), targetLang), result, nil, nil
}

// agentLoopMaxIterations Agent Loop 最大迭代次数
// 防止 LLM 无限调用工具或陷入循环。
// 默认 5；运行期调参存数据库 system_config_kv[agent.settings].max_loop_iterations，
// 由 LoadAgentSettingsConfig 读取覆盖；也可由 SetAgentLoopMaxIterations 注入（测试/内嵌）。
// 注意：必须 >= 2。LLM 在第 1 轮返回 tool_calls 后，需要第 2 轮（携带工具结果）才能生成最终
// 文本回复；若设为 1，工具调用永远无法产出答案，会被降级为“空回复→转人工”，工具调用形同失效。
// 默认 5，兼顾多工具串联与 follow-up 问答；受 agentLoopTotalTimeout(默认180s) 约束。
var agentLoopMaxIterations = 5

// SetAgentLoopMaxIterations 注入 Agent Loop 最大迭代次数
// 由 main.go 启动时调用；≤0 时保持默认 5
func SetAgentLoopMaxIterations(n int) {
	if n <= 0 {
		return
	}
	agentLoopMaxIterations = n
}

// agentLoopMaxTools Agent Loop 向 LLM 注入的工具数量上限（默认优先级模式下）。
// 当 Agent 未配置 Tools 白名单时，limitToolsForAgent 按默认优先级取前 agentLoopMaxTools 个工具。
// 默认 18（覆盖原式硬编码的 10，使电商客服关键工具
// reach.card.send / reach.sms.send / aftersale.* / logistics.track 默认可见）；
// 运行期调参存数据库 system_config_kv[agent.settings].max_tools，由 LoadAgentSettingsConfig
// 读取覆盖；也可由 SetAgentLoopMaxTools 注入（测试/内嵌）。
var agentLoopMaxTools = 18

// SetAgentLoopMaxTools 注入 Agent Loop 工具数量上限
func SetAgentLoopMaxTools(n int) {
	if n <= 0 {
		return
	}
	agentLoopMaxTools = n
}

// resolveAgentSettings 解析 Agent Loop 运行期调参（数据库 system_config_kv[agent.settings]
// 为唯一真相源；缺配置/读取失败时回退到代码内默认值，尊重 SetAgentLoop* 注入）。
func resolveAgentSettings(ctx context.Context) (maxTools, maxIter int) {
	maxTools, maxIter = agentLoopMaxTools, agentLoopMaxIterations
	if cfg, err := LoadAgentSettingsConfig(ctx); err == nil && cfg != nil {
		if cfg.MaxTools > 0 {
			maxTools = cfg.MaxTools
		}
		if cfg.MaxLoopIterations > 0 {
			maxIter = cfg.MaxLoopIterations
		}
	}
	return
}

// agentLoopTotalTimeout Agent Loop wall-clock 总超时
//
// 演进：
// 120s（1.5B Q4 CPU 推理 35-60s）
// 改为可配置，由 main.go 启动时从 inference.llm.timeout_seconds 注入
//
// 设计：默认 180s（保守值，覆盖大多数 CPU 推理场景）。
// 开发模式可在 config.yaml 设大值（如 720s）确保 LLM 调用不被 ctx 掐断；
// 生产环境推荐 120-180s，超时后由 fallback 兜底。
// 由 SetAgentLoopTimeout 注入；与 dispatcher.MaxLatency、llm_service.httpClient.Timeout
// 共享同一配置源（inference.llm.timeout_seconds），全链路一致。
var agentLoopTotalTimeout = 180 * time.Second

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
	// 回复语言链路：解析目标语种，跨语言路径追加语种指令与术语表块。
	targetLang := e.resolveTargetLang(ctx, req.UserMessage)

	// 计算 agent 标识与工具白名单（来自 AgentContext.Tools）
	agentIDStr := fmt.Sprintf("%d", agentIDFromCtx(req))
	allowed := agentContextToolNames(req)
	// 双层防护第二层（执行期权限检查）：按 Agent 白名单覆盖式设置放行名单。
	// 仅当本 agent 显式配置 Tools 白名单时生效；否则不覆盖（保留管理 API 设置的名单或默认放行）。
	if len(allowed) > 0 && e.permissionChecker != nil {
		e.permissionChecker.SetAgentWhitelist(agentIDStr, allowed)
	}

	// 1. 构造工具定义列表（AgentToolDef → llm.ToolDefinition）
	// 限制工具数量，避免 prompt 过长超出上下文窗口（双层防护第一层：注入期过滤）
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
		// 所有工具序列化失败，降级到无工具调用
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
			// 无工具分支：LLM 返回空内容时同样降级，避免空白回复。
			content = e.emptyReplyFallback()
		}
		return e.calibrate(ctx, content, targetLang), result, nil, nil
	}

	// 2. 构造初始 messages（system + user）
	messages := make([]llm.ChatMessage, 0, 4+maxIter*2)
	messages = append(messages, llm.ChatMessage{
		Role:    "system",
		Content: buildAgentSystemPrompt(e.personaWithLang(ctx, req.Config.Persona, targetLang), intent, mem, customer, ragChunks),
	})
	messages = append(messages, llm.ChatMessage{
		Role:    "user",
		Content: prompt,
	})

	// 2.1 注入多轮对话历史（修复：原 agent loop 仅依赖 DialogueMemory.KeyFacts，但
	//     KeyFacts 的生产写入链路从未接线 → AI 无任何上下文、自认“第一次对话”。
	//     此处直接从 session_messages 按会话取最近 N 轮，作为 user/assistant 历史插入
	//     system 与当前 user 之间，使 LLM 真正获得多轮上下文。）
	if e.db != nil && req.SessionID != "" {
		var hist []model.SessionMessage
		if err := e.db.Where("session_id = ?", req.SessionID).
			Order("id desc").Limit(20).Find(&hist).Error; err == nil && len(hist) > 0 {
			// 去掉本轮用户消息（若最新一条即当前问题），仅保留历史
			if hist[0].Content == req.UserMessage {
				hist = hist[1:]
			}
			if len(hist) > 0 {
				// 倒序还原为正序
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
				// messages = [system, 历史..., 当前user]
				newMessages := make([]llm.ChatMessage, 0, len(messages)+len(historyMsgs))
				newMessages = append(newMessages, messages[0])
				newMessages = append(newMessages, historyMsgs...)
				newMessages = append(newMessages, messages[1:]...)
				messages = newMessages
			}
		}
	}

	// 3. Agent Loop（：wall-clock 总超时 30s 兜底，防止最坏 5min 卡死）
	// 总超时设计：30s 默认。即使 LLM 响应慢 + 工具慢，也保证 30s 内返回给用户
	agentLoopCtx, agentLoopCancel := context.WithTimeout(ctx, agentLoopTotalTimeout)
	defer agentLoopCancel()

	var lastResult *llm.DispatchResult
	totalToolCalls := 0
	var firstLLMError error // 记录首次 LLM 调用错误（用于最终降级返回）
	curMaxTokens := req.Config.MaxTokens
	if curMaxTokens <= 0 {
		// 默认 2048：推理模型（如 deepseek-v4-flash）在 reasoning 阶段需占用较多 token，
		// 过小的上限会导致 reasoning 耗尽 token 后无法产出 content/tool_calls。
		curMaxTokens = 2048
	}
	curTools := toolDefs
	lengthRetryDone := false
	var collectedCards []model.RichCard // 收集工具产出的结构化卡片，随最终回复一并下发
	for iter := 1; iter <= maxIter; iter++ {
		// 检查总超时
		if agentLoopCtx.Err() != nil {
			logger.Warnf("[AgentLoop] wall-clock timeout at iter=%d total_tool_calls=%d, fallback to last content",
				iter, totalToolCalls)
			break
		}

		// 3.1 调用 LLM（携带 tools + 完整对话历史）
		logger.Infof("[AgentLoop] iter=%d messages=%d tools=%d prompt_len=%d max_tokens=%d", iter, len(messages), len(curTools), len(prompt), curMaxTokens)
		logger.Infof("[AgentLoop] prompt_preview=%s", truncate(prompt, 300))
		result, err := e.dispatcher.Dispatch(agentLoopCtx, llm.DispatchRequest{
			Scenario:     scenario,
			Prompt:       prompt, // 仅在 Messages 为空时使用，这里 Messages 非空
			SystemPrompt: e.personaWithLang(ctx, req.Config.Persona, targetLang),
			MaxTokens:    curMaxTokens,
			Temperature:  req.Config.Temperature,
			Tools:        curTools,
			ToolChoice:   "auto",
			Messages:     messages,
		})
		if err != nil {
			// LLM 调用失败时不直接 return，降级到 fallback 内容
			// 首次失败记录错误；后续失败也降级，避免直接抛错给用户
			if firstLLMError == nil {
				firstLLMError = err
			}
			logger.Warnf("[AgentLoop] iter=%d LLM dispatch failed: %v, fallback to text response", iter, err)
			// 将错误信息作为 role=user 消息回灌，引导 LLM 下一轮降级处理
			// （若 agentLoopCtx 未超时，下一轮仍可尝试）
			break
		}
		lastResult = result

		// 3.2 判断 finish_reason
		if result.FinishReason != "tool_calls" || len(result.ToolCalls) == 0 {
			// DeepSeek 等推理模型在 reasoning 阶段可能耗尽 max_tokens，
			// 导致 finish_reason=length 且 content 为空。此时重试一次：放大 max_tokens，
			// 但【保留 tools】——Agent Loop 中必须允许模型在重试轮继续调用工具（含 card.show），
			// 否则卡片等工具能力将彻底失效。
			if result.FinishReason == "length" && strings.TrimSpace(result.Content) == "" && !lengthRetryDone {
				lengthRetryDone = true
				curMaxTokens = curMaxTokens * 2
				logger.Warnf("[AgentLoop] iter=%d 推理模型耗尽 token(content 为空)，重试: max_tokens=%d（保留工具）", iter, curMaxTokens)
				continue
			}
			// LLM 给出最终文本回复，结束循环。
			content := strings.TrimSpace(result.Content)
			if content == "" {
				// 最终文本回复为空（非 length 截断重试场景）：视为一次失败并记录首错，
				// 跳出到下方降级逻辑，避免向用户返回空白回复（铁律：LLM 返回空必须兜底）。
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

		// 3.3 LLM 决定调用工具：将 assistant 消息（含 tool_calls）追加到对话历史
		assistantMsg := llm.ChatMessage{
			Role:      "assistant",
			Content:   result.Content,
			ToolCalls: result.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		// 3.4 调用 AgentToolExecutor 并发执行所有 tool_call
		toolCtx := AgentToolContext{
			AgentID:    agentIDStr,
			SessionID:  req.SessionID,
			CustomerID: req.CustomerID,
			Source:     "agent",
		}
		// 将 llm.ToolCall 转为 AgentToolCall
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

		// 3.5 将每个工具结果作为 role=tool 消息回灌
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

	// 4. 循环退出：达到最大迭代次数 / wall-clock 超时 / LLM 调用失败
	// 降级策略（按优先级）：
	//   a. 若有 lastResult（LLM 至少成功响应过一次），使用其 Content
	//   b. 若无 lastResult 但有 firstLLMError，返回友好降级提示（不抛 error 给用户）
	//   c. 兜底：返回空内容 + error
	logger.Warnf("[AgentLoop] exited: total_tool_calls=%d, has_last_result=%v, llm_error=%v",
		totalToolCalls, lastResult != nil, firstLLMError)
	if lastResult != nil {
		content := strings.TrimSpace(lastResult.Content)
		if content == "" {
			// LLM 曾成功响应但内容为空（被截断或未产出任何文本）：返回降级提示，
			// 避免向用户展示空白回复（铁律：LLM 返回空必须兜底）。
			content = e.emptyReplyFallback()
		}
		return content, lastResult, collectedCards, nil
	}
	if firstLLMError != nil {
		// LLM 调用失败时返回友好降级提示，而非抛 error 给用户
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

	// 工具使用指引：确保结构化卡片在商品推荐场景被真正调用，而非仅在文本里“口头描述卡片”。
	// 推理模型容易把“已为您整理在上方卡片里”写进正文却不去调用 card.show，导致前端拿不到 ai_cards。
	sb.WriteString("\n\n# 工具使用指引\n")
	sb.WriteString("- 当用户询问商品/产品推荐、清单、对比，或提到“有哪些产品/适合新手/推荐几款/给我看看”时，必须调用 card.show 工具返回结构化卡片，禁止只在文本里描述卡片。\n")
	// 编排器已在步骤5完成 RAG 召回，相关片段已置于下方【知识库参考】。
	// 优先基于其作答，避免 Agent Loop 内再走一轮「决定调用 rag.search → 等待检索 → 重新生成」的 LLM 往返，
	// 可显著降低端到端延迟。仅当用户问题明显超出已给范围、确需补充资料时，才调用 rag.search。
	sb.WriteString("- 系统已为你预检索相关知识库内容（见下方【知识库参考】），优先基于其回答，避免重复调用 rag.search；仅当用户问题明显超出已给范围、需要更多资料时，才调用 rag.search 补充检索。\n")

	// 知识库预检索上下文：编排器已在步骤5完成召回，直接注入供模型作答，
	// 消除 Agent Loop 内重复检索带来的一整轮 LLM 往返（约 5s + 一次 LLM 调用）。
	if len(ragChunks) > 0 {
		sb.WriteString("\n【知识库参考】:\n")
		// 相较非 Agent 路径截断到 200，此处给到 400：Agent Loop 直接依赖该上下文作答，
		// 不再走 rag.search 工具二次检索，故留出更完整的片段以提升回答质量。
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
	// 默认优先级：知识库 > 客户 > 订单 > 私信 > 触达(卡片/短信) > 售后 > 物流 > 其他
	// card.show（会话内结构化卡片）为核心通用能力，固定高优先级（2），确保默认始终注入。
	// 该 map 必须覆盖当前所有已注册工具名（共 41 个），否则未登记工具默认 score=100
	// 会在截断阈值下被随机丢弃，导致后置工具永远不可达。新增工具时务必在此维护优先级。
	// 命名分组：关键路径 rag/customer/order/pm/reach.*/aftersale.*/logistics.*/follow_task.*/card.show。
	priority := map[string]int{
		// —— 知识库 / 核心检索（1-4）——
		"rag.search": 1, "card.show": 2, "knowledge.feedback": 3, "knowledge.add_doc": 4, "knowledge.list_kb": 5,
		// —— 客户（6-13）——
		"customer.search": 6, "customer.get": 7, "customer.create": 8, "customer.update": 9,
		"customer.merge": 10, "customer.add_tag": 11, "customer.remove_tag": 12, "customer.segment": 13,
		// —— 订单 / 售后 / 物流（14-19）——
		"order.lookup": 14, "aftersale.create": 15, "aftersale.query": 16, "logistics.track": 17,
		// —— 私信（18-20）——
		"pm.session.open": 18, "pm.session.read": 19, "pm.message.send": 20,
		// —— 跟进任务（21-22）——
		"follow_task.create": 21, "follow_task.update": 22,
		// —— 触达：核心卡片/短信/网页（23-25）——
		"reach.card.send": 23, "reach.sms.send": 24, "reach.web.send": 25,
		// —— 触达：社交/IM 渠道（26-34）——
		"reach.weixin.send": 26, "reach.wecom.send": 27, "reach.feishu.send": 28, "reach.dingtalk.send": 29,
		"reach.telegram.send": 30, "reach.whatsapp.send": 31, "reach.douyin.send": 32, "reach.kuaishou.send": 33,
		"reach.xhs.send": 34,
		// —— 触达：批量/计划/召回/健康检查/历史/模板/账号（35-41）——
		"reach.batch": 35, "reach.schedule": 36, "reach.recall": 37, "reach.health": 38,
		"reach.history": 39, "reach.template.apply": 40, "reach.account.list": 41,
	}

	// 场景一：agent 显式白名单（按白名单顺序保留已注册工具）。
	// 会话内卡片工具 card.show 作为通用能力，即便白名单未显式包含也强制注入。
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

	// 场景二：默认优先级
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
			// 未登记工具：用「100 + 输入切片下标」作确定性兜底 score，
			// 避免所有未登记工具塌成同一 score=100 导致排序随机、工具被随机丢弃。
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
			return list // 已包含
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
		return selected // 未注册，跳过
	}
	for _, t := range selected {
		if t.Name == cardShowToolName {
			return selected // 已在
		}
	}
	if len(selected) < maxTools {
		return append(selected, *cardTool)
	}
	res := make([]AgentToolDef, len(selected))
	copy(res, selected)
	res[len(res)-1] = *cardTool // 替换最低优先级工具
	return res
}

// structToMap 将结构体（或任意值）转为 map[string]any
// 用于将 ToolParameters 序列化为 OpenAI 兼容的 JSON Schema map
func structToMap(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{"type": "object"}, nil
	}
	// 先 marshal 再 unmarshal，绕过 Go 反射对 map[string]any 的限制
	bytes, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(bytes, &m); err != nil {
		return nil, err
	}
	return m, nil
}
