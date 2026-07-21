package controller

import (
	"encoding/json"
	"fmt"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/platform"
	"marketing/internal/system/license"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

type PlatformController struct {
	platformClient *platform.Client
}

func NewPlatformController() *PlatformController {
	var client *platform.Client
	merchantKey := platform.GetMerchantKey()
	if merchantKey != "" {
		client = platform.NewClient(merchantKey)
	}
	return &PlatformController{
		platformClient: client,
	}
}

// platformCall 统一的平台 API 代理调用入口
// 当 platformClient 未初始化(配置缺失)时,返回友好错误,不 panic
func (pc *PlatformController) platformCall(c *gin.Context, method, path string, req, resp any, errMsg string) {
	if pc.platformClient == nil {
		response.Error(c, http.StatusServiceUnavailable, "平台客户端未初始化,请检查 config/platform.yaml 配置")
		return
	}
	if err := pc.platformClient.Do(method, path, req, resp); err != nil {
		// R2 修复：按结构化 *platform.PlatformError 的状态码分支，废弃脆弱的
		// strings.Contains(err, "404"/"400") 字符串匹配。
		if perr, ok := err.(*platform.PlatformError); ok {
			switch perr.StatusCode {
			case http.StatusNotFound, http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
				response.Error(c, perr.StatusCode, errMsg+": "+perr.Msg())
			default:
				response.Error(c, http.StatusBadGateway, errMsg+": "+perr.Error())
			}
		} else {
			response.Error(c, http.StatusBadGateway, errMsg+": "+err.Error())
		}
		return
	}
	response.Success(c, resp, "获取成功")
}

// =============================================================================
// 保留不变的方法(已正确实现)
// =============================================================================

// 获取最新消息
func (pc *PlatformController) GetLatestMessage(c *gin.Context) {
	message, err := platform.GetLatestMessage()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "获取最新消息失败", err.Error())
		return
	}

	if message == nil {
		response.Success(c, nil, "暂无消息")
		return
	}

	response.Success(c, message, "获取成功")
}

// 标记消息已读(调用平台 API,失败时回退到本地处理)
func (pc *PlatformController) MarkMessageRead(c *gin.Context) {
	messageId := c.Param("id")
	if messageId == "" {
		response.Error(c, http.StatusBadRequest, "消息ID不能为空")
		return
	}

	// 优先调用平台 API 标记消息已读
	if pc.platformClient != nil {
		path := fmt.Sprintf("/merchant-api/messages/%s/read", messageId)
		var resp map[string]any
		if err := pc.platformClient.Do("POST", path, nil, &resp); err == nil {
			response.Success(c, resp, "标记成功")
			return
		}
		// 平台 API 调用失败,回退到本地处理
	}

	// 本地处理:平台 API 不可用时,仅记录已读状态并返回
	response.Success(c, gin.H{
		"message_id": messageId,
		"read":       true,
		"processed":  "local",
	}, "标记成功(本地处理)")
}

// 获取授权状态(读取 install.lock)
// P1-9/P1-12 修复：通过 auth.GetInstallLockPath() 获取路径，遵循环境变量/默认路径优先级
func (pc *PlatformController) GetLicenseStatus(c *gin.Context) {
	filePath := auth.GetInstallLockPath()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 授权文件不存在，返回未安装状态
			response.Success(c, gin.H{
				"status":         "not_installed",
				"expire_at":      "",
				"remaining_days": 0,
				"valid":          false,
				"license_key":    "",
			}, "授权文件未安装")
			return
		}
		response.Error(c, http.StatusInternalServerError, "读取授权文件失败", err.Error())
		return
	}

	var licenseInfo struct {
		LicenseKey  string    `json:"license_key"`
		ExpireAt    time.Time `json:"expire_at"`
		CreatedAt   time.Time `json:"created_at"`
		LastCheckAt time.Time `json:"last_check_at"`
		IsValid     bool      `json:"is_valid"`
	}

	if err := json.Unmarshal(data, &licenseInfo); err != nil {
		response.Error(c, http.StatusInternalServerError, "解析授权文件失败", err.Error())
		return
	}

	remainingDays := int(licenseInfo.ExpireAt.Sub(time.Now()).Hours() / 24)
	status := "active"
	if time.Now().After(licenseInfo.ExpireAt) {
		status = "expired"
	}

	response.Success(c, gin.H{
		"status":         status,
		"expire_at":      licenseInfo.ExpireAt.Format("2006-01-02 15:04:05"),
		"remaining_days": remainingDays,
		"valid":          licenseInfo.IsValid,
		"license_key":    licenseInfo.LicenseKey,
	}, "获取成功")
}

