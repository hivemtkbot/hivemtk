package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"marketing/internal/middleware"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/platform"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// SystemInitController 系统初始化控制器
//
// 开源版说明（hivemtk 已全面开源）：
//   - 不再有授权码（LicenseKey）/ 授权流程，移除原 init-license 步骤。
//   - 初始化流程简化为：创建超管 -> 直接上报安装信息到平台（一个安装信息 = 一个商户）。
//   - 不再强制"新账号首次登录改密"，初始化即完成。
//
// 提供 3 个公开端点：
//   - GET  /api/system/init-status   获取初始化状态
//   - POST /api/system/init-admin    创建超管 + 上报安装信息
//   - POST /api/system/init-complete 完成初始化
type SystemInitController struct {
	initSvc     *service.SystemInitService
	userService *service.SystemUserService
}

// NewSystemInitController 创建系统初始化控制器
func NewSystemInitController() *SystemInitController {
	return &SystemInitController{
		initSvc:     service.NewSystemInitService(),
		userService: service.NewSystemUserService(),
	}
}

// GetInitStatus 公开：获取系统初始化状态
// @Summary 获取系统初始化状态
// @Description 返回系统状态机（NOT_INSTALLED/HAS_ADMIN/INITIALIZED）
// @Tags 系统初始化
// @Produce json
// @Success 200 {object} object{data=install.Status}
// @Router /api/system/init-status [get]
func (c *SystemInitController) GetInitStatus(ctx *gin.Context) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Success(ctx, gin.H{
			"state":       "NOT_INSTALLED",
			"initialized": false,
			"has_admin":   false,
			"version":     "unknown",
		}, "授权检查器未初始化")
		return
	}
	status := checker.GetInitStatus()
	response.Success(ctx, status, "ok")
}

// InitAdminRequest 创建超管请求
// 开源版：手机号、邮箱、姓名均为选填（初始化时上报平台，作为商户联系信息）。
type InitAdminRequest struct {
	Username     string `json:"username" binding:"required,min=3,max=20"`
	Password     string `json:"password" binding:"required,min=8"`
	Email        string `json:"email" binding:"omitempty,email"`
	RealName     string `json:"real_name"`
	ContactPhone string `json:"contact_phone"`
}

// InitAdmin 公开：创建超管（并完成安装信息上报）
// @Summary 创建系统超管
// @Description 创建系统第一个超管；创建成功后直接将安装信息上报到平台（开源版无授权码流程）
// @Tags 系统初始化
// @Accept json
// @Produce json
// @Param request body InitAdminRequest true "超管账号 + 选填联系信息"
// @Success 200 {object} object{data=object{admin_username=string}}
// @Failure 400 {object} object{message=string}
// @Router /api/system/init-admin [post]
func (c *SystemInitController) InitAdmin(ctx *gin.Context) {
	var req InitAdminRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "授权检查器未初始化")
		return
	}
	// 开源版：已创建超管则拒绝重复创建（防超管替换攻击）
	if checker.HasInstallLockAdmin() {
		response.Error(ctx, http.StatusConflict, "系统已创建超管账号，如需重置请走找回密码流程（个人中心）")
		return
	}

	// 半初始化自愈：install.lock 未记录超管，但 DB 已存在用户
	// （旧构建 init-admin 把超管写库却未回写 lock，导致 init-complete 永远报"请先创建超管"）
	// 此时从 DB 取出已有超管用户名，补写 install.lock，使初始化向导可继续完成。
	if count, _ := c.userService.CountUsers(); count > 0 {
		existing, gerr := c.userService.GetFirstAdminUsername()
		if gerr != nil || existing == "" {
			response.Error(ctx, http.StatusConflict, "系统已存在用户但未找到超管账号，请走找回密码流程（个人中心）重置")
			return
		}
		if err := checker.SetAdminInit(existing); err != nil {
			response.Error(ctx, http.StatusInternalServerError, "同步初始化状态失败: "+err.Error())
			return
		}
		response.Success(ctx, gin.H{
			"admin_username":       existing,
			"must_change_password": false,
			"message":              "已检测到已存在的超管账号，初始化状态已恢复，请使用该账号登录（如忘记密码可在个人中心重置）",
		}, "超管初始化状态已恢复")
		return
	}

	// 1. 创建超管
	err := c.initSvc.CreateInitAdmin(&service.CreateInitAdminParams{
		Username: strings.TrimSpace(req.Username),
		Password: req.Password,
		Email:    strings.TrimSpace(req.Email),
		RealName: strings.TrimSpace(req.RealName),
	})
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	// 2. 同步 install.lock（标记超管已初始化；开源版不再要求首次改密）
	if err := checker.SetAdminInit(strings.TrimSpace(req.Username)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "同步初始化状态失败: "+err.Error())
		return
	}

	// 3. 开源版：直接将安装信息上报到平台（一个安装信息 = 一个商户，统计用）
	c.reportInstall(checker, ctx, &req)

	response.Success(ctx, gin.H{
		"admin_username":       req.Username,
		"must_change_password": false,
		"message":              "超管创建成功，安装信息已上报，请使用此账号登录系统",
	}, "超管创建成功")
}

// reportInstall 异步（best-effort）将安装信息上报至平台。失败仅记录日志，不影响初始化结果。
func (c *SystemInitController) reportInstall(checker *middleware.LicenseChecker, ctx *gin.Context, req *InitAdminRequest) {
	lock := checker.GetInstallLock()
	if lock == nil || lock.InstallID == "" {
		return
	}
	installID := lock.InstallID
	merchantName := strings.TrimSpace(req.RealName)
	if merchantName == "" {
		merchantName = strings.TrimSpace(req.Username)
	}
	report := &platform.ReportInstallReq{
		InstallID:         installID,
		MerchantName:      merchantName,
		ContactEmail:      strings.TrimSpace(req.Email),
		ContactPhone:      strings.TrimSpace(req.ContactPhone),
		ContactName:       strings.TrimSpace(req.RealName),
		DeviceFingerprint: deviceFingerprint(ctx),
		ClientIP:          ctx.ClientIP(),
		Version:           checker.CurrentVersion(),
	}
	go func() {
		if err := platform.ReportInstallDefault(report); err != nil {
			logger.Warn("安装信息上报平台失败（已忽略）: " + err.Error())
		}
	}()
}

// deviceFingerprint 生成设备指纹（基于客户端 IP + User-Agent + 语言偏好哈希）
func deviceFingerprint(ctx *gin.Context) string {
	seed := ctx.ClientIP() + "|" +
		ctx.GetHeader("User-Agent") + "|" +
		ctx.GetHeader("Accept-Language")
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

// InitComplete 公开：完成初始化向导
// @Summary 完成系统初始化向导
// @Description 标记初始化向导完成（开源版：超管已创建即视为完成，不再强制改密）
// @Tags 系统初始化
// @Produce json
// @Success 200 {object} object{message=string}
// @Failure 400 {object} object{message=string}
// @Router /api/system/init-complete [post]
func (c *SystemInitController) InitComplete(ctx *gin.Context) {
	checker := middleware.GetLicenseChecker()
	if checker == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "授权检查器未初始化")
		return
	}
	if !checker.HasInstallLockAdmin() {
		response.Error(ctx, http.StatusBadRequest, "请先创建超管")
		return
	}

	// 开源版：只要超管已创建即视为初始化完成，请直接登录
	response.Success(ctx, gin.H{
		"message":        "系统已初始化完成，请使用超管账号登录",
		"initialized":    true,
		"admin_username": checker.GetAdminUsername(),
		"next_action":    "login",
	}, "初始化完成")
}
