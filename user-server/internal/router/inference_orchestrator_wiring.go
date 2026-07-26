package router

import (
	"context"
	"net/http"
	"sync"
	"time"

	agent_runtime "marketing/internal/aiagent/agent/runtime"
	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// 推理闭环编排器装配（P1-6）
// ----------------------------------------------------------------------------
// 文档依据：docs/企业级架构优化/认知决策大脑层.md / 核心数据流向.md
//
// 本文件负责：
//   1. 创建 memoryProviderAdapter：包装 service.MemorySystem 为
//      agent_runtime.EpisodicMemoryProvider，让跨会话情境记忆（L1/L2/L3/L4）
//      注入 InferenceCycle，使规划阶段（PlannerStage）的 ReplyHint 能引用
//      历史对话与客户画像
//   2. 装配全局 CoreDataFlowOrchestrator 单例：串联
//      感知→对齐→门禁→规划 4 阶段推理闭环
//   3. 暴露 GetInferenceOrchestrator() 供路由层调用
//
// 设计要点：
//   - 不修改 SalesEngine：CoreDataFlowOrchestrator 作为独立推理决策服务暴露，
//     不影响现有 SalesEngine 9 步流水线（避免破坏生产链路）
//   - 记忆读取失败仅告警不阻塞：InferenceCycle.RunOnce 内部已处理 err
//   - 单例 + sync.Once：重复调用安全
//   - 五层架构合规：router 层装配，依赖 service（L3）+ agent_runtime（L3 内部组件）
// ============================================================================

// memoryProviderAdapter 把 service.MemorySystem 适配为
// agent_runtime.EpisodicMemoryProvider
//
// 签名完全一致（BuildFullContext 与 LoadEpisodicMemory 都是
// (ctx, sessionID, customerID) → (string, error)），适配器仅做接口转换，
// 避免 service 包反向依赖 agent_runtime 包
type memoryProviderAdapter struct {
	ms *service.MemorySystem
}

// LoadEpisodicMemory 实现 EpisodicMemoryProvider 接口
func (a *memoryProviderAdapter) LoadEpisodicMemory(ctx context.Context, sessionID, customerID string) (string, error) {
	if a == nil || a.ms == nil {
		return "", nil
	}
	return a.ms.BuildFullContext(ctx, sessionID, customerID)
}

// 全局单例
var (
	globalInferenceOrchestrator *agent_runtime.CoreDataFlowOrchestrator
	inferenceOrchestratorOnce   sync.Once
)

// initInferenceOrchestrator 装配全局推理闭环编排器
//
// 调用方：router.Setup()（在 initGlobalToolExecutor 之后调用，
// 依赖 service.InitMemorySystem 已完成）
//
// 装配内容：
//   - 创建 InferenceCycle（含默认 4 阶段：感知/对齐/门禁/规划）
//   - 注入 EpisodicMemoryProvider（包装 MemorySystem）
//   - 创建 CoreDataFlowOrchestrator（暂不注入 AssetLoader/ToolRouter/Publisher，
//     由后续 P1-6b/P1-6c 逐步激活）
func initInferenceOrchestrator() {
	inferenceOrchestratorOnce.Do(func() {
		cycle := agent_runtime.NewInferenceCycle()

		// 注入跨会话情境记忆提供者
		if ms := service.GetMemorySystem(); ms != nil {
			cycle.SetMemoryProvider(&memoryProviderAdapter{ms: ms})
			logger.Info("[inference] ✅ EpisodicMemoryProvider 已注入（包装 MemorySystem.BuildFullContext）")
		} else {
			logger.Warn("[inference] ⚠️ MemorySystem 未初始化，推理闭环将跳过情境记忆读取")
		}

		globalInferenceOrchestrator = agent_runtime.NewCoreDataFlowOrchestrator(cycle, nil)
		logger.Info("[inference] ✅ CoreDataFlowOrchestrator 已装配（4 阶段推理闭环：感知→对齐→门禁→规划）")
	})
}

// GetInferenceOrchestrator 返回全局推理闭环编排器
//
// 未初始化时返回 nil（调用方需 nil-check）
func GetInferenceOrchestrator() *agent_runtime.CoreDataFlowOrchestrator {
	return globalInferenceOrchestrator
}

// ============================================================================
// 路由注册
// ============================================================================

// inferenceRunRequest 推理闭环运行请求
type inferenceRunRequest struct {
	ChannelType string `json:"channel_type" binding:"required"`
	CustomerID  string `json:"customer_id" binding:"required"`
	SessionID   string `json:"session_id"`
	Content     string `json:"content" binding:"required"`
	MessageType string `json:"message_type"`
	TraceID     string `json:"trace_id"`
}

// setupInferenceRoutes 注册推理闭环 API 路由
//
// 端点：
//   - POST /api/agent/inference/run    触发一次推理闭环（感知→对齐→门禁→规划）
//   - GET  /api/agent/inference/stats  查询编排器统计
//
// 调用方：router.Setup() 的 auth 路由组
func setupInferenceRoutes(auth *gin.RouterGroup) {
	auth.POST("/agent/inference/run", handleInferenceRun)
	auth.GET("/agent/inference/stats", handleInferenceStats)
}

// handleInferenceRun 处理推理闭环运行请求
func handleInferenceRun(c *gin.Context) {
	orch := GetInferenceOrchestrator()
	if orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "inference orchestrator not initialized",
		})
		return
	}

	var req inferenceRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request: " + err.Error(),
		})
		return
	}

	// 构造 CustomerMessagePayload
	payload := agent_runtime.CustomerMessagePayload{
		ChannelType: req.ChannelType,
		CustomerID:  req.CustomerID,
		SessionID:   req.SessionID,
		Content:     req.Content,
		MessageType: req.MessageType,
		Timestamp:   time.Now(),
		TraceID:     req.TraceID,
	}
	if payload.MessageType == "" {
		payload.MessageType = "text"
	}
	if payload.TraceID == "" {
		payload.TraceID = "inference-" + payload.SessionID + "-" + payload.CustomerID
	}

	// 执行推理闭环
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	result, err := orch.Process(ctx, payload, nil)
	if err != nil {
		logger.Errorf("[inference_api] run failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"session_id":       result.SessionID,
		"final_reply":      result.FinalReply,
		"handoff_to_human": result.HandoffToHuman,
		"handoff_reason":   result.HandoffReason,
		"tool_call_count":  result.ToolCallCount,
		"crisis_level":     result.CrisisLevel,
		"total_duration":   result.TotalDuration.String(),
	})
}

