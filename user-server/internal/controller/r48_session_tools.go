// r48_session_tools.go R48 T4/T5 控制器（宏 + AI 会话摘要）
package controller

import (
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// MacroController 宏控制器
type MacroController struct {
	svc *service.MacroService
}

// NewMacroController 构造
func NewMacroController() *MacroController {
	return &MacroController{svc: service.NewMacroServiceFromGlobal()}
}

// List GET /api/macros
func (c *MacroController) List(ctx *gin.Context) {
	list, err := c.svc.List(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// Create POST /api/macros {name, actions:[{type,value}]}
func (c *MacroController) Create(ctx *gin.Context) {
	var req struct {
		Name    string                `json:"name" binding:"required"`
		Actions []service.MacroAction `json:"actions" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}
	m, err := c.svc.Create(ctx.Request.Context(), req.Name, req.Actions)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, m, "宏已创建")
}

// Delete DELETE /api/macros/:id
func (c *MacroController) Delete(ctx *gin.Context) {
	id, ok := parseUintParam(ctx, "id")
	if !ok {
		return
	}
	if err := c.svc.Delete(ctx.Request.Context(), id); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"deleted": true}, "已删除")
}

// Apply POST /api/macros/:id/apply {session_id}
func (c *MacroController) Apply(ctx *gin.Context) {
	id, ok := parseUintParam(ctx, "id")
	if !ok {
		return
	}
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
		Operator  string `json:"operator"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "session_id 必填")
		return
	}
	res, err := c.svc.Apply(ctx.Request.Context(), id, req.SessionID, req.Operator)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, "宏已执行")
}

// SessionAIController AI 摘要控制器
type SessionAIController struct {
	svc *service.SessionAIService
}

// NewSessionAIController 构造
func NewSessionAIController(svc *service.SessionAIService) *SessionAIController {
	return &SessionAIController{svc: svc}
}

// Generate POST /api/customer-sessions/:id/ai-summary
func (c *SessionAIController) Generate(ctx *gin.Context) {
	rec, err := c.svc.Generate(ctx.Request.Context(), ctx.Param("id"))
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, rec, "摘要已生成")
}

// Get GET /api/customer-sessions/:id/ai-summary
func (c *SessionAIController) Get(ctx *gin.Context) {
	rec, ok, err := c.svc.GetLatest(ctx.Request.Context(), ctx.Param("id"))
	if HandleServiceError(ctx, err) {
		return
	}
	if !ok {
		response.Success(ctx, nil, "尚未生成摘要")
		return
	}
	response.Success(ctx, rec, "ok")
}
