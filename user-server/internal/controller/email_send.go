package controller

import (
	"context"
	"hivemtk-user/internal/dto"
	email "hivemtk-user/internal/email/service"
	"hivemtk-user/internal/pkg/utils/response"
	"net/http"

	"github.com/gin-gonic/gin"
)

type EmailSendController struct {
	svc *email.EmailSendService
}

func NewEmailSendController() *EmailSendController {
	return &EmailSendController{svc: email.NewEmailSendService()}
}

// 发送邮件
func (c *EmailSendController) SendEmail(ctx *gin.Context) {
	var req dto.SendEmailRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}

	email, err := c.svc.SendEmail(context.Background(), req)
	if err != nil {
		response.ErrorFromDB(ctx, err, "发送失败："+err.Error())
		return
	}

	response.Success(ctx, email, "发送成功")
}


