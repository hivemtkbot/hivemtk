package cron

import (
	"context"
	"fmt"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/service"
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
	// 每分钟执行一次
	_, err := mgr.AddTask("0 * * * * *", func() {
		EmailListCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加email列表定时任务失败 %s", err.Error()))
		panic(err)
	}

	// 每小时执行一次活码轮询任务
	_, err = mgr.AddTask("0 0 * * * *", func() {
		LiveCodeRotateCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加活码轮询定时任务失败 %s", err.Error()))
		panic(err)
	}
	// 每日清理自动回复日志
	_, err = mgr.AddTask("0 0 2 * * *", func() {
		AutoReplyCleanupCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加自动回复日志清理任务失败 %s", err.Error()))
		panic(err)
	}
}

// LiveCodeRotateCron 活码轮询定时任务
func LiveCodeRotateCron() {
	logger.Info("开始执行活码轮询定时任务...")

	// 创建活码服务实例
	liveCodeService := service.NewLiveCodeService(db.GetDB())

	// 执行轮询
	err := liveCodeService.RotateLiveCodes(context.Background())
	if err != nil {
		logger.Error(err, "活码轮询定时任务执行失败")
		return
	}

	logger.Info("活码轮询定时任务执行完成")
}