// 上报 API 日志(异步上报)
func (pc *PlatformController) ReportAPILog(c *gin.Context) {
	var req struct {
		Method     string `json:"method" binding:"required"`
		Path       string `json:"path" binding:"required"`
		StatusCode int    `json:"status_code"`
		Duration   int64  `json:"duration"`
		UserAgent  string `json:"user_agent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	go func() {
		logReq := platform.ReportAPILogReq{
			Method:     req.Method,
			Path:       req.Path,
			StatusCode: req.StatusCode,
			Duration:   req.Duration,
			UserAgent:  req.UserAgent,
		}
		platform.ReportAPILog(logReq)
	}()

	response.Success(c, nil, "上报成功")
}

// =============================================================================
// 平台 API 代理方法(真实 HTTP 调用)
// =============================================================================

func (pc *PlatformController) RegisterMerchant(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		ContactEmail string `json:"contact_email"`
		ContactPhone string `json:"contact_phone"`
		DeviceInfo   string `json:"device_info"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	platformReq := platform.RegisterMerchantReq{
		Name:         req.Name,
		ContactEmail: req.ContactEmail,
		ContactPhone: req.ContactPhone,
		DeviceInfo:   req.DeviceInfo,
	}

	var resp platform.BaseResp
	pc.platformCall(c, "POST", "/merchant-api/merchant/register", platformReq, &resp, "注册商户失败")
}

// 平台端方法 - 驾驶舱 - GET /platform/dashboard
func (pc *PlatformController) GetDashboard(c *gin.Context) {
	var resp struct {
		TotalMerchants  int64   `json:"total_merchants"`
		ActiveMerchants int64   `json:"active_merchants"`
		TotalRevenue    float64 `json:"total_revenue"`
		TodayNew        int     `json:"today_new"`
	}
	pc.platformCall(c, "GET", "/platform/dashboard", nil, &resp, "获取驾驶舱失败")
}

func (pc *PlatformController) GetMerchantList(c *gin.Context) {
	page, pageSize, err := pagination.Parse(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	status := c.Query("status")

	var resp struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}

	path := fmt.Sprintf("/platform/merchants/list?page=%d&page_size=%d", page, pageSize)
	if status != "" {
		path += "&status=" + status
	}

	pc.platformCall(c, "GET", path, nil, &resp, "获取商户列表失败")
}

func (pc *PlatformController) UpdateMerchantLicense(c *gin.Context) {
	var req struct {
		LicenseKey string `json:"license_key" binding:"required"`
		ExpireAt   string `json:"expire_at" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	path := fmt.Sprintf("/platform/merchants/%s/license", req)
	var resp map[string]any
	pc.platformCall(c, "PUT", path, req, &resp, "更新商户授权失败")
}

func (pc *PlatformController) GetMerchantStats(c *gin.Context) {
	merchantID := c.Param("id")
	if merchantID == "" {
		response.Error(c, http.StatusBadRequest, "商户ID不能为空")
		return
	}

	var resp struct {
		TotalUsers   int64   `json:"total_users"`
		ActiveUsers  int64   `json:"active_users"`
		TotalRevenue float64 `json:"total_revenue"`
	}
	path := fmt.Sprintf("/platform/merchants/%s/stats", c.Param("id"))
	pc.platformCall(c, "GET", path, nil, &resp, "获取商户统计失败")
}

// 平台端方法 - 版本管理 - GET /platform/version/list
func (pc *PlatformController) GetVersionList(c *gin.Context) {
	page, pageSize, err := pagination.Parse(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	var resp struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}

	path := fmt.Sprintf("/platform/version/list?page=%d&page_size=%d", page, pageSize)
	pc.platformCall(c, "GET", path, nil, &resp, "获取版本列表失败")
}

// 平台端方法 - 创建版本 - POST /platform/version
func (pc *PlatformController) CreateVersion(c *gin.Context) {
	var req struct {
		Version     string `json:"version" binding:"required"`
		Description string `json:"description"`
		DownloadURL string `json:"download_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	var resp map[string]any
	pc.platformCall(c, "POST", "/platform/version", req, &resp, "创建版本失败")
}

// 平台端方法 - 更新版本 - PUT /platform/version/{id}
func (pc *PlatformController) UpdateVersion(c *gin.Context) {
	versionID := c.Param("id")
	if versionID == "" {
		response.Error(c, http.StatusBadRequest, "版本ID不能为空")
		return
	}

	var req struct {
		Version     string `json:"version"`
		Description string `json:"description"`
		DownloadURL string `json:"download_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	path := fmt.Sprintf("/platform/version/%s", versionID)
	var resp map[string]any
	pc.platformCall(c, "PUT", path, req, &resp, "更新版本失败")
}

// 平台端方法 - 删除版本 - DELETE /platform/version/{id}
func (pc *PlatformController) DeleteVersion(c *gin.Context) {
	versionID := c.Param("id")
	if versionID == "" {
		response.Error(c, http.StatusBadRequest, "版本ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/version/%s", versionID)
	var resp map[string]any
	pc.platformCall(c, "DELETE", path, nil, &resp, "删除版本失败")
}

// 平台端方法 - 站内信列表 - GET /platform/message/list
func (pc *PlatformController) GetMessageList(c *gin.Context) {
	page, pageSize, err := pagination.Parse(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	var resp struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}

	path := fmt.Sprintf("/platform/message/list?page=%d&page_size=%d", page, pageSize)
	pc.platformCall(c, "GET", path, nil, &resp, "获取站内信列表失败")
}

// 平台端方法 - 发送站内信 - POST /platform/message/send
func (pc *PlatformController) SendMessage(c *gin.Context) {
	var req struct {
		Title   string   `json:"title" binding:"required"`
		Content string   `json:"content" binding:"required"`
		Type    string   `json:"type"`
		Targets []string `json:"targets"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	var resp map[string]any
	pc.platformCall(c, "POST", "/platform/message/send", req, &resp, "发送站内信失败")
}

// 平台端方法 - 标记站内信已读 - POST /platform/message/{id}/read
func (pc *PlatformController) MarkPlatformMessageRead(c *gin.Context) {
	messageID := c.Param("id")
	if messageID == "" {
		response.Error(c, http.StatusBadRequest, "消息ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/message/%s/read", messageID)
	var resp map[string]any
	pc.platformCall(c, "POST", path, nil, &resp, "标记站内信已读失败")
}

// 平台端方法 - 用户管理 - GET /platform/user/list
func (pc *PlatformController) GetUserList(c *gin.Context) {
	page, pageSize, err := pagination.Parse(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	var resp struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}

	path := fmt.Sprintf("/platform/user/list?page=%d&page_size=%d", page, pageSize)
	pc.platformCall(c, "GET", path, nil, &resp, "获取用户列表失败")
}

// 平台端方法 - 创建用户 - POST /platform/user/create
func (pc *PlatformController) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Role     string `json:"role" binding:"required"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	var resp map[string]any
	pc.platformCall(c, "POST", "/platform/user/create", req, &resp, "创建用户失败")
}

// 平台端方法 - 删除用户 - DELETE /platform/user/{id}
func (pc *PlatformController) DeleteUser(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		response.Error(c, http.StatusBadRequest, "用户ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/user/%s", userID)
	var resp map[string]any
	pc.platformCall(c, "DELETE", path, nil, &resp, "删除用户失败")
}

// 平台端方法 - 系统统计 - GET /platform/stats/system
func (pc *PlatformController) GetSystemStats(c *gin.Context) {
	var resp struct {
		ServerTime  string `json:"server_time"`
		Uptime      string `json:"uptime"`
		MemoryUsage string `json:"memory_usage"`
		CPUUsage    string `json:"cpu_usage"`
		DiskUsage   string `json:"disk_usage"`
	}
	pc.platformCall(c, "GET", "/platform/stats/system", nil, &resp, "获取系统统计失败")
}

// 平台端方法 - 平台总览统计 - GET /platform/stats/overview
func (pc *PlatformController) GetPlatformStats(c *gin.Context) {
	var resp struct {
		TotalMerchants  int64   `json:"total_merchants"`
		ActiveMerchants int64   `json:"active_merchants"`
		TotalUsers      int64   `json:"total_users"`
		TotalRevenue    float64 `json:"total_revenue"`
		TodayStats      struct {
			NewMerchants int     `json:"new_merchants"`
			ActiveUsers  int     `json:"active_users"`
			Revenue      float64 `json:"revenue"`
		} `json:"today_stats"`
	}
	pc.platformCall(c, "GET", "/platform/stats/overview", nil, &resp, "获取平台统计失败")
}

func (pc *PlatformController) GetPlatformMerchantStats(c *gin.Context) {
	days := c.DefaultQuery("days", "7")

	var resp struct {
		Days  string           `json:"days"`
		Stats []map[string]any `json:"stats"`
	}

	path := fmt.Sprintf("/platform/stats/merchant?days=%s", days)
	pc.platformCall(c, "GET", path, nil, &resp, "获取商户统计失败")
}

// 检查更新 - GET /public/version/check-update
func (pc *PlatformController) CheckUpdate(c *gin.Context) {
	currentVersion := c.DefaultQuery("current_version", "1.0.0")
	clientType := c.DefaultQuery("client_type", "frontend")

	var resp map[string]any
	path := fmt.Sprintf("/public/version/check-update?current_version=%s&client_type=%s", currentVersion, clientType)
	pc.platformCall(c, "GET", path, nil, &resp, "检查更新失败")
}

// =============================================================================
// 版本管理 - 补充方法
// =============================================================================

// 平台端方法 - 获取版本详情 - GET /platform/version/{id}
func (pc *PlatformController) GetVersionByID(c *gin.Context) {
	versionID := c.Param("id")
	if versionID == "" {
		response.Error(c, http.StatusBadRequest, "版本ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/version/%s", versionID)
	var resp map[string]any
	pc.platformCall(c, "GET", path, nil, &resp, "获取版本详情失败")
}

// 平台端方法 - 发布版本 - POST /platform/version/{id}/publish
func (pc *PlatformController) PublishVersion(c *gin.Context) {
	versionID := c.Param("id")
	if versionID == "" {
		response.Error(c, http.StatusBadRequest, "版本ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/version/%s/publish", versionID)
	var resp map[string]any
	pc.platformCall(c, "POST", path, nil, &resp, "发布版本失败")
}

// 平台端方法 - 归档版本 - POST /platform/version/{id}/archive
func (pc *PlatformController) ArchiveVersion(c *gin.Context) {
	versionID := c.Param("id")
	if versionID == "" {
		response.Error(c, http.StatusBadRequest, "版本ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/version/%s/archive", versionID)
	var resp map[string]any
	pc.platformCall(c, "POST", path, nil, &resp, "归档版本失败")
}

// 平台端方法 - 获取最新版本 - GET /platform/version/latest
func (pc *PlatformController) GetLatestVersion(c *gin.Context) {
	var resp map[string]any
	pc.platformCall(c, "GET", "/platform/version/latest", nil, &resp, "获取最新版本失败")
}

// =============================================================================
// 授权管理 - 代理方法
// =============================================================================

// 平台端方法 - 授权列表 - GET /platform/license/list
func (pc *PlatformController) GetLicenseList(c *gin.Context) {
	page, pageSize, err := pagination.Parse(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	var resp struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}

	if pc.platformClient == nil {
		response.Error(c, http.StatusServiceUnavailable, "平台客户端未初始化,请检查 config/platform.yaml 配置")
		return
	}

	path := fmt.Sprintf("/platform/license/list?page=%d&page_size=%d", page, pageSize)
	if err := pc.platformClient.Do("GET", path, nil, &resp); err != nil {
		// R2 修复：按结构化 *platform.PlatformError 的状态码分支。
		if perr, ok := err.(*platform.PlatformError); ok {
			switch perr.StatusCode {
			case http.StatusNotFound, http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
				response.Error(c, perr.StatusCode, "获取授权列表失败: "+perr.Msg())
			default:
				response.Error(c, http.StatusBadGateway, "获取授权列表失败: "+perr.Error())
			}
		} else {
			response.Error(c, http.StatusBadGateway, "获取授权列表失败: "+err.Error())
		}
		return
	}
	response.Success(c, resp, "获取成功")
}

// 平台端方法 - 授权详情 - GET /platform/license/{id}
func (pc *PlatformController) GetLicenseByID(c *gin.Context) {
	licenseID := c.Param("id")
	if licenseID == "" {
		response.Error(c, http.StatusBadRequest, "授权ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/license/%s", licenseID)
	var resp map[string]any
	pc.platformCall(c, "GET", path, nil, &resp, "获取授权详情失败")
}

// 平台端方法 - 创建授权 - POST /platform/license
func (pc *PlatformController) CreateLicense(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	var resp map[string]any
	pc.platformCall(c, "POST", "/platform/license", req, &resp, "创建授权失败")
}

// 平台端方法 - 更新授权 - PUT /platform/license/{id}
func (pc *PlatformController) UpdateLicense(c *gin.Context) {
	licenseID := c.Param("id")
	if licenseID == "" {
		response.Error(c, http.StatusBadRequest, "授权ID不能为空")
		return
	}

	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	path := fmt.Sprintf("/platform/license/%s", licenseID)
	var resp map[string]any
	pc.platformCall(c, "PUT", path, req, &resp, "更新授权失败")
}

// 平台端方法 - 删除授权 - DELETE /platform/license/{id}
func (pc *PlatformController) DeleteLicense(c *gin.Context) {
	licenseID := c.Param("id")
	if licenseID == "" {
		response.Error(c, http.StatusBadRequest, "授权ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/license/%s", licenseID)
	var resp map[string]any
	pc.platformCall(c, "DELETE", path, nil, &resp, "删除授权失败")
}

// 平台端方法 - 续期授权 - POST /platform/license/{id}/renew
func (pc *PlatformController) RenewLicense(c *gin.Context) {
	licenseID := c.Param("id")
	if licenseID == "" {
		response.Error(c, http.StatusBadRequest, "授权ID不能为空")
		return
	}

	var req map[string]any
	_ = c.ShouldBindJSON(&req)

	path := fmt.Sprintf("/platform/license/%s/renew", licenseID)
	var resp map[string]any
	pc.platformCall(c, "POST", path, req, &resp, "续期授权失败")
}

// 平台端方法 - 禁用授权 - POST /platform/license/{id}/disable
func (pc *PlatformController) DisableLicense(c *gin.Context) {
	licenseID := c.Param("id")
	if licenseID == "" {
		response.Error(c, http.StatusBadRequest, "授权ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/license/%s/disable", licenseID)
	var resp map[string]any
	pc.platformCall(c, "POST", path, nil, &resp, "禁用授权失败")
}

// 平台端方法 - 启用授权 - POST /platform/license/{id}/enable
func (pc *PlatformController) EnableLicense(c *gin.Context) {
	licenseID := c.Param("id")
	if licenseID == "" {
		response.Error(c, http.StatusBadRequest, "授权ID不能为空")
		return
	}

	path := fmt.Sprintf("/platform/license/%s/enable", licenseID)
	var resp map[string]any
	pc.platformCall(c, "POST", path, nil, &resp, "启用授权失败")
}

// 平台端方法 - 校验授权 - POST /platform/license/verify
func (pc *PlatformController) VerifyLicense(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}

	var resp map[string]any
	pc.platformCall(c, "POST", "/platform/license/verify", req, &resp, "校验授权失败")
}

// 平台端方法 - 授权状态(代理到平台服务) - GET /platform/license/status
// 注意:与读取本地 install.lock 的 GetLicenseStatus 不同,此方法代理到平台服务获取授权状态
func (pc *PlatformController) GetLicenseStatusProxy(c *gin.Context) {
	var resp map[string]any
	pc.platformCall(c, "GET", "/platform/license/status", nil, &resp, "获取授权状态失败")
}
