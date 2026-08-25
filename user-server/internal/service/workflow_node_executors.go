package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// TriggerNodeExecutor 触发器节点执行器
type TriggerNodeExecutor struct{}

func (e *TriggerNodeExecutor) NodeType() string { return WorkflowNodeTypeTrigger }

func (e *TriggerNodeExecutor) IsAsync() bool { return false }

func (e *TriggerNodeExecutor) Execute(ctx context.Context, wctx *WorkflowExecContext) (*WorkflowNodeExecResult, error) {
	triggeredAt := time.Now().Format(time.RFC3339)
	output := model.JSONMap{
		"_triggered_at": triggeredAt,
	}
	if wctx.Input != nil {
		for k, v := range wctx.Input {
			output[k] = v
		}
	}
	if wctx.Execution != nil && wctx.Execution.TriggerPayload != nil {
		for k, v := range wctx.Execution.TriggerPayload {
			if _, exists := output[k]; !exists {
				output[k] = v
			}
		}
	}
	return &WorkflowNodeExecResult{
		Status:      NodeStatusCompleted,
		Output:      output,
		NextNodeID:  "",
		SideEffects: []string{WorkflowSideEffectKey(wctx, "triggered")},
	}, nil
}

// ActionNodeExecutor 动作节点执行器
type ActionNodeExecutor struct{}

func (e *ActionNodeExecutor) NodeType() string { return WorkflowNodeTypeAction }

func (e *ActionNodeExecutor) IsAsync() bool { return false }

func (e *ActionNodeExecutor) Execute(ctx context.Context, wctx *WorkflowExecContext) (*WorkflowNodeExecResult, error) {
	actionType := ""
	if wctx.NodeConfig != nil {
		if v, ok := wctx.NodeConfig["action_type"].(string); ok {
			actionType = v
		}
	}

	sideEffectKey := WorkflowSideEffectKey(wctx, "action_"+actionType)
	if HasWorkflowSideEffect(wctx, sideEffectKey) {
		return &WorkflowNodeExecResult{
			Status:      NodeStatusCompleted,
			Output:      model.JSONMap{"_already_executed": true},
			NextNodeID:  "",
			SideEffects: []string{sideEffectKey},
		}, nil
	}

	switch actionType {
	case "log":
		message := ""
		if wctx.NodeConfig != nil {
			if v, ok := wctx.NodeConfig["message"].(string); ok {
				message = v
			}
		}
		logger.Ctx(ctx).Info().Str("workflow_action", "log").Str("message", message).Msg("ActionNodeExecutor: log")
		AppendWorkflowSideEffect(wctx, sideEffectKey)
		return &WorkflowNodeExecResult{
			Status:      NodeStatusCompleted,
			Output:      model.JSONMap{"message": message, "_action_type": "log"},
			NextNodeID:  "",
			SideEffects: []string{sideEffectKey},
		}, nil

	case "http":
		url := ""
		if wctx.NodeConfig != nil {
			if v, ok := wctx.NodeConfig["url"].(string); ok {
				url = v
			}
		}
		if url == "" {
			url = "http://placeholder"
		}
		logger.Ctx(ctx).Info().Str("workflow_action", "http").Str("url", url).Msg("ActionNodeExecutor: http (mock)")
		AppendWorkflowSideEffect(wctx, sideEffectKey)
		return &WorkflowNodeExecResult{
			Status:      NodeStatusCompleted,
			Output:      model.JSONMap{"url": url, "_action_type": "http", "_mock": true},
			NextNodeID:  "",
			SideEffects: []string{sideEffectKey, "http_called:" + url},
		}, nil

	case "delay":
		durationMs := 0
		if wctx.NodeConfig != nil {
			switch v := wctx.NodeConfig["duration_ms"].(type) {
			case float64:
				durationMs = int(v)
			case int:
				durationMs = v
			case string:
				fmt.Sscanf(v, "%d", &durationMs)
			}
		}
		if durationMs > 5000 {
			durationMs = 5000
		}
		if durationMs > 0 {
			select {
			case <-time.After(time.Duration(durationMs) * time.Millisecond):
			case <-ctx.Done():
				return &WorkflowNodeExecResult{
					Status:       NodeStatusFailed,
					ErrorMessage: "delay cancelled",
					Retryable:    false,
				}, nil
			}
		}
		waitUntil := time.Now().Add(time.Duration(durationMs) * time.Millisecond)
		AppendWorkflowSideEffect(wctx, sideEffectKey)
		return &WorkflowNodeExecResult{
			Status:      NodeStatusCompleted,
			Output:      model.JSONMap{"duration_ms": durationMs, "_action_type": "delay"},
			NextNodeID:  "",
			WaitUntil:   &waitUntil,
			WaitEvent:   WaitEventTimer,
			SideEffects: []string{sideEffectKey},
		}, nil

	default:
		logger.Ctx(ctx).Warn().Str("action_type", actionType).Msg("ActionNodeExecutor: unknown action_type, using noop")
		noop := &WorkflowNoopExecutor{nodeType: WorkflowNodeTypeAction}
		return noop.Execute(ctx, wctx)
	}
}

// ConditionNodeExecutor 条件节点执行器
type ConditionNodeExecutor struct{}

func (e *ConditionNodeExecutor) NodeType() string { return WorkflowNodeTypeCondition }

func (e *ConditionNodeExecutor) IsAsync() bool { return false }

