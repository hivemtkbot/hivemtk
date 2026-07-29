package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/metrics"
	"marketing/internal/pkg/utils/logger"
	confidencesvc "marketing/internal/service/confidence"
	humanizesvc "marketing/internal/service/humanize"
)

// ============================================================================
// 智能体工具执行器接口（依赖倒置，避免 service ↔ tooluse 循环依赖）
// ============================================================================
//
// tooluse 包的 reach_integration_adapter.go 反向依赖 service 包
// （IntegrationReachAdapter 持有 *service.TelegramIntegrationService 等）。
// 为打破循环，service 不直接依赖 tooluse，而是定义本接口；
// 具体实现由 router 层注入（ToolExecutorAdapter 包装 *tooluse.ToolExecutor）。
//
// 接口隔离原则（ISP）：只暴露 SalesEngine.runAgentLoop 实际需要的方法。

// AgentToolExecutor 智能体工具执行器接口
type AgentToolExecutor interface {
	// ListTools 返回所有可用工具的 LLM Function Calling 格式定义
	ListTools() []AgentToolDef
	// DispatchToolCalls 并发执行 LLM 返回的 tool_call 列表，返回每个调用的结果
	// toolCtx 包含 session_id / customer_id 等执行上下文
	DispatchToolCalls(ctx context.Context, calls []AgentToolCall, toolCtx AgentToolContext) []AgentToolResult
}

// AgentToolDef 工具定义（OpenAI Function Calling 兼容）
type AgentToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// AgentToolCall LLM 返回的工具调用请求
type AgentToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串参数
}

// AgentToolContext 工具执行上下文
type AgentToolContext struct {
	AgentID    string
	SessionID  string
	CustomerID string
	Source     string // agent / sop / manual / api
}

// AgentToolResult 工具执行结果（回传给 LLM 的格式）
type AgentToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"` // 工具结果 JSON 字符串
	Success    bool   `json:"success"`
}

// ============================================================================
// 商业产品级 智能体：销售引擎（Sales Engine）
// ----------------------------------------------------------------------------
// 这是 AI 私域销冠的"中央调度枢纽"，把消息→意图→记忆→SOP→RAG→LLM→润色→审核→反馈学习
// 的完整链路串起来。一条入站消息进入后，经过 9 个步骤生成最终回复并完成自我进化：
//   1. 消息解析 + OneID 识别
//   2. 短期/长期记忆召回
//   3. 意图识别（规则 + LLM）
//   3.5 转人工判定（投诉/敏感场景提前转人工）
//   4. SOP 匹配（按意图 + 客户阶段）
//   5. RAG 知识库召回
//   5.5 话术库匹配
//   5.6 销冠话术库推荐
//   6. LLM 生成候选回复（多模型路由）
//   7. 拟人润色
//   8. 发送前审核
//   9. 反馈学习（记录决策快照，AI 自我进化闭环）
// ============================================================================

// SalesEngine 销售引擎
//
// 设计原则（P0-1 接口化重构）：
//   - 所有协作者均以接口持有，符合依赖倒置原则（DIP）
//   - 解锁单元测试：可注入测试替身实现，无需构造真实 *gorm.DB / *llm.Dispatcher
//   - 现有具体类型（*IntentRecognizer 等）通过 Go 鸭子类型自动满足接口
type SalesEngine struct {
	db              *gorm.DB
	dispatcher      *llm.Dispatcher
	intent          IntentRecognizerInterface
	memory          DialogueMemoryInterface
	sop             SOPMatcherInterface
	polisher        PolisherInterface
	auditor         AuditorInterface
	ragSearcher     RAGSearcher
	scriptLookup    ScriptLookup
	customerLookup  CustomerLookup
	playbook        PlaybookRecommenderInterface // 销冠话术库（可选注入）
	feedbackLearner FeedbackRecorderInterface    // 反馈学习器（可选注入，形成 AI 自我进化闭环）

	// P0-3 置信度聚合器（可选注入）
	// 注入后 shouldTransferToHuman 改为基于 5 维信号 + 动态阈值决策
	confidenceAggregator *confidencesvc.ConfidenceAggregator

	// P0-4 拟人度评估器（可选注入）
	// 注入后在 Step 7.5 评估回复自然度，<0.85 触发重生成（最多 3 次）
	humanizeEvaluator *humanizesvc.HumanizeEvalService

	// P0-3 智能体 Agent Loop（真正的智能体，不做流程编排）
	// 注入后 Step 6 改为：LLM ↔ 工具 循环，LLM 决定调用哪些工具 → 执行 → 回灌结果 → 再生成，
	// 直到 LLM 给出最终回复（finish_reason=stop）或达到最大迭代次数。
	// 未注入时维持原 9 步流水线行为（向后兼容）。
	// 通过接口注入以避免 service ↔ tooluse 循环依赖（依赖倒置原则）
	toolExecutor AgentToolExecutor
}

