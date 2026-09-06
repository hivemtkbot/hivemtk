package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupCardRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {

	crossPubCtrl := controller.NewCardCrossPublishController(
		service.NewDouyinCardService(gormDB),
		service.NewKuaishouCardService(gormDB),
		service.NewXiaohongshuCardService(gormDB),
		service.NewXianyuCardService(gormDB),
	)
	auth.POST("/cards/cross-publish", crossPubCtrl.CrossPublish)

	douyinCardCtrl := controller.NewDouyinCardController(service.NewDouyinCardService(gormDB))
	auth.GET("/douyin-card/list", douyinCardCtrl.GetList)
	auth.POST("/douyin-card", douyinCardCtrl.Create)
	auth.PUT("/douyin-card/:id", douyinCardCtrl.Update)
	auth.DELETE("/douyin-card/:id", douyinCardCtrl.Delete)
	auth.GET("/douyin-card/:id", douyinCardCtrl.GetByID)
	auth.GET("/douyin/list", douyinCardCtrl.GetList)
	auth.GET("/douyin/:id", douyinCardCtrl.GetByID)
	auth.POST("/douyin/create", douyinCardCtrl.Create)
	auth.PUT("/douyin/update", douyinCardCtrl.Update)
	auth.DELETE("/douyin/delete/:id", douyinCardCtrl.Delete)
	auth.GET("/douyin/view/:id", douyinCardCtrl.GetByID)
	auth.POST("/douyin/:id/generate-short-link", douyinCardCtrl.GenerateShortLink)

	kuaishouCardCtrl := controller.NewKuaishouCardController(service.NewKuaishouCardService(gormDB))
	auth.GET("/kuaishou-card/list", kuaishouCardCtrl.GetList)
	auth.POST("/kuaishou-card", kuaishouCardCtrl.Create)
	auth.PUT("/kuaishou-card/:id", kuaishouCardCtrl.Update)
	auth.DELETE("/kuaishou-card/:id", kuaishouCardCtrl.Delete)
	auth.GET("/kuaishou-card/:id", kuaishouCardCtrl.GetByID)
	auth.GET("/kuaishou/list", kuaishouCardCtrl.GetList)
	auth.GET("/kuaishou/:id", kuaishouCardCtrl.GetByID)
	auth.POST("/kuaishou/create", kuaishouCardCtrl.Create)
	auth.PUT("/kuaishou/update", kuaishouCardCtrl.Update)
	auth.DELETE("/kuaishou/delete/:id", kuaishouCardCtrl.Delete)
	auth.GET("/kuaishou/view/:id", kuaishouCardCtrl.GetByID)
	auth.POST("/kuaishou/like/:id", kuaishouCardCtrl.LikeCard)
	auth.POST("/kuaishou/share/:id", kuaishouCardCtrl.ShareCard)
	auth.POST("/kuaishou/:id/generate-short-link", kuaishouCardCtrl.GenerateShortLink)

	xiaohongshuCardCtrl := controller.NewXiaohongshuCardController(service.NewXiaohongshuCardService(gormDB))
	auth.GET("/xiaohongshu-card/list", xiaohongshuCardCtrl.GetList)
	auth.POST("/xiaohongshu-card", xiaohongshuCardCtrl.Create)
	auth.PUT("/xiaohongshu-card/:id", xiaohongshuCardCtrl.Update)
	auth.DELETE("/xiaohongshu-card/:id", xiaohongshuCardCtrl.Delete)
	auth.GET("/xiaohongshu-card/:id", xiaohongshuCardCtrl.GetByID)
	auth.GET("/xiaohongshu/list", xiaohongshuCardCtrl.GetList)
	auth.GET("/xiaohongshu/:id", xiaohongshuCardCtrl.GetByID)
	auth.POST("/xiaohongshu/create", xiaohongshuCardCtrl.Create)
	auth.PUT("/xiaohongshu/update", xiaohongshuCardCtrl.Update)
	auth.DELETE("/xiaohongshu/delete/:id", xiaohongshuCardCtrl.Delete)
	auth.GET("/xiaohongshu/view/:id", xiaohongshuCardCtrl.GetByID)
	auth.POST("/xiaohongshu/:id/generate-short-link", xiaohongshuCardCtrl.GenerateShortLink)

	xianyuCardCtrl := controller.NewXianyuCardController(service.NewXianyuCardService(gormDB), service.NewXianyuCardStatsService(gormDB))
	auth.GET("/xianyu-card/list", xianyuCardCtrl.GetList)
	auth.POST("/xianyu-card", xianyuCardCtrl.Create)
	auth.PUT("/xianyu-card/:id", xianyuCardCtrl.Update)
	auth.DELETE("/xianyu-card/:id", xianyuCardCtrl.Delete)
	auth.GET("/xianyu-card/:id", xianyuCardCtrl.GetByID)
	auth.GET("/xianyu/list", xianyuCardCtrl.GetList)
	auth.GET("/xianyu/:id", xianyuCardCtrl.GetByID)
	auth.POST("/xianyu/create", xianyuCardCtrl.Create)
	auth.PUT("/xianyu/update", xianyuCardCtrl.Update)
	auth.DELETE("/xianyu/delete/:id", xianyuCardCtrl.Delete)
	auth.GET("/xianyu/view/:id", xianyuCardCtrl.GetByID)
	auth.POST("/xianyu/:id/generate-short-link", xianyuCardCtrl.GenerateShortLink)
}

func setupCardStatsRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	douyinStatsCtrl := controller.NewDouyinCardStatsController(service.NewDouyinCardStatsService(gormDB))
	auth.GET("/douyin-card/stats/:id", douyinStatsCtrl.GetCardStats)
	auth.GET("/douyin-card/overall-stats", douyinStatsCtrl.GetOverallStats)
	auth.GET("/douyin/stats/card/:id", douyinStatsCtrl.GetCardStats)
	auth.GET("/douyin/stats/overall", douyinStatsCtrl.GetOverallStats)

	kuaishouStatsCtrl := controller.NewKuaishouCardStatsController(service.NewKuaishouCardStatsService(gormDB))
	auth.GET("/kuaishou-card/stats/:id", kuaishouStatsCtrl.GetCardStats)
	auth.GET("/kuaishou-card/overall-stats", kuaishouStatsCtrl.GetOverallStats)
	auth.GET("/kuaishou/stats/card/:id", kuaishouStatsCtrl.GetCardStats)
	auth.GET("/kuaishou/stats/overall", kuaishouStatsCtrl.GetOverallStats)

	xiaohongshuStatsCtrl := controller.NewXiaohongshuCardStatsController(service.NewXiaohongshuCardStatsService(gormDB))
	auth.GET("/xiaohongshu-card/stats/:id", xiaohongshuStatsCtrl.GetCardStats)
	auth.GET("/xiaohongshu-card/overall-stats", xiaohongshuStatsCtrl.GetOverallStats)
	auth.GET("/xiaohongshu/stats/card/:id", xiaohongshuStatsCtrl.GetCardStats)
	auth.GET("/xiaohongshu/stats/overall", xiaohongshuStatsCtrl.GetOverallStats)

	xianyuStatsCtrl := controller.NewXianyuCardStatsController(service.NewXianyuCardStatsService(gormDB))
	auth.GET("/xianyu-card/stats/:id", xianyuStatsCtrl.GetCardStats)
	auth.GET("/xianyu-card/overall-stats", xianyuStatsCtrl.GetOverallStats)
	auth.GET("/xianyu/stats/card/:id", xianyuStatsCtrl.GetCardStats)
	auth.GET("/xianyu/stats/overall", xianyuStatsCtrl.GetOverallStats)

	factory := controller.NewCardStatsFactoryController(
		service.NewPlatformDouyinCardStatsAdapter(service.NewDouyinCardStatsService(gormDB)),
		service.NewPlatformKuaishouCardStatsAdapter(service.NewKuaishouCardStatsService(gormDB)),
		service.NewPlatformXiaohongshuCardStatsAdapter(service.NewXiaohongshuCardStatsService(gormDB)),
		service.NewPlatformXianyuCardStatsAdapter(service.NewXianyuCardStatsService(gormDB)),
		service.NewPlatformTiktokCardStatsAdapter(service.NewTikTokCardServiceWithDB(gormDB), gormDB),
	)
	auth.GET("/card-stats/:platform/stats/:id", factory.GetCardStats)
	auth.GET("/card-stats/:platform/overall", factory.GetOverallStats)
}
