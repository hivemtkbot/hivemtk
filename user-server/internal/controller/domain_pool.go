package controller

import (
	"errors"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// DomainPoolController 域名池控制器
// G 域 P1：扩展健康度评分、平台黑名单、自动切换相关端点
type DomainPoolController struct {
	domainPoolService service.DomainPoolService
	healthService     service.DomainHealthService
}

// NewDomainPoolController 创建域名池控制器实例
func NewDomainPoolController(domainPoolService service.DomainPoolService, healthService service.DomainHealthService) *DomainPoolController {
	return &DomainPoolController{
		domainPoolService: domainPoolService,
		healthService:     healthService,
	}
}

// Create 创建域名池
func (c *DomainPoolController) Create(ctx *gin.Context) {
	var req dto.DomainPoolCreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	domainPool, err := c.domainPoolService.Create(req.Domain, req.Port, req.Purpose)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, domainPool, "创建成功")
}

// Update 更新域名池
func (c *DomainPoolController) Update(ctx *gin.Context) {
	var req dto.DomainPoolUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	domainPool, err := c.domainPoolService.Update(req.ID, req.Domain, req.Port, req.Purpose, req.Status)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, domainPool, "更新成功")
}

// Delete 删除域名池
func (c *DomainPoolController) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	err = c.domainPoolService.Delete(id)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetByID 根据ID获取域名池
func (c *DomainPoolController) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	domainPool, err := c.domainPoolService.GetByID(id)
	if HandleDBError(ctx, err, "获取域名池") {
		return
	}

	domainResponse := toDomainResponse(domainPool)
	response.Success(ctx, domainResponse, "获取成功")
}

// List 获取域名池列表
func (c *DomainPoolController) List(ctx *gin.Context) {
	var req dto.DomainPoolListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	domainPools, total, err := c.domainPoolService.List(req.Page, req.PageSize, req.Domain, req.Status)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	var list []dto.DomainPoolResponse
	for _, domainPool := range domainPools {
		list = append(list, toDomainResponse(domainPool))
	}

	response.Success(ctx, dto.DomainPoolListResponse{
		List:  list,
		Total: total,
	}, "获取成功")
}

// CheckDomain 检查单个域名是否可访问
func (c *DomainPoolController) CheckDomain(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	accessible, err := c.domainPoolService.CheckDomain(id)
	if HandleServiceError(ctx, err) {
		return
	}

	status := 2
	msg := "不可访问"
	if accessible {
		status = 1
		msg = "可访问"
	}

	response.Success(ctx, dto.DomainPoolCheckResponse{
		ID:     id,
		Status: status,
		Msg:    msg,
	}, "检查完成")
}

// CheckAllDomains 检查所有域名是否可访问
func (c *DomainPoolController) CheckAllDomains(ctx *gin.Context) {
	results, err := c.domainPoolService.CheckAllDomains()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, results, "检查完成")
}

// ============== G 域 P1 健康度自动切换 ==============

// HealthCheck 单个域名健康度探测（含评分）
// @Summary 健康度探测
// @Description DNS + HTTP HEAD + 黑名单综合探测，写入评分与日志
// @Tags 域名池
// @Param id path int true "域名 ID"
// @Success 200 {object} object{data=service.HealthCheckResult}
// @Router /api/domainpool/{id}/health-check [post]
func (c *DomainPoolController) HealthCheck(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	result, err := c.healthService.CheckOne(id)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, result, "探测完成")
}

// HealthCheckAll 全部域名健康度探测
// @Summary 全部域名健康度探测
// @Tags 域名池
// @Success 200 {object} object{data=[]service.HealthCheckResult}
// @Router /api/domainpool/health-check-all [post]
func (c *DomainPoolController) HealthCheckAll(ctx *gin.Context) {
	results, err := c.healthService.CheckAll()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, results, "探测完成")
}

// SwitchActive 手动切换活跃域名
// @Summary 切换活跃域名
// @Description 将指定域名标记为活跃（先 deactive 所有，再激活该域名）
// @Tags 域名池
// @Param id path int true "目标域名 ID"
// @Success 200 {object} object{data=model.DomainPool}
// @Router /api/domainpool/{id}/switch [post]
func (c *DomainPoolController) SwitchActive(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	if err := c.healthService.SwitchActive(id, "手动切换"); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	dp, err := c.domainPoolService.GetByID(id)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, toDomainResponse(dp), "切换成功")
}

// AutoSwitchBest 自动切换到评分最高的可用域名
// @Summary 自动切换到最优域名
// @Tags 域名池
// @Success 200 {object} object{data=model.DomainPool}
// @Router /api/domainpool/switch-best [post]
func (c *DomainPoolController) AutoSwitchBest(ctx *gin.Context) {
	best, err := c.healthService.SwitchToBest("API 触发自动切换")
	if err != nil {
		if errors.Is(err, errors.New("no available")) || (err != nil && err.Error() != "" && containsAny(err.Error(), "无可用", "no available")) {
			response.Error(ctx, http.StatusNotFound, err.Error())
			return
		}
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, toDomainResponse(best), "切换成功")
}

