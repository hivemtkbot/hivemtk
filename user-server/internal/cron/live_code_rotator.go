package cron

import (
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/service"
	"time"
)

// LiveCodeRotator 活码轮询任务
type LiveCodeRotator struct {
	liveCodeService service.LiveCodeService
}

// NewLiveCodeRotator 创建活码轮询任务实例
func NewLiveCodeRotator(liveCodeService service.LiveCodeService) *LiveCodeRotator {
	return &LiveCodeRotator{
		liveCodeService: liveCodeService,
	}
}

// Start 启动活码轮询任务
func (r *LiveCodeRotator) Start() {
	// 立即执行一次
	go r.rotate()

	// 设置定时器，每小时执行一次
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go r.rotate()
		}
	}
}

// rotate 执行轮询逻辑
func (r *LiveCodeRotator) rotate() {
	logger.Info("开始执行活码轮询任务...")

	err := r.liveCodeService.RotateLiveCodes()
	if err != nil {
		logger.Errorf("活码轮询任务执行失败: %v", err)
		return
	}

	logger.Info("活码轮询任务执行完成")
}
