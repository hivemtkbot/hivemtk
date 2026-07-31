package controller

// rag_safety_guard_controller.go RAG 内容风控卫士控制器
//
// 五层架构归属: L3 业务层
//
// 路由（全部鉴权）：
//   - POST /api/rag/safety/check           内容风控检测
//   - GET  /api/rag/safety/lexicon         查看词库
//   - POST /api/rag/safety/lexicon         替换词库
//   - POST /api/rag/safety/sensitive       新增敏感词
//   - POST /api/rag/safety/competitor      新增竞品词

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// RagSafetyGuardController 内容风控控制器
type RagSafetyGuardController struct {
	svc *service.RagSafetyGuardService
}

// NewRagSafetyGuardController 创建控制器
func NewRagSafetyGuardController(svc *service.RagSafetyGuardService) *RagSafetyGuardController {
	return &RagSafetyGuardController{svc: svc}
}

// SafetyCheckRequest 检测请求
type SafetyCheckRequest struct {
	UserID  string                 `json:"userId" binding:"required"`
	AgentID string                 `json:"agentId"` // 智能体 ID (原 tenantId, 私域唯一隔离维度)
	Content string                 `json:"content"`
	Stage   string                 `json:"stage"`
	Sources []service.SafetySource `json:"sources"`
}

// SafetyCheck godoc
// @Summary      RAG 内容风控检测
// @Description  对内容做敏感词 / 广告法 / 竞品 / 画像越权 4 项检测
// @Tags         RAG Safety
// @Accept       json
// @Produce      json
// @Param        body  body      SafetyCheckRequest  true  "检测请求"
// @Success      200   {object}  service.SafetyCheckResult
// @Router       /api/rag/safety/check [post]
func (c *RagSafetyGuardController) SafetyCheck(ctx *gin.Context) {
	var req SafetyCheckRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	res, err := c.svc.Check(ctx.Request.Context(), &service.SafetyCheckRequest{
		UserID:   req.UserID,
		AgentID:  req.AgentID,
		Content:  req.Content,
		Stage:    req.Stage,
		Sources:  req.Sources,
	})
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "检测失败："+err.Error())
		return
	}
	response.Success(ctx, res, "ok")
}

// GetLexicon godoc
// @Summary      查看风控词库
// @Description  返回当前生效的敏感词 / 广告法 / 竞品词列表
// @Tags         RAG Safety
// @Produce      json
// @Success      200  {object}  service.SafetyLexicon
// @Router       /api/rag/safety/lexicon [get]
func (c *RagSafetyGuardController) GetLexicon(ctx *gin.Context) {
	response.Success(ctx, c.svc.GetLexicon(context.Background()), "ok")
}

// UpdateLexiconRequest 替换词库
type UpdateLexiconRequest struct {
	SensitiveWords  []string `json:"sensitive_words"`
	AdPhrases       []string `json:"ad_phrases"`
	CompetitorWords []string `json:"competitor_words"`
}

// UpdateLexicon godoc
// @Summary      替换风控词库
// @Description  整体替换词库（管理后台使用）
// @Tags         RAG Safety
// @Accept       json
// @Produce      json
// @Param        body  body      UpdateLexiconRequest  true  "词库"
// @Success      200   {object}  map[string]string
// @Router       /api/rag/safety/lexicon [post]
func (c *RagSafetyGuardController) UpdateLexicon(ctx *gin.Context) {
	var req UpdateLexiconRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	c.svc.SetLexicon(context.Background(), service.SafetyLexicon{
		SensitiveWords:  req.SensitiveWords,
		AdPhrases:       req.AdPhrases,
		CompetitorWords: req.CompetitorWords,
	})
	response.Success(ctx, gin.H{"updatedAt": c.svc.LastUpdate(context.Background())}, "ok")
}

// AddWordRequest 新增词
type AddWordRequest struct {
	Word string `json:"word" binding:"required"`
}

// AddSensitiveWord godoc
// @Summary      新增敏感词
// @Tags         RAG Safety
// @Accept       json
// @Produce      json
// @Param        body  body      AddWordRequest  true  "词"
// @Success      200   {object}  map[string]string
// @Router       /api/rag/safety/sensitive [post]
func (c *RagSafetyGuardController) AddSensitiveWord(ctx *gin.Context) {
	var req AddWordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	if err := c.svc.AddSensitiveWord(context.Background(), req.Word); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{"word": req.Word}, "已添加")
}

// AddCompetitorWord godoc
// @Summary      新增竞品词
// @Tags         RAG Safety
// @Accept       json
// @Produce      json
// @Param        body  body      AddWordRequest  true  "词"
// @Success      200   {object}  map[string]string
// @Router       /api/rag/safety/competitor [post]
func (c *RagSafetyGuardController) AddCompetitorWord(ctx *gin.Context) {
	var req AddWordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	if err := c.svc.AddCompetitorWord(context.Background(), req.Word); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{"word": req.Word}, "已添加")
}

// RegisterRoutes 注册路由
func (c *RagSafetyGuardController) RegisterRoutes(auth *gin.RouterGroup) {
	group := auth.Group("/rag/safety")
	{
		group.POST("/check", c.SafetyCheck)
		group.GET("/lexicon", c.GetLexicon)
		group.POST("/lexicon", c.UpdateLexicon)
		group.POST("/sensitive", c.AddSensitiveWord)
		group.POST("/competitor", c.AddCompetitorWord)
	}
}
