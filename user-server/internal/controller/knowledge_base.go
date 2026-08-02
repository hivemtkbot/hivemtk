package controller

// knowledge_base_controller.go 知识库 L3 Controller
//
// 五层架构归属: L3 API 接入层
// 设计依据: 强 1对1 改造 (知识库管理)
//
// 接口前缀: /api/knowledge-bases
//   GET    /api/knowledge-bases                列表查询 (前端知识库管理页面)
//   GET    /api/knowledge-bases/:id            详情
//   POST   /api/knowledge-bases                新增
//   PUT    /api/knowledge-bases/:id            更新
//   DELETE /api/knowledge-bases/:id            删除 (业务级联: 同步删除 agent_kb_bindings)
//   GET    /api/knowledge-bases/by-agent/:aid  查某智能体可用 KB
//   GET    /api/knowledge-bases/by-type/:type  按类型查
//   POST   /api/knowledge-bases/:id/bind       绑定到智能体
//   POST   /api/knowledge-bases/:id/unbind     从智能体解绑
//
// 业务规则:
//   - owner_type=private 时 owner_agent_id 必填
//   - owner_type=shared  时 owner_agent_id 必为空
//   - type 必为 faq / rag / sop

import (
	"net/http"
	"strconv"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// KnowledgeBaseController 知识库控制器
type KnowledgeBaseController struct {
	svc *service.KnowledgeBaseService
}

// NewKnowledgeBaseController 创建知识库控制器
func NewKnowledgeBaseController(db *gorm.DB) *KnowledgeBaseController {
	return &KnowledgeBaseController{
		svc: service.NewKnowledgeBaseService(db),
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

// List 列表查询
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"list":  list,
		"total": total,
	}, "查询成功")
}

// Get 详情
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, list, "查询成功")
}

// knowledgeBaseCreateReq 创建请求体
type knowledgeBaseCreateReq struct {
	KBCode      string `json:"kb_code" binding:"required"`
	Type        string `json:"type" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	OwnerType   string `json:"owner_type"`
	OwnerAgentID *uint `json:"owner_agent_id"`
	Enabled     *bool  `json:"enabled"`
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"kb_id": kbID, "agent_id": req.AgentID}, "解绑成功")
}
