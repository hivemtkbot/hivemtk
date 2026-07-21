package controller

import (
	"strconv"

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// ObsConfigController 对象存储配置控制器
type ObsConfigController struct {
	service service.ObsConfigService
}

// NewObsConfigController 创建对象存储配置控制器
func NewObsConfigController() *ObsConfigController {
	return &ObsConfigController{
		service: service.NewObsConfigService(),
	}
}

// GetConfigList 获取配置列表
func (c *ObsConfigController) GetConfigList(ctx *gin.Context) {
	licenseID := ctx.Query("license_id")
	if licenseID == "" {
		if licenseIDFromContext, exists := ctx.Get("license_id"); exists {
			if v, ok := licenseIDFromContext.(string); ok {
				licenseID = v
			}
		}
	}
	// 若仍为空，不强制报错，返回空列表以保证页面可用
	if licenseID == "" {
		empty := []any{}
		response.SuccessWithPage(ctx, empty, 1, 10, 0)
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "10"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	resp, err := c.service.GetConfigList(licenseID, page, limit)
	if err != nil {
		response.Error(ctx, 500, "查询OBS配置列表失败: "+err.Error())
		return
	}

	response.Success(ctx, resp, "查询成功")
}

// GetConfig 获取配置详情
func (c *ObsConfigController) GetConfig(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		response.Error(ctx, 400, "INVALID_PARAM", "配置ID不能为空")
		return
	}

	config, err := c.service.GetConfig(id)
	if err != nil {
		if isNotFoundError(err) {
			response.Error(ctx, 404, "NOT_FOUND", err.Error())
			return
		}
		response.Error(ctx, 500, "INTERNAL_ERROR", err.Error())
		return
	}

	response.Success(ctx, config, "查询成功")
}

// CreateConfig 创建配置
func (c *ObsConfigController) CreateConfig(ctx *gin.Context) {
	var req dto.CreateObsConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "INVALID_PARAM", "请求参数错误: "+err.Error())
		return
	}

	config, err := c.service.CreateConfig(&req)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, config, "创建成功")
}

// UpdateConfig 更新配置
func (c *ObsConfigController) UpdateConfig(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		response.Error(ctx, 400, "INVALID_PARAM", "配置ID不能为空")
		return
	}

	var req dto.UpdateObsConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "INVALID_PARAM", "请求参数错误: "+err.Error())
		return
	}

	config, err := c.service.UpdateConfig(id, &req)
	if HandleDBError(ctx, err, "更新OBS配置") {
		return
	}

	response.Success(ctx, config, "更新成功")
}

// DeleteConfig 删除配置
func (c *ObsConfigController) DeleteConfig(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		response.Error(ctx, 400, "INVALID_PARAM", "配置ID不能为空")
		return
	}

	if HandleDBError(ctx, c.service.DeleteConfig(id), "删除OBS配置") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// TestConnection 测试连接
func (c *ObsConfigController) TestConnection(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		response.Error(ctx, 400, "INVALID_PARAM", "配置ID不能为空")
		return
	}

	config, err := c.service.GetConfig(id)
	if err != nil {
		if isNotFoundError(err) {
			response.Error(ctx, 404, "NOT_FOUND", err.Error())
			return
		}
		response.Error(ctx, 500, "INTERNAL_ERROR", err.Error())
		return
	}

	if err := c.service.TestConnection(config); err != nil {
		response.Error(ctx, 400, "CONNECTION_FAILED", err.Error())
		return
	}

	response.Success(ctx, nil, "连接测试成功")
}

// SetDefault 设为默认配置
func (c *ObsConfigController) SetDefault(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		response.Error(ctx, 400, "INVALID_PARAM", "配置ID不能为空")
		return
	}

	// 从JWT中获取licenseID
	licenseID, exists := ctx.Get("license_id")
	if !exists {
		response.Error(ctx, 400, "INVALID_PARAM", "无法获取许可证信息")
		return
	}

	if err := c.service.SetDefaultConfig(id, licenseID.(string)); err != nil {
		if isNotFoundError(err) {
			response.Error(ctx, 404, "NOT_FOUND", err.Error())
			return
		}
		response.Error(ctx, 400, "SET_DEFAULT_FAILED", err.Error())
		return
	}

	response.Success(ctx, nil, "设为默认配置成功")
}

// GetDefaultConfig 获取默认配置
func (c *ObsConfigController) GetDefaultConfig(ctx *gin.Context) {
	licenseID := ctx.Query("license_id")
	if licenseID == "" {
		if licenseIDFromContext, exists := ctx.Get("license_id"); exists {
			if v, ok := licenseIDFromContext.(string); ok {
				licenseID = v
			}
		}
	}
	if licenseID == "" {
		response.Error(ctx, 400, "INVALID_PARAM", "license_id不能为空")
		return
	}

	config, err := c.service.GetDefaultConfig(licenseID)
	if err != nil {
		if isNotFoundError(err) {
			// 不存在时返回空数据而非 404，便于前端统一处理
			response.Success(ctx, nil, "暂无默认配置")
			return
		}
		response.Error(ctx, 500, "INTERNAL_ERROR", err.Error())
		return
	}

	response.Success(ctx, config, "查询成功")
}
