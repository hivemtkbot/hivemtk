package router

import (
	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// setupCardRoutes 卡片管理路由（抖音、快手、小红书、闲鱼）
func setupCardRoutes(auth *gin.RouterGroup) {
	// 抖音卡片
	douyinCardCtrl := controller.NewDouyinCardController(service.NewDouyinCardService(db.GetDB()))
	auth.GET("/douyin-card/list", douyinCardCtrl.GetList)
	auth.POST("/douyin-card", douyinCardCtrl.Create)
	auth.PUT("/douyin-card/:id", douyinCardCtrl.Update)
	auth.DELETE("/douyin-card/:id", douyinCardCtrl.Delete)
	auth.GET("/douyin-card/:id", douyinCardCtrl.GetByID)
	auth.GET("/douyin/list", douyinCardCtrl.GetList)
	auth.GET("/douyin/:id", douyinCardCtrl.GetByID)
	auth.POST("/douyin/create", douyinCardCtrl.Create)
	auth.PUT("/douyin/update", func(c *gin.Context) { douyinCardCtrl.Update(c) })
	auth.DELETE("/douyin/delete/:id", douyinCardCtrl.Delete)
	auth.GET("/douyin/view/:id", douyinCardCtrl.GetByID)
	auth.POST("/douyin/:id/generate-short-link", douyinCardCtrl.GenerateShortLink)

	// 快手卡片
	kuaishouCardCtrl := controller.NewKuaishouCardController(service.NewKuaishouCardService(db.GetDB()))
	auth.GET("/kuaishou-card/list", kuaishouCardCtrl.GetList)
	auth.POST("/kuaishou-card", kuaishouCardCtrl.Create)
	auth.PUT("/kuaishou-card/:id", kuaishouCardCtrl.Update)
	auth.DELETE("/kuaishou-card/:id", kuaishouCardCtrl.Delete)
	auth.GET("/kuaishou-card/:id", kuaishouCardCtrl.GetByID)
	auth.GET("/kuaishou/list", kuaishouCardCtrl.GetList)
	auth.GET("/kuaishou/:id", kuaishouCardCtrl.GetByID)
	auth.POST("/kuaishou/create", kuaishouCardCtrl.Create)
	auth.PUT("/kuaishou/update", func(c *gin.Context) { kuaishouCardCtrl.Update(c) })
	auth.DELETE("/kuaishou/delete/:id", kuaishouCardCtrl.Delete)
	auth.GET("/kuaishou/view/:id", kuaishouCardCtrl.GetByID)
	auth.POST("/kuaishou/like/:id", kuaishouCardCtrl.LikeCard)
	auth.POST("/kuaishou/share/:id", kuaishouCardCtrl.ShareCard)
	auth.POST("/kuaishou/:id/generate-short-link", kuaishouCardCtrl.GenerateShortLink)

	// 小红书卡片
	xiaohongshuCardCtrl := controller.NewXiaohongshuCardController(service.NewXiaohongshuCardService(db.GetDB()))
	auth.GET("/xiaohongshu-card/list", xiaohongshuCardCtrl.GetList)
	auth.POST("/xiaohongshu-card", xiaohongshuCardCtrl.Create)
	auth.PUT("/xiaohongshu-card/:id", xiaohongshuCardCtrl.Update)
	auth.DELETE("/xiaohongshu-card/:id", xiaohongshuCardCtrl.Delete)
	auth.GET("/xiaohongshu-card/:id", xiaohongshuCardCtrl.GetByID)
	auth.GET("/xiaohongshu/list", xiaohongshuCardCtrl.GetList)
	auth.GET("/xiaohongshu/:id", xiaohongshuCardCtrl.GetByID)
	auth.POST("/xiaohongshu/create", xiaohongshuCardCtrl.Create)
	auth.PUT("/xiaohongshu/update", func(c *gin.Context) { xiaohongshuCardCtrl.Update(c) })
	auth.DELETE("/xiaohongshu/delete/:id", xiaohongshuCardCtrl.Delete)
	auth.GET("/xiaohongshu/view/:id", xiaohongshuCardCtrl.GetByID)
	auth.POST("/xiaohongshu/:id/generate-short-link", xiaohongshuCardCtrl.GenerateShortLink)

	// 闲鱼卡片
	xianyuCardCtrl := controller.NewXianyuCardController(service.NewXianyuCardService(db.GetDB()), service.NewXianyuCardStatsService(db.GetDB()))
	auth.GET("/xianyu-card/list", xianyuCardCtrl.GetList)
	auth.POST("/xianyu-card", xianyuCardCtrl.Create)
	auth.PUT("/xianyu-card/:id", xianyuCardCtrl.Update)
	auth.DELETE("/xianyu-card/:id", xianyuCardCtrl.Delete)
	auth.GET("/xianyu-card/:id", xianyuCardCtrl.GetByID)
	auth.GET("/xianyu/list", xianyuCardCtrl.GetList)
	auth.GET("/xianyu/:id", xianyuCardCtrl.GetByID)
	auth.POST("/xianyu/create", xianyuCardCtrl.Create)
	auth.PUT("/xianyu/update", func(c *gin.Context) { xianyuCardCtrl.Update(c) })
	auth.DELETE("/xianyu/delete/:id", xianyuCardCtrl.Delete)
	auth.GET("/xianyu/view/:id", xianyuCardCtrl.GetByID)
	auth.POST("/xianyu/:id/generate-short-link", xianyuCardCtrl.GenerateShortLink)
}

// setupCardStatsRoutes 卡片统计路由
func setupCardStatsRoutes(auth *gin.RouterGroup) {
	// 抖音统计
	douyinStatsCtrl := controller.NewDouyinCardStatsController(service.NewDouyinCardStatsService(db.GetDB()))
	auth.GET("/douyin-card/stats/:id", douyinStatsCtrl.GetCardStats)
	auth.GET("/douyin-card/overall-stats", douyinStatsCtrl.GetOverallStats)
	auth.GET("/douyin/stats/card/:id", douyinStatsCtrl.GetCardStats)
	auth.GET("/douyin/stats/overall", douyinStatsCtrl.GetOverallStats)

	// 快手统计
	kuaishouStatsCtrl := controller.NewKuaishouCardStatsController(service.NewKuaishouCardStatsService(db.GetDB()))
	auth.GET("/kuaishou-card/stats/:id", kuaishouStatsCtrl.GetCardStats)
	auth.GET("/kuaishou-card/overall-stats", kuaishouStatsCtrl.GetOverallStats)
	auth.GET("/kuaishou/stats/card/:id", kuaishouStatsCtrl.GetCardStats)
	auth.GET("/kuaishou/stats/overall", kuaishouStatsCtrl.GetOverallStats)

	// 小红书统计
	xiaohongshuStatsCtrl := controller.NewXiaohongshuCardStatsController(service.NewXiaohongshuCardStatsService(db.GetDB()))
	auth.GET("/xiaohongshu-card/stats/:id", xiaohongshuStatsCtrl.GetCardStats)
	auth.GET("/xiaohongshu-card/overall-stats", xiaohongshuStatsCtrl.GetOverallStats)
	auth.GET("/xiaohongshu/stats/card/:id", xiaohongshuStatsCtrl.GetCardStats)
	auth.GET("/xiaohongshu/stats/overall", xiaohongshuStatsCtrl.GetOverallStats)

	// 闲鱼统计
	xianyuStatsCtrl := controller.NewXianyuCardStatsController(service.NewXianyuCardStatsService(db.GetDB()))
	auth.GET("/xianyu-card/stats/:id", xianyuStatsCtrl.GetCardStats)
	auth.GET("/xianyu-card/overall-stats", xianyuStatsCtrl.GetOverallStats)
	auth.GET("/xianyu/stats/card/:id", xianyuStatsCtrl.GetCardStats)
	auth.GET("/xianyu/stats/overall", xianyuStatsCtrl.GetOverallStats)
}
