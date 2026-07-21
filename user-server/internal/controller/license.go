package controller

import (
	"fmt"
	"marketing/internal/middleware"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/system/license"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// LicenseController 商户端授权控制器
// 处理授权码绑定、状态查询、OTA 检查等
type LicenseController struct{}

// NewLicenseController 创建授权控制器
func NewLicenseController() *LicenseController {
	return &LicenseController{}
}

// BindLicenseRequest 绑定授权码请求
type BindLicenseRequest struct {
	LicenseKey string `json:"license_key" binding:"required" example:"XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX"`
}

// BindLicenseResponse 绑定授权码响应
type BindLicenseResponse struct {
	LicenseKey string    `json:"license_key"`
	Company    string    `json:"company"`
	ExpiresAt  time.Time `json:"expires_at"`
	Trial      bool      `json:"trial"`
	MaxUsers   int       `json:"max_users"`
	Features   []string  `json:"features"`
	InstallID  string    `json:"install_id"`
	IsValid    bool      `json:"is_valid"`
}

// BindLicense 绑定授权码
// @Summary 绑定授权码
// @Description 首次安装或换绑时，验证授权码并写入 install.lock
// @Tags 授权管理
// @Accept json
// @Produce json
// @Param request body BindLicenseRequest true "授权码"
// @Success 200 {object} object{data=BindLicenseResponse}
// @Failure 400 {object} object{message=string}
// @Router /api/license/bind [post]
func (lc *LicenseController) BindLicense(c *gin.Context) {
	var req BindLicenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Error(c, http.StatusServiceUnavailable, "授权检查器未初始化")
		return
	}

	lock, err := checker.BindLicense(req.LicenseKey)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "授权码绑定失败: "+err.Error())
		return
	}

	resp := BindLicenseResponse{
		LicenseKey: lock.LicenseKey,
		Company:    lock.Company,
		ExpiresAt:  lock.ExpiresAt,
		Trial:      lock.Trial,
		MaxUsers:   lock.MaxUsers,
		Features:   lock.Features,
		InstallID:  lock.InstallID,
		IsValid:    !lock.IsExpired(),
	}

	response.Success(c, resp, "授权码绑定成功")
}

// LicenseStatusResponse 授权状态响应
type LicenseStatusResponse struct {
	Status        string    `json:"status"`         // active/suspended/expired/revoked/offline/missing
	IsLicensed    bool      `json:"is_licensed"`    // 是否已授权
	LicenseKey    string    `json:"license_key"`    // 授权码
	Company       string    `json:"company"`        // 公司名
	ExpiresAt     time.Time `json:"expires_at"`     // 过期时间
	RemainingDays int       `json:"remaining_days"` // 剩余天数
	Trial         bool      `json:"trial"`          // 是否试用
	InstallID     string    `json:"install_id"`     // 安装ID
	Version       string    `json:"version"`        // 当前版本
}

// GetStatus 获取授权状态
// @Summary 获取授权状态
// @Description 查询当前授权状态和剩余天数
// @Tags 授权管理
// @Produce json
// @Success 200 {object} object{data=LicenseStatusResponse}
// @Router /api/license/status [get]
func (lc *LicenseController) GetStatus(c *gin.Context) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Success(c, LicenseStatusResponse{
			Status: "missing",
		}, "授权检查器未初始化")
		return
	}

	lock := checker.GetInstallLock()
	status := string(checker.Status())

	resp := LicenseStatusResponse{
		Status:     status,
		IsLicensed: checker.IsLicensed(),
	}

	if lock != nil {
		resp.LicenseKey = lock.LicenseKey
		resp.Company = lock.Company
		resp.ExpiresAt = lock.ExpiresAt
		resp.RemainingDays = lock.RemainingDays()
		resp.Trial = lock.Trial
		resp.InstallID = lock.InstallID
		resp.Version = lock.Version
	}

	response.Success(c, resp, "")
}

// GetInfo 获取授权详情
// @Summary 获取授权详情
// @Description 获取 install.lock 中的完整授权信息
// @Tags 授权管理
// @Produce json
// @Success 200 {object} object{data=auth.InstallLock}
// @Router /api/license/info [get]
func (lc *LicenseController) GetInfo(c *gin.Context) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Error(c, http.StatusServiceUnavailable, "授权检查器未初始化")
		return
	}

	lock := checker.GetInstallLock()
	if lock == nil {
		response.Error(c, http.StatusNotFound, "未找到授权信息，请先绑定授权码")
		return
	}

	response.Success(c, lock, "")
}

