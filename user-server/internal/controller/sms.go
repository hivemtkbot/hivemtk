package controller

import (
	"context"
	"net/http"
	"strconv"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/pagination"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// SmsController 短信控制器
type SmsController struct {
	service service.SmsService
}

// NewSmsController 创建短信控制器
func NewSmsController(service service.SmsService) *SmsController {
	return &SmsController{service: service}
}

// RegisterRoutes 注册路由
func (c *SmsController) RegisterRoutes(router *gin.RouterGroup) {
	sms := router.Group("/sms")
	{
		// 配置相关
		sms.GET("/config", c.GetConfig)
		sms.POST("/config", c.SaveConfig)

		// 短信记录相关
		sms.GET("/list", c.GetSmsList)
		sms.GET("/detail/:id", c.GetSmsDetail)
		sms.POST("/send", c.SendSms)
		sms.POST("/resend/:id", c.ResendSms)

		// 草稿相关
		sms.GET("/draft/list", c.GetDraftList)
		sms.GET("/draft/:id", c.GetDraft)
		sms.POST("/draft", c.CreateDraft)
		sms.PUT("/draft/:id", c.UpdateDraft)
		sms.DELETE("/draft/:id", c.DeleteDraft)
		sms.POST("/draft/send/:id", c.SendDraft)
		sms.POST("/draft/:id/send", c.SendDraft)

		// 任务相关
		sms.GET("/job/list", c.GetJobList)
		sms.GET("/job/:id", c.GetJob)
		sms.POST("/job", c.CreateJob)
		sms.POST("/job/pause/:id", c.PauseJob)
		sms.POST("/job/resume/:id", c.ResumeJob)
		sms.POST("/job/stop/:id", c.StopJob)
		sms.POST("/job/:id/pause", c.PauseJob)
		sms.POST("/job/:id/resume", c.ResumeJob)
		sms.POST("/job/:id/stop", c.StopJob)
		sms.DELETE("/job/:id", c.DeleteJob)
		sms.GET("/job/:id/records", c.GetJobRecords)
	}
}

// 移除自定义响应结构，使用统一的响应工具函数

// GetConfig 获取短信配置
func (c *SmsController) GetConfig(ctx *gin.Context) {
	config, err := c.service.GetConfig(context.Background())
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取配置失败: "+err.Error())
		return
	}

	response.Success(ctx, config, "success")
}

// SaveConfig 保存短信配置
func (c *SmsController) SaveConfig(ctx *gin.Context) {
	var req dto.SmsConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	if err := c.service.SaveConfig(context.Background(), &req); err != nil {
		response.ErrorFromDB(ctx, err, "保存配置失败: "+err.Error())
		return
	}

	response.Success(ctx, nil, "success")
}

// DeleteJob 删除任务
func (c *SmsController) DeleteJob(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	if HandleDBError(ctx, c.service.DeleteJob(context.Background(), uint(id)), "删除任务") {
		return
	}

	response.Success(ctx, nil, "success")
}

// GetJobRecords 获取任务记录
func (c *SmsController) GetJobRecords(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	// 解析分页参数
	page, limit, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	records, total, err := c.service.GetJobRecords(context.Background(), uint(id), page, limit)
	if HandleDBError(ctx, err, "获取任务记录") {
		return
	}

	response.Success(ctx, gin.H{
		"list":  records,
		"total": total,
	}, "success")
}

// GetSmsList 获取短信列表
func (c *SmsController) GetSmsList(ctx *gin.Context) {
	var req dto.SmsListRequest

	// 解析分页参数
	page, limit, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	phone := ctx.Query("phone")
	status := ctx.Query("status")
	startDate := ctx.Query("startDate")
	endDate := ctx.Query("endDate")

	req = dto.SmsListRequest{
		Page:      page,
		Limit:     limit,
		Phone:     phone,
		Status:    status,
		StartDate: startDate,
		EndDate:   endDate,
	}

	list, total, err := c.service.GetSmsList(context.Background(), &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取短信列表失败: "+err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":  list,
		"total": total,
	}, "success")
}

// GetSmsDetail 获取短信详情
func (c *SmsController) GetSmsDetail(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	sms, err := c.service.GetSmsByID(context.Background(), uint(id))
	if HandleDBError(ctx, err, "获取短信详情") {
		return
	}

	response.Success(ctx, sms, "success")
}

// SendSms 发送短信
func (c *SmsController) SendSms(ctx *gin.Context) {
	var req dto.SmsSendRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	// 在调用外部短信服务前，验证短信提供商凭证是否已配置
	config, err := c.service.GetConfig(context.Background())
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "SMS service not configured")
		return
	}
	configured, err := c.service.IsProviderConfigured(context.Background(), config.DefaultProvider)
	if err != nil || !configured {
		response.Error(ctx, http.StatusBadRequest, "SMS service not configured")
		return
	}

	if err := c.service.SendSms(context.Background(), &req); err != nil {
		response.ErrorFromDB(ctx, err, "发送短信失败: "+err.Error())
		return
	}

	response.Success(ctx, nil, "success")
}

