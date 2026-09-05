package cron

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// SnoozeRecoveryCron 会话暂缓到期恢复定时任务
type SnoozeRecoveryCron struct {
	interval  time.Duration
	executeFn func(ctx context.Context) (int64, error)

	stop      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
}

// NewSnoozeRecoveryCron 构造（executeFn 由装配处注入：service 层 RecoverSnoozed）
func NewSnoozeRecoveryCron(executeFn func(ctx context.Context) (int64, error)) *SnoozeRecoveryCron {
	return &SnoozeRecoveryCron{interval: 5 * time.Minute, executeFn: executeFn, stop: make(chan struct{})}
}

// Start 幂等启动
func (c *SnoozeRecoveryCron) Start() {
	c.startOnce.Do(func() {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[SnoozeCron] panic 隔离: %v", r)
				}
			}()
			ticker := time.NewTicker(c.interval)
			defer ticker.Stop()
			for {
				select {
				case <-c.stop:
					return
				case <-ticker.C:
					n, err := c.executeFn(context.Background())
					if err != nil {
						logger.Warnf("[SnoozeCron] 到期恢复失败: %v", err)
						continue
					}
					if n > 0 {
						logger.Infof("[SnoozeCron] 已恢复 %d 个暂缓会话", n)
					}
				}
			}
		}()
	})
}

// Stop 停止
func (c *SnoozeRecoveryCron) Stop() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
	c.wg.Wait()
}
