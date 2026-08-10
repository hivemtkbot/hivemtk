package middleware

import (
	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/gin-gonic/gin"
)

// InitGuard 系统初始化守卫（开源版）
//
// 行为：
//   - 系统 NOT_INSTALLED：返回 INIT_REQUIRED，前端跳 /setup
//   - 系统 INITIALIZED：放行
//   - 白名单路由（init-*/login/health 等）直接放行
//
// 授权过期/暂停/吊销拦截、首次强制改密拦截未启用
// （hivemtk 已全面开源，无授权流程，且不再强制新账号改密）。
func InitGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.FullPath()
		// 白名单：无需 InitGuard 校验
		if isInitWhitelist(path) {
			c.Next()
			return
		}
		checker := GetLicenseChecker()
		if checker == nil {
			// 授权检查器未初始化（开发模式兜底）
			c.Next()
			return
		}
		status := checker.GetInitStatus()
		if status == nil {
			c.Next()
			return
		}

		// 仅判断"是否已初始化"，未初始化则引导去 /setup
		if !status.Initialized {
			logger.Info("InitGuard: 未初始化 path=" + path)
			c.AbortWithStatusJSON(200, gin.H{
				"code":     "INIT_REQUIRED",
				"message":  "系统未初始化",
				"redirect": "/setup",
				"data":     status,
			})
			return
		}

		// INITIALIZED，放行
		c.Next()
	}
}

// InitState 常量（与 auth.InitState* 保持一致）
const (
	InitStateNotInstalled     = "NOT_INSTALLED"
	InitStateHasLicense       = "HAS_LICENSE"
	InitStateHasAdmin         = "HAS_ADMIN"
	InitStateInitialized      = "INITIALIZED"
	InitStateLicenseExpired   = "LICENSE_EXPIRED"
	InitStateLicenseSuspended = "LICENSE_SUSPENDED"
	InitStateLicenseRevoked   = "LICENSE_REVOKED"
)

// isInitWhitelist 初始化白名单（无需 InitGuard 校验）
// 这些路径是初始化流程的必经节点，必须始终可访问
func isInitWhitelist(path string) bool {
	whitelist := map[string]bool{
		// 状态查询
		"/api/system/init-status": true,
		"/api/system/info":        true,
		// 初始化流程
		"/api/system/init-admin":    true,
		"/api/system/init-complete": true,
		// 鉴权流程（个人中心改密 / 登录）
		"/api/auth/login":           true,
		"/api/auth/refresh-token":   true,
		"/api/auth/change-password": true, // 个人中心重置密码（开源版唯一改密入口）
		"/api/auth/current-user":    true,
		// 商户自部署兼容
		"/api/merchant/init": true,
		// 系统用户默认超管（保留旧接口兼容）
		"/api/system/create-default-admin": true,
		// 公开回调（短链/活码跳转）
		"/s/:code": true,
		"/l/:code": true,
		// 健康检查
		"/health":  true,
		"/healthz": true,
		"/readyz":  true,
	}
	return whitelist[path]
}