// CheckNow 立即检查授权
// @Summary 立即检查授权
// @Description 手动触发一次授权检查（联网验证）
// @Tags 授权管理
// @Produce json
// @Success 200 {object} object{data=object{valid=bool,status=string}}
// @Router /api/license/check [post]
func (lc *LicenseController) CheckNow(c *gin.Context) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Error(c, http.StatusServiceUnavailable, "授权检查器未初始化")
		return
	}

	err := checker.Check()
	if err != nil {
		response.Success(c, gin.H{
			"valid":  false,
			"status": string(checker.Status()),
			"error":  err.Error(),
		}, "授权检查失败")
		return
	}

	response.Success(c, gin.H{
		"valid":  true,
		"status": string(checker.Status()),
	}, "授权检查通过")
}

// HeartbeatNow 立即上报心跳
// @Summary 立即上报心跳
// @Description 手动触发一次心跳上报
// @Tags 授权管理
// @Produce json
// @Success 200 {object} object{message=string}
// @Router /api/license/heartbeat [post]
func (lc *LicenseController) HeartbeatNow(c *gin.Context) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Error(c, http.StatusServiceUnavailable, "授权检查器未初始化")
		return
	}

	if err := checker.SendHeartbeat(); err != nil {
		response.Error(c, http.StatusBadGateway, "心跳上报失败: "+err.Error())
		return
	}

	response.Success(c, nil, "心跳上报成功")
}

// OTACheckResponse OTA 检查响应
type OTACheckResponse struct {
	HasUpdate    bool      `json:"has_update"`
	Version      string    `json:"version,omitempty"`
	Strategy     string    `json:"strategy,omitempty"`
	MinVersion   string    `json:"min_version,omitempty"`
	DownloadURL  string    `json:"download_url,omitempty"`
	Checksum     string    `json:"checksum,omitempty"`
	Size         int64     `json:"size,omitempty"`
	ReleaseNotes string    `json:"release_notes,omitempty"`
	ReleasedAt   time.Time `json:"released_at,omitempty"`
}

// CheckOTA 检查 OTA 升级
// @Summary 检查 OTA 升级
// @Description 检查是否有新版本可升级
// @Tags 授权管理
// @Produce json
// @Success 200 {object} object{data=OTACheckResponse}
// @Router /api/license/ota/check [get]
func (lc *LicenseController) CheckOTA(c *gin.Context) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Error(c, http.StatusServiceUnavailable, "授权检查器未初始化")
		return
	}

	lock := checker.GetInstallLock()
	if lock == nil {
		response.Error(c, http.StatusBadRequest, "未绑定授权码，无法检查 OTA")
		return
	}

	// 调用平台 /api/platform/ota/check
	ota, err := checker.CheckOTAUpdate()
	if err != nil {
		response.Error(c, http.StatusBadGateway, "OTA 检查失败: "+err.Error())
		return
	}

	if ota == nil {
		response.Success(c, OTACheckResponse{
			HasUpdate: false,
		}, "已是最新版本")
		return
	}

	resp := OTACheckResponse{
		HasUpdate:    true,
		Version:      ota.Version,
		Strategy:     ota.Strategy,
		MinVersion:   ota.MinVersion,
		DownloadURL:  ota.DownloadURL,
		Checksum:     ota.Checksum,
		Size:         ota.Size,
		ReleaseNotes: ota.ReleaseNotes,
		ReleasedAt:   ota.ReleasedAt,
	}

	msg := "发现新版本"
	switch ota.Strategy {
	case "force":
		msg = "发现强制升级版本"
	case "security":
		msg = "发现安全升级版本"
	case "recommended":
		msg = "发现推荐升级版本"
	}

	response.Success(c, resp, msg)
}

// UnbindLicense 解绑授权码
// @Summary 解绑授权码
// @Description 删除本地 install.lock 文件（需管理员权限）
// @Tags 授权管理
// @Produce json
// @Success 200 {object} object{message=string}
// @Router /api/license/unbind [delete]
func (lc *LicenseController) UnbindLicense(c *gin.Context) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Error(c, http.StatusServiceUnavailable, "授权检查器未初始化")
		return
	}

	if err := checker.UnbindLicense(); err != nil {
		response.Error(c, http.StatusInternalServerError, "解绑失败: "+err.Error())
		return
	}

	response.Success(c, nil, "授权码已解绑")
}

// verifyLicenseForBinding 内部使用：验证授权码（不写入 install.lock）
func (lc *LicenseController) verifyLicenseForBinding(licenseKey string) (*auth.InstallLock, error) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		return nil, fmt.Errorf("授权检查器未初始化")
	}
	return checker.VerifyLicense(licenseKey)
}
