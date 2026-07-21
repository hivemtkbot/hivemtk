package router

// tuning_routes.go 注册 置信度/拟人度/反馈学习 统一管理 API
//
// 五层架构归属: L2 网关层
// 设计依据: docs/核心链路优化.md 第十五/十六/十七章

import (
	"marketing/internal/controller"

	"github.com/gin-gonic/gin"
)

// setupTuningRoutes 注册 置信度/拟人度/反馈学习 统一管理 API
//
// 路由前缀：/api/admin/tuning/
// 中间件：继承 auth group（InitGuard + JWTAuthMiddleware + LicenseGuard）
//
// 涵盖：
//   - 置信度信号/校准/阈值策略
//   - 拟人度评分/销冠基线
//   - 反馈事件/销冠对话/Prompt 候选/Bandit 臂
//   - 低质样本
func setupTuningRoutes(auth *gin.RouterGroup) {
	tuning := auth.Group("/admin/tuning")
	ctrl := controller.NewTuningController()

	// 1. 置信度信号
	tuning.GET("/confidence/signals", ctrl.ListConfidenceSignals)
	tuning.GET("/confidence/signals/:id", ctrl.GetConfidenceSignal)
	tuning.GET("/confidence/signals/stats", ctrl.StatsConfidenceSignals)

	// 2. 置信度校准
	tuning.GET("/confidence/calibrations", ctrl.ListCalibrations)

	// 3. 阈值策略
	tuning.GET("/confidence/policies", ctrl.ListThresholdPolicies)
	tuning.PUT("/confidence/policies", ctrl.UpsertThresholdPolicy)

	// 4. 拟人度评分
	tuning.GET("/humanize/scores", ctrl.ListHumanizeScores)
	tuning.GET("/humanize/scores/stats", ctrl.StatsHumanizeScores)

	// 5. 销冠基线
	tuning.GET("/humanize/baselines", ctrl.ListChampionBaselines)

	// 6. 反馈事件
	tuning.GET("/feedback/events", ctrl.ListFeedbackEvents)
	tuning.GET("/feedback/events/stats", ctrl.StatsFeedbackEvents)

	// 7. 销冠对话
	tuning.GET("/feedback/dialogues", ctrl.ListChampionDialogues)

	// 8. Prompt 候选
	tuning.GET("/prompt/candidates", ctrl.ListPromptCandidates)
	tuning.PUT("/prompt/candidates/:id/status", ctrl.UpdatePromptCandidateStatus)

	// 9. Bandit 臂
	tuning.GET("/bandit/arms", ctrl.ListBanditArms)

	// 10. 低质样本
	tuning.GET("/humanize/low-quality", ctrl.ListLowQualitySamples)
}