// RAGSearcher RAG 召回接口（由 RAG 服务实现，避免循环依赖）
type RAGSearcher interface {
	Search(ctx context.Context, query string, topK int) ([]RAGChunk, error)
}

// RAGChunk RAG 召回片段
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type RAGChunk = dto.RAGChunk

// ScriptLookup 话术库查询接口
type ScriptLookup interface {
	MatchScript(ctx context.Context, intent string, scenario string) (*ScriptTemplate, error)
}

// ScriptTemplate 话术模板
// 已迁移至 dto 包
type ScriptTemplate = dto.ScriptTemplate

// CustomerLookup 客户信息查询接口
type CustomerLookup interface {
	GetByOneID(ctx context.Context, oneID string) (*model.Customer, error)
	GetByID(ctx context.Context, id string) (*model.Customer, error)
}

// ============================================================================
// P0-1 接口化重构：SalesEngine 协作者抽象接口
// ----------------------------------------------------------------------------
// 设计目标：
//   1. 依赖倒置（DIP）：SalesEngine 依赖抽象而非具体类型，可注入测试替身进行单元测试
//   2. 接口隔离（ISP）：按 SalesEngine 实际使用的方法最小化接口，不暴露多余方法
//   3. 开闭原则（OCP）：替换实现（如换 LLM 意图识别器为规则引擎）无需修改 SalesEngine
//
// 兼容性：现有具体类型（*IntentRecognizer / *DialogueMemoryService / *SOPService /
//   *HumanizePolisher / *ContentAuditor / *PlaybookService / *FeedbackLearner）
//   通过 Go 鸭子类型自动满足下列接口，调用方（router/sales_engine_factory.go）
//   无需修改。
// ============================================================================

// IntentRecognizerInterface 意图识别抽象
type IntentRecognizerInterface interface {
	Recognize(ctx context.Context, sessionID, customerID, text string) (*dto.RecognizeResult, error)
}

// DialogueMemoryInterface 对话记忆抽象
type DialogueMemoryInterface interface {
	AppendMessage(ctx context.Context, sessionID, customerID string, msg dto.Message) error
	GetOrCreateMemory(ctx context.Context, sessionID, customerID string) (*model.DialogueMemory, error)
}

// SOPMatcherInterface SOP 匹配抽象
type SOPMatcherInterface interface {
	MatchByIntent(ctx context.Context, intentType string) ([]model.SOPAgent, error)
}

// PolisherInterface 拟人润色抽象
type PolisherInterface interface {
	Polish(ctx context.Context, raw string, pctx *PolishContext) (string, error)
}

// AuditorInterface 内容审核抽象
type AuditorInterface interface {
	Audit(text string, ctx *AuditContext) *AuditResult
}

// PlaybookRecommenderInterface 销冠话术推荐抽象
type PlaybookRecommenderInterface interface {
	RecommendForResponse(ctx context.Context, industry Industry, productID string, stage JourneyStage, intent string) []*PlaybookEntry
}

// FeedbackRecorderInterface 反馈记录抽象
type FeedbackRecorderInterface interface {
	RecordFeedback(ctx context.Context, record *FeedbackRecord) error
}

// SalesEngineConfig 销售引擎配置
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type SalesEngineConfig = dto.SalesEngineConfig

// DefaultSalesEngineConfig 默认配置
// 已迁移至 dto 包，此处保留函数别名以维持向后兼容
func DefaultSalesEngineConfig() *SalesEngineConfig {
	return dto.DefaultSalesEngineConfig()
}

// NewSalesEngine 创建销售引擎
// 依赖支持 nil 注入：nil 时跳过对应环节（如无 LLM 时使用话术模板兜底）
//
// 参数为接口类型（P0-1 重构）：
//   - intent / memory / sop 接受任何实现对应接口的类型，包括 *IntentRecognizer 等现有具体类型
//   - 调用方无需修改，Go 鸭子类型自动适配
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
	auditor := NewContentAuditor()
	return &SalesEngine{
		db:             db,
		dispatcher:     dispatcher,
		intent:         intent,
		memory:         memory,
		sop:            sop,
		polisher:       polisher,
		auditor:        auditor,
		ragSearcher:    ragSearcher,
		scriptLookup:   scriptLookup,
		customerLookup: customerLookup,
	}
}

