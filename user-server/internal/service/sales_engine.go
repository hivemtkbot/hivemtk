package service

import (
	"context"

	"fmt"

	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/llm"

	"hivemtk-user/internal/dto"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	confidencesvc "hivemtk-user/internal/service/confidence"

	humanizesvc "hivemtk-user/internal/service/humanize"
)

type AgentToolExecutor interface {
	// ListTools 返回所有可用工具的 LLM Function Calling 格式定义
	ListTools() []AgentToolDef
	// DispatchToolCalls 并发执行 LLM 返回的 tool_call 列表，返回每个调用的结果
	// toolCtx 包含 session_id / customer_id 等执行上下文
	DispatchToolCalls(ctx context.Context, calls []AgentToolCall, toolCtx AgentToolContext) []AgentToolResult
}

type AgentToolPermissionChecker interface {
	// SetAgentWhitelist 覆盖式设置某 Agent 允许使用的工具名单。
	SetAgentWhitelist(agentID string, tools []string)
}

type AgentToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

type AgentToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串参数
}

type AgentToolContext struct {
	AgentID    string
	SessionID  string
	CustomerID string
	Source     string // agent / sop / manual / api
}

type AgentToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Content    string          `json:"content"` // 工具结果 JSON 字符串
	Success    bool            `json:"success"`
	Card       *model.RichCard `json:"card,omitempty"` // 工具产出的结构化富卡片
}

type SalesEngine struct {
	db              *gorm.DB
	dispatcher      *llm.Dispatcher
	intent          IntentRecognizerInterface
	memory          DialogueMemoryInterface
	sop             SOPMatcherInterface
	polisher        PolisherInterface
	ragSearcher     RAGSearcher
	scriptLookup    ScriptLookup
	customerLookup  CustomerLookup
	playbook        PlaybookRecommenderInterface // 销冠话术库（可选注入）
	feedbackLearner FeedbackRecorderInterface    // 反馈学习器（可选注入，形成 AI 自我进化闭环）

	// 置信度聚合器（可选注入）
	// 注入后 shouldTransferToHuman 改为基于 5 维信号 + 动态阈值决策
	confidenceAggregator *confidencesvc.ConfidenceAggregator

	// 拟人度评估器（可选注入）
	// 注入后在 Step 7.5 评估回复自然度，<0.85 触发重生成（最多 3 次）
	humanizeEvaluator *humanizesvc.HumanizeEvalService

	// 智能体 Agent Loop（真正的智能体，不做流程编排）
	// 注入后 Step 6 改为：LLM ↔ 工具 循环，LLM 决定调用哪些工具 → 执行 → 回灌结果 → 再生成，
	// 直到 LLM 给出最终回复（finish_reason=stop）或达到最大迭代次数。
	// 未注入时维持原 9 步流水线行为（向后兼容）。
	// 通过接口注入以避免 service ↔ tooluse 循环依赖（依赖倒置原则）
	toolExecutor AgentToolExecutor

	// 双层架构 LayerRouter（可选注入, /）
	// 注入后 Step 6 (generateCandidate) 顶部先做 Layer1 FAQ/SOP 路由:
	//   - 命中且 conf >= 0.6 -> SkipLLM, 用 FAQ 答案或 SOP 模板回复
	//   - 命中但 conf <  0.6 -> 走 Layer2 LLM
	//   - FAQ/SOP 均未命中   -> 走 Layer2 LLM
	// 未注入时维持原 LLM 单一路径 (向后兼容)
	layerRouter *LayerRouter

	// permissionChecker 工具执行期权限检查器（可选注入，依赖倒置避免 service↔tooluse 循环依赖）。
	// 注入后 runAgentLoop 每轮执行前按 Agent 白名单（AgentContext.Tools）覆盖式设置执行期放行名单，
	// 与 limitToolsForAgent 的“注入期过滤”形成双层防护。同一全局 *tooluse.WhitelistPermissionChecker
	// 单例由 router 注入，管理 API（/api/agent/tools/permission/agents/:agent_id）亦可设置。
	permissionChecker AgentToolPermissionChecker

	// 回复语言链路（可选注入）：术语表渲染器 + 输出后置校准器。
	// 注入后 generateCandidate / runAgentLoop 的跨语言路径会追加语种指令与术语表块，
	// 并对 LLM 输出做术语校准与敏感模式保护（依赖倒置，由 service/translation.GlossaryService 适配）。
	// 与 ragcustomerservice 同源逻辑，使主力 AI 客服对话也按客户语言回复。
	glossary   GlossaryRenderer
	calibrator OutputCalibrator
}

