package agent_runtime

import (
	"context"
	"time"
)

// ============================================================================
// 11. defaultAgentRuntime 方法实现（骨架）
// ============================================================================

// HandleCustomerMessage 处理客户消息事件
//
// 骨架实现：仅打印日志 + 返回默认值
// 完整实现在任务 6（订阅实现）+ 后续业务联调
func (r *defaultAgentRuntime) HandleCustomerMessage(ctx context.Context, payload CustomerMessagePayload) (*SalesResponse, error) {
	if r.stopped {
		return nil, ErrRuntimeStopped
	}

	// 1. 加载智能体上下文
	agentCtx, err := r.LoadAgentContext(ctx, payload.ChannelType, payload.AccountID)
	if err != nil {
		return nil, err
	}

	// 2. 构造请求
	req := &SalesRequest{
		Channel:    payload.ChannelType,
		AccountID:  payload.AccountID,
		CustomerID: payload.CustomerID,
		Content:    payload.Content,
		TraceID:    payload.TraceID,
		Raw:        payload.Raw,
	}

	// 3. 根据 AgentType 路由
	switch agentCtx.AgentType {
	case "sales":
		if r.salesSales == nil {
			return r.fallbackResponse(payload, agentCtx), nil
		}
		return r.salesSales.HandleWithAgent(ctx, agentCtx, req)
	case "customer_service":
		if r.csBridge == nil {
			return r.fallbackResponse(payload, agentCtx), nil
		}
		return r.csBridge.HandleIncomingWithAgent(ctx, agentCtx, req)
	case "hybrid":
		// 混合类型：优先销售，失败转客服
		if r.salesSales != nil {
			resp, err := r.salesSales.HandleWithAgent(ctx, agentCtx, req)
			if err == nil && !resp.HandoffToHuman {
				return resp, nil
			}
		}
		if r.csBridge != nil {
			return r.csBridge.HandleIncomingWithAgent(ctx, agentCtx, req)
		}
		return r.fallbackResponse(payload, agentCtx), nil
	default:
		return r.fallbackResponse(payload, agentCtx), nil
	}
}

// LoadAgentContext 加载智能体上下文（带缓存）
func (r *defaultAgentRuntime) LoadAgentContext(ctx context.Context, channelType, accountID string) (*AgentContext, error) {
	if r.loader == nil {
		// 骨架阶段：返回默认上下文
		return &AgentContext{
			AgentID:   0,
			AgentCode: "default",
			Name:      "默认智能体",
			AgentType: "sales",
			LoadedAt:  time.Now(),
			Channel:   channelType,
			AccountID: accountID,
		}, nil
	}

	key := cacheKey{Channel: channelType, AccountID: accountID}

	// 读缓存
	r.mu.RLock()
	if cached, ok := r.cache[key]; ok {
		if time.Since(cached.cachedAt) < r.cacheTTL {
			r.mu.RUnlock()
			return cached.ctx, nil
		}
	}
	r.mu.RUnlock()

	// 加载
	agentCtx, err := r.loader.LoadByChannelAccount(ctx, channelType, accountID)
	if err != nil {
		return nil, err
	}
	agentCtx.LoadedAt = time.Now()
	agentCtx.Channel = channelType
	agentCtx.AccountID = accountID

	// 写缓存
	r.mu.Lock()
	r.cache[key] = &cachedContext{ctx: agentCtx, cachedAt: time.Now()}
	r.mu.Unlock()

	return agentCtx, nil
}

// RefreshCache 刷新指定智能体的缓存
func (r *defaultAgentRuntime) RefreshCache(ctx context.Context, agentID uint) error {
	if r.loader != nil {
		if err := r.loader.Invalidate(ctx, agentID); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// 失效所有引用该 agentID 的缓存
	for k, c := range r.cache {
		if c.ctx.AgentID == agentID {
			delete(r.cache, k)
		}
	}
	return nil
}

// Stop 优雅关闭运行时
func (r *defaultAgentRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	r.cache = nil
	return nil
}

// fallbackResponse 降级响应（无 engine bridge 时的兜底）
func (r *defaultAgentRuntime) fallbackResponse(payload CustomerMessagePayload, agentCtx *AgentContext) *SalesResponse {
	return &SalesResponse{
		ReplyContent: "系统暂不可用，请稍后再试。",
		ReplyType:    "text",
		Confidence:   0,
		AgentID:      agentCtx.AgentID,
		AgentCode:    agentCtx.AgentCode,
		Channel:      payload.ChannelType,
		CustomerID:   payload.CustomerID,
		TraceID:      payload.TraceID,
		StopReason:   "no_engine_bridge",
		Duration:     0,
	}
}

// ============================================================================
// 12. 错误定义
// ============================================================================

// ErrRuntimeStopped 运行时已停止
var ErrRuntimeStopped = &RuntimeError{Code: "stopped", Message: "agent runtime stopped"}

// RuntimeError 运行时错误
type RuntimeError struct {
	Code    string
	Message string
}

func (e *RuntimeError) Error() string {
	return e.Code + ": " + e.Message
}
