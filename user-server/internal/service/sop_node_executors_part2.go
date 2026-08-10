package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"time"
)

// 拆分自 sop_node_executors.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。

func (e *WaitExecutor) Execute(ctx context.Context, ec *ExecutionContext) (*NodeExecResult, error) {
	waitSeconds, _ := ec.Node.Config["wait_seconds"].(float64)
	waitEventStr, _ := ec.Node.Config["wait_event"].(string)
	waitUntilStr, _ := ec.Node.Config["wait_until"].(string)

	// 计算等待截止时间
	var waitUntil time.Time
	waitEvent := WaitEventTimer // 默认 timer

	if waitUntilStr != "" {
		// 绝对时间（RFC3339）
		if t, err := time.Parse(time.RFC3339, waitUntilStr); err == nil {
			waitUntil = t
		}
	}
	if waitUntil.IsZero() && waitSeconds > 0 {
		waitUntil = time.Now().Add(time.Duration(int64(waitSeconds)) * time.Second)
	}
	if waitUntil.IsZero() {
		// 事件等待：默认 24h 超时防卡死
		waitEvent = WaitEventCustomerReply
		if waitEventStr != "" {
			waitEvent = waitEventStr
		}
		waitUntil = time.Now().Add(24 * time.Hour)
	} else if waitEventStr != "" {
		waitEvent = waitEventStr
	}

	// 写入 sop_timers 表（OutboxDispatcher 周期扫描）
	if e.timerRepo != nil {
		timer := &model.SOPTimer{
			ExecutionID: ec.Execution.ID,
			NodeID:      ec.Node.ID,
			WaitEvent:   waitEvent,
			WaitUntil:   waitUntil,
			Status:      "pending",
			Payload: model.JSONMap{
				"trace_id":    ec.TraceID,
				"customer_id": ec.CustomerID,
				"session_id":  ec.SessionID,
				"attempt":     ec.Attempt,
			},
		}
		if err := e.timerRepo.Create(ctx, timer); err != nil {
			logger.Ctx(ctx).Error().Err(err).
				Str("node_id", ec.Node.ID).
				Msg("create sop_timer failed")
			return &NodeExecResult{
				Status:       NodeStatusFailed,
				ErrorMessage: fmt.Sprintf("create timer failed: %v", err),
				Retryable:    true,
			}, err
		}
		logger.Ctx(ctx).Info().
			Uint("timer_id", timer.ID).
			Str("node_id", ec.Node.ID).
			Str("wait_event", waitEvent).
			Time("wait_until", waitUntil).
			Msg("sop timer created")
	}

	return &NodeExecResult{
		Status:    NodeStatusWaiting,
		Output:    model.JSONMap{"_wait_event": waitEvent, "_wait_until": waitUntil.Format(time.RFC3339)},
		WaitUntil: &waitUntil,
		WaitEvent: waitEvent,
	}, nil
}

// NewWaitExecutor 创建等待节点执行器
//
// 当 deps.DB 非 nil 时构造 timerRepo，否则 timerRepo 为 nil（Execute 会跳过 DB 写入）。
func NewWaitExecutor(deps *SOPNodeExecutorDeps) *WaitExecutor {
	e := &WaitExecutor{db: deps.DB}
	if deps.DB != nil {
		e.timerRepo = repository.NewSOPTimerRepository(deps.DB)
	}
	return e
}

// ============================================================================
// 6. 旧版节点兼容执行器：message / action / send_offer / ai_decide / branch
// ============================================================================

// 旧版节点映射到新执行器（通过 RegisterLegacyExecutors 注册）

// RegisterAllNodeExecutors 注册所有 14 种节点执行器 + 5 种旧版兼容执行器
//
// 应在 SOPExecutionDispatcher 初始化时调用一次。
// 重复注册会 panic（启动期错误）。
func RegisterAllNodeExecutors(registry *NodeExecutorRegistry, deps *SOPNodeExecutorDeps) {
	// 1. 空动作类
	registry.Register(context.Background(), &StartExecutor{})
	registry.Register(context.Background(), &EndExecutor{})

	// 2. 消息发送类（9 种商用节点）
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeGreeting, llm.ScenarioFriendlyChat, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeInquire, llm.ScenarioSOPReply, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeIntroduce, llm.ScenarioSOPReply, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeHandle, llm.ScenarioObjection, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeClose, llm.ScenarioHighQuality, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeInvite, llm.ScenarioSOPReply, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeFollowUp, llm.ScenarioFriendlyChat, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeActivate, llm.ScenarioFriendlyChat, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeNurture, llm.ScenarioFriendlyChat, deps))

	// 3. 条件路由类
	registry.Register(context.Background(), &ConditionExecutor{nodeType: SOPNodeTypeCondition})

	// 4. LLM 决策类
	registry.Register(context.Background(), NewLLMNodeExecutor(SOPNodeTypeLLM, deps))

	// 5. 等待类
	registry.Register(context.Background(), NewWaitExecutor(deps))

	// 6. 旧版节点兼容映射
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeMessage, llm.ScenarioFriendlyChat, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeAction, llm.ScenarioSOPReply, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeSendOffer, llm.ScenarioObjection, deps))
	registry.Register(context.Background(), NewLLMNodeExecutor(SOPNodeTypeAIDecide, deps))
	registry.Register(context.Background(), &ConditionExecutor{nodeType: SOPNodeTypeBranch})

	logger.GetLogger().Info().
		Strs("registered_types", registry.AllRegistered(context.Background())).
		Msg("all sop node executors registered")
}

// ===== 辅助函数 =====

// firstOrEmpty 返回切片第一个元素，空切片返回空字符串
func firstOrEmpty(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// containsString 检查字符串切片是否包含指定值
func containsString(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}
