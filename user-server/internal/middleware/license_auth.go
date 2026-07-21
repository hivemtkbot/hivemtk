package middleware

import (
	"context"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/system/license"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	licenseChecker *auth.LicenseChecker
	licenseCtx     context.Context
	licenseCancel  context.CancelFunc
	shutdownOnce   sync.Once
)

// InitLicenseChecker 初始化授权检查器（install.lock + 3 分钟心跳 + 9 分钟容错）
// 对应 LICENSE_OTA_DESIGN.md 第 7.1 节启动流程
func InitLicenseChecker(platformURL string, _ string) {
	cfg := auth.DefaultLicenseCheckerConfig()
	if platformURL != "" {
		cfg.ServerURL = platformURL
	}
	// 读取平台共享密钥（install.lock HMAC 签名）。缺失则按开发模式（跳过签名校验）。
	// 该密钥用于本地防篡改校验，必须与平台端 PLATFORM_LICENSE_SECRET 一致。
	if secret := os.Getenv("PLATFORM_LICENSE_SECRET"); secret != "" {
		cfg.PlatformSecret = secret
	}

	checker := auth.NewLicenseChecker(cfg)

	// 尝试加载本地 install.lock
	if err := checker.LoadInstallLock(); err != nil {
		if err == auth.ErrLicenseMissing {
			logger.Warn("install.lock 不存在，请通过授权码绑定接口进行绑定")
		}
		// 授权文件缺失不阻止启动，由 LicenseGuard 中间件拦截业务请求
	}
	// 设置优雅停机回调
	checker.SetShutdownFunc(func() {
		performGracefulShutdown()
	})

	licenseChecker = checker

	// 启动后台心跳检测
	licenseCtx, licenseCancel = context.WithCancel(context.Background())
	go checker.Run(licenseCtx)

	// 监听系统信号，确保优雅退出
	go watchSystemSignals()
}

// performGracefulShutdown 优雅停机
// 对应 LICENSE_OTA_DESIGN.md 第 3.6 节
func performGracefulShutdown() {
	shutdownOnce.Do(func() {
		logger.Warn("授权无效，开始优雅停机...")

		// 停止心跳检测
		if licenseCancel != nil {
			licenseCancel()
		}

		// 等待当前请求完成（最多 30 秒）
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		<-ctx.Done()

		logger.Warn("优雅停机完成，退出进程")
		os.Exit(1)
	})
}

// watchSystemSignals 监听系统信号
func watchSystemSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	performGracefulShutdown()
}

// LicenseGuard 授权守卫中间件
//
// 开源版说明：hivemtk 现已全面开源，不再有任何授权码（LicenseKey）/ 授权流程，
// 授权校验已无意义。本中间件保留为兼容占位，始终放行（no-op），
// 业务路由分组里的 LicenseGuard() 调用无需改动即可继续工作。
func LicenseGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// GetLicenseChecker 获取授权检查器实例（供控制器使用）
func GetLicenseChecker() *auth.LicenseChecker {
	return licenseChecker
}

// IsLicenseReady 授权检查器是否已就绪
func IsLicenseReady() bool {
	return licenseChecker != nil
}