func (e *SalesEngine) SetPermissionChecker(pc AgentToolPermissionChecker) {
	e.permissionChecker = pc
}

type RAGSearcher interface {
	Search(ctx context.Context, query string, topK int) ([]RAGChunk, error)
}

type RAGChunk = dto.RAGChunk

type ScriptLookup interface {
	MatchScript(ctx context.Context, intent string, scenario string) (*ScriptTemplate, error)
}

type ScriptTemplate = dto.ScriptTemplate

type CustomerLookup interface {
	GetByOneID(ctx context.Context, oneID string) (*model.Customer, error)
	GetByID(ctx context.Context, id string) (*model.Customer, error)
}

type IntentRecognizerInterface interface {
	Recognize(ctx context.Context, sessionID, customerID, text string) (*dto.RecognizeResult, error)
}

type DialogueMemoryInterface interface {
	AppendMessage(ctx context.Context, sessionID, customerID string, msg dto.Message) error
	GetOrCreateMemory(ctx context.Context, sessionID, customerID string) (*model.DialogueMemory, error)
}

type SOPMatcherInterface interface {
	MatchByIntent(ctx context.Context, intentType string) ([]model.SOPAgent, error)
}

type PolisherInterface interface {
	Polish(ctx context.Context, raw string, pctx *PolishContext) (string, error)
}

type PlaybookRecommenderInterface interface {
	RecommendForResponse(ctx context.Context, industry Industry, productID string, stage JourneyStage, intent string) []*PlaybookEntry
}

type FeedbackRecorderInterface interface {
	RecordFeedback(ctx context.Context, record *FeedbackRecord) error
}

type SalesEngineConfig = dto.SalesEngineConfig

func DefaultSalesEngineConfig() *SalesEngineConfig {
	return dto.DefaultSalesEngineConfig()
}

func NewSalesEngine(

	db *gorm.DB,

	dispatcher *llm.Dispatcher,

	intent IntentRecognizerInterface,

	memory DialogueMemoryInterface,

	sop SOPMatcherInterface,

	ragSearcher RAGSearcher,

	scriptLookup ScriptLookup,

	customerLookup CustomerLookup,

) *SalesEngine {
	polisher := NewHumanizePolisher()
	return &SalesEngine{
		db:             db,
		dispatcher:     dispatcher,
		intent:         intent,
		memory:         memory,
		sop:            sop,
		polisher:       polisher,
		ragSearcher:    ragSearcher,
		scriptLookup:   scriptLookup,
		customerLookup: customerLookup,
	}
}

func (e *SalesEngine) SetFeedbackLearner(ctx context.Context, fl FeedbackRecorderInterface) {
	e.feedbackLearner = fl
}

func (e *SalesEngine) SetConfidenceAggregator(ctx context.Context, agg *confidencesvc.ConfidenceAggregator) {
	e.confidenceAggregator = agg
}

func (e *SalesEngine) SetHumanizeEvaluator(ctx context.Context, ev *humanizesvc.HumanizeEvalService) {
	e.humanizeEvaluator = ev
}

func (e *SalesEngine) SetToolExecutor(ctx context.Context, exec AgentToolExecutor) {
	e.toolExecutor = exec
}

func (e *SalesEngine) SetLayerRouter(ctx context.Context, lr *LayerRouter) {
	e.layerRouter = lr
}

type SalesRequest = dto.SalesRequest

func agentIDFromCtx(req *SalesRequest) uint {
	if req == nil || req.AgentContext == nil {
		return 0
	}
	return req.AgentContext.AgentID
}

type SalesResponse = dto.SalesResponse

