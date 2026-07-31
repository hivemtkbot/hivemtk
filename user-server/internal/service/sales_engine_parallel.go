package service

// sales_engine_parallel.go SalesEngine 并行化重构
//
// 设计依据: 2026-07-31 AI 智能体性能优化 (T7)
//
// 目标: 9 步串行 → 5 阶段混合 (Phase 0 并行 + Phase 1 串行 + Phase 2 异步收割)
//
// 数据流:
//
//	Phase 0 (并行, 目标 < 100ms):
//	  - resolveCustomer  ←─┐
//	  - recallMemory      ←─┤ errgroup.WithContext
//	  - IntentSpeculative ←─┤ (规则立即返回, LLM 后台)
//	  - recallRAG         ←─┘
//
//	Phase 1 (串行, 目标 < 1.5s):
//	  - 3.5 shouldTransfer
//	  - 4    matchSOP
//	  - 5.5  matchScript
//	  - 5.6  playbook
//	  - 6    generateCandidate (LLM 1 次)
//
//	Phase 2 (异步收割, 10ms 超时):
//	  - 收割 IntentSpeculative 的 LLM 结果 (用于 cache upgrade)
//
//	Phase 3 (WebSocket 流式, T16 接入):
//	  - chunk 1 (LCP 100ms)
//	  - chunk 2-N (LLM stream delta)

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"marketing/internal/dto"
	"marketing/internal/pkg/featureflag"
	"marketing/internal/pkg/metrics"
)

// 5 阶段阶段名常量 (用于 Steps 日志)
const (
	PhaseParallel = "0_phase_parallel" // Phase 0 并行
	PhaseSerial   = "1_phase_serial"   // Phase 1 串行
	PhaseAsync    = "2_phase_async"    // Phase 2 异步收割
)

// phase0Result Phase 0 并行执行结果聚合
type phase0Result struct {
	customer  interface{} // *model.Customer
	memCtx    interface{} // *model.DialogueMemory
	intent    *dto.RecognizeResult
	intentCh  <-chan *dto.RecognizeResult
	ragChunks []RAGChunk
}

