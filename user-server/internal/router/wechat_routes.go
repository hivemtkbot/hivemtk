package router

import (
	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

func setupCardShareRoutes(r *gin.Engine) {
	douyinCardCtrl := controller.NewDouyinCardController(service.NewDouyinCardService(db.GetDB()))
	r.GET("/share/douyin/:id", douyinCardCtrl.SharePage)

	kuaishouCardCtrl := controller.NewKuaishouCardController(service.NewKuaishouCardService(db.GetDB()))
	r.GET("/share/kuaishou/:id", kuaishouCardCtrl.SharePage)

	xiaohongshuCardCtrl := controller.NewXiaohongshuCardController(service.NewXiaohongshuCardService(db.GetDB()))
	r.GET("/share/xiaohongshu/:id", xiaohongshuCardCtrl.SharePage)

	xianyuCardCtrl := controller.NewXianyuCardController(service.NewXianyuCardService(db.GetDB()), service.NewXianyuCardStatsService(db.GetDB()))
	r.GET("/share/xianyu/:id", xianyuCardCtrl.SharePage)
}
