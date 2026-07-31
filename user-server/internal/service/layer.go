package service

// layer_router.go 双层架构路由决策器
//
// 五层架构归属: L4 业务编排层
// 设计依据: 2026-07-31 AI 智能体性能优化 (T11)
//
// 决策流程:
//   1. FF_LAYER1 关闭 -> 直接返回 Layer2 (LLM 兜底)
//   2. 查 FAQ 库, 高分命中 (>= faqHitThresh) -> Layer1 SkipLLM
//   3. 查 SOP 模板, 高分命中 (>= sopHitThresh) -> Layer1 SkipLLM
//   4. 否则 -> Layer2 (LLM 兜底)
//
// 输出: *dto.LayerDecision (供 SalesEngine.generateCandidate 使用)

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/featureflag"
	appmetrics "marketing/internal/pkg/metrics"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

const (
	// sopHitThresh SOP 模板命中阈值 (高于此值进 Layer1)
	sopHitThresh = 0.65
)

// FAQMatcher FAQ 匹配接口 (2026-07-31 DIP 重构, 便于 mock 单测)
//
// 满足接口的最小方法集: MatchByKeyword + MatchByAgent (Task 15 强 1对1) + IncrementHitCount
type FAQMatcher interface {
	MatchByKeyword(ctx context.Context, msg string, topK int) ([]model.FAQEntry, error)
	MatchByAgent(ctx context.Context, agentID uint, msg string, topK int) ([]model.FAQEntry, error)
	IncrementHitCount(ctx context.Context, id uint) error
}

// LayerRouter 双层路由决策器
type LayerRouter struct {
	db      *gorm.DB
	faqRepo FAQMatcher
	sopRepo *repository.SOPTemplateRepository
	logRepo *repository.LayerDecisionLogRepository
	faqSvc  *FAQService
	sopSvc  *SOPTemplateService
	traceFunc func() string // trace_id 生成函数 (注入避免强耦合)
}

// NewLayerRouter 创建 LayerRouter
func NewLayerRouter(
	db *gorm.DB,
	faqRepo FAQMatcher,
	sopRepo *repository.SOPTemplateRepository,
	logRepo *repository.LayerDecisionLogRepository,
	faqSvc *FAQService,
	sopSvc *SOPTemplateService,
) *LayerRouter {
	// 兼容旧 API: 允许传入 *repository.FAQRepository, Go 鸭子类型自动适配 FAQMatcher
	if faqRepo == nil && db != nil {
		faqRepo = repository.NewFAQRepository(db)
	}
	if sopRepo == nil && db != nil {
		sopRepo = repository.NewSOPTemplateRepository(db)
	}
	if logRepo == nil && db != nil {
		logRepo = repository.NewLayerDecisionLogRepository(db)
	}
	return &LayerRouter{
		db:      db,
		faqRepo: faqRepo,
		sopRepo: sopRepo,
		logRepo: logRepo,
		faqSvc:  faqSvc,
		sopSvc:  sopSvc,
	}
}

// RouteRequest 路由请求参数
//
// Task 15 变更:
//   - AgentFAQIDs (旧字段) 被弃用, 改用 AgentID 强 1对1
//   - AgentSOPTemplateIDs (旧字段) 同样, 后续 Task 16 改造 SOP
type RouteRequest struct {
	SessionID   string
	CustomerID  string
	UserMessage string
	Intent      *dto.RecognizeResult
	RAGChunks   []RAGChunk
	Stage       string
	// Task 15 强 1对1: 智能体 ID (uint); 0 = 无 agent (走 Layer2)
	AgentID uint
	// 旧字段保留 (向后兼容), 后续移除
	// 2026-07-31 P1-A: 智能体绑定的 FAQ / SOP 模板 ID 集合
	// 空切片 = 全局共享, 非空 = 仅在绑定的 ID 集合内匹配
	AgentFAQIDs         []string
	AgentSOPTemplateIDs []string
}

