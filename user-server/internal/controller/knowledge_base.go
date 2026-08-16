package controller


import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// KnowledgeBaseController 知识库控制器
type KnowledgeBaseController struct {
	svc *service.KnowledgeBaseService
}

// NewKnowledgeBaseController 创建知识库控制器
func NewKnowledgeBaseController() *KnowledgeBaseController {
	return &KnowledgeBaseController{
		svc: service.NewKnowledgeBaseServiceDefault(),
	}
}

// RegisterRoutes 注册路由
func (c *KnowledgeBaseController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/knowledge-bases")
	{
		g.GET("", c.List)
		g.GET("/by-agent/:aid", c.ListByAgent)
		g.GET("/by-type/:type", c.ListByType)
		g.GET("/:id", c.Get)
		g.POST("", c.Create)
		g.PUT("/:id", c.Update)
		g.DELETE("/:id", c.Delete)
		g.POST("/:id/bind", c.BindToAgent)
		g.POST("/:id/unbind", c.UnbindFromAgent)
	}
}

// List godoc
// @Summary      知识库列表
// @Description  按类型/所有者/关键词分页查询知识库
// @Tags         Knowledge Base
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        type        query  string  false  "知识库类型：product/faq/policy"
// @Param        owner_type  query  string  false  "所有者类型"
// @Param        agent_id    query  int     false  "绑定的智能体 ID"
// @Param        keyword     query  string  false  "关键词"
// @Success      200  {object}  response.Response  "成功"
// @Router       /api/knowledge-bases [get]
func (c *KnowledgeBaseController) List(ctx *gin.Context) {
	kbType := ctx.Query("type")
	ownerType := ctx.Query("owner_type")
	keyword := ctx.Query("keyword")
	var agentID uint
	if v := ctx.Query("agent_id"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			response.Error(ctx, http.StatusBadRequest, "无效的 agent_id")
			return
		}
		agentID = uint(n)
	}
	list, total, err := c.svc.ListKBs(ctx.Request.Context(), kbType, ownerType, agentID, keyword)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"list":  list,
		"total": total,
	}, "查询成功")
}

// Get godoc
// @Summary      知识库详情
// @Description  根据 ID 返回知识库完整定义
// @Tags         Knowledge Base
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "知识库 ID"
// @Success      200  {object}  response.Response  "成功"
// @Failure      404  {object}  response.Response  "未找到"
// @Router       /api/knowledge-bases/{id} [get]
func (c *KnowledgeBaseController) Get(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	kb, err := c.svc.GetKB(ctx.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(ctx, "知识库不存在")
		return
	}
	if kb == nil {
		response.NotFound(ctx, "知识库不存在")
		return
	}
	response.Success(ctx, kb, "查询成功")
}

// ListByAgent 查某智能体可用的知识库
func (c *KnowledgeBaseController) ListByAgent(ctx *gin.Context) {
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

// ListByType 按类型查
func (c *KnowledgeBaseController) ListByType(ctx *gin.Context) {
	kbType := ctx.Param("type")
	if !service.IsValidKBType(kbType) {
		response.Error(ctx, http.StatusBadRequest, "type 必须为 faq/rag/sop")
		return
	}
	list, err := c.svc.ListByType(ctx.Request.Context(), kbType)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, list, "查询成功")
}

// knowledgeBaseCreateReq 创建请求体
type knowledgeBaseCreateReq struct {
	KBCode       string `json:"kb_code" binding:"required"`
	Type         string `json:"type" binding:"required"`
	Name         string `json:"name" binding:"required"`
	Description  string `json:"description"`
	OwnerType    string `json:"owner_type"`
	OwnerAgentID *uint  `json:"owner_agent_id"`
	Enabled      *bool  `json:"enabled"`
}

// Create 新增
func (c *KnowledgeBaseController) Create(ctx *gin.Context) {
	var req knowledgeBaseCreateReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	kb := &model.KnowledgeBase{
		KBCode:       req.KBCode,
		Type:         req.Type,
		Name:         req.Name,
		Description:  req.Description,
		OwnerType:    req.OwnerType,
		OwnerAgentID: req.OwnerAgentID,
		Enabled:      req.Enabled,
	}
	if err := c.svc.CreateKB(ctx.Request.Context(), kb); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, kb, "创建成功")
}

// knowledgeBaseUpdateReq 更新请求体 (全量替换字段)
type knowledgeBaseUpdateReq struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	OwnerType    string `json:"owner_type"`
	OwnerAgentID *uint  `json:"owner_agent_id"`
	Enabled      *bool  `json:"enabled"`
}

// Update 更新
func (c *KnowledgeBaseController) Update(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	var req knowledgeBaseUpdateReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	kb := &model.KnowledgeBase{
		Type:         req.Type,
		Name:         req.Name,
		Description:  req.Description,
		OwnerType:    req.OwnerType,
		OwnerAgentID: req.OwnerAgentID,
		Enabled:      req.Enabled,
	}
	if err := c.svc.UpdateKB(ctx.Request.Context(), uint(id), kb); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "更新成功")
}

// Delete 删除 (业务级联: 同步删除 agent_kb_bindings)
func (c *KnowledgeBaseController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	if err := c.svc.DeleteKB(ctx.Request.Context(), uint(id)); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "删除成功")
}

// knowledgeBaseBindReq 绑定请求体
type knowledgeBaseBindReq struct {
	AgentID uint `json:"agent_id" binding:"required"`
}

// BindToAgent 绑定到智能体
func (c *KnowledgeBaseController) BindToAgent(ctx *gin.Context) {
	kbID, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	var req knowledgeBaseBindReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.AgentID == 0 {
		response.Error(ctx, http.StatusBadRequest, "agent_id 必填且 > 0")
		return
	}
	if err := c.svc.BindToAgent(ctx.Request.Context(), uint(kbID), req.AgentID); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"kb_id": kbID, "agent_id": req.AgentID}, "绑定成功")
}

// UnbindFromAgent 从智能体解绑
func (c *KnowledgeBaseController) UnbindFromAgent(ctx *gin.Context) {
	kbID, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的知识库 ID")
		return
	}
	var req knowledgeBaseBindReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.AgentID == 0 {
		response.Error(ctx, http.StatusBadRequest, "agent_id 必填且 > 0")
		return
	}
	if err := c.svc.UnbindFromAgent(ctx.Request.Context(), uint(kbID), req.AgentID); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"kb_id": kbID, "agent_id": req.AgentID}, "解绑成功")
}