func (e *SalesEngine) Handle(ctx context.Context, req *SalesRequest) (*SalesResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserMessage == "" {
		return nil, fmt.Errorf("user_message is empty")
	}
	if req.Config == nil {
		req.Config = DefaultSalesEngineConfig()
	}

	// 并行化开关 (FeatureFlag 灰度)
	// 启用时走 5 阶段并行版本; 关闭时走原 9 步串行 (向后兼容)
	if e.shouldUseParallel() {
		return e.HandleParallel(ctx, req)
	}

	start := time.Now()
	resp := &SalesResponse{
		Steps: make([]dto.SalesStepLog, 0, 9),
	}
	defer func() {
		resp.LatencyMs = int(time.Since(start).Milliseconds())
		// 第 8 步：反馈学习（AI 自我进化闭环）
		// 不论本次是否转人工，都记录决策快照，
		// 后续客户下一条消息/人工接管时由 SmartCSOrchestrator 更新 CustomerAccept
		e.recordFeedback(ctx, req, resp)
		// 私域: 无 Prometheus 端点, 指标已落库 (layer_decision_logs)
	}()

	// 步骤 1：消息解析 + OneID 识别
	stepStart := time.Now()
	customer, err := e.resolveCustomer(ctx, req)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "1_resolve_customer", Status: "fail", Error: err.Error(),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "1_resolve_customer", Status: "ok", LatencyMs: ms(stepStart),
			Detail: fmt.Sprintf("customer_id=%s", safeID(customer)),
		})
	}

	// 步骤 2：短期/长期记忆召回
	stepStart = time.Now()
	memCtx, err := e.recallMemory(ctx, req)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "2_recall_memory", Status: "fail", Error: err.Error(),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "2_recall_memory", Status: "ok", LatencyMs: ms(stepStart),
			Extra: map[string]any{"key_facts": memCtx.KeyFacts},
		})
	}
	resp.Memory = DialogueMemoryToDTO(memCtx)

	// 步骤 3：意图识别（intent 注入为 nil 时使用规则兜底）
	stepStart = time.Now()
	var intentResult *dto.RecognizeResult
	if e.intent != nil {
		intentResult, err = e.intent.Recognize(ctx, req.SessionID, req.CustomerID, req.UserMessage)
		if err != nil {
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "3_recognize_intent", Status: "fail", Error: err.Error(),
			})
			intentResult = &dto.RecognizeResult{IntentType: IntentUnknown, Confidence: 0.3}
		} else {
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "3_recognize_intent", Status: "ok", LatencyMs: ms(stepStart),
				Detail: fmt.Sprintf("%s (conf=%.2f)", intentResult.IntentType, intentResult.Confidence),
			})
		}
	} else {
		intentResult = &dto.RecognizeResult{IntentType: IntentUnknown, Confidence: 0.3, Method: "fallback"}
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "3_recognize_intent", Status: "skip", LatencyMs: ms(stepStart),
			Detail: "intent_recognizer not injected",
		})
	}
	resp.Intent = intentResult

	// 步骤 3.5：判断是否转人工
	transfer, reason := e.shouldTransferToHuman(ctx, intentResult, memCtx, req)
	if transfer {
		resp.TransferredToHuman = true
		resp.TransferReason = reason
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "3.5_transfer_check", Status: "ok",
			Detail: "transferred: " + resp.TransferReason,
		})
		resp.Reply = "[系统自动转人工] " + resp.TransferReason
		return resp, nil
	}

	// 步骤 4：SOP 匹配
	stepStart = time.Now()
	matchedSOP, sopStage, err := e.matchSOP(ctx, intentResult, customer)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "4_match_sop", Status: "fail", Error: err.Error(),
		})
	} else if matchedSOP != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "4_match_sop", Status: "ok", LatencyMs: ms(stepStart),
			Detail: fmt.Sprintf("sop=%s stage=%s", matchedSOP.Name, sopStage),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "4_match_sop", Status: "skip", LatencyMs: ms(stepStart),
		})
	}
	resp.MatchedSOP = SOPAgentToDTO(matchedSOP)

	// 步骤 5：RAG 召回
	stepStart = time.Now()
	ragChunks, err := e.recallRAG(ctx, req, intentResult)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5_recall_rag", Status: "fail", Error: err.Error(),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5_recall_rag", Status: "ok", LatencyMs: ms(stepStart),
			Extra: map[string]any{"chunk_count": len(ragChunks)},
		})
	}
	resp.RAGChunks = ragChunks

	// 步骤 5.5：话术库匹配（可选）
	stepStart = time.Now()
	script, err := e.matchScript(ctx, intentResult)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.5_match_script", Status: "fail", Error: err.Error(),
		})
	} else if script != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.5_match_script", Status: "ok", LatencyMs: ms(stepStart),
			Detail: fmt.Sprintf("script_id=%s rate=%.2f", script.ID, script.MatchRate),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.5_match_script", Status: "skip", LatencyMs: ms(stepStart),
		})
	}
	resp.ScriptTemplate = script

	// 步骤 5.6：销冠话术库推荐（按客户行业+阶段+意图推荐 3-5 条话术）
	// 商业产品级：销售辅助模式下，销售可一键采用最匹配话术
	stepStart = time.Now()
	if e.playbook != nil && intentResult != nil {
		// 推断行业（默认通用，留待接入客户档案时替换为真实行业）
		industry := Industry("")
		productID := ""
		stage := stageToJourneyStage(sopStage)
		resp.PlaybookSuggestions = e.fetchPlaybookSuggestions(context.Background(), industry, productID, stage, intentResult.IntentType)
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.6_playbook_suggest", Status: "ok", LatencyMs: ms(stepStart),
			Detail: fmt.Sprintf("count=%d stage=%s intent=%s", len(resp.PlaybookSuggestions), stage, intentResult.IntentType),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.6_playbook_suggest", Status: "skip", LatencyMs: ms(stepStart),
		})
	}

	// 步骤 6：LLM 生成候选回复
	stepStart = time.Now()
	candidate, llmResult, cards, err := e.generateCandidate(ctx, req, intentResult, memCtx, matchedSOP, sopStage, ragChunks, script, customer)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "6_generate_candidate", Status: "fail", Error: err.Error(),
		})
		// LLM 失败时按优先级兜底：
		//   1) 话术模板（script）
		//   2) RAG 检索顶部 chunk（ragChunks[0]）
		//   3) 终极兜底：默认问候语
		switch {
		case script != nil && script.Content != "":
			candidate = script.Content
		case len(ragChunks) > 0 && ragChunks[0].Content != "":
			// 直接使用 RAG 顶部检索内容作为回复（带轻微包装）
			candidate = "根据知识库：\n" + truncateForReply(ragChunks[0].Content, 280)
		default:
			candidate = "您好，请问有什么可以帮您？"
		}
	} else {
		// generateCandidate 可能返回 (candidate, nil, nil)（如走 script/rag 兜底）
		// 此时 llmResult 为 nil，不能直接访问其字段，否则 panic
		if llmResult != nil {
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "6_generate_candidate", Status: "ok", LatencyMs: ms(stepStart),
				Extra: map[string]any{
					"provider": llmResult.Provider, "model": llmResult.Model,
					"tokens": llmResult.TotalTokens, "cost": llmResult.Cost,
				},
			})
		} else {
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "6_generate_candidate", Status: "ok", LatencyMs: ms(stepStart),
				Extra: map[string]any{
					"provider": "", "model": "",
					"tokens": 0, "cost": float64(0),
					"note": "candidate from script/rag fallback (no LLM call)",
				},
			})
		}
	}
	if llmResult != nil {
		resp.LLMProvider = llmResult.Provider
		resp.LLMModel = llmResult.Model
		resp.CostTokens = llmResult.TotalTokens
	}

	// 步骤 7：拟人润色
	stepStart = time.Now()
	finalReply := candidate
	// 会话内结构化卡片随最终回复一并下发（来自 agent 工具产出）
	resp.Cards = RichCardsToDTO(cards)
	if req.Config.EnableHumanizePolish {
		polished, err := e.polisher.Polish(ctx, candidate, &PolishContext{
			Persona:      req.Config.Persona,
			Intent:       intentResult.IntentType,
			Platform:     req.Platform,
			Stage:        sopStage,
			CustomerName: customerNameOf(customer),
		})
		if err != nil {
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "7_polish", Status: "fail", Error: err.Error(),
			})
		} else {
			finalReply = polished
			resp.Polished = true
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "7_polish", Status: "ok", LatencyMs: ms(stepStart),
			})
		}
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "7_polish", Status: "skip", LatencyMs: ms(stepStart),
		})
	}

	// 步骤 7.5：拟人度评估
	// 注入 HumanizeEvalService 时启用，<0.85 触发重生成（最多 3 次）
	// 私域本地 LLM 部署下由 HumanizeEvaluatorEnabled 开关跳过本步骤（避免 1.5B q4 模型被 0.85 阈值反复打回）
	if e.humanizeEvaluator != nil && HumanizeEvaluatorEnabled {
		stepStart = time.Now()
		evalInput := &dto.HumanizeEvalInput{
			SessionID:       req.SessionID,
			CustomerID:      req.CustomerID,
			CustomerMessage: req.UserMessage,
			AIReply:         finalReply,
			Persona:         req.Config.Persona,
			Intent:          intentResult.IntentType,
			Platform:        req.Platform,
		}
		evalResult, err := e.humanizeEvaluator.Evaluate(ctx, evalInput)
		if err != nil {
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "7.5_humanize_eval", Status: "fail", LatencyMs: ms(stepStart),
				Error: err.Error(),
			})
		} else if evalResult != nil {
			resp.HumanizeScore = evalResult.TotalScore
			resp.HumanizePassed = evalResult.Passed
			resp.HumanizeAttempt = evalResult.AttemptCount
			if !evalResult.Passed {
				// 拟人度未达标：保留 AI 回复并正常下发，不再因“不够拟人”而丢弃回复、转人工。
				// 设计意图（humanize_init.go 背景说明）：私域/本地 LLM 部署下 LLM 推理成功即应
				// 自动回复，由真实人工按需接管；若直接转人工且无在线客服，访客将收不到任何回复，
				// 这与“客服对话必须有返回”的预期相悖。因此此处仅记录评分、保留最优回复并继续
				// 走下发流程，绝不吞掉已生成的有效回复。
				resp.HumanizeScore = evalResult.TotalScore
				resp.HumanizePassed = false
				resp.HumanizeAttempt = evalResult.AttemptCount
				if evalResult.FinalReply != "" {
					finalReply = evalResult.FinalReply
				}
				resp.Steps = append(resp.Steps, dto.SalesStepLog{
					Step: "7.5_humanize_eval", Status: "fail_soft", LatencyMs: ms(stepStart),
					Detail: fmt.Sprintf("拟人度未达标（%.2f < 0.85），保留 AI 回复下发（不转人工）", evalResult.TotalScore),
				})
				// 注意：不设置 TransferredToHuman，继续走审核/下发流程
			}
			// 通过评估但可能替换了 finalReply（重生成后达标）
			if evalResult.FinalReply != "" && evalResult.FinalReply != finalReply {
				finalReply = evalResult.FinalReply
			}
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "7.5_humanize_eval", Status: "ok", LatencyMs: ms(stepStart),
				Extra: map[string]any{
					"score":    evalResult.TotalScore,
					"strategy": evalResult.SampleStrategy,
					"attempts": evalResult.AttemptCount,
				},
			})
		}
	}

	resp.Reply = finalReply

	// 记录到对话记忆
	if e.memory != nil {
		_ = e.memory.AppendMessage(ctx, req.SessionID, req.CustomerID, dto.Message{
			Role:      "ai",
			Content:   finalReply,
			Timestamp: time.Now(),
		})
	}

	return resp, nil
}

