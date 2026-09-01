package cron

import (
	"context"
	"fmt"
	geoservice "hivemtk-user/internal/geo/service"
	opsservice "hivemtk-user/internal/ops/service"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"
	"sync"

	"github.com/robfig/cron/v3"
)

var (
	taskManager *TaskManager
	once        sync.Once
)

type TaskManager struct {
	cron *cron.Cron
	mu   sync.Mutex
}

func GetTaskManager() *TaskManager {
	once.Do(func() {
		taskManager = &TaskManager{
			cron: cron.New(cron.WithSeconds()),
		}
		taskManager.cron.Start()
	})
	return taskManager
}

func (tm *TaskManager) AddTask(spec string, cmd func()) (cron.EntryID, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.cron.AddFunc(spec, cmd)
}

func (tm *TaskManager) RemoveTask(id cron.EntryID) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cron.Remove(id)
}

func InitCron() {
	mgr := GetTaskManager()
	_, err := mgr.AddTask("0 * * * * *", func() {
		EmailListCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加email列表定时任务失败 %s", err.Error()))
		panic(err)
	}

	_, err = mgr.AddTask("0 0 * * * *", func() {
		LiveCodeRotateCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加活码轮询定时任务失败 %s", err.Error()))
		panic(err)
	}

	_, err = mgr.AddTask("0 30 3 * * *", func() {
		ChurnCalculationCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加流失预测计算任务失败 %s", err.Error()))
		panic(err)
	}

	_, err = mgr.AddTask("0 0 4 * * *", func() {
		PasswordResetTokenCleanupCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加密码重置令牌清理任务失败 %s", err.Error()))
		panic(err)
	}

	// G10: 离线消息回扫 - 每 5 分钟扫描 bridge_metrics 发现离线渠道并回扫累积消息
	_, err = mgr.AddTask("0 */5 * * * *", func() {
		go service.NewBridgeOfflineReplayService().RunOnce(context.Background())
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加离线消息回扫定时任务失败 %s", err.Error()))
		panic(err)
	}

	// G14: 工单升级/转派链 - 每 5 分钟扫描未解决会话执行规则链
	_, err = mgr.AddTask("5 */5 * * * *", func() {
		go service.NewHandoffChainService().RunCron(context.Background(), 200)
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加工单升级链定时任务失败 %s", err.Error()))
		panic(err)
	}

	// GEO: SOV 刷新 — 每天凌晨 2:00
	_, err = mgr.AddTask("0 0 2 * * *", func() {
		go geoservice.SOVRefreshCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加 GEO SOV 刷新定时任务失败 %s", err.Error()))
		panic(err)
	}

	// GEO: 负面监控 — 每 30 分钟
	_, err = mgr.AddTask("0 */30 * * * *", func() {
		go geoservice.NegativeMonitorCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加 GEO 负面监控定时任务失败 %s", err.Error()))
		panic(err)
	}

	// GEO: 信源目录同步 — 每天凌晨 3:00
	_, err = mgr.AddTask("0 0 3 * * *", func() {
		go geoservice.SourceCatalogSyncCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加 GEO 信源同步定时任务失败 %s", err.Error()))
		panic(err)
	}

	// GEO: 竞品监控爬虫 — 每 6 小时爬一次竞品站点，真实 HTTP 请求记录访问
	_, err = mgr.AddTask("0 0 */6 * * *", func() {
		go geoservice.CrawlerMonitorCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加 GEO 竞品监控爬虫定时任务失败 %s", err.Error()))
		panic(err)
	}
}

// ChurnCalculationCron 流失预测定时任务：从 customer_events 真实聚合全部客户行为并执行流失计算。
// 解决此前 churn_predictions 表永远为空（流失页面无真实数据）的断头功能问题。
func ChurnCalculationCron() {
	logger.Info("开始执行流失预测计算定时任务...")
	churnService := opsservice.NewChurnPredictionService()
	n, err := churnService.RunChurnCalculationForAllCustomers(context.Background())
	if err != nil {
		logger.Error(err, "流失预测计算定时任务执行失败")
		return
	}
	logger.Info(fmt.Sprintf("流失预测计算定时任务执行完成，覆盖客户数=%d", n))
}

// LiveCodeRotateCron 活码轮询定时任务
func LiveCodeRotateCron() {
	logger.Info("开始执行活码轮询定时任务...")

	liveCodeService := service.NewLiveCodeService(db.GetDB())

	err := liveCodeService.RotateLiveCodes(context.Background())
	if err != nil {
		logger.Error(err, "活码轮询定时任务执行失败")
		return
	}

	logger.Info("活码轮询定时任务执行完成")
}



// PasswordResetTokenCleanupCron 密码重置令牌清理定时任务
func PasswordResetTokenCleanupCron() {
	logger.Info("开始执行密码重置令牌清理定时任务...")
	resetService := service.NewPasswordResetService(db.GetDB())
	if err := resetService.CleanupExpiredTokens(context.Background()); err != nil {
		logger.Error(err, "密码重置令牌清理定时任务执行失败")
		return
	}
	logger.Info("密码重置令牌清理定时任务执行完成")
}