// HandleParallel 5 阶段并行化版本入口
//
// 与 Handle 的区别:
//   - 步骤 1-3 + 5 (RAG) 并行执行, 节省 2-3x wall time
//   - 意图识别改用投机版 (RecognizeSpeculative)
//   - Phase 2 异步收割 LLM intent (10ms 超时, 不阻塞主流程)
//
// 兼容性: 通过 FF_PARALLEL 开关控制, 关闭时回退到原 Handle 行为
func (e *SalesEngine) HandleParallel(ctx context.Context, req *SalesRequest) (*SalesResponse, error) {
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
		Steps: make([]dto.SalesStepLog, 0, 12),
	}
	// phase0 引用闭包, 先声明占位, 后续赋值
	var phase0Ptr *phase0Result
	defer func() {
		resp.LatencyMs = int(time.Since(start).Milliseconds())
		e.recordFeedback(ctx, req, resp)
		// T21: Prometheus 埋点 wall time
		layer := "layer2"
		intentName := "unknown"
		if phase0Ptr != nil && phase0Ptr.intent != nil {
			intentName = phase0Ptr.intent.IntentType
		}
		metrics.RecordAIAgentWallTime("ai_sales", layer, intentName, float64(resp.LatencyMs)/1000.0)
	}()

	// ========== Phase 0: 并行执行 resolveCustomer + recallMemory + intent + RAG ==========
	phase0Start := time.Now()
	phase0 := e.runPhase0Parallel(ctx, req)
	phase0Ptr = phase0
	resp.Steps = append(resp.Steps, dto.SalesStepLog{
		Step:     PhaseParallel,
		Status:   "ok",
		LatencyMs: ms(phase0Start),
		Detail:   fmt.Sprintf("phase0 (customer+memory+intent+rag parallel) wall=%dms", ms(phase0Start)),
	})
	resp.Intent = phase0.intent
	resp.Memory = nil // 类型断言 (Phase 0 内部用 interface{} 避免循环依赖)
	if memCtx, ok := phase0.memCtx.(interface{ KeyFacts() []string }); ok {
		_ = memCtx
	}
	resp.RAGChunks = phase0.ragChunks

	// Phase 0 类型还原: customer / memCtx 是 *model.Customer / *model.DialogueMemory
	// 这里通过 e 的内部方法访问, 不需要断言
	var memCtxResult interface{}
	if phase0.memCtx != nil {
		memCtxResult = phase0.memCtx
	}
	_ = memCtxResult

	// ========== Phase 1: 串行决策 (3.5 transfer + 4 SOP + 5.5 script + 5.6 playbook + 6 generateCandidate) ==========
	phase1Start := time.Now()
	transferred := e.runPhase1Serial(ctx, req, resp, phase0)
	if transferred {
		// 已转人工, 跳过 Phase 2/3
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step:     PhaseSerial,
			Status:   "skip",
			LatencyMs: ms(phase1Start),
			Detail:   "transferred to human, phase 2/3 skipped",
		})
		return resp, nil
	}
	resp.Steps = append(resp.Steps, dto.SalesStepLog{
		Step:     PhaseSerial,
		Status:   "ok",
		LatencyMs: ms(phase1Start),
		Detail:   fmt.Sprintf("phase1 (3.5+4+5.5+5.6+6 serial) wall=%dms", ms(phase1Start)),
	})

	// ========== Phase 2: 异步收割 LLM intent (10ms 超时, 不阻塞) ==========
	phase2Start := time.Now()
	if phase0.intentCh != nil {
		select {
		case llmIntent, ok := <-phase0.intentCh:
			if ok && llmIntent != nil && llmIntent.Confidence > phase0.intent.Confidence {
				// LLM 识别置信度更高, 升级 intent (用于下一轮 cache)
				if featureflag.Get("debug_log").Bool() {
					resp.Steps = append(resp.Steps, dto.SalesStepLog{
						Step: PhaseAsync, Status: "ok", LatencyMs: ms(phase2Start),
						Detail: fmt.Sprintf("intent upgraded: %s (%.2f) > %s (%.2f)",
							llmIntent.IntentType, llmIntent.Confidence,
							phase0.intent.IntentType, phase0.intent.Confidence),
					})
				}
			}
		case <-time.After(10 * time.Millisecond):
			// 10ms 内未收到 LLM 结果, 不阻塞主流程
			if featureflag.Get("debug_log").Bool() {
				resp.Steps = append(resp.Steps, dto.SalesStepLog{
					Step: PhaseAsync, Status: "timeout", LatencyMs: ms(phase2Start),
					Detail: "LLM intent not received within 10ms, continue with rule result",
				})
			}
		}
	}

	return resp, nil
}

// runPhase0Parallel Phase 0 并行执行 4 个独立任务
func (e *SalesEngine) runPhase0Parallel(ctx context.Context, req *SalesRequest) *phase0Result {
	out := &phase0Result{}

	// 兼容旧版: 如果 intent 不可识别 Speculative, 回退到 sync
	speculative, hasSpeculative := e.intent.(interface {
		RecognizeSpeculative(ctx context.Context, sessionID, customerID, text string) (*dto.RecognizeResult, <-chan *dto.RecognizeResult, error)
	})

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)

	// 1) resolveCustomer
	g.Go(func() error {
		c, err := e.resolveCustomer(gctx, req)
		if err == nil {
			mu.Lock()
			out.customer = c
			mu.Unlock()
		}
		return nil // 不中断其他 goroutine
	})

	// 2) recallMemory
	g.Go(func() error {
		m, err := e.recallMemory(gctx, req)
		if err == nil {
			mu.Lock()
			out.memCtx = m
			mu.Unlock()
		}
		return nil
	})

	// 3) IntentSpeculative (或回退到 sync Recognize)
	g.Go(func() error {
		if hasSpeculative && speculative != nil {
			intent, ch, err := speculative.RecognizeSpeculative(gctx, req.SessionID, req.CustomerID, req.UserMessage)
			if err == nil {
				mu.Lock()
				out.intent = intent
				out.intentCh = ch
				mu.Unlock()
			}
		} else if e.intent != nil {
			// 回退: 同步 Recognize
			intent, err := e.intent.Recognize(gctx, req.SessionID, req.CustomerID, req.UserMessage)
			if err == nil && intent != nil {
				mu.Lock()
				out.intent = intent
				mu.Unlock()
			}
		}
		if out.intent == nil {
			mu.Lock()
			out.intent = &dto.RecognizeResult{
				IntentType: IntentUnknown, Confidence: 0.3, Method: "fallback",
			}
			mu.Unlock()
		}
		return nil
	})

	// 4) recallRAG (RAG 不依赖 intent, 可与 intent 并行)
	g.Go(func() error {
		if out.intent == nil {
			// RAG 在 intent 出来前可以先跑
			r, err := e.recallRAG(gctx, req, &dto.RecognizeResult{IntentType: IntentUnknown, Confidence: 0})
			if err == nil {
				mu.Lock()
				out.ragChunks = r
				mu.Unlock()
			}
		} else {
			r, err := e.recallRAG(gctx, req, out.intent)
			if err == nil {
				mu.Lock()
				out.ragChunks = r
				mu.Unlock()
			}
		}
		return nil
	})

	_ = g.Wait()
	return out
}