// ResendSms 重发短信
func (c *SmsController) ResendSms(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	if HandleServiceError(ctx, c.service.ResendSms(context.Background(), uint(id))) {
		return
	}

	response.Success(ctx, nil, "success")
}

// GetDraftList 获取草稿列表
func (c *SmsController) GetDraftList(ctx *gin.Context) {
	var req dto.SmsDraftListRequest

	// 解析分页参数
	page, limit, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	title := ctx.Query("title")

	req = dto.SmsDraftListRequest{
		Page:  page,
		Limit: limit,
		Title: title,
	}

	list, total, err := c.service.GetDraftList(context.Background(), &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取草稿列表失败: "+err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":  list,
		"total": total,
	}, "success")
}

// GetDraft 获取草稿详情
func (c *SmsController) GetDraft(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	draft, err := c.service.GetDraftByID(context.Background(), uint(id))
	if HandleDBError(ctx, err, "获取草稿详情") {
		return
	}

	response.Success(ctx, draft, "success")
}

// CreateDraft 创建草稿
func (c *SmsController) CreateDraft(ctx *gin.Context) {
	var req dto.SmsDraftCreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	if err := c.service.CreateDraft(context.Background(), &req); err != nil {
		response.ErrorFromDB(ctx, err, "创建草稿失败: "+err.Error())
		return
	}

	response.Success(ctx, nil, "success")
}

// UpdateDraft 更新草稿
func (c *SmsController) UpdateDraft(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	var req dto.SmsDraftUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	if HandleDBError(ctx, c.service.UpdateDraft(context.Background(), uint(id), &req), "更新草稿") {
		return
	}

	response.Success(ctx, nil, "success")
}

// DeleteDraft 删除草稿
func (c *SmsController) DeleteDraft(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	if HandleDBError(ctx, c.service.DeleteDraft(context.Background(), uint(id)), "删除草稿") {
		return
	}

	response.Success(ctx, nil, "success")
}

// SendDraft 发送草稿
func (c *SmsController) SendDraft(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	// 获取手机号参数
	phone := ctx.PostForm("phone")
	if phone == "" {
		// 尝试从JSON体中获取
		var req struct {
			Phone string `json:"phone" binding:"required"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			response.Error(ctx, http.StatusBadRequest, "手机号不能为空")
			return
		}
		phone = req.Phone
	}

	if HandleDBError(ctx, c.service.SendDraft(context.Background(), uint(id), phone), "发送草稿") {
		return
	}

	response.Success(ctx, nil, "success")
}

// GetJobList 获取任务列表
func (c *SmsController) GetJobList(ctx *gin.Context) {
	var req dto.SmsJobListRequest

	// 解析分页参数
	page, limit, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	status := ctx.Query("status")
	name := ctx.Query("name")

	req = dto.SmsJobListRequest{
		Page:   page,
		Limit:  limit,
		Status: status,
		Name:   name,
	}

	list, total, err := c.service.GetJobList(context.Background(), &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取任务列表失败: "+err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":  list,
		"total": total,
	}, "success")
}

// GetJob 获取任务详情
func (c *SmsController) GetJob(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	job, err := c.service.GetJobByID(context.Background(), uint(id))
	if HandleDBError(ctx, err, "获取任务详情") {
		return
	}

	response.Success(ctx, job, "success")
}

// CreateJob 创建任务
func (c *SmsController) CreateJob(ctx *gin.Context) {
	var req dto.SmsJobCreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	if err := c.service.CreateJob(context.Background(), &req); err != nil {
		response.ErrorFromDB(ctx, err, "创建任务失败: "+err.Error())
		return
	}

	response.Success(ctx, nil, "success")
}

// PauseJob 暂停任务
func (c *SmsController) PauseJob(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	if HandleDBError(ctx, c.service.PauseJob(context.Background(), uint(id)), "暂停任务") {
		return
	}

	response.Success(ctx, nil, "success")
}

// ResumeJob 继续任务
func (c *SmsController) ResumeJob(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	if HandleDBError(ctx, c.service.ResumeJob(context.Background(), uint(id)), "继续任务") {
		return
	}

	response.Success(ctx, nil, "success")
}

// StopJob 停止任务
func (c *SmsController) StopJob(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	if HandleDBError(ctx, c.service.StopJob(context.Background(), uint(id)), "停止任务") {
		return
	}

	response.Success(ctx, nil, "success")
}