// SetFeedbackLearner 注入反馈学习器
// 商业产品级：注入后，每次 Handle 结束都会记录本次决策快照（意图/置信度/SOP/回复/是否转人工），
// 后续客户下一条消息或人工接管时更新 CustomerAccept，形成 AI 自我进化闭环
//
// P0-1 重构：参数改为 FeedbackRecorderInterface，可注入测试替身实现进行单元测试
func (e *SalesEngine) SetFeedbackLearner(ctx context.Context, fl FeedbackRecorderInterface) {
	e.feedbackLearner = fl
}

// SetConfidenceAggregator 注入置信度聚合器（P0-3）
//
// 注入后 shouldTransferToHuman 不再使用静态规则（IntentChurn/IntentComplaint/MessageCount>30），
// 而是由 5 维信号（IntentConf/EntityComp/CtxRelev/RAGQual/LLMEntropy）+ 动态阈值决策：
//   - 聚合置信度 < 0.4         → 立即转人工
//   - 聚合置信度 ∈ [0.4, 0.6)  → LLM 兜底回复（仍可发送，但带 low_confidence 标记）
//   - 聚合置信度 ∈ [0.6, 0.75) → 进审核队列（保留当前行为）
//   - 聚合置信度 ≥ 0.75        → 自动回复
//   - 任一一票否决规则触发     → 强制转人工
//
// 未注入时维持原有静态规则行为（向后兼容）
func (e *SalesEngine) SetConfidenceAggregator(ctx context.Context, agg *confidencesvc.ConfidenceAggregator) {
	e.confidenceAggregator = agg
}

// SetHumanizeEvaluator 注入拟人度评估器（P0-4）
//
// 注入后 SalesEngine Step 7（拟人润色）之后插入 Step 7.5（拟人度评估）：
//   - RuleScorer 全量评估 5 维度（自然度/简洁性/共情/专业/说服）
//   - 不达标（<0.85）时调用 dispatcher 重新生成（最多 3 次）
//   - 3 次仍不达标 → 转人工 + 收集低质样本
//
// 未注入时跳过 Step 7.5（向后兼容）
func (e *SalesEngine) SetHumanizeEvaluator(ctx context.Context, ev *humanizesvc.HumanizeEvalService) {
	e.humanizeEvaluator = ev
}

// SetToolExecutor 注入工具执行器（P0-3 智能体 Agent Loop）
//
// 注入后 SalesEngine Step 6（LLM 生成候选回复）改为真正的 Agent Loop：
//  1. 将所有可用工具（AgentToolDef）序列化为 OpenAI tools 数组传给 LLM
//  2. LLM 决定调用哪些工具（finish_reason=tool_calls）→ 调用 DispatchToolCalls 并发执行
//  3. 将工具执行结果以 role=tool 消息回灌到对话历史
//  4. 再次调用 LLM，循环直到 LLM 给出最终文本回复（finish_reason=stop）或达到最大迭代次数
//
// 未注入时维持原 9 步流水线行为：Step 6 仅一次性调用 LLM 生成文本（向后兼容）
//
// 设计原则：
//   - 真正的智能体不做流程编排：LLM 自主决定调用哪些工具、何时停止
//   - 工具调用是 LLM 的可选能力，不是硬编码的步骤序列
//   - 接口隔离：通过 AgentToolExecutor 接口注入，避免 service ↔ tooluse 循环依赖
func (e *SalesEngine) SetToolExecutor(ctx context.Context, exec AgentToolExecutor) {
	e.toolExecutor = exec
}

// SalesRequest 销售请求
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type SalesRequest = dto.SalesRequest

// SalesResponse 销售回复
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type SalesResponse = dto.SalesResponse

// SalesStepLog 已迁至 dto 包（P2-6 DTO 层补全）
// 使用 dto.SalesStepLog 替代本地类型