// handleInferenceStats 返回编排器统计
func handleInferenceStats(c *gin.Context) {
	orch := GetInferenceOrchestrator()
	if orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "inference orchestrator not initialized",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   orch.GetStats(),
	})
}

// ============================================================================
// P2-8: 工具权限白名单管理 API
// ============================================================================

// 全局权限检查器单例（由 tool_executor_wiring.go 初始化时注入，
// 或通过 SetGlobalPermissionChecker 手动注入；未初始化时返回 nil）
var (
	globalPermissionChecker     *tooluse.WhitelistPermissionChecker
	globalPermissionCheckerOnce sync.Once
)

// SetGlobalPermissionChecker 注入全局权限检查器
// 调用方：tool_executor_wiring.go 在创建 ToolExecutor 时同步注入
func SetGlobalPermissionChecker(pc *tooluse.WhitelistPermissionChecker) {
	globalPermissionChecker = pc
}

// GetGlobalPermissionChecker 获取全局权限检查器
// 未初始化时惰性创建一个默认实例（defaultAllow=true，向后兼容）
// 返回 nil 的情况：理论上不会发生（惰性初始化保证非 nil）
func GetGlobalPermissionChecker() *tooluse.WhitelistPermissionChecker {
	globalPermissionCheckerOnce.Do(func() {
		if globalPermissionChecker == nil {
			globalPermissionChecker = tooluse.NewWhitelistPermissionChecker()
		}
	})
	return globalPermissionChecker
}

