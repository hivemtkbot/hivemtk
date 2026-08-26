package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	confidencesvc "hivemtk-user/internal/service/confidence"
	humanizesvc "hivemtk-user/internal/service/humanize"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type AgentToolExecutor interface {
	ListTools() []AgentToolDef
	DispatchToolCalls(ctx context.Context, calls []AgentToolCall, toolCtx AgentToolContext) []AgentToolResult
}

type AgentToolPermissionChecker interface {
	SetAgentWhitelist(agentID string, tools []string)
}

type AgentToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` 
}

type AgentToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` 
}

type AgentToolContext struct {
	AgentID    string
	SessionID  string
	CustomerID string
	Source     string 
}

type AgentToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Content    string          `json:"content"` 
	Success    bool            `json:"success"`
	Card       *model.RichCard `json:"card,omitempty"` 
}

type SalesEngine struct {
	// db 字段用于 agent_loop 中的 session_messages 历史读取（OPT-ARC-03 二期：迁移到 SessionMessageRepository）
	db              *gorm.DB
	dispatcher      *llm.Dispatcher
	intent          IntentRecognizerInterface
	memory          DialogueMemoryInterface
	sop             SOPMatcherInterface
	polisher        PolisherInterface
	behaviorPl     *BehavioralPlanBuilder
	ragSearcher     RAGSearcher
	scriptLookup    ScriptLookup
	customerLookup  CustomerLookup
	playbook        PlaybookRecommenderInterface 
	feedbackLearner FeedbackRecorderInterface    

	confidenceAggregator *confidencesvc.ConfidenceAggregator

	humanizeEvaluator *humanizesvc.HumanizeEvalService

	toolExecutor AgentToolExecutor

	layerRouter *LayerRouter

	permissionChecker AgentToolPermissionChecker

	glossary   GlossaryRenderer
	calibrator OutputCalibrator
}

func (e *SalesEngine) SetPermissionChecker(pc AgentToolPermissionChecker) {
	e.permissionChecker = pc
}

type PolisherInterface interface {
	Polish(ctx context.Context, raw string, pctx *PolishContext) (string, error)
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
		behaviorPl:     NewBehavioralPlanBuilder(),
		ragSearcher:    ragSearcher,
		scriptLookup:   scriptLookup,
		customerLookup: customerLookup,
	}
}

// SetBehavioralHumanize 注入行为层拟人开关
// 关闭时使用 noop（直接返回原文本，单条消息，0 延迟）
func (e *SalesEngine) SetBehavioralHumanize(enabled bool) {
	if e.behaviorPl == nil {
		e.behaviorPl = NewBehavioralPlanBuilder()
	}
	e.behaviorPl.SetEnabled(enabled)
}