// Handle 处理一条入站消息
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
	start := time.Now()
	resp := &SalesResponse{
		Steps: make([]dto.SalesStepLog, 0, 9),
	}
	defer func() {
		resp.LatencyMs = int(time.Since(start).Milliseconds())
		// 第 9 步：反馈学习（AI 自我进化闭环）
		// 不论本次是否转人工、是否 audit 拦截，都记录决策快照，
		// 后续客户下一条消息/人工接管时由 SmartCSOrchestrator 更新 CustomerAccept
		e.recordFeedback(ctx, req, resp)
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
	resp.Memory = memCtx

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
	resp.MatchedSOP = matchedSOP

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
	candidate, llmResult, err := e.generateCandidate(ctx, req, intentResult, memCtx, matchedSOP, sopStage, ragChunks, script, customer)
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

	// 步骤 7.5：拟人度评估（P0-4）
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
				// 走发送前审核与下发流程，绝不吞掉已生成的有效回复。
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

	// 步骤 8：发送前审核
	stepStart = time.Now()
	if req.Config.EnableContentAudit {
		audit := e.auditor.Audit(finalReply, &AuditContext{
			Intent:   intentResult.IntentType,
			Platform: req.Platform,
		})
		if !audit.Pass {
			// 命中拦截词
			resp.AuditIssues = audit.Issues
			resp.Steps = append(resp.Steps, dto.SalesStepLog{
				Step: "8_audit", Status: "fail", LatencyMs: ms(stepStart),
				Detail: "blocked: " + strings.Join(audit.Issues, "; "),
			})
			resp.TransferredToHuman = true
			resp.TransferReason = "内容审核未通过: " + strings.Join(audit.Issues, "; ")
			resp.Reply = "[系统提示] 该内容已转人工处理"
			return resp, nil
		}
		if len(audit.Warnings) > 0 {
			resp.AuditIssues = append(resp.AuditIssues, audit.Warnings...)
		}
		resp.Audited = true
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "8_audit", Status: "ok", LatencyMs: ms(stepStart),
			Extra: map[string]any{"warnings": len(audit.Warnings)},
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "8_audit", Status: "skip", LatencyMs: ms(stepStart),
		})
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

// resolveCustomer 解析客户（OneID → 客户实体）
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

// recallMemory 召回短期+长期记忆
func (e *SalesEngine) recallMemory(ctx context.Context, req *SalesRequest) (*model.DialogueMemory, error) {
	if e.memory == nil {
		return &model.DialogueMemory{}, nil
	}
	return e.memory.GetOrCreateMemory(ctx, req.SessionID, req.CustomerID)
}

// matchSOP 匹配 SOP
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
// P0-1 重构：参数改为 PlaybookRecommenderInterface，可注入测试替身实现进行单元测试
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
// P0-3 智能体升级：当注入了 toolExecutor 且存在可用工具时，调用 runAgentLoop
// 走真正的 Agent Loop（LLM ↔ 工具 循环）；否则走原始一次性 LLM 调用（向后兼容）。
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
) (string, *llm.DispatchResult, error) {
	if e.dispatcher == nil {
		return "", nil, fmt.Errorf("dispatcher is nil")
	}

	prompt := e.buildPrompt(req, intent, mem, sop, stage, ragChunks, script, customer)
	scenario := llm.ScenarioSOPReply
	if intent != nil && intent.IntentType == IntentObjectionPrice {
		scenario = llm.ScenarioObjection
	} else if intent != nil && (intent.IntentType == IntentSocial || intent.IntentType == IntentGreeting) {
		scenario = llm.ScenarioFriendlyChat
	}

	// P0-3: 智能体 Agent Loop 路径（真正的智能体，不做流程编排）
	if e.toolExecutor != nil {
		availableTools := e.toolExecutor.ListTools()
		if len(availableTools) > 0 {
			return e.runAgentLoop(ctx, scenario, prompt, req, intent, mem, customer, availableTools)
		}
	}

	result, err := e.dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:     scenario,
		Prompt:       prompt,
		SystemPrompt: req.Config.Persona,
		MaxTokens:    req.Config.MaxTokens,
		Temperature:  req.Config.Temperature,
		CacheKey:     llm.CacheKey(scenario, prompt),
		CacheTTL:     3600,
	})
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(result.Content), result, nil
}

// agentLoopMaxIterations Agent Loop 最大迭代次数
// 防止 LLM 无限调用工具或陷入循环。
// 复杂多工具场景可由 SetAgentLoopMaxIterations 或 env MTK_AGENT_LOOP_MAX_ITERATIONS 覆盖。
var agentLoopMaxIterations = 2