// GetActiveDomain 获取当前活跃域名
// @Summary 获取活跃域名
// @Tags 域名池
// @Success 200 {object} object{data=model.DomainPool}
// @Router /api/domainpool/active [get]
func (c *DomainPoolController) GetActiveDomain(ctx *gin.Context) {
	active, err := c.healthService.GetActiveDomain()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if active == nil {
		response.Success(ctx, nil, "当前无活跃域名")
		return
	}
	response.Success(ctx, toDomainResponse(active), "")
}

// ListAvailableDomains 列出可用域名（评分 >= minScore 且未在黑名单）
// @Summary 可用域名列表
// @Tags 域名池
// @Param min_score query int false "最低评分，默认 80"
// @Success 200 {object} object{data=[]model.DomainPool}
// @Router /api/domainpool/available [get]
func (c *DomainPoolController) ListAvailableDomains(ctx *gin.Context) {
	minScore, _ := strconv.Atoi(ctx.DefaultQuery("min_score", "80"))
	rows, err := c.healthService.ListAvailable(minScore)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]dto.DomainPoolResponse, 0, len(rows))
	for _, dp := range rows {
		out = append(out, toDomainResponse(dp))
	}
	response.Success(ctx, gin.H{"list": out, "total": len(out)}, "")
}

// ListHealthLogs 查询健康度日志
// @Summary 健康度日志
// @Tags 域名池
// @Param id path int true "域名 ID"
// @Param limit query int false "条数，默认 50"
// @Success 200 {object} object{data=[]model.DomainHealthLog}
// @Router /api/domainpool/{id}/health-log [get]
func (c *DomainPoolController) ListHealthLogs(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	logs, err := c.healthService.ListHealthLogs(id, limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"list": logs, "total": len(logs)}, "")
}

// AddBlacklistRequest 添加黑名单
type AddBlacklistRequest struct {
	Domain   string `json:"domain" binding:"required"`
	Platform string `json:"platform"`
	Reason   string `json:"reason"`
	Source   string `json:"source"`
	TTLHours int    `json:"ttl_hours"` // 0 表示永久
}

// AddBlacklist 添加平台黑名单
// @Summary 添加域名黑名单
// @Tags 域名池
// @Param body body AddBlacklistRequest true "黑名单"
// @Success 200 {object} object{message=string}
// @Router /api/domainpool/blacklist [post]
func (c *DomainPoolController) AddBlacklist(ctx *gin.Context) {
	var req AddBlacklistRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := c.domainPoolService.AddBlacklist(req.Domain, req.Platform, req.Reason, req.Source, req.TTLHours); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, nil, "已加入黑名单")
}

// RemoveBlacklist 移除黑名单
// @Summary 移除域名黑名单
// @Tags 域名池
// @Param domain path string true "域名"
// @Success 200 {object} object{message=string}
// @Router /api/domainpool/blacklist/{domain} [delete]
func (c *DomainPoolController) RemoveBlacklist(ctx *gin.Context) {
	domain := ctx.Param("domain")
	if domain == "" {
		response.Error(ctx, http.StatusBadRequest, "域名不能为空")
		return
	}
	if err := c.domainPoolService.RemoveBlacklist(domain); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, nil, "已移出黑名单")
}

// ListBlacklist 黑名单列表
// @Summary 域名黑名单
// @Tags 域名池
// @Success 200 {object} object{data=[]model.DomainBlacklist}
// @Router /api/domainpool/blacklist [get]
func (c *DomainPoolController) ListBlacklist(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	rows, total, err := c.domainPoolService.ListBlacklist(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"list": rows, "total": total, "page": page, "page_size": pageSize}, "")
}

// ============== 工具函数 ==============

// toDomainResponse model → dto
func toDomainResponse(dp *model.DomainPool) dto.DomainPoolResponse {
	return dto.DomainPoolResponse{
		ID:                  dp.ID,
		Domain:              dp.Domain,
		Port:                dp.Port,
		Purpose:             dp.Purpose,
		Status:              dp.Status,
		StatusStr:           getStatusStr(dp.Status),
		LastCheck:           dp.LastCheck.Format("2006-01-02 15:04:05"),
		CreatedAt:           dp.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:           dp.UpdatedAt.Format("2006-01-02 15:04:05"),
		HealthScore:         dp.HealthScore,
		ConsecutiveFailures: dp.ConsecutiveFailures,
		DNSResolved:         dp.DNSResolved,
		DNSError:            dp.DNSError,
		LastHTTPStatus:      dp.LastHTTPStatus,
		LastLatencyMs:       dp.LastLatencyMs,
		OnBlacklist:         dp.OnBlacklist,
		AutoSwitchEnabled:   dp.AutoSwitchEnabled,
		IsActive:            dp.IsActive,
	}
}

// getStatusStr 获取状态字符串
func getStatusStr(status int) string {
	switch status {
	case 1:
		return "正常"
	case 2:
		return "不可访问"
	case 3:
		return "风险"
	case 4:
		return "已停用"
	default:
		return "未知"
	}
}

// containsAny 简易字符串包含判断
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) == 0 {
			continue
		}
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}
