package router

import (
	knowledgectrl "hivemtk-user/internal/aiagent/knowledge/controller"
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/system/install"

	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupPlatformRoutes 平台端管理路由（需要平台权限）
//
// OTA 版本管理与 License 授权管理路由未启用。
// 仅保留统计/安装/心跳信息相关的只读能力。
func setupPlatformRoutes(platform *gin.RouterGroup, platformCtrl *controller.PlatformController) {
	platform.GET("/dashboard", platformCtrl.GetDashboard)

	platform.GET("/merchant/list", platformCtrl.GetMerchantList)
	platform.GET("/merchant/:id/stats", platformCtrl.GetMerchantStats)


	platform.GET("/message/list", platformCtrl.GetMessageList)
	platform.GET("/message/latest", platformCtrl.GetLatestMessage)
	platform.POST("/message/send", platformCtrl.SendMessage)
	platform.POST("/message/:id/read", platformCtrl.MarkPlatformMessageRead)

	platform.GET("/user/list", platformCtrl.GetUserList)
	platform.POST("/user", platformCtrl.CreateUser)
	platform.DELETE("/user/:id", platformCtrl.DeleteUser)

	platform.GET("/stats/system", platformCtrl.GetSystemStats)
	platform.GET("/stats/overview", platformCtrl.GetPlatformStats)
	platform.GET("/stats/merchant", platformCtrl.GetPlatformMerchantStats)
}

// setupPublicRoutes 公开路由（不需要认证）
func setupPublicRoutes(public *gin.RouterGroup, liveCodeController *controller.LiveCodeController, platformCtrl *controller.PlatformController, db *gorm.DB) {
	public.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	systemInfoCtrl := controller.NewSystemInfoController()
	public.GET("/system/info", systemInfoCtrl.GetSystemInfo)

	public.POST("/auth/login", middleware.BruteForceGuard("auth.login"), controller.NewAuthController().Login)

	public.POST("/auth/mfa/verify", controller.NewAuthController().VerifyMFALogin)

	systemInitCtrl := controller.NewSystemInitController()
	authCtrl := controller.NewAuthController()
	public.GET("/system/init-status", systemInitCtrl.GetInitStatus)
	public.POST("/system/init-admin", func(ctx *gin.Context) {
		if install.GetStatus().Initialized {
			response.Error(ctx, http.StatusForbidden, response.ErrSystemAlreadyInitialized)
			return
		}
		authCtrl.InitAdmin(ctx)
	})
	public.POST("/system/init-complete", systemInitCtrl.InitComplete)

	public.GET("/license/status", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"code": 200,
			"data": gin.H{
				"edition":  "open_source",
				"licensed": true,
				"status":   "active",
				"message":  "开源版无需授权",
			},
			"msg": "ok",
		})
	})
	public.GET("/license/features", func(c *gin.Context) {
		c.JSON(200, gin.H{"code": 200, "data": []interface{}{}, "msg": "ok"})
	})

	redirectCtrl := controller.NewRedirectController(
		service.NewShortLinkService(db),
		service.NewDouyinCardService(db),
		service.NewKuaishouCardService(db),
		service.NewXiaohongshuCardService(db),
		service.NewXianyuCardService(db),
		service.NewDouyinCardStatsService(db),
		service.NewKuaishouCardStatsService(db),
		service.NewXiaohongshuCardStatsService(db),
		service.NewXianyuCardStatsService(db),
		service.NewTikTokCardServiceWithDB(db),
	)
	public.GET("/s/:code", redirectCtrl.RedirectShortLink)
	public.GET("/l/:code", liveCodeController.RedirectLiveCode)

	public.GET("/livecode/:id", liveCodeController.RenderLiveCodePage)
	public.POST("/livecode/:id/click", liveCodeController.RecordClick)

	public.POST("/platform/register", platformCtrl.RegisterMerchant)

	deps := wirePublicDependencies(db)
	public.POST("/knowledge-merchant/external/import", deps.knowledgeMerchantCtrl.ExternalImport)

	deps.emailUnsubscribeCtrl.RegisterRoutes(public, nil)

	deps.smsUnsubscribeCtrl.RegisterRoutes(public, nil)

	deps.smsDeliveryTrackerCtrl.RegisterRoutes(public, nil)

	deps.emailOpenTrackerCtrl.RegisterRoutes(public, nil)

	public.POST("/chat/ingress", deps.inboxIngressCtrl.Ingress)
}

// publicDeps 聚合 setupPublicRoutes 所需的仓储/服务/控制器依赖。
//
// 审计 M7（路由层直接构造 service/repository）：原本这些 new 散落在路由注册处，
// 既不便测试也加深路由层与具体实现的耦合。此处集中构造，路由层只负责“消费依赖 + 注册路由”，
// 后续可平滑替换为 wire/fx 等 DI 容器（当前规模下显式 wiring 已足够清晰且低风险）。
type publicDeps struct {
	knowledgeMerchantCtrl  *knowledgectrl.KnowledgeMerchantController
	emailUnsubscribeCtrl   *controller.EmailUnsubscribeController
	smsUnsubscribeCtrl     *controller.SmsUnsubscribeController
	smsDeliveryTrackerCtrl *controller.SmsDeliveryTrackerController
	emailOpenTrackerCtrl   *controller.EmailOpenTrackerController
	inboxIngressCtrl       *controller.InboxIngressController
}

func wirePublicDependencies(db *gorm.DB) publicDeps {
	emailUnsubscribeRepo := repository.NewEmailUnsubscribeRepository(db)
	smsUnsubscribeRepo := repository.NewSmsUnsubscribeRepository(db)
	return publicDeps{
		knowledgeMerchantCtrl:  knowledgectrl.NewKnowledgeMerchantController(),
		emailUnsubscribeCtrl:   controller.NewEmailUnsubscribeController(service.NewEmailUnsubscribeService(emailUnsubscribeRepo)),
		smsUnsubscribeCtrl:     controller.NewSmsUnsubscribeController(service.NewSmsUnsubscribeService(smsUnsubscribeRepo)),
		smsDeliveryTrackerCtrl: controller.NewSmsDeliveryTrackerController(service.NewSmsDeliveryTrackerService(db, nil, nil)),
		emailOpenTrackerCtrl:   controller.NewEmailOpenTrackerController(service.NewEmailOpenTrackerService(nil, nil)),
		inboxIngressCtrl:       controller.NewInboxIngressController(service.NewInboxIngressService()),
	}
}