// isFirstMessageOf 判断是否首条消息（决定是否需要 thinking pause）
//
// 业界依据：WhatsApp IM 行为研究
//   - 首条消息：客户主动发起，无思考停顿
//   - 后续消息：AI 接续上文，需要 ~3s 思考停顿（让用户感觉"在思考"）
func isFirstMessageOf(req *SalesRequest) bool {
	// 简化判定：session_id 变化 OR 显式标记
	// 更精确实现需要查 session 消息历史
	return req != nil && req.IsFirstTurn
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

	if e.shouldUseParallel() {
		return e.HandleParallel(ctx, req)
	}

	start := time.Now()
	resp := &SalesResponse{
		Steps: make([]dto.SalesStepLog, 0, 9),
	}
	defer func() {
		resp.LatencyMs = int(time.Since(start).Milliseconds())

		e.recordFeedback(ctx, req, resp)

	}()

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

	// M4 I-3：clarify 意图 → 发澄清话术并结束本轮（不强选意图、不转人工、不触发 SOP）
	if intentResult.IntentType == IntentClarify {
		reply := BuildClarifyReply(intentResult)
		resp.Reply = reply
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "3.5_clarify", Status: "ok", LatencyMs: 0, Detail: reply,
		})
		return resp, nil
	}

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
			// T-2 归因闭环：所用销冠话术 script_id 结构化落 trace span extra
			Extra: map[string]any{"script_id": script.ID},
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.5_match_script", Status: "skip", LatencyMs: ms(stepStart),
		})
	}
	resp.ScriptTemplate = script

	stepStart = time.Now()
	if e.playbook != nil && intentResult != nil {

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

	stepStart = time.Now()
	candidate, llmResult, cards, err := e.generateCandidate(ctx, req, intentResult, memCtx, matchedSOP, sopStage, ragChunks, script, customer)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "6_generate_candidate", Status: "fail", Error: err.Error(),
		})

		switch {
		case script != nil && script.Content != "":
			candidate = script.Content
		case len(ragChunks) > 0 && ragChunks[0].Content != "":

			candidate = "根据知识库：\n" + truncateForReply(ragChunks[0].Content, 280)
		default:
			candidate = "您好，请问有什么可以帮您？"
		}
	} else {

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

	stepStart = time.Now()
	finalReply := candidate

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

	if e.humanizeEvaluator != nil && HumanizeEvaluatorEnabled {
		stepStart = time.Now()
		// v3 审计 P0-#4 增强：行为层拟人（打字延迟 + 分条发送）
		//   - 业界依据：文本层拟人（polisher）效果有限；行为层拟人（分条+延迟）真实感更强
		//   - 默认关闭（A/B 灰度）；通过 SetBehavioralHumanize(true) 启用
		//   - 完全独立于 humanize_polisher 文本层润色（两者正交）
		if e.behaviorPl != nil && e.behaviorPl.IsEnabled() {
			plan := e.behaviorPl.Build(finalReply, isFirstMessageOf(req))
			resp.SendPlan = &dto.SendPlanDTO{
				Messages:   plan.Messages,
				Intervals:  plan.Intervals,
				TotalDelay: plan.TotalDelay,
			}
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step:      "6.5_behavioral",
				Status:    "ok",
				LatencyMs: ms(stepStart),
				Detail:    "behavioral humanize: " + strconv.Itoa(len(plan.Messages)) + " segments",
			})
		}

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

			}

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

	startChunk := &dto.StreamChunk{
		Type:    dto.ChunkTypeStart,
		TraceID: logger.TraceIDFromContext(ctx),
		Step:    "start",
	}
	if !onChunk(startChunk) {
		return ctx.Err()
	}

	if e.layerRouter != nil {

		decision := e.layerRouter.Route(ctx, &RouteRequest{
			SessionID:   req.SessionID,
			CustomerID:  req.CustomerID,
			UserMessage: req.UserMessage,
			Intent:      nil,
			RAGChunks:   nil,
			Stage:       "",

			AgentID: agentIDFromCtx(req),
		})
		if decision != nil && decision.SkipLLM && decision.Reply != "" {

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

	reply := resp.Reply
	if reply == "" {

	} else {
		runes := []rune(reply)
		const batchSize = 4 
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

// agentLoopMaxIterations Agent Loop 最大迭代次数
// 防止 LLM 无限调用工具或陷入循环。
// 默认 5；运行期调参存数据库 system_config_kv[agent.settings].max_loop_iterations，
// 由 LoadAgentSettingsConfig 读取覆盖；也可由 SetAgentLoopMaxIterations 注入（测试/内嵌）。
// 注意：必须 >= 2。LLM 在第 1 轮返回 tool_calls 后，需要第 2 轮（携带工具结果）才能生成最终
// 文本回复；若设为 1，工具调用永远无法产出答案，会被降级为“空回复→转人工”，工具调用形同失效。
// 默认 5，兼顾多工具串联与 follow-up 问答；受 agentLoopTotalTimeout(默认180s) 约束。
var agentLoopMaxIterations = 5

// agentLoopMaxTools Agent Loop 向 LLM 注入的工具数量上限（默认优先级模式下）。
// 当 Agent 未配置 Tools 白名单时，limitToolsForAgent 按默认优先级取前 agentLoopMaxTools 个工具。
// 默认 18（覆盖原式硬编码的 10，使电商客服关键工具
// reach.card.send / reach.sms.send / aftersale.* / logistics.track 默认可见）；
// 运行期调参存数据库 system_config_kv[agent.settings].max_tools，由 LoadAgentSettingsConfig
// 读取覆盖；也可由 SetAgentLoopMaxTools 注入（测试/内嵌）。
var agentLoopMaxTools = 18

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

// agentLoopMaxTotalTokens Agent Loop 累计 token 预算（所有 LLM 调用之和）。
//
// 业界依据（OpenAI Agents SDK / LangGraph 均支持）：
//   - 仅 wall-clock 限不够：单次快但 token 多的循环（如长 system + 多 tool result）会爆 128K 上下文。
//   - 仅 max_iterations 不够：每轮 token 大小差异巨大（tool result 可能 1k-10k tokens）。
//
// 设计：默认 50000（约 12.5 轮 4k 上下文），与业界 32k-128k 区间对齐偏保守。
// 由 SetAgentLoopMaxTotalTokens 注入；<=0 时按默认值。
// 达到上限时停止后续 LLM 调用，使用最后一次成功 result 的 content 兜底。
var agentLoopMaxTotalTokens = 50000

// agentLoopMaxPerIterTimeout 单次 LLM 调用超时（per-iteration）。
//
// 业界依据：总超时 180s 情况下，单次 LLM 调用应 < 总超时 50%，否则后续工具执行
// + 结果回灌会无时间预算。默认 60s 留出 3 轮（60+60+60=180s）的安全边界。
// 由 SetAgentLoopMaxPerIterTimeout 注入；<=0 时按默认值。
var agentLoopMaxPerIterTimeout = 60 * time.Second

// SetAgentLoopMaxTotalTokens 注入 Agent Loop 累计 token 预算
// 由 main.go 启动时调用；≤0 时保持默认 50000
func SetAgentLoopMaxTotalTokens(n int) {
	if n <= 0 {
		return
	}
	agentLoopMaxTotalTokens = n
}

// SetAgentLoopMaxPerIterTimeout 注入单次 LLM 调用超时（秒）
// 由 main.go 启动时调用；≤0 时保持默认 60s
func SetAgentLoopMaxPerIterTimeout(seconds int) {
	if seconds <= 0 {
		return
	}
	agentLoopMaxPerIterTimeout = time.Duration(seconds) * time.Second
}

// ms 计算耗时（毫秒）
func ms(start time.Time) int {
	return int(time.Since(start).Milliseconds())
}

// ProcessIncomingMessage 多渠道统一入口
func (e *SalesEngine) ProcessIncomingMessage(ctx context.Context, msg *ChannelMessage) (*SalesResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}
	if msg.Content == "" && msg.MediaURL == "" {

		return &SalesResponse{}, nil
	}

	content, sessionID, customerID := e.normalizeChannelMessage(context.Background(), msg)

	req := &SalesRequest{
		SessionID:   sessionID,
		CustomerID:  customerID,
		OneID:       customerID,
		UserMessage: content,
		Platform:    msg.Channel,
		AutoExecute: true,
		Config:      DefaultSalesEngineConfig(),
	}

	if msg.Channel == "feishu" {
		req.Config.Persona = "你是飞书上的企业助手，回复简洁专业。"
	} else if msg.Channel == "telegram" {
		req.Config.Persona = "你是 Telegram 上的销售助手，语气亲切。"
	}

	return e.Handle(ctx, req)
}

// stageToJourneyStage SOP 阶段字符串 → JourneyStage
// 商业产品级：把 SOP 引擎的语义阶段映射到客户旅程的标准化阶段
// 便于话术库按客户实际所处阶段精准推荐
func stageToJourneyStage(stage string) JourneyStage {
	switch stage {
	case "churn_risk":
		return StageSleeping
	case "active":
		return StageContact
	case "default":
		return StageLead
	default:
		return StageLead
	}
}