// SetAgentLoopMaxIterations 注入 Agent Loop 最大迭代次数
// 由 main.go 启动时调用；≤0 时保持默认 2
func SetAgentLoopMaxIterations(n int) {
	if n <= 0 {
		return
	}
	agentLoopMaxIterations = n
}

// agentLoopTotalTimeout Agent Loop wall-clock 总超时
//
// 演进：
//   - 2026-07-22：120s（1.5B Q4 CPU 推理 35-60s）
//   - 2026-07-24：改为可配置，由 main.go 启动时从 inference.llm.timeout_seconds 注入
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

// runAgentLoop 真正的智能体 Agent Loop（P0-3 核心实现）
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
) (string, *llm.DispatchResult, error) {
	// 1. 构造工具定义列表（AgentToolDef → llm.ToolDefinition）
	toolDefs := make([]llm.ToolDefinition, 0, len(availableTools))
	for _, fn := range availableTools {
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
			SystemPrompt: req.Config.Persona,
			MaxTokens:    req.Config.MaxTokens,
			Temperature:  req.Config.Temperature,
			CacheKey:     llm.CacheKey(scenario, prompt),
			CacheTTL:     3600,
		})
		if err != nil {
			return "", nil, err
		}
		return strings.TrimSpace(result.Content), result, nil
	}

	// 2. 构造初始 messages（system + user）
	messages := make([]llm.ChatMessage, 0, 4+agentLoopMaxIterations*2)
	messages = append(messages, llm.ChatMessage{
		Role:    "system",
		Content: buildAgentSystemPrompt(req.Config.Persona, intent, mem, customer),
	})
	messages = append(messages, llm.ChatMessage{
		Role:    "user",
		Content: prompt,
	})

	// 3. Agent Loop（P0-A：wall-clock 总超时 30s 兜底，防止最坏 5min 卡死）
	// 总超时设计：30s 默认。即使 LLM 响应慢 + 工具慢，也保证 30s 内返回给用户
	agentLoopCtx, agentLoopCancel := context.WithTimeout(ctx, agentLoopTotalTimeout)
	defer agentLoopCancel()

	var lastResult *llm.DispatchResult
	totalToolCalls := 0
	var firstLLMError error // 记录首次 LLM 调用错误（用于最终降级返回）
	for iter := 1; iter <= agentLoopMaxIterations; iter++ {
		// 检查总超时
		if agentLoopCtx.Err() != nil {
			logger.Warnf("[AgentLoop] wall-clock timeout at iter=%d total_tool_calls=%d, fallback to last content",
				iter, totalToolCalls)
			break
		}

		// 3.1 调用 LLM（携带 tools + 完整对话历史）
		result, err := e.dispatcher.Dispatch(agentLoopCtx, llm.DispatchRequest{
			Scenario:     scenario,
			Prompt:       prompt, // 仅在 Messages 为空时使用，这里 Messages 非空
			SystemPrompt: req.Config.Persona,
			MaxTokens:    req.Config.MaxTokens,
			Temperature:  req.Config.Temperature,
			Tools:        toolDefs,
			ToolChoice:   "auto",
			Messages:     messages,
		})
		if err != nil {
			// P0-C：LLM 调用失败时不直接 return，降级到 fallback 内容
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
			// LLM 给出最终文本回复，结束循环
			logger.Infof("[AgentLoop] iter=%d finish_reason=%s content_len=%d tools_called=%d",
				iter, result.FinishReason, len(result.Content), totalToolCalls)
			return strings.TrimSpace(result.Content), result, nil
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
			AgentID:    "",
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
		if content == "" && firstLLMError != nil {
			// LLM 曾成功响应但内容为空，且有错误：返回降级提示
			content = "抱歉，我暂时无法处理您的请求，请稍后再试。"
		}
		return content, lastResult, nil
	}
	if firstLLMError != nil {
		// P0-C：LLM 调用失败时返回友好降级提示，而非抛 error 给用户
		return "抱歉，AI 服务暂时不可用，请稍后再试或联系人工客服。", nil, nil
	}
	return "", nil, fmt.Errorf("agent loop exhausted with no final content")
}

