package controller


import (
	"net/http"
	"strconv"
	"strings"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// AgentKBBindingController 智能体知识库绑定控制器
type AgentKBBindingController struct {
	svc *service.AgentKBBindingService
}

// NewAgentKBBindingController 创建绑定控制器
func NewAgentKBBindingController() *AgentKBBindingController {
	return &AgentKBBindingController{
		svc: service.NewAgentKBBindingServiceDefault(),
	}
}

// RegisterRoutes 注册路由
//
// 注意: 前端 user-web 实际请求的路由为 /by-agent/:id、/by-kb/:id、
// DELETE /:agentId/:kbId、PUT /by-agent/:id；此处同时注册旧路径别名
// (/agent/:aid、/kb/:kid、body 解绑) 以保持向后兼容。
func (c *AgentKBBindingController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/agent-kb-bindings")
	{
		g.GET("/by-agent/:agentId", c.ListByAgent)
		g.GET("/agent/:aid", c.ListByAgent) 
		g.GET("/by-kb/:kbId", c.ListByKB)
		g.GET("/kb/:kid", c.ListByKB) 
		g.PUT("/by-agent/:agentId", c.ReplaceByAgent)
		g.DELETE("/:agentId/:kbId", c.UnbindByPath)
		g.POST("", c.Bind)
		g.POST("/batch", c.BatchBind)
		g.DELETE("", c.Unbind) 
	}
}

// agentIDParam 兼容前端路径参数名 agentId 与旧路径 aid
func agentIDParam(ctx *gin.Context) string {
	if v := ctx.Param("agentId"); v != "" {
		return v
	}
	return ctx.Param("aid")
}

// ListByAgent 查某智能体的所有绑定
func (c *AgentKBBindingController) ListByAgent(ctx *gin.Context) {
	aid, err := strconv.ParseUint(agentIDParam(ctx), 10, 64)
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
	idStr := ctx.Param("kbId")
	if idStr == "" {
		idStr = ctx.Param("kid")
	}
	kid, err := strconv.ParseUint(idStr, 10, 64)
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

// agentKBReplaceReq 全量替换某智能体知识库挂载请求
type agentKBReplaceReq struct {
	KbIDs []string `json:"kb_ids"`
}

// ReplaceByAgent 全量替换某智能体的知识库挂载（编辑页保存场景）
func (c *AgentKBBindingController) ReplaceByAgent(ctx *gin.Context) {
	aid, err := strconv.ParseUint(agentIDParam(ctx), 10, 64)
	if err != nil || aid == 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的智能体 ID")
		return
	}
	var req agentKBReplaceReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	kbIDs := make([]uint, 0, len(req.KbIDs))
	for _, s := range req.KbIDs {
		v, e := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
		if e != nil {
			response.Error(ctx, http.StatusBadRequest, "知识库 ID 非法: "+s)
			return
		}
		kbIDs = append(kbIDs, uint(v))
	}
	if err := c.svc.ReplaceByAgent(ctx.Request.Context(), uint(aid), kbIDs); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"agent_id": aid, "count": len(kbIDs)}, "同步成功")
}

// UnbindByPath 按路径参数解绑（兼容前端 DELETE /:agentId/:kbId）
func (c *AgentKBBindingController) UnbindByPath(ctx *gin.Context) {
	aid, err1 := strconv.ParseUint(ctx.Param("agentId"), 10, 64)
	kbId, err2 := strconv.ParseUint(ctx.Param("kbId"), 10, 64)
	if err1 != nil || err2 != nil || aid == 0 || kbId == 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的 agent_id 或 kb_id")
		return
	}
	if err := c.svc.Unbind(ctx.Request.Context(), uint(aid), uint(kbId)); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"agent_id": aid, "knowledge_base_id": kbId}, "解绑成功")
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

