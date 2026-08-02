package controller

// self_learning_controller.go 对话驱动自我学习三位一体机制 HTTP 控制器
//
// 五层架构归属: L1 网关层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.4
//
// 路由（全部鉴权）：
//   开关 API（用户开启即全自动执行）
//     GET    /api/self-learning/switch                 获取开关状态
//     PUT    /api/self-learning/switch                 更新开关配置
//   看板 API
//     GET    /api/self-learning/dashboard              总看板（今日统计 + 失败率 + 熔断）
//     GET    /api/self-learning/supervision            RAG 5 维监督看板
//     GET    /api/self-learning/supervision/asset      资产包 5 维专属监督看板
//     GET    /api/self-learning/orchestrator/stats     Orchestrator 运行时统计
//   日志 API
//     POST   /api/self-learning/logs/list              自我学习日志列表
//   候选管理 API
//     POST   /api/self-learning/candidates/list        资产包候选列表
//   A/B 实验 API
//     POST   /api/self-learning/ab-tests/list          A/B 实验列表
//     POST   /api/self-learning/ab-tests/promote       人工晋升（supervised 模式）
//   矫正动作 API
//     POST   /api/self-learning/corrections/list       矫正动作审计列表
//     POST   /api/self-learning/corrections/:id/approve 人工批准（supervised 模式）
//     POST   /api/self-learning/corrections/:id/reject  人工拒绝
//
// 设计原则：
//   1. 严格按五层架构：Controller 仅做 HTTP 适配，业务放 Service
//   2. 入参绑定 DTO，出参 JSON
//   3. 不直接持有 db / repository（全部由 service 屏蔽）

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// SelfLearningController 自我学习三位一体机制 HTTP 控制器
type SelfLearningController struct {
	svc *service.SelfLearningService
}

// NewSelfLearningController 构造控制器
func NewSelfLearningController(svc *service.SelfLearningService) *SelfLearningController {
	return &SelfLearningController{svc: svc}
}

// Register 注册路由
func (c *SelfLearningController) Register(rg *gin.RouterGroup) {
	g := rg.Group("/self-learning")
	// 1. 开关 API
	g.GET("/switch", c.GetSwitch)
	g.PUT("/switch", c.UpdateSwitch)
	// 2. 看板 API
	g.GET("/dashboard", c.GetDashboard)
	g.GET("/supervision", c.GetRAGSupervision)
	g.GET("/supervision/asset", c.GetAssetSupervision)
	g.GET("/orchestrator/stats", c.GetOrchestratorStats)
	// 3. 日志 API
	g.POST("/logs/list", c.ListLogs)
	// 4. 候选管理 API
	g.POST("/candidates/list", c.ListCandidates)
	// 5. A/B 实验 API
	g.POST("/ab-tests/list", c.ListABTests)
	g.POST("/ab-tests/promote", c.PromoteABTest)
	// 6. 矫正动作 API
	g.POST("/corrections/list", c.ListCorrections)
	g.POST("/corrections/:id/approve", c.ApproveCorrection)
	g.POST("/corrections/:id/reject", c.RejectCorrection)
}

// ============================================================================
// 1. 开关 API
// ============================================================================

// GetSwitch 获取开关状态
func (c *SelfLearningController) GetSwitch(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	status, err := c.svc.GetSwitchStatus(ctx.Request.Context())
	if err != nil {
		logger.Errorf("[self-learning] get switch failed: %v", err)
		response.ErrorFromDB(ctx, err, "获取开关状态失败: "+err.Error())
		return
	}
	response.Success(ctx, status, "ok")
}

// UpdateSwitch 更新开关配置（用户开启/关闭全自动机制）
//
// 三级自治等级：
//   - manual      仅采集，不自动执行（人工审核每个动作）
//   - supervised  自动执行 + 人工审核关键决策（promote/rollback 需人工）
//   - autonomous  全自动执行（含 promote/rollback，仅告警通知）
func (c *SelfLearningController) UpdateSwitch(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	var req dto.SwitchConfigRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	// operatorID 从上下文获取（私域独立部署默认 0；后续接入用户系统时填充）
	operatorID := uint(0)
	if uid, exists := ctx.Get("user_id"); exists {
		if id, ok := uid.(uint); ok {
			operatorID = id
		} else if id, ok := uid.(int64); ok {
			operatorID = uint(id)
		}
	}
	status, err := c.svc.UpdateSwitch(ctx.Request.Context(), &req, operatorID)
	if err != nil {
		logger.Errorf("[self-learning] update switch failed: %v", err)
		response.ErrorFromDB(ctx, err, "更新开关失败: "+err.Error())
		return
	}
	response.Success(ctx, status, "ok")
}

// ============================================================================
// 2. 看板 API
// ============================================================================

// GetDashboard 获取总看板（今日统计 + 失败率 + 熔断状态 + 待审动作）
func (c *SelfLearningController) GetDashboard(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	dash, err := c.svc.GetDashboard(ctx.Request.Context())
	if err != nil {
		logger.Errorf("[self-learning] get dashboard failed: %v", err)
		response.ErrorFromDB(ctx, err, "获取看板失败: "+err.Error())
		return
	}
	response.Success(ctx, dash, "ok")
}

// GetRAGSupervision 获取 RAG 5 维监督看板
//
// 查询参数 range: 24h / 7d / 30d（默认 24h）
func (c *SelfLearningController) GetRAGSupervision(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	rangeStr := ctx.DefaultQuery("range", "24h")
	dash, err := c.svc.GetRAGSupervisionDashboard(ctx.Request.Context(), rangeStr)
	if err != nil {
		logger.Errorf("[self-learning] get rag supervision failed: %v", err)
		response.ErrorFromDB(ctx, err, "获取 RAG 监督看板失败: "+err.Error())
		return
	}
	response.Success(ctx, dash, "ok")
}