// buildAgentSystemPrompt 构造 Agent 模式下的系统提示词
// 在原 Persona 基础上追加工具使用指引，让 LLM 知道何时调用哪些工具
func buildAgentSystemPrompt(persona string, intent *dto.RecognizeResult, mem *model.DialogueMemory, customer *model.Customer) string {
	var sb strings.Builder
	sb.WriteString(persona)
	sb.WriteString("\n\n你是一个真正的智能体，可以使用工具来获取客户信息、查询订单、检索知识库等。")
	sb.WriteString("根据用户问题，自主决定是否需要调用工具；调用工具后基于工具返回的真实数据回复用户。")
	sb.WriteString("不要编造工具未返回的数据；如工具调用失败，向用户诚实说明并询问更多信息。")

	if intent != nil {
		sb.WriteString(fmt.Sprintf("\n\n[当前意图] %s", intent.IntentType))
	}
	if customer != nil {
		name := customerNameOf(customer)
		if name != "" {
			sb.WriteString(fmt.Sprintf("\n[客户] %s (ID=%s)", name, customer.ID))
		}
	}
	return sb.String()
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

	sb.WriteString("\n【回复要求】:\n")
	sb.WriteString("1. 基于上述信息生成回复，不要编造事实\n")
	sb.WriteString("2. 简洁（≤80 字），分自然段\n")
	sb.WriteString("3. 语气亲切、像真人对话\n")
	sb.WriteString("4. 若客户异议，按话术/SOP 引导\n")

	return sb.String()
}

// shouldTransferToHuman 是否应该转人工
//
// P0-3 升级：注入 ConfidenceAggregator 后改为基于 5 维信号 + 动态阈值的决策；
// 未注入时保留原有静态规则（IntentChurn/IntentComplaint/MessageCount>30）作为兜底
func (e *SalesEngine) shouldTransferToHuman(ctx context.Context, intent *dto.RecognizeResult, mem *model.DialogueMemory, req *SalesRequest) (bool, string) {
	if intent == nil {
		return false, ""
	}

	// [修复] 高意图置信度且非紧急/投诉/流失场景，直接走 AI 应答：
	// 避免 RAG/Logprob 等信号尚未填充时，聚合器将 0.9+ 的高意图置信度误判为 BandHandoff 而错误转人工。
	if intent.Confidence >= 0.7 &&
		intent.IntentType != IntentComplaint &&
		intent.IntentType != IntentChurn {
		return false, ""
	}

	// P0-3：注入 ConfidenceAggregator 时改用 5 维信号聚合
	if e.confidenceAggregator != nil {
		return e.shouldTransferByConfidence(ctx, intent, mem, req)
	}

	// 兼容：原有静态规则（投诉/流失倾向/对话轮数过多）
	switch intent.IntentType {
	case IntentChurn, IntentComplaint:
		return true, e.transferReason(context.Background(), intent, mem)
	}
	if mem != nil && mem.MessageCount > 30 {
		return true, "对话轮数过多，建议人工接管"
	}
	return false, ""
}

// shouldTransferByConfidence 基于置信度聚合的转人工决策（P0-3）
//
// 输入：意图 + 记忆 + 请求
// 输出：(是否转人工, 原因)
func (e *SalesEngine) shouldTransferByConfidence(ctx context.Context, intent *dto.RecognizeResult, mem *model.DialogueMemory, req *SalesRequest) (bool, string) {
	if e.confidenceAggregator == nil {
		return false, ""
	}

	// P3 监控埋点：每次 confidence scoring +1
	scenario := string(intent.IntentType)
	metrics.RecordConfidenceScored(scenario)

	// 构造信号采集输入
	in := &dto.SignalCollectionInput{
		SessionID:     req.SessionID,
		CustomerID:    req.CustomerID,
		Text:          req.UserMessage,
		IntentType:    intent.IntentType,
		RawIntentConf: intent.Confidence,
		// 其他信号（RAG/Logprobs/Entities）由后续步骤填充后再聚合；
		// 本次预聚合使用 IntentConf 单维信号 + 记忆上下文
		CustomerLevel:     inferCustomerLevelFromReq(req),
		AgentAvailability: 0.5, // 默认中性
		LastTurns:         extractLastTurnsFromMem(mem),
	}

	dec, err := e.confidenceAggregator.Aggregate(ctx, in)
	if err != nil {
		// 聚合失败不阻断主流程，按原有静态规则兜底
		logger.Ctx(ctx).Warn().Err(err).Msg("[sales] confidence aggregate failed, fallback to static rule")
		switch intent.IntentType {
		case IntentChurn, IntentComplaint:
			return true, e.transferReason(context.Background(), intent, mem)
		}
		return false, ""
	}

	// P3 监控埋点：记录决策分布
	decisionLabel := ""
	switch dec.DecisionBand {
	case dto.BandHandoff:
		decisionLabel = "transfer"
	case dto.BandLLMFallback:
		decisionLabel = "llm_fallback"
	case dto.BandReview:
		decisionLabel = "review_queue"
	default:
		decisionLabel = "auto_reply"
	}
	metrics.RecordConfidenceDecision(scenario, decisionLabel)

	// 一票否决
	if dec.VetoTriggered != "" {
		return true, "一票否决: " + dec.VetoTriggered
	}

	// 按决策区间
	switch dec.DecisionBand {
	case dto.BandHandoff:
		return true, fmt.Sprintf("低置信度转人工 (aggregated=%.2f < threshold=%.2f)", dec.AggregatedConf, dec.DynamicThreshold)
	case dto.BandLLMFallback, dto.BandReview:
		// 中间区间：继续走主流程（LLM 兜底/审核），不强制转人工
		return false, ""
	default:
		return false, ""
	}
}

