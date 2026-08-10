package service

// customer_session_cron.go 客服会话 TTL 定时任务
//
// 五层架构归属: L3 业务层
// 设计依据: docs/audit/TELEGRAM_FULLCHAIN_AUDIT_2026_07_28.md §S2-3
//
// 单一职责：每小时检查一次「活跃但超过 24h 无互动」的会话，自动 close。
//   - 仅作用于 pending / ai_handling / waiting / human_handling 四种状态
//   - 已 resolved / closed 的会话不重复处理
//   - 单批最多 500 条（repository 层 limit + 分批 UPDATE）
//   - 失败仅记录日志，不阻塞下一次调度
//
// 启动方式：通过 init() 在包加载时自动启动 1 个后台 goroutine；进程退出由
// sessionTTLCron.Stop 优雅关闭（与 SelfLearningCron.Stop 同模式）。

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// SessionTTLCron 会话 TTL 自动关闭定时任务
type SessionTTLCron struct {
	sessionSvc *CustomerSessionService
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

// NewSessionTTLCron 启动会话 TTL 定时任务
//
// 立即执行一次（避免首次 1h 延迟），之后按 hour ticker 触发。
// sessionSvc 不可为 nil（nil 时直接返回不启动）。
func NewSessionTTLCron(sessionSvc *CustomerSessionService) *SessionTTLCron {
	if sessionSvc == nil {
		return nil
	}
	c := &SessionTTLCron{
		sessionSvc: sessionSvc,
		stopCh:     make(chan struct{}),
	}
	c.wg.Add(1)
	go c.run(context.Background())
	return c
}

// Stop 优雅停止（与 SelfLearningCron.Stop 同模式）
func (c *SessionTTLCron) Stop(ctx context.Context) {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		logger.Errorf("[session_ttl_cron] Stop ctx done before goroutine exited: %v", ctx.Err())
	}
}

// run 每小时跑一次
func (c *SessionTTLCron) run(ctx context.Context) {
	defer c.wg.Done()
	// 启动后立即执行一次（清理历史堆积）；
	// DB 未就绪时静默等待（避免 init() 时机早于 db.InitDB 的 panic）
	c.tryTriggerWithDBWait(ctx)
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.tryTriggerWithDBWait(ctx)
		case <-c.stopCh:
			return
		}
	}
}

// tryTriggerWithDBWait 在 DB 未就绪时静默跳过（init 时机早于 db.InitDB 的常见现象）
//
// 判断方式：直接尝试触发一次 trigger，DB 未就绪会让 service.repo 拿不到 db
// 句柄，repository 层在 AutoCloseStaleSessions 内部会因 nil DB panic → 已 recover。
// 此处只判断更轻量的：拿一次 sessionRepo.GetDB，看是否非 nil；为 nil 时跳过本次。
func (c *SessionTTLCron) tryTriggerWithDBWait(ctx context.Context) {
	if c.sessionSvc == nil || c.sessionSvc.sessionRepo == nil {
		// service 未装配（DB 未就绪），跳过本次
		return
	}
	if db := c.sessionSvc.sessionRepo.GetDB(ctx); db == nil {
		return
	}
	c.trigger(ctx)
}

func (c *SessionTTLCron) trigger(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[session_ttl_cron] panic: %v", r)
		}
	}()
	_, err := c.sessionSvc.AutoCloseStaleSessions(ctx)
	if err != nil {
		logger.Errorf("[session_ttl_cron] auto close failed: %v", err)
	}
}

// sessionTTLCron 全局单例（init 启动 + Stop 优雅退出）
var sessionTTLCron *SessionTTLCron

// init 包加载时自动启动 TTL cron 任务
//
// 为什么这里用 init 而非 NewSystemInitService 显式调用：
//   - InitSystemInitService 在该项目里也是 dead code（无 caller），沿用同模式
//   - init 触发保证 cron 在 main 启动后立即就位，无需修改 main.go
//   - DB 初始化时序风险：cron 立即跑一次可能拿到空 DB（repository 会返回 0 行不报错）
//   - 即使 DB 暂未就绪，下一次 ticker 仍会重试 → 自愈
func init() {
	sessionTTLCron = NewSessionTTLCron(NewCustomerSessionService())
}

// StopSessionTTLCron 进程退出时由 main 调用（与 defer 配合）
func StopSessionTTLCron(ctx context.Context) {
	if sessionTTLCron != nil {
		sessionTTLCron.Stop(ctx)
	}
}
