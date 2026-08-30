package controller

import (
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// ManageTypingPredictController 管理端打字意图预测控制器
type ManageTypingPredictController struct {
	svc *service.TypingPredictService
}

// NewManageTypingPredictController 构造
func NewManageTypingPredictController() *ManageTypingPredictController {
	return &ManageTypingPredictController{svc: service.GetTypingPredictService()}
}

// Predict GET /api/manage/typing-predict?text=&session_id=
func (c *ManageTypingPredictController) Predict(ctx *gin.Context) {
	text := ctx.Query("text")
	sessionID := ctx.Query("session_id")
	if text == "" {
		response.Error(ctx, 400, "text 不能为空")
		return
	}
	pred, err := c.svc.Predict(ctx.Request.Context(), text, sessionID)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, pred, "ok")
}