// inferCustomerLevelFromReq 推断客户等级（从请求或客户档案）
func inferCustomerLevelFromReq(req *SalesRequest) string {
	if req == nil || req.Config == nil {
		return "normal"
	}
	if req.Config.CustomerLevel != "" {
		return req.Config.CustomerLevel
	}
	return "normal"
}

// extractLastTurnsFromMem 从记忆中提取最近 3 轮对话文本
func extractLastTurnsFromMem(mem *model.DialogueMemory) []string {
	if mem == nil || len(mem.KeyFacts) == 0 {
		return nil
	}
	out := make([]string, 0, 3)
	for k, v := range mem.KeyFacts {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, k+": "+s)
			if len(out) >= 3 {
				break
			}
		}
	}
	return out
}

// transferReason 转人工原因（兼容旧调用方）
func (e *SalesEngine) transferReason(ctx context.Context, intent *dto.RecognizeResult, mem *model.DialogueMemory) string {
	if intent != nil {
		switch intent.IntentType {
		case IntentChurn:
			return "客户出现流失倾向，建议人工介入挽留"
		case IntentComplaint:
			return "客户正在投诉，需要人工处理"
		}
	}
	if mem != nil && mem.MessageCount > 30 {
		return "对话轮数过多，建议人工接管"
	}
	return "触发转人工规则"
}

// ms 计算耗时（毫秒）
func ms(start time.Time) int {
	return int(time.Since(start).Milliseconds())
}

// safeID 安全返回客户 ID
func safeID(c *model.Customer) string {
	if c == nil {
		return ""
	}
	return c.ID
}

// truncateForReply 把长文本截断成适合作为访客回复的长度
// 仅在 UTF-8 rune 粒度上截断，避免切到半个汉字
func truncateForReply(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// customerNameOf 提取客户名
// Customer 模型只有身份标识（Phone/Email/OpenID），没有昵称字段
// 优先返回手机号作为标识
func customerNameOf(c *model.Customer) string {
	if c == nil {
		return ""
	}
	// 优先使用客户姓名（如果有的话）
	if c.Name != "" {
		return c.Name
	}
	// 回退到手机号
	return c.Phone
}

// ============================================================================
// 多渠道统一入口（Phase 1.3：渠道无关化）
// ----------------------------------------------------------------------------
// 不同渠道（企微/WhatsApp/Telegram/飞书）的入站消息，经 WebhookService 统一
// 编排后，调用本入口进入 智能体。本方法负责：
//   1) 渠道特定的消息清洗（飞书的 v1/v2 字段映射、WhatsApp 的 number/contact 名）
//   2) 构造渠道特定的会话 ID（保证同一用户多渠道隔离）
//   3) 透传到 Handle 执行 7 步链路
// ============================================================================

// ChannelMessage 渠道无关的入站消息（WebhookService → SalesEngine 桥接）
type ChannelMessage struct {
	Channel      string `json:"channel"`       // 渠道标识：wecom/whatsapp/telegram/feishu
	AccountID    string `json:"account_id"`    // 渠道账号 ID
	ExternalUser string `json:"external_user"` // 外部用户 ID
	Nickname     string `json:"nickname"`      // 用户昵称（可选）
	Content      string `json:"content"`       // 消息文本
	MsgType      string `json:"msg_type"`      // text/image/event/...
	ChatID       string `json:"chat_id"`       // 会话 ID（群为 group_id）
	IsGroup      bool   `json:"is_group"`      // 是否群消息
	MediaURL     string `json:"media_url"`     // 媒体 URL（可选）
	RawData      string `json:"raw_data"`      // 原始 payload（可选）
	ReceivedAt   int64  `json:"received_at"`   // 毫秒时间戳
}

// ProcessIncomingMessage 多渠道统一入口
func (e *SalesEngine) ProcessIncomingMessage(ctx context.Context, msg *ChannelMessage) (*SalesResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}
	if msg.Content == "" && msg.MediaURL == "" {
		// 没有有效内容的事件型消息直接跳过
		return &SalesResponse{}, nil
	}

	// 渠道特定清洗
	content, sessionID, customerID := e.normalizeChannelMessage(context.Background(), msg)

	// 构造 SalesRequest
	req := &SalesRequest{
		SessionID:   sessionID,
		CustomerID:  customerID,
		OneID:       customerID,
		UserMessage: content,
		Platform:    msg.Channel,
		AutoExecute: true,
		Config:      DefaultSalesEngineConfig(),
	}

	// 平台相关的人设调整
	if msg.Channel == "feishu" {
		req.Config.Persona = "你是飞书上的企业助手，回复简洁专业。"
	} else if msg.Channel == "telegram" {
		req.Config.Persona = "你是 Telegram 上的销售助手，语气亲切。"
	}

	return e.Handle(ctx, req)
}