// Route 路由决策 (核心入口)
func (r *LayerRouter) Route(ctx context.Context, req *RouteRequest) *dto.LayerDecision {
	start := time.Now()
	decision := &dto.LayerDecision{
		Layer:    dto.Layer2,
		SkipLLM:  false,
		Reason:   dto.ReasonFallback,
		Intent:   intentType(req.Intent),
		WallMs:   0,
	}
	defer func() {
		decision.WallMs = int(time.Since(start).Milliseconds())
		r.record(ctx, req, decision)
		// T21: Prometheus 埋点
		appmetrics.RecordAIAgentLayerDecision(decision.Layer, decision.Reason)
	}()

	// 1. FeatureFlag: 关闭时直接 Layer2
	if !featureflag.Get("layer1").Bool() {
		decision.Layer = dto.Layer2
		decision.Reason = dto.ReasonLayer1Disabled
		return decision
	}

	// 2. 意图为未知 -> Layer2
	if req.Intent != nil && (req.Intent.IntentType == "" || req.Intent.IntentType == IntentUnknown) {
		// 也允许 FAQ 命中跳过 (FAQ 不依赖 intent)
	}

	// 3. FAQ 匹配 (Task 15 强 1对1: 按 agentID 匹配)
	// agentID == 0: 跳过 FAQ, 走 Layer2 (移除"空数组=全局"分支)
	// agentID > 0: 走 MatchByAgent 强匹配
	//
	// 优先用 faqSvc (L4 Service 封装, 含缓存/业务策略);
	// 单测可直接注入 faqRepo (FAQMatcher) 走快速路径, 不依赖 DB/Service
	if req.AgentID == 0 {
		// Task 15: 无 agentID 不再触发 FAQ 匹配, 直接跳到 SOP / Layer2
	} else if r.faqSvc != nil && strings.TrimSpace(req.UserMessage) != "" {
		matches, err := r.faqSvc.MatchByAgent(ctx, req.AgentID, req.UserMessage, 3)
		if err == nil && len(matches) > 0 {
			top := matches[0]
			if top.Score >= faqHitThresh {
				decision.Layer = dto.Layer1
				decision.SkipLLM = true
				if top.Entry != nil {
					decision.Reply = top.Entry.Answer
					decision.FAQID = top.Entry.ID
				}
				decision.Reason = dto.ReasonFAQHit
				decision.Confidence = top.Score
				if top.Entry != nil {
					decision.Intent = top.Entry.Intent
				}
				if intentType(req.Intent) == "" && top.Entry != nil {
					decision.Intent = top.Entry.Intent
				}
				// 命中计数 (异步, 不阻塞)
				if top.Entry != nil {
					go func(id uint) {
						_ = r.faqRepo.IncrementHitCount(context.Background(), id)
					}(top.Entry.ID)
				}
				return decision
			}
			// 命中但置信度低, 仍记录以供分析
			decision.Reason = dto.ReasonLowConfidenceSkip
		}
	} else if r.faqRepo != nil && strings.TrimSpace(req.UserMessage) != "" {
		// 单测回退路径: 直接用 FAQMatcher (跳过 faqSvc 业务封装)
		// Task 15: agentID > 0 时走 MatchByAgent; agentID == 0 时不再兜底
		if req.AgentID > 0 {
			entries, err := r.faqRepo.MatchByAgent(ctx, req.AgentID, req.UserMessage, 3)
			if err == nil && len(entries) > 0 {
				top := entries[0]
				if top.Confidence >= faqHitThresh {
					decision.Layer = dto.Layer1
					decision.SkipLLM = true
					decision.Reply = top.Answer
					decision.FAQID = top.ID
					decision.Reason = dto.ReasonFAQHit
					decision.Confidence = top.Confidence
					decision.Intent = top.Intent
					if intentType(req.Intent) == "" {
						decision.Intent = top.Intent
					}
					// 命中计数 (异步, 不阻塞)
					go func(id uint) {
						_ = r.faqRepo.IncrementHitCount(context.Background(), id)
					}(top.ID)
					return decision
				}
				// 命中但置信度低, 仍记录以供分析
				decision.Reason = dto.ReasonLowConfidenceSkip
			}
		}
	}

	// 4. SOP 模板匹配 (Task 16 强 1对1: 按 (agentID, intent, stage) 严格匹配)
	//
	// 行为:
	//   - agentID == 0: 跳过 SOP 匹配, 走 Layer2 (强 1对1: 移除"空数组=全局"分支)
	//   - agentID > 0: 走 MatchByAgent(ctx, agentID, intent, stage, topK) 强匹配
	//   - 命中且 confidence >= sopHitThresh: 模板渲染后返回 Layer1
	if r.sopSvc != nil && req.AgentID > 0 && req.Intent != nil && req.Intent.IntentType != "" && req.Intent.IntentType != IntentUnknown {
		tpls, err := r.sopSvc.MatchByAgent(ctx, req.AgentID, req.Intent.IntentType, req.Stage, sopTopK)
		if err == nil && len(tpls) > 0 {
			top := tpls[0]
			if top.Confidence >= sopHitThresh {
				decision.Layer = dto.Layer1
				decision.SkipLLM = true
				decision.Reason = dto.ReasonSOPHit
				decision.Confidence = top.Confidence
				decision.SOPID = top.ID
				decision.Intent = top.Intent
				// 模板渲染 (vars 来自 customer / memCtx)
				rendered, rErr := r.sopSvc.BuildLayer1Reply(&top, map[string]any{
					"customer_id":  req.CustomerID,
					"intent":       req.Intent.IntentType,
					"intent_name":  req.Intent.IntentName,
					"stage":        req.Stage,
					"user_message": req.UserMessage,
				})
				if rErr == nil && rendered != "" {
					decision.Reply = rendered
				} else {
					// 渲染失败 -> 回退到 Layer2
					decision.Layer = dto.Layer2
					decision.SkipLLM = false
					decision.Reason = dto.ReasonFallback
					if featureflag.Get("debug_log").Bool() {
						logger.Warnf("[LayerRouter] SOP render failed: %v", rErr)
					}
					return decision
				}
				go func(id uint) {
					_ = r.sopRepo.IncrementHitCount(context.Background(), id)
				}(top.ID)
				return decision
			}
		}
	}

	// 5. 默认 Layer2
	decision.Layer = dto.Layer2
	decision.SkipLLM = false
	if decision.Reason == "" {
		decision.Reason = dto.ReasonFallback
	}
	return decision
}