// GetAssetSupervision 获取资产包 5 维专属监督看板
//
// 查询参数 range: 24h / 7d / 30d（默认 24h）
func (c *SelfLearningController) GetAssetSupervision(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	rangeStr := ctx.DefaultQuery("range", "24h")
	dash, err := c.svc.GetAssetSupervisionDashboard(ctx.Request.Context(), rangeStr)
	if err != nil {
		logger.Errorf("[self-learning] get asset supervision failed: %v", err)
		response.ErrorFromDB(ctx, err, "获取资产包监督看板失败: "+err.Error())
		return
	}
	response.Success(ctx, dash, "ok")
}

// GetOrchestratorStats 获取 Orchestrator 运行时统计
func (c *SelfLearningController) GetOrchestratorStats(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	stats := c.svc.GetOrchestratorStats()
	response.Success(ctx, stats, "ok")
}

// ============================================================================
// 3. 日志 API
// ============================================================================

// ListLogs 查询自我学习日志列表
func (c *SelfLearningController) ListLogs(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	var req dto.SelfLearningLogListRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	list, err := c.svc.ListLogs(ctx.Request.Context(), &req)
	if err != nil {
		logger.Errorf("[self-learning] list logs failed: %v", err)
		response.ErrorFromDB(ctx, err, "查询日志失败: "+err.Error())
		return
	}
	response.SuccessWithList(ctx, list.List, list.Total)
}

// ============================================================================
// 4. 候选管理 API
// ============================================================================

// ListCandidates 查询资产包候选列表
func (c *SelfLearningController) ListCandidates(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	var req dto.CandidateListRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	list, err := c.svc.ListCandidates(ctx.Request.Context(), &req)
	if err != nil {
		logger.Errorf("[self-learning] list candidates failed: %v", err)
		response.ErrorFromDB(ctx, err, "查询候选失败: "+err.Error())
		return
	}
	response.SuccessWithList(ctx, list.List, list.Total)
}

// ============================================================================
// 5. A/B 实验 API
// ============================================================================

// ListABTests 查询 A/B 实验列表
func (c *SelfLearningController) ListABTests(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	var req dto.ABTestListRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	list, err := c.svc.ListABTests(ctx.Request.Context(), &req)
	if err != nil {
		logger.Errorf("[self-learning] list ab-tests failed: %v", err)
		response.ErrorFromDB(ctx, err, "查询 A/B 实验失败: "+err.Error())
		return
	}
	response.SuccessWithList(ctx, list.List, list.Total)
}

// PromoteABTest 人工晋升 A/B 实验结果（supervised 模式下人工确认）
func (c *SelfLearningController) PromoteABTest(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	var req dto.ABTestPromoteRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	if err := c.svc.PromoteABTest(ctx.Request.Context(), &req); err != nil {
		logger.Errorf("[self-learning] promote ab-test failed: %v", err)
		response.ErrorFromDB(ctx, err, "晋升 A/B 实验失败: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{"experiment_id": req.ExperimentID, "winner_arm": req.WinnerArm}, "ok")
}

// ============================================================================
// 6. 矫正动作审计 API
// ============================================================================

// ListCorrections 查询矫正动作审计列表
func (c *SelfLearningController) ListCorrections(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	var req dto.CorrectionListRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	list, err := c.svc.ListCorrections(ctx.Request.Context(), &req)
	if err != nil {
		logger.Errorf("[self-learning] list corrections failed: %v", err)
		response.ErrorFromDB(ctx, err, "查询矫正动作失败: "+err.Error())
		return
	}
	response.SuccessWithList(ctx, list.List, list.Total)
}

// ApproveCorrection 人工批准待审矫正动作（supervised 模式）
func (c *SelfLearningController) ApproveCorrection(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	actionID := ctx.Param("id")
	if actionID == "" {
		response.Error(ctx, http.StatusBadRequest, "invalid action id")
		return
	}
	var req dto.CorrectionRollbackRequest
	if !response.BindJSON(ctx, &req) {
		// 允许空 body，使用 path 参数
		req = dto.CorrectionRollbackRequest{ActionID: actionID}
	}
	req.ActionID = actionID
	if err := c.svc.ApproveCorrection(ctx.Request.Context(), &req); err != nil {
		logger.Errorf("[self-learning] approve correction failed: %v", err)
		response.ErrorFromDB(ctx, err, "批准矫正动作失败: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{"action_id": actionID, "status": "applied"}, "ok")
}

// RejectCorrection 人工拒绝待审矫正动作
func (c *SelfLearningController) RejectCorrection(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "self_learning service not initialized")
		return
	}
	actionID := ctx.Param("id")
	if actionID == "" {
		response.Error(ctx, http.StatusBadRequest, "invalid action id")
		return
	}
	var req dto.CorrectionRollbackRequest
	if !response.BindJSON(ctx, &req) {
		req = dto.CorrectionRollbackRequest{ActionID: actionID}
	}
	req.ActionID = actionID
	if err := c.svc.RejectCorrection(ctx.Request.Context(), &req); err != nil {
		logger.Errorf("[self-learning] reject correction failed: %v", err)
		response.ErrorFromDB(ctx, err, "拒绝矫正动作失败: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{"action_id": actionID, "status": "skipped"}, "ok")
}