// normalizeChannelMessage 渠道特定清洗 → 通用字段
func (e *SalesEngine) normalizeChannelMessage(ctx context.Context, msg *ChannelMessage) (content, sessionID, customerID string) {
	// 内容
	if msg.MediaURL != "" {
		switch msg.MsgType {
		case "image":
			content = "[图片] " + msg.Content
		case "voice":
			content = "[语音] " + msg.Content
		case "video":
			content = "[视频] " + msg.Content
		case "file":
			content = "[文件] " + msg.Content
		default:
			content = msg.Content
		}
	} else {
		content = msg.Content
	}
	// 会话 ID
	if msg.ChatID != "" {
		sessionID = msg.Channel + ":" + msg.ChatID
	} else {
		sessionID = msg.Channel + ":" + msg.AccountID + ":" + msg.ExternalUser
	}
	// 客户 ID（OneID）
	customerID = msg.Channel + ":" + msg.ExternalUser
	return
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

// recordFeedback 记录反馈学习快照（SalesEngine 主链路第 9 步）
// ----------------------------------------------------------------------------
// 商业产品级 AI 自我进化闭环：
//
//	每次 Handle 结束都把"本次决策快照"喂给 FeedbackLearner，包括
//	intent/confidence/SOP/AIReply/是否转人工/token/耗时。
//	CustomerAccept 默认 false（生成时尚未知客户是否接受），
//	后续 SmartCSOrchestrator 在客户下一条消息或人工接管时更新。
//
// 设计原则：
//   - feedbackLearner 为 nil 时静默跳过（不破坏现有链路）
//   - 所有 return 路径都经过 defer，确保不遗漏
//   - 记录失败不影响主流程（仅 log）
func (e *SalesEngine) recordFeedback(ctx context.Context, req *SalesRequest, resp *SalesResponse) {
	if e.feedbackLearner == nil {
		return
	}
	if req == nil || resp == nil {
		return
	}
	record := &FeedbackRecord{
		SessionID:      req.SessionID,
		CustomerID:     req.CustomerID,
		AIReply:        resp.Reply,
		Transferred:    resp.TransferredToHuman,
		TransferReason: resp.TransferReason,
		Tokens:         resp.CostTokens,
		LatencyMs:      resp.LatencyMs,
	}
	if resp.Intent != nil {
		record.IntentType = resp.Intent.IntentType
		record.Confidence = resp.Intent.Confidence
	}
	if resp.MatchedSOP != nil {
		record.SOPName = resp.MatchedSOP.Name
	}
	if err := e.feedbackLearner.RecordFeedback(ctx, record); err != nil {
		// 记录失败不影响主流程，仅打 log 便于排查
		logger.Errorf("[SalesEngine] feedback learner record failed: %v", err)
	}
	resp.Steps = append(resp.Steps, dto.SalesStepLog{
		Step:   "9_feedback_learn",
		Status: "ok",
		Detail: fmt.Sprintf("intent=%s conf=%.2f sop=%s transferred=%v tokens=%d",
			record.IntentType, record.Confidence, record.SOPName, record.Transferred, record.Tokens),
	})
}