// setupToolPermissionRoutes 注册工具权限白名单管理路由
//
// 端点：
//   - GET    /api/agent/tools/permission/default           查询默认放行策略
//   - PUT    /api/agent/tools/permission/default           设置默认放行策略
//   - GET    /api/agent/tools/permission/global            查询全局白名单
//   - POST   /api/agent/tools/permission/global            追加全局白名单工具
//   - GET    /api/agent/tools/permission/agents            列出已配置白名单的 Agent
//   - GET    /api/agent/tools/permission/agents/:agent_id  查询指定 Agent 的白名单
//   - POST   /api/agent/tools/permission/agents/:agent_id  设置指定 Agent 的白名单（覆盖式）
//   - DELETE /api/agent/tools/permission/agents/:agent_id  移除指定 Agent 的白名单配置
func setupToolPermissionRoutes(auth *gin.RouterGroup) {
	auth.GET("/tools/permission/default", handleGetPermissionDefault)
	auth.PUT("/tools/permission/default", handleSetPermissionDefault)
	auth.GET("/tools/permission/global", handleGetGlobalWhitelist)
	auth.POST("/tools/permission/global", handleAddGlobalWhitelist)
	auth.GET("/tools/permission/agents", handleListConfiguredAgents)
	auth.GET("/tools/permission/agents/:agent_id", handleGetAgentWhitelist)
	auth.POST("/tools/permission/agents/:agent_id", handleSetAgentWhitelist)
	auth.DELETE("/tools/permission/agents/:agent_id", handleRemoveAgentWhitelist)
}

func handleGetPermissionDefault(c *gin.Context) {
	pc := GetGlobalPermissionChecker()
	if pc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "permission checker not initialized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"default_allow": pc.GetDefaultAllow(),
	})
}

type setPermissionDefaultRequest struct {
	DefaultAllow bool `json:"default_allow"`
}

func handleSetPermissionDefault(c *gin.Context) {
	pc := GetGlobalPermissionChecker()
	if pc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "permission checker not initialized"})
		return
	}
	var req setPermissionDefaultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request: " + err.Error()})
		return
	}
	pc.SetDefaultAllow(req.DefaultAllow)
	logger.Infof("[permission] default_allow set to %v", req.DefaultAllow)
	c.JSON(http.StatusOK, gin.H{"success": true, "default_allow": req.DefaultAllow})
}

func handleGetGlobalWhitelist(c *gin.Context) {
	pc := GetGlobalPermissionChecker()
	if pc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "permission checker not initialized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"tools":   pc.ListGlobalWhitelist(),
		"count":   len(pc.ListGlobalWhitelist()),
	})
}

type addGlobalWhitelistRequest struct {
	Tools []string `json:"tools" binding:"required"`
}

func handleAddGlobalWhitelist(c *gin.Context) {
	pc := GetGlobalPermissionChecker()
	if pc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "permission checker not initialized"})
		return
	}
	var req addGlobalWhitelistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request: " + err.Error()})
		return
	}
	pc.AddGlobalWhitelist(req.Tools)
	logger.Infof("[permission] global whitelist added %d tools", len(req.Tools))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"tools":   pc.ListGlobalWhitelist(),
		"count":   len(pc.ListGlobalWhitelist()),
	})
}

func handleListConfiguredAgents(c *gin.Context) {
	pc := GetGlobalPermissionChecker()
	if pc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "permission checker not initialized"})
		return
	}
	agents := pc.ListConfiguredAgents()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"agents":  agents,
		"count":   len(agents),
	})
}

func handleGetAgentWhitelist(c *gin.Context) {
	pc := GetGlobalPermissionChecker()
	if pc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "permission checker not initialized"})
		return
	}
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "agent_id required"})
		return
	}
	tools := pc.ListAgentWhitelist(agentID)
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"agent_id": agentID,
		"tools":    tools,
		"count":    len(tools),
	})
}

type setAgentWhitelistRequest struct {
	Tools []string `json:"tools" binding:"required"`
}

func handleSetAgentWhitelist(c *gin.Context) {
	pc := GetGlobalPermissionChecker()
	if pc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "permission checker not initialized"})
		return
	}
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "agent_id required"})
		return
	}
	var req setAgentWhitelistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request: " + err.Error()})
		return
	}
	pc.SetAgentWhitelist(agentID, req.Tools)
	logger.Infof("[permission] agent=%s whitelist set (%d tools)", agentID, len(req.Tools))
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"agent_id": agentID,
		"tools":    pc.ListAgentWhitelist(agentID),
		"count":    len(pc.ListAgentWhitelist(agentID)),
	})
}

func handleRemoveAgentWhitelist(c *gin.Context) {
	pc := GetGlobalPermissionChecker()
	if pc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "permission checker not initialized"})
		return
	}
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "agent_id required"})
		return
	}
	pc.RemoveAgentWhitelist(agentID)
	logger.Infof("[permission] agent=%s whitelist removed (fallback to default policy)", agentID)
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"agent_id": agentID,
		"message":  "whitelist removed, fallback to default policy",
	})
}
