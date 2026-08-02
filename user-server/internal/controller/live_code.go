package controller

import (
	"context"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/reach/card/template"
	"marketing/internal/service"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// LiveCodeController 活码控制器
type LiveCodeController struct {
	liveCodeService service.LiveCodeService
}

// NewLiveCodeController 创建活码控制器实例
func NewLiveCodeController(liveCodeService service.LiveCodeService) *LiveCodeController {
	return &LiveCodeController{
		liveCodeService: liveCodeService,
	}
}

// Create 创建活码
func (c *LiveCodeController) Create(ctx *gin.Context) {
	var req dto.CreateLiveCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	liveCode, err := c.liveCodeService.Create(context.Background(), &req)
	if HandleDBError(ctx, err, "创建活码") {
		return
	}

	response.Success(ctx, liveCode, "创建成功")
}

// Update 更新活码
func (c *LiveCodeController) Update(ctx *gin.Context) {
	idStr := ctx.Param("id")

	var req dto.UpdateLiveCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	liveCode, err := c.liveCodeService.Update(context.Background(), idStr, &req)
	if HandleDBError(ctx, err, "更新活码") {
		return
	}

	response.Success(ctx, liveCode, "更新成功")
}

// Delete 删除活码
func (c *LiveCodeController) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")

	err := c.liveCodeService.Delete(context.Background(), idStr)
	if HandleDBError(ctx, err, "删除活码") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetByID 根据ID获取活码
func (c *LiveCodeController) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")

	liveCode, err := c.liveCodeService.GetByID(context.Background(), idStr)
	if HandleDBError(ctx, err, "获取活码") {
		return
	}

	response.Success(ctx, liveCode, "获取成功")
}

// GetList 获取活码列表
func (c *LiveCodeController) GetList(ctx *gin.Context) {
	pageStr := ctx.DefaultQuery("page", "1")
	pageSizeStr := ctx.DefaultQuery("pageSize", "10")
	name := ctx.Query("name")
	status := ctx.Query("status")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	liveCodes, total, err := c.liveCodeService.GetList(context.Background(), page, pageSize, name, status)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, dto.LiveCodeListResponse{
		List:  liveCodes,
		Total: total,
	}, "获取成功")
}

// GetStats 获取活码统计
func (c *LiveCodeController) GetStats(ctx *gin.Context) {
	idStr := ctx.Param("id")

	stats, err := c.liveCodeService.GetStats(context.Background(), idStr)
	if HandleDBError(ctx, err, "获取活码统计") {
		return
	}

	response.Success(ctx, stats, "获取成功")
}

// GenerateQRCode 生成活码二维码
func (c *LiveCodeController) GenerateQRCode(ctx *gin.Context) {
	idStr := ctx.Param("id")

	var req dto.GenerateQRCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	qrCode, err := c.liveCodeService.GenerateQRCode(context.Background(), idStr, &req)
	if HandleDBError(ctx, err, "生成活码二维码") {
		return
	}

	response.Success(ctx, qrCode, "生成成功")
}

// GetQRCodes 获取活码二维码列表
func (c *LiveCodeController) GetQRCodes(ctx *gin.Context) {
	idStr := ctx.Param("id")

	qrCodes, err := c.liveCodeService.GetQRCodes(context.Background(), idStr)
	if HandleDBError(ctx, err, "获取活码二维码列表") {
		return
	}

	response.Success(ctx, qrCodes, "获取成功")
}

// GetQRStats 获取活码二维码统计
func (c *LiveCodeController) GetQRStats(ctx *gin.Context) {
	qrIDStr := ctx.Param("qrId")

	stats, err := c.liveCodeService.GetQRStats(context.Background(), qrIDStr)
	if HandleDBError(ctx, err, "获取活码二维码统计") {
		return
	}

	response.Success(ctx, stats, "获取成功")
}

// Share 分享活码
func (c *LiveCodeController) Share(ctx *gin.Context) {
	idStr := ctx.Param("id")

	req := dto.ShareLiveCodeRequest{
		IPAddress: ctx.ClientIP(),
		UserAgent: ctx.GetHeader("User-Agent"),
	}

	shareResponse, err := c.liveCodeService.Share(context.Background(), idStr, &req)
	if HandleDBError(ctx, err, "分享活码") {
		return
	}

	response.Success(ctx, shareResponse, "分享成功")
}