type StreamChunkCallback = func(chunk *dto.StreamChunk) bool

func (e *SalesEngine) HandleStream(ctx context.Context, req *SalesRequest, onChunk StreamChunkCallback) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if req.UserMessage == "" {
		return fmt.Errorf("user_message is empty")
	}
	if onChunk == nil {
		return fmt.Errorf("onChunk callback is nil")
	}
	if req.Config == nil {
		req.Config = DefaultSalesEngineConfig()
	}

	// 1) start chunk：通知客户端 trace_id / 启动
	startChunk := &dto.StreamChunk{
		Type:    dto.ChunkTypeStart,
		TraceID: logger.TraceIDFromContext(ctx),
		Step:    "start",
	}
	if !onChunk(startChunk) {
		return ctx.Err()
	}

	// 2) : 真正流式 - 先做 Layer1 路由, 命中立即推 delta + final
	// 避免走 5 阶段并行 + LLM 推理导致 LCP 19.6s
	if e.layerRouter != nil {
		// 浅意图识别 (规则级, < 5ms) 走 Speculative 不可用回退到 nil
		// LayerRouter 内部已经处理 nil intent, 不依赖这里先做 intent
		decision := e.layerRouter.Route(ctx, &RouteRequest{
			SessionID:   req.SessionID,
			CustomerID:  req.CustomerID,
			UserMessage: req.UserMessage,
			Intent:      nil, // Stream 路径不阻塞 intent, 让 LayerRouter 内部 FAQ/SOP 走 keyword
			RAGChunks:   nil,
			Stage:       "",
			// 传 agentID 实现按智能体隔离的 FAQ/SOP 匹配
			AgentID: agentIDFromCtx(req),
		})
		if decision != nil && decision.SkipLLM && decision.Reply != "" {
			// Layer1 命中 -> 立即推 delta (完整 reply) + final
			// 整个 LCP < 100ms, 客户端秒收回复
			_ = onChunk(&dto.StreamChunk{
				Type:    dto.ChunkTypeDelta,
				TraceID: startChunk.TraceID,
				Text:    decision.Reply,
				Layer:   dto.Layer1,
				Step:    "layer1_delta",
			})
			finalChunk := &dto.StreamChunk{
				Type:    dto.ChunkTypeFinal,
				TraceID: startChunk.TraceID,
				Text:    decision.Reply,
				Layer:   dto.Layer1,
				Step:    "layer1_final",
				WallMs:  decision.WallMs,
				Steps: []dto.SalesStepLog{{
					Step: "layer1_fastpath", Status: "ok", LatencyMs: decision.WallMs,
					Detail: fmt.Sprintf("layer1 hit (reason=%s faq_id=%d sop_id=%d)",
						decision.Reason, decision.FAQID, decision.SOPID),
				}},
				Metadata: fmt.Sprintf(`{"layer1_reason":"%s","faq_id":%d,"sop_id":%d}`,
					decision.Reason, decision.FAQID, decision.SOPID),
			}
			if !onChunk(finalChunk) {
				return ctx.Err()
			}
			return nil
		}
	}

	// 3) Layer2 路径: 先推 placeholder (LCP < 100ms), 再调 Handle() 拿最终结果
	placeholder := "正在为您查询…"
	_ = onChunk(&dto.StreamChunk{
		Type:    dto.ChunkTypeDelta,
		TraceID: startChunk.TraceID,
		Text:    placeholder,
		Layer:   dto.Layer2,
		Step:    "placeholder",
	})

	resp, err := e.Handle(ctx, req)
	if err != nil {
		_ = onChunk(&dto.StreamChunk{
			Type:    dto.ChunkTypeError,
			TraceID: startChunk.TraceID,
			Error:   err.Error(),
		})
		return fmt.Errorf("handle stream: %w", err)
	}

	// 4) Layer2: 把 Handle 拿到的回复按字符切片"模拟"流式（直到 LLM Dispatcher 切到真流式）
	// 注意: 已保证 start + placeholder 在 < 100ms 抵达, 切片延迟只影响后续体感
	reply := resp.Reply
	if reply == "" {
		// 无文本（可能已转人工）— 直接发 final
	} else {
		runes := []rune(reply)
		const batchSize = 4 // Layer2 路径可稍大 (4 字符), 减少 chunk 数
		interval := 15 * time.Millisecond
		for i := 0; i < len(runes); i += batchSize {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			end := i + batchSize
			if end > len(runes) {
				end = len(runes)
			}
			delta := &dto.StreamChunk{
				Type:    dto.ChunkTypeDelta,
				TraceID: startChunk.TraceID,
				Text:    string(runes[i:end]),
				Layer:   dto.Layer2,
				Step:    "layer2_delta",
			}
			if !onChunk(delta) {
				return ctx.Err()
			}
			if end < len(runes) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(interval):
				}
			}
		}
	}

	// 5) final chunk
	finalChunk := &dto.StreamChunk{
		Type:    dto.ChunkTypeFinal,
		TraceID: startChunk.TraceID,
		Text:    resp.Reply,
		Steps:   resp.Steps,
		WallMs:  resp.LatencyMs,
		Model:   resp.LLMModel,
		Tokens:  resp.CostTokens,
		Layer:   dto.Layer2,
		Step:    "layer2_final",
	}
	if resp.TransferredToHuman {
		finalChunk.Metadata = `{"transferred_to_human":true,"reason":"` + resp.TransferReason + `"}`
	}
	if !onChunk(finalChunk) {
		return ctx.Err()
	}
	return nil
}

func layerOfResponse(resp *SalesResponse) string {
	if resp == nil {
		return dto.Layer2
	}
	if resp.Confidence != nil && resp.Confidence.DecisionBand != "" {
		// handoff/llm_fallback/review/auto 都映射到 layer2 (单 LLM 路径)
		return dto.Layer2
	}
	return dto.Layer2
}

func (e *SalesEngine) resolveCustomer(ctx context.Context, req *SalesRequest) (*model.Customer, error) {
	if e.customerLookup == nil {
		return nil, fmt.Errorf("customer_lookup is nil")
	}
	if req.OneID != "" {
		c, err := e.customerLookup.GetByOneID(ctx, req.OneID)
		if err == nil && c != nil {
			return c, nil
		}
	}
	if req.CustomerID != "" {
		return e.customerLookup.GetByID(ctx, req.CustomerID)
	}
	return nil, fmt.Errorf("no customer identifier provided")
}

func (e *SalesEngine) recallMemory(ctx context.Context, req *SalesRequest) (*model.DialogueMemory, error) {
	if e.memory == nil {
		return &model.DialogueMemory{}, nil
	}
	return e.memory.GetOrCreateMemory(ctx, req.SessionID, req.CustomerID)
}
