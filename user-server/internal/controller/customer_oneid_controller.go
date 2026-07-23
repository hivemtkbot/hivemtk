package controller

import (
	"net/http"

	"marketing/internal/identity"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// CustomerOneIDController 客户 360 OneID 控制器
// 负责多渠道身份合并、冲突解决与身份映射查询
//
// P2-2 修复：严格遵循五层架构 Controller → Service → Repository → Model，
// 移除原先对 repository.CustomerRepository 的直接依赖，改为通过
// service.CustomerQueryService 访问数据。
type CustomerOneIDController struct {
	identitySvc  *service.CustomerIdentityService
	custQuerySvc *service.CustomerQueryService
}

// NewCustomerOneIDController 创建 OneID 控制器
func NewCustomerOneIDController() *CustomerOneIDController {
	return &CustomerOneIDController{
		identitySvc:  service.NewCustomerIdentityService(),
		custQuerySvc: service.NewCustomerQueryService(),
	}
}

// mergeRequest 合并身份请求
type mergeRequest struct {
	PrimaryID   string `json:"primary_id" binding:"required"`
	SecondaryID string `json:"secondary_id" binding:"required"`
}

// MergeIdentity 合并客户身份
func (c *CustomerOneIDController) MergeIdentity(ctx *gin.Context) {
	var req mergeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.PrimaryID == req.SecondaryID {
		response.Error(ctx, http.StatusBadRequest, "不能合并同一客户")
		return
	}
	custSvc := service.NewCustomerService()
	if err := custSvc.MergeCustomers(req.PrimaryID, req.SecondaryID); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	primary, _ := c.custQuerySvc.GetCustomerByID(ctx.Request.Context(), req.PrimaryID)
	response.Success(ctx, gin.H{"primary": primary, "merged_id": req.SecondaryID}, "合并成功")
}

// ListOneID OneID 列表
func (c *CustomerOneIDController) ListOneID(ctx *gin.Context) {
	page := parsePage(ctx.Query("page"))
	pageSize := parsePageSize(ctx.Query("page_size"), 20)
	keyword := ctx.Query("keyword")
	list, total := c.custQuerySvc.ListCustomers(ctx.Request.Context(), page, pageSize, keyword)
	response.Success(ctx, gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// ListConflicts 列出潜在的身份冲突（同一手机号/邮箱/openid 关联到不同客户）
func (c *CustomerOneIDController) ListConflicts(ctx *gin.Context) {
	page := parsePage(ctx.Query("page"))
	pageSize := parsePageSize(ctx.Query("page_size"), 20)
	conflicts, total := c.custQuerySvc.ListConflicts(ctx.Request.Context(), page, pageSize)
	response.Success(ctx, gin.H{
		"list":      conflicts,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// ResolveConflictRequest 解决冲突请求
type ResolveConflictRequest struct {
	PrimaryID   string `json:"primary_id" binding:"required"`
	SecondaryID string `json:"secondary_id" binding:"required"`
	Action      string `json:"action"` // merge（默认）/ ignore
}

// ResolveConflict 解决身份冲突
func (c *CustomerOneIDController) ResolveConflict(ctx *gin.Context) {
	id := ctx.Param("id")
	var req ResolveConflictRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.Action == "" {
		req.Action = "merge"
	}
	if req.Action == "ignore" {
		response.Success(ctx, gin.H{"id": id, "action": "ignored"}, "已忽略冲突")
		return
	}
	if req.PrimaryID == req.SecondaryID {
		response.Error(ctx, http.StatusBadRequest, "不能合并同一客户")
		return
	}
	custSvc := service.NewCustomerService()
	if err := custSvc.MergeCustomers(req.PrimaryID, req.SecondaryID); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	primary, _ := c.custQuerySvc.GetCustomerByID(ctx.Request.Context(), req.PrimaryID)
	response.Success(ctx, gin.H{"id": id, "primary": primary, "merged_id": req.SecondaryID}, "冲突已解决")
}

// GetIdentityMappings 获取指定客户的所有身份映射
func (c *CustomerOneIDController) GetIdentityMappings(ctx *gin.Context) {
	customerID := ctx.Param("customer_id")
	if customerID == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少 customer_id")
		return
	}
	customer, err := c.custQuerySvc.GetCustomerByID(ctx.Request.Context(), customerID)
	if err != nil || customer == nil || customer.ID == "" {
		response.NotFound(ctx, "客户不存在")
		return
	}
	// 构建身份映射详情：列出所有已绑定的身份标识及其来源
	identities := []gin.H{}
	if customer.Phone != "" {
		identities = append(identities, gin.H{"type": "phone", "value": customer.Phone, "source": "手机号"})
	}
	if customer.Email != "" {
		identities = append(identities, gin.H{"type": "email", "value": customer.Email, "source": "邮箱"})
	}
	if customer.WechatOpenID != "" {
		identities = append(identities, gin.H{"type": "wechat_open_id", "value": customer.WechatOpenID, "source": "微信"})
	}
	if customer.DouyinOpenID != "" {
		identities = append(identities, gin.H{"type": "douyin_open_id", "value": customer.DouyinOpenID, "source": "抖音"})
	}
	if customer.XiaohongshuID != "" {
		identities = append(identities, gin.H{"type": "xiaohongshu_id", "value": customer.XiaohongshuID, "source": "小红书"})
	}
	response.Success(ctx, gin.H{
		"customer":   customer,
		"identities": identities,
	}, "获取成功")
}

// LinkIdentity 链接新身份到指定客户
func (c *CustomerOneIDController) LinkIdentity(ctx *gin.Context) {
	customerID := ctx.Param("customer_id")
	if customerID == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少 customer_id")
		return
	}
	var identifiers identity.Identifiers
	if err := ctx.ShouldBindJSON(&identifiers); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := c.identitySvc.LinkIdentity(customerID, identifiers.Phone, identifiers.Email, identifiers.WechatOpenID, identifiers.DouyinOpenID); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{"customer_id": customerID, "identifiers": identifiers}, "链接成功")
}

// ResolveIdentity 解析身份标识（识别或创建）
// 接收一个或多个渠道标识，返回所有匹配到的客户及归一化结果
func (c *CustomerOneIDController) ResolveIdentity(ctx *gin.Context) {
	var identifiers identity.Identifiers
	if err := ctx.ShouldBindJSON(&identifiers); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	customers, err := c.identitySvc.ResolveIdentity(identifiers)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	// 计算归一化后的标识（不直接创建，让前端决定是否要创建）
	normalized := identity.Identifiers{
		Phone:         identity.NormalizePhone(identifiers.Phone),
		Email:         identity.NormalizeEmail(identifiers.Email),
		WechatOpenID:  identity.NormalizeOpenID(identifiers.WechatOpenID),
		DouyinOpenID:  identity.NormalizeOpenID(identifiers.DouyinOpenID),
		XiaohongshuID: identifiers.XiaohongshuID,
	}
	if len(customers) == 0 {
		response.Success(ctx, gin.H{
			"customers":      []any{},
			"matched":        false,
			"identifiers":    identifiers,
			"normalized_ids": normalized,
		}, "未找到匹配客户")
		return
	}
	response.Success(ctx, gin.H{
		"customers":   customers,
		"matched":     true,
		"count":       len(customers),
		"identifiers": identifiers,
	}, "解析成功")
}

// parsePage 解析页码
func parsePage(s string) int {
	if s == "" {
		return 1
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 {
		return 1
	}
	return n
}

// parsePageSize 解析页大小
func parsePageSize(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 {
		return def
	}
	if n > 100 {
		return 100
	}
	return n
}