// RedirectLiveCode 活码短链重定向
func (c *LiveCodeController) RedirectLiveCode(ctx *gin.Context) {
	code := ctx.Param("code")
	if code == "" {
		ctx.String(http.StatusBadRequest, "无效的短链")
		return
	}

	// 根据短链获取活码
	liveCode, err := c.liveCodeService.GetByShortLink(context.Background(), code)
	if err != nil {
		ctx.String(http.StatusNotFound, "活码不存在")
		return
	}

	// 检查活码状态
	if liveCode.Status != 1 {
		ctx.String(http.StatusNotFound, "活码已停用")
		return
	}

	// 分享活码（增加访问量等）
	req := &dto.ShareLiveCodeRequest{
		IPAddress: ctx.ClientIP(),
		UserAgent: ctx.GetHeader("User-Agent"),
	}

	response, err := c.liveCodeService.Share(context.Background(), liveCode.ID, req)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "分享失败: %v", err)
		return
	}

	// 根据User-Agent判断设备类型，决定重定向到入口链接还是落地链接
	userAgent := strings.ToLower(ctx.GetHeader("User-Agent"))

	// 如果是微信浏览器，重定向到入口链接
	if strings.Contains(userAgent, "micromessenger") {
		ctx.Redirect(http.StatusMovedPermanently, response.EntryLink)
		return
	}

	// 其他浏览器重定向到落地链接
	ctx.Redirect(http.StatusMovedPermanently, response.LandingLink)
}

// RenderLiveCodePage 渲染活码页面
func (c *LiveCodeController) RenderLiveCodePage(ctx *gin.Context) {
	// 获取活码ID
	idStr := ctx.Param("id")

	// 获取活码信息
	liveCode, err := c.liveCodeService.GetByID(context.Background(), idStr)
	if err != nil {
		ctx.String(http.StatusNotFound, "活码不存在")
		return
	}

	// 获取活码统计
	stats, err := c.liveCodeService.GetStats(context.Background(), idStr)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "获取活码统计失败")
		return
	}

	// 获取二维码数量
	qrCodes, err := c.liveCodeService.GetQRCodes(context.Background(), idStr)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "获取二维码列表失败")
		return
	}

	// 准备模板数据
	templateData := &template.LiveCodeTemplateData{
		ID:          liveCode.ID,
		Title:       liveCode.Name,
		Description: "点击跳转到目标页面",
		ImageURL:    liveCode.ImageURL,
		EntryURL:    liveCode.EntryURL,
		LandingURL:  liveCode.LandingURL,
		ShowStats:   true,
		ShowQR:      len(qrCodes) > 0,
		TotalClicks: stats.TotalClicks,
		TodayClicks: stats.TodayClicks,
		QRCount:     len(qrCodes),
	}

	// 如果有二维码，使用第一个二维码的图片
	if len(qrCodes) > 0 {
		templateData.QRImageURL = qrCodes[0].QRImageURL
	}

	// 创建模板服务
	templateService := template.NewTemplateService("internal/template")

	// 生成HTML页面
	html, err := templateService.GenerateLiveCodePage(templateData)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "生成页面失败")
		return
	}

	// 返回HTML页面
	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.String(http.StatusOK, html)
}

// RecordClick 记录点击统计
func (c *LiveCodeController) RecordClick(ctx *gin.Context) {
	// 获取活码ID
	idStr := ctx.Param("id")

	// 获取请求数据
	var req struct {
		UserAgent string `json:"user_agent"`
		Referrer  string `json:"referrer"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "请求参数错误")
		return
	}

	// 记录点击统计
	err := c.liveCodeService.RecordClick(context.Background(), idStr, req.UserAgent, req.Referrer)
	if err != nil {
		response.ErrorFromDB(ctx, err, "记录点击统计失败")
		return
	}

	response.Success(ctx, nil, "记录成功")
}

// DeleteLiveCodeQR 删除活码二维码
func (c *LiveCodeController) DeleteLiveCodeQR(ctx *gin.Context) {
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, 400, "二维码ID不能为空")
		return
	}

	err := c.liveCodeService.DeleteQRCode(context.Background(), idStr)
	if err != nil {
		if isNotFoundError(err) {
			response.Error(ctx, http.StatusNotFound, err.Error())
			return
		}
		response.ErrorFromDB(ctx, err, "删除二维码失败", err.Error())
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// UpdateLiveCodeQR 更新活码二维码
func (c *LiveCodeController) UpdateLiveCodeQR(ctx *gin.Context) {
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, 400, "二维码ID不能为空")
		return
	}

	var req dto.UpdateLiveCodeQRRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "请求参数错误", err.Error())
		return
	}

	err := c.liveCodeService.UpdateQRCode(context.Background(), idStr, &req)
	if HandleDBError(ctx, err, "更新二维码") {
		return
	}

	response.Success(ctx, nil, "更新成功")
}
