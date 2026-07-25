package controller

import (
	"fmt"
	"time"

	"marketing/internal/cache"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/platform"
	"net/http"

	"github.com/gin-gonic/gin"
)

type PlatformController struct {
	platformClient *platform.Client
}

func NewPlatformController() *PlatformController {
	var client *platform.Client
	merchantKey := platform.GetMerchantKey()
	if merchantKey != "" {
		client = platform.NewPlatformClient(merchantKey)
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

	// 审计 L6：对幂等的 GET 代理请求做短时缓存，降低对平台 API 的重复调用压力。
	// 缓存命中直接返回；未命中在调用成功后写入；写类请求(POST/DELETE/...)不缓存。
	const platformCacheTTL = 20 * time.Second
	cacheKey := method + ":" + path
	if method == http.MethodGet {
		if gc := cache.GetGlobalCache(); gc != nil {
			if err := gc.GetJSON(c.Request.Context(), cacheKey, resp); err == nil {
				response.Success(c, resp, "获取成功")
				return
			}
		}
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

	if method == http.MethodGet {
		if gc := cache.GetGlobalCache(); gc != nil {
			_ = gc.SetJSON(c.Request.Context(), cacheKey, resp, platformCacheTTL)
		}
	}
	response.Success(c, resp, "获取成功")
}

// =============================================================================
// 保留不变的方法(已正确实现)
// =============================================================================

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

// 平台端方法 - 最新站内信 - GET /platform/message/latest
// 供前端 MessageNotification 全局轮询。平台客户端未初始化或平台不可达/无该端点时，
// 静默返回 null（而非 404/5xx），避免每次页面加载都产生控制台噪声。
func (pc *PlatformController) GetLatestMessage(c *gin.Context) {
	if pc.platformClient == nil {
		response.Success(c, nil, "")
		return
	}
	var resp struct {
		List []map[string]any `json:"list"`
	}
	path := "/platform/message/list?page=1&page_size=1"
	if err := pc.platformClient.Do(http.MethodGet, path, nil, &resp); err != nil {
		response.Success(c, nil, "")
		return
	}
	if len(resp.List) > 0 {
		response.Success(c, resp.List[0], "")
		return
	}
	response.Success(c, nil, "")
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
