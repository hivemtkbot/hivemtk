package cron

import (
	"context"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
	"time"
)

// DomainHealthCheckJob 域名健康度定时探测任务
// G 域 ：每 5 分钟探测一次所有域名，自动切换到评分最高的健康域名
type DomainHealthCheckJob struct {
	healthSvc service.DomainHealthService
	repo      repository.DomainPoolRepository
	interval  time.Duration
}

// NewDomainHealthCheckJob 创建健康度探测任务
func NewDomainHealthCheckJob(healthSvc service.DomainHealthService, repo repository.DomainPoolRepository) *DomainHealthCheckJob {
	return &DomainHealthCheckJob{
		healthSvc: healthSvc,
		repo:      repo,
		interval:  5 * time.Minute,
	}
}

// Start 启动探测循环（阻塞，应放入独立 goroutine）
func (j *DomainHealthCheckJob) Start() {
	logger.Info("[domain-health] 启动域名健康度定时探测任务")

	// 启动后立即探测一次
	go j.runOnce()

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go j.runOnce()
		}
	}
}

// runOnce 单次执行探测
func (j *DomainHealthCheckJob) runOnce() {
	// 修复：探测 goroutine（Start 中 go j.runOnce()）未 recover，若 CheckAll 内层 panic
	// 会直接击穿进程。recover 后仅记日志，不影响下一次 ticker 触发。
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[domain-health] runOnce panic recovered: %v", r)
		}
	}()
	results, err := j.healthSvc.CheckAll(context.Background())
	if err != nil {
		logger.Errorf("[domain-health] 探测失败: %v", err)
		return
	}
	healthy := 0
	unhealthy := 0
	for _, r := range results {
		if r.HTTPOk && r.DNSOK && !r.OnBlacklist {
			healthy++
		} else {
			unhealthy++
		}
	}
	logger.Infof("[domain-health] 探测完成 total=%d healthy=%d unhealthy=%d", len(results), healthy, unhealthy)
}