func (e *ConditionNodeExecutor) Execute(ctx context.Context, wctx *WorkflowExecContext) (*WorkflowNodeExecResult, error) {
	rules := []model.JSONMap{}
	if wctx.NodeConfig != nil {
		if v, ok := wctx.NodeConfig["rules"].([]any); ok {
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					rules = append(rules, m)
				}
			}
		}
	}

	sideEffectKey := WorkflowSideEffectKey(wctx, "condition")
	if HasWorkflowSideEffect(wctx, sideEffectKey) {
		return &WorkflowNodeExecResult{
			Status:      NodeStatusCompleted,
			Output:      model.JSONMap{"_already_executed": true},
			NextNodeID:  "",
			SideEffects: []string{sideEffectKey},
		}, nil
	}

	for _, rule := range rules {
		field := getString(rule, "field")
		op := getString(rule, "op")
		value := rule["value"]
		branch := getString(rule, "branch")

		if field == "" || op == "" {
			continue
		}

		ctxVal := wctx.Context[field]
		matched := evaluateCondition(ctxVal, op, value)
		if matched {
			nextNode := NextWorkflowNode(wctx.Graph, wctx.NodeID, branch)
			if nextNode == nil {
				logger.Ctx(ctx).Warn().
					Str("branch", branch).
					Str("node_id", wctx.NodeID).
					Msg("ConditionNodeExecutor: branch target not found in graph, falling back to default")
			} else {
				AppendWorkflowSideEffect(wctx, sideEffectKey)
				return &WorkflowNodeExecResult{
					Status:      NodeStatusCompleted,
					Output:      model.JSONMap{"matched_rule": rule, "_condition_branch": branch},
					NextNodeID:  nextNode.ID,
					SideEffects: []string{sideEffectKey},
				}, nil
			}
		}
	}

	defaultNode := NextWorkflowNode(wctx.Graph, wctx.NodeID, "")
	AppendWorkflowSideEffect(wctx, sideEffectKey)
	if defaultNode == nil {
		return &WorkflowNodeExecResult{
			Status:      NodeStatusCompleted,
			Output:      model.JSONMap{"matched": false, "_condition_branch": "end"},
			NextNodeID:  "_end_",
			SideEffects: []string{sideEffectKey},
		}, nil
	}
	return &WorkflowNodeExecResult{
		Status:      NodeStatusCompleted,
		Output:      model.JSONMap{"matched": false, "_condition_branch": "default"},
		NextNodeID:  defaultNode.ID,
		SideEffects: []string{sideEffectKey},
	}, nil
}

func wfGetString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func evaluateCondition(ctxVal any, op string, target any) bool {
	if ctxVal == nil {
		return false
	}
	switch op {
	case "eq":
		return fmt.Sprintf("%v", ctxVal) == fmt.Sprintf("%v", target)
	case "ne":
		return fmt.Sprintf("%v", ctxVal) != fmt.Sprintf("%v", target)
	case "gt":
		return compareNumber(ctxVal, target) > 0
	case "lt":
		return compareNumber(ctxVal, target) < 0
	case "contains":
		ctxStr := fmt.Sprintf("%v", ctxVal)
		targetStr := fmt.Sprintf("%v", target)
		return strings.Contains(ctxStr, targetStr)
	default:
		return false
	}
}

func compareNumber(a, b any) int {
	fa, ok1 := toFloat64(a)
	fb, ok2 := toFloat64(b)
	if !ok1 || !ok2 {
		return 0
	}
	if fa < fb {
		return -1
	}
	if fa > fb {
		return 1
	}
	return 0
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case string:
		var f float64
		_, err := fmt.Sscanf(x, "%f", &f)
		return f, err == nil
	default:
		return 0, false
	}
}

// SubflowNodeExecutor 子流程节点执行器
type SubflowNodeExecutor struct{}

func (e *SubflowNodeExecutor) NodeType() string { return WorkflowNodeTypeSubflow }

func (e *SubflowNodeExecutor) IsAsync() bool { return false }

func (e *SubflowNodeExecutor) Execute(ctx context.Context, wctx *WorkflowExecContext) (*WorkflowNodeExecResult, error) {
	subWorkflowID := ""
	if wctx.NodeConfig != nil {
		if v, ok := wctx.NodeConfig["sub_workflow_id"].(string); ok {
			subWorkflowID = v
		}
	}

	sideEffectKey := WorkflowSideEffectKey(wctx, "subflow")
	if HasWorkflowSideEffect(wctx, sideEffectKey) {
		return &WorkflowNodeExecResult{
			Status:      NodeStatusCompleted,
			Output:      model.JSONMap{"_already_executed": true},
			NextNodeID:  "",
			SideEffects: []string{sideEffectKey},
		}, nil
	}

	logger.Ctx(ctx).Info().
		Str("sub_workflow_id", subWorkflowID).
		Str("node_id", wctx.NodeID).
		Msg("SubflowNodeExecutor: invoking subflow (placeholder)")

	AppendWorkflowSideEffect(wctx, sideEffectKey)
	output := model.JSONMap{"_subflow_invoked": subWorkflowID}
	if subWorkflowID != "" {
		output["sub_workflow_id"] = subWorkflowID
	}
	// TODO: 真实嵌套调用 - 需要创建子执行实例并等待完成
	// 目前仅标记为 completed 推进下一节点
	return &WorkflowNodeExecResult{
		Status:      NodeStatusCompleted,
		Output:      output,
		NextNodeID:  "",
		SideEffects: []string{sideEffectKey},
	}, nil
}