// record 落库 Layer 决策日志 (异步, 失败不阻塞主流程)
func (r *LayerRouter) record(ctx context.Context, req *RouteRequest, d *dto.LayerDecision) {
	if r.logRepo == nil {
		return
	}
	confIn := 0.0
	if req.Intent != nil {
		confIn = req.Intent.Confidence
	}
	llmSkipped := d.SkipLLM
	log := &model.LayerDecisionLog{
		TraceID:    r.traceID(),
		SessionID:  req.SessionID,
		CustomerID: req.CustomerID,
		Layer:      d.Layer,
		Reason:     d.Reason,
		Intent:     d.Intent,
		ConfIn:     confIn,
		ConfOut:    d.Confidence,
		WallMs:     d.WallMs,
		LLMSkipped: &llmSkipped,
		Extra:      fmt.Sprintf("faq_id=%d sop_id=%d", d.FAQID, d.SOPID),
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.logRepo.Record(bgCtx, log); err != nil {
			if featureflag.Get("debug_log").Bool() {
				logger.Warnf("[LayerRouter] record failed: %v", err)
			}
		}
	}()
}

func (r *LayerRouter) traceID() string {
	if r.traceFunc != nil {
		return r.traceFunc()
	}
	return fmt.Sprintf("lr-%d", time.Now().UnixNano())
}

func intentType(r *dto.RecognizeResult) string {
	if r == nil {
		return ""
	}
	return r.IntentType
}