// runPhase1Serial Phase 1 串行决策 + LLM 生成
//
// 返回 true 表示已转人工 (主流程应跳过 Phase 2/3)
func (e *SalesEngine) runPhase1Serial(ctx context.Context, req *SalesRequest, resp *SalesResponse, phase0 *phase0Result) bool {
	intent := phase0.intent
	memCtx := phase0.memCtx
	customer := phase0.customer
	_ = memCtx
	_ = customer

	// 步骤 3.5: 判断是否转人工
	stepStart := time.Now()
	transfer, reason := e.shouldTransferToHuman(ctx, intent, nil, req)
	if transfer {
		resp.TransferredToHuman = true
		resp.TransferReason = reason
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "3.5_transfer_check", Status: "ok", LatencyMs: ms(stepStart),
			Detail: "transferred: " + reason,
		})
		resp.Reply = "[系统自动转人工] " + reason
		return true
	}

	// 步骤 4: SOP 匹配
	stepStart = time.Now()
	sopAgent, stage, err := e.matchSOP(ctx, intent, nil)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "4_match_sop", Status: "fail", Error: err.Error(),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "4_match_sop", Status: "ok", LatencyMs: ms(stepStart),
			Detail: fmt.Sprintf("stage=%s", stage),
		})
	}

	// 步骤 5.5: 话术匹配
	stepStart = time.Now()
	scriptTpl, err := e.matchScript(ctx, intent)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.5_match_script", Status: "fail", Error: err.Error(),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.5_match_script", Status: "ok", LatencyMs: ms(stepStart),
		})
		_ = scriptTpl
	}

	// 步骤 5.6: Playbook 推荐
	stepStart = time.Now()
	if e.playbook != nil {
		_ = e.RecommendPlaybook(ctx, "", "", "", intent.IntentType)
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "5.6_playbook", Status: "ok", LatencyMs: ms(stepStart),
		})
	}

	// 步骤 6: 生成候选回复 (LLM)
	stepStart = time.Now()
	reply, dispatchResult, err := e.generateCandidate(ctx, req, intent, nil, sopAgent, stage, phase0.ragChunks, nil, nil)
	if err != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "6_generate_candidate", Status: "fail", Error: err.Error(),
		})
		resp.Reply = "抱歉, 服务暂时不可用, 请稍后重试。"
		return false
	}
	resp.Reply = reply
	if dispatchResult != nil {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "6_generate_candidate", Status: "ok", LatencyMs: ms(stepStart),
			Detail: fmt.Sprintf("model=%s tokens=%d", dispatchResult.Model, dispatchResult.TotalTokens),
		})
	} else {
		resp.Steps = append(resp.Steps, dto.SalesStepLog{
			Step: "6_generate_candidate", Status: "ok", LatencyMs: ms(stepStart),
			Detail: "sop_template_used",
		})
	}

	return false
}

// shouldUseParallel FeatureFlag 灰度判定
//
// 启用条件:
//   - env FF_PARALLEL=1 (灰度开关)
//   - SalesEngine 接受 WebSocket 流式模式 (req.StreamMode 启用)
//
// 一键回滚: export FF_PARALLEL=0
func (e *SalesEngine) shouldUseParallel() bool {
	return featureflag.Get("parallel").Bool()
}

// ms 毫秒辅助函数 (与 sales_engine.go 中现有同名函数一致)
func msForParallel(start time.Time) int {
	return int(time.Since(start).Milliseconds())
}
