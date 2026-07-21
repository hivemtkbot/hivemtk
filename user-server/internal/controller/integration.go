package controller

import (
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// IntegrationController 第三方对接控制器
type IntegrationController struct {
	integrationService *service.IntegrationService
}

// NewIntegrationController 创建第三方对接控制器实例
func NewIntegrationController() *IntegrationController {
	return &IntegrationController{
		integrationService: service.NewIntegrationService(),
	}
}

// CreateAccount 创建对接账号
func (c *IntegrationController) CreateAccount(ctx *gin.Context) {

	var req service.CreateIntegrationAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	account, err := c.integrationService.CreateIntegrationAccount(&req)
	if HandleDBError(ctx, err, "创建对接账号") {
		return
	}

	response.Success(ctx, account, "创建成功")
}

// GetAccountList 获取对接账号列表
func (c *IntegrationController) GetAccountList(ctx *gin.Context) {

	accounts, err := c.integrationService.GetIntegrationAccountList()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, accounts, "获取成功")
}

// GetAccountByID 获取对接账号详情
func (c *IntegrationController) GetAccountByID(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.integrationService.GetIntegrationAccountByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, account, "获取成功")
}

// UpdateAccount 更新对接账号
func (c *IntegrationController) UpdateAccount(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	var req service.CreateIntegrationAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	account, err := c.integrationService.UpdateIntegrationAccount(uint(id), &req)
	if HandleDBError(ctx, err, "更新对接账号") {
		return
	}

	response.Success(ctx, account, "更新成功")
}

// DeleteAccount 删除对接账号
func (c *IntegrationController) DeleteAccount(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	if HandleDBError(ctx, c.integrationService.DeleteIntegrationAccount(uint(id)), "删除对接账号") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// SyncCustomers 同步客户数据
func (c *IntegrationController) SyncCustomers(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.integrationService.GetIntegrationAccountByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	count, err := c.integrationService.SyncCustomers(account)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{"count": count}, "同步成功")
}

// SyncOrders 同步订单数据
func (c *IntegrationController) SyncOrders(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.integrationService.GetIntegrationAccountByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	count, err := c.integrationService.SyncOrders(account)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{"count": count}, "同步成功")
}

// SyncProducts 同步商品数据
func (c *IntegrationController) SyncProducts(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.integrationService.GetIntegrationAccountByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	count, err := c.integrationService.SyncProducts(account)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{"count": count}, "同步成功")
}

// TestIntegration 测试对接账号连接
func (c *IntegrationController) TestIntegration(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.integrationService.GetIntegrationAccountByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	if err := c.integrationService.TestConnection(account); err != nil {
		response.Error(ctx, http.StatusBadRequest, "连接测试失败: "+err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"account_id": account.ID,
		"platform":   account.Platform,
		"status":     "ok",
	}, "连接测试成功")
}

// GetSyncLogs 获取同步日志
func (c *IntegrationController) GetSyncLogs(ctx *gin.Context) {

	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	logs, total, err := c.integrationService.GetSyncLogs(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetExternalCustomers 获取外部客户列表
func (c *IntegrationController) GetExternalCustomers(ctx *gin.Context) {

	platform := ctx.Query("platform")
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	customers, total, err := c.integrationService.GetExternalCustomers(platform, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      customers,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetExternalOrders 获取外部订单列表
func (c *IntegrationController) GetExternalOrders(ctx *gin.Context) {

	platform := ctx.Query("platform")
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	orders, total, err := c.integrationService.GetExternalOrders(platform, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      orders,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetExternalProducts 获取外部商品列表
func (c *IntegrationController) GetExternalProducts(ctx *gin.Context) {

	platform := ctx.Query("platform")
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	products, total, err := c.integrationService.GetExternalProducts(platform, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      products,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}
