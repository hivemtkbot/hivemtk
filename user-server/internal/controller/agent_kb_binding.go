package controller

// agent_kb_binding_controller.go 智能体知识库绑定 L3 Controller
//
// 五层架构归属: L3 API 接入层
// 设计依据: 强 1对1 改造 (知识库管理)
//
// 接口前缀: /api/agent-kb-bindings
//   GET    /api/agent-kb-bindings/agent/:aid      查某智能体的所有绑定
//   GET    /api/agent-kb-bindings/kb/:kid         查某知识库被哪些智能体绑定
//   POST   /api/agent-kb-bindings                 单个绑定 (重复自动覆盖)
//   POST   /api/agent-kb-bindings/batch           批量绑定 (事务)
//   DELETE /api/agent-kb-bindings                 单个解绑 (?agent_id&kb_id)
//
// 业务规则:
//   - (agent_id, knowledge_base_id) 唯一; 重复 bind 自动覆盖
//   - 同一智能体可绑多个不同类型的知识库
//   - 批量绑定走事务, 任一失败整体回滚

import (
	"net/http"
	"strconv"

	"gorm.io/gorm"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// AgentKBBindingController 智能体知识库绑定控制器
type AgentKBBindingController struct {
	svc *service.AgentKBBindingService
}

// NewAgentKBBindingController 创建绑定控制器
func NewAgentKBBindingController(db *gorm.DB) *AgentKBBindingController {
	return &AgentKBBindingController{
		svc: service.NewAgentKBBindingService(db),
	}
}

// RegisterRoutes 注册路由
func (c *AgentKBBindingController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/agent-kb-bindings")
	{
		g.GET("/agent/:aid", c.ListByAgent)
		g.GET("/kb/:kid", c.ListByKB)
		g.POST("", c.Bind)
		g.POST("/batch", c.BatchBind)
		g.DELETE("", c.Unbind)
	}
}

// ListByAgent 查某智能体的所有绑定
func (c *AgentKBBindingController) ListByAgent(ctx *gin.Context) {
	aid, err := strconv.ParseUint(ctx.Param("aid"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的智能体 ID")
		return
	}
	if aid == 0 {
		response.Error(ctx, http.StatusBadRequest, "agent_id 必填且 > 0")
		return
	}
	list, err := c.svc.ListByAgent(ctx.Request.Context(), uint(aid))
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, list, "查询成功")
}

// ListByKB 查某知识库被哪些智能体绑定
func (c *AgentKBBindingController) ListByKB(ctx *gin.Context) {
	kid, err := strconv.ParseUint(ctx.Param("kid"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	if kid == 0 {
		response.Error(ctx, http.StatusBadRequest, "knowledge_base_id 必填且 > 0")
		return
	}
	list, err := c.svc.ListByKB(ctx.Request.Context(), uint(kid))
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, list, "查询成功")
}

// agentKBBindReq 单个绑定请求
type agentKBBindReq struct {
	AgentID         uint `json:"agent_id" binding:"required"`
	KnowledgeBaseID uint `json:"knowledge_base_id" binding:"required"`
	Priority        int  `json:"priority"`
}

// Bind 单个绑定 (重复自动覆盖)
func (c *AgentKBBindingController) Bind(ctx *gin.Context) {
	var req agentKBBindReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.AgentID == 0 {
		response.Error(ctx, http.StatusBadRequest, "agent_id 必填且 > 0")
		return
	}
	if req.KnowledgeBaseID == 0 {
		response.Error(ctx, http.StatusBadRequest, "knowledge_base_id 必填且 > 0")
		return
	}
	if err := c.svc.Bind(ctx.Request.Context(), req.AgentID, req.KnowledgeBaseID, req.Priority); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"agent_id":          req.AgentID,
		"knowledge_base_id": req.KnowledgeBaseID,
		"priority":          req.Priority,
	}, "绑定成功")
}

// agentKBBatchBindReq 批量绑定请求
type agentKBBatchBindReq struct {
	Items []service.BatchBindItem `json:"items" binding:"required"`
}

// BatchBind 批量绑定 (事务, 任一失败整体回滚)
func (c *AgentKBBindingController) BatchBind(ctx *gin.Context) {
	var req agentKBBatchBindReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := c.svc.BatchBind(ctx.Request.Context(), req.Items); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"count": len(req.Items),
	}, "批量绑定成功")
}

// agentKBUnbindReq 单个解绑请求
type agentKBUnbindReq struct {
	AgentID         uint `json:"agent_id" binding:"required"`
	KnowledgeBaseID uint `json:"knowledge_base_id" binding:"required"`
}

// Unbind 单个解绑
func (c *AgentKBBindingController) Unbind(ctx *gin.Context) {
	var req agentKBUnbindReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.AgentID == 0 {
		response.Error(ctx, http.StatusBadRequest, "agent_id 必填且 > 0")
		return
	}
	if req.KnowledgeBaseID == 0 {
		response.Error(ctx, http.StatusBadRequest, "knowledge_base_id 必填且 > 0")
		return
	}
	if err := c.svc.Unbind(ctx.Request.Context(), req.AgentID, req.KnowledgeBaseID); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"agent_id":          req.AgentID,
		"knowledge_base_id": req.KnowledgeBaseID,
	}, "解绑成功")
}
