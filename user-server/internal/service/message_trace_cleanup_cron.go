package service

import (
	"context"
	"regexp"
	"sync"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
)

// MessageTraceCleanupTask message_trace 正文 TTL 分层清理任务（M16 表 TR-2 / TR-3 轻量版）。
//
// 决策源：docs/architecture/MASTER_COMPETITIVE_DECISIONS.md M16 表 TR-2、TR-3。
//   - TR-2 正文 TTL：每日清理 90 天前的 input/output 正文置 NULL（结构字段保留），
//     满足 GDPR 存储限制；llm_routing_logs 保留不动。
//   - TR-3 PII 脱敏（修订落地口径）：清理任务执行时对"未到 90 天但已超过 30 天"的
//     正文做手机号/邮箱打码（正则替换）；实时链路零开销，不增加请求延迟。
//
// 打码具备天然幂等性：已打码文本不再命中正则（手机号中间段被 * 替换后不足 11 位连续数字，
// 邮箱本地部分被截断为 首字符+*** 后不再匹配完整邮箱形态），重复执行不会二次破坏内容。
type MessageTraceCleanupTask struct {
	db       *gorm.DB
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	// nowFn 测试注入用时钟
	nowFn func() time.Time
}

func NewMessageTraceCleanupTask(database *gorm.DB) *MessageTraceCleanupTask {
	if database == nil {
		database = db.GetDB()
	}
	return &MessageTraceCleanupTask{
		db:     database,
		stopCh: make(chan struct{}),
		nowFn:  time.Now,
	}
}

// ===== PII 正则掩码（TR-3） =====

var (
	// piiPhoneRe 中国大陆手机号（11 位连续数字）：保留前 3 后 2，中间打码。13812345678 → 138****78
	piiPhoneRe = regexp.MustCompile(`(1[3-9]\d)\d{4}\d{2}(\d{2})`)
	// piiEmailRe 邮箱：本地部分仅保留首字符。john.doe@example.com → j***@example.com
	piiEmailRe = regexp.MustCompile(`([A-Za-z0-9._%+-])[A-Za-z0-9._%+-]*(@[A-Za-z0-9.-]+\.[A-Za-z]{2,})`)
)

// MaskPII 对文本中的手机号/邮箱做轻量打码（幂等）。
func MaskPII(text string) string {
	if text == "" {
		return text
	}
	out := piiPhoneRe.ReplaceAllString(text, "$1****$2")
	out = piiEmailRe.ReplaceAllString(out, "$1***$2")
	return out
}

const (
	// tracePIIMaskAge 正文超过该天数即进入 PII 打码窗口
	tracePIIMaskAge = 30 * 24 * time.Hour
	// traceTTLLimitAge 正文超过该天数被置 NULL（结构字段保留）
	traceTTLLimitAge = 90 * 24 * time.Hour
	// traceMaskBatchSize 打码阶段单批扫描行数
	traceMaskBatchSize = 500
)

// RunOnce 执行一轮清理：
//  1. PII 打码：30~90 天区间的 input/output 做手机号/邮箱掩码；
//  2. TTL 置 NULL：90 天前的 input/output 置 NULL（保留全部结构字段）。
//
// 返回 (打码行数, 置 NULL 行数, error)。DB 未就绪时返回 (0,0,nil) 静默跳过（自愈）。
func (t *MessageTraceCleanupTask) RunOnce(ctx context.Context) (masked int64, nulled int64, err error) {
	if t.db == nil {
		return 0, 0, nil
	}
	masked, err = t.maskAgedBodies(ctx)
	if err != nil {
		return masked, 0, err
	}
	nulled, err = t.nullExpiredBodies(ctx)
	return masked, nulled, err
}

// maskAgedBodies 对 30~90 天的正文分批扫描并打码（仅更新发生变化的行）。
func (t *MessageTraceCleanupTask) maskAgedBodies(ctx context.Context) (int64, error) {
	now := t.nowFn()
	upper := now.Add(-tracePIIMaskAge)
	lower := now.Add(-traceTTLLimitAge)
	var masked int64
	var lastID uint
	for {
		var rows []model.MessageTrace
		if err := t.db.WithContext(ctx).
			Where("created_at <= ? AND created_at > ? AND id > ?", upper, lower, lastID).
			Order("id ASC").Limit(traceMaskBatchSize).
			Select("id", "input", "output").
			Find(&rows).Error; err != nil {
			return masked, err
		}
		if len(rows) == 0 {
			return masked, nil
		}
		for _, r := range rows {
			lastID = r.ID
			newIn := MaskPII(r.Input)
			newOut := MaskPII(r.Output)
			if newIn == r.Input && newOut == r.Output {
				continue
			}
			res := t.db.WithContext(ctx).Model(&model.MessageTrace{}).
				Where("id = ?", r.ID).
				Updates(map[string]any{"input": newIn, "output": newOut})
			if res.Error != nil {
				return masked, res.Error
			}
			masked += res.RowsAffected
		}
		if len(rows) < traceMaskBatchSize {
			return masked, nil
		}
	}
}

// nullExpiredBodies 将 90 天前的正文置 NULL（结构字段与元数据保留）。
func (t *MessageTraceCleanupTask) nullExpiredBodies(ctx context.Context) (int64, error) {
	cutoff := t.nowFn().Add(-traceTTLLimitAge)
	res := t.db.WithContext(ctx).Model(&model.MessageTrace{}).
		Where("created_at < ?", cutoff).
		Where("(input IS NOT NULL OR output IS NOT NULL)").
		UpdateColumns(map[string]any{"input": gorm.Expr("NULL"), "output": gorm.Expr("NULL")})
	return res.RowsAffected, res.Error
}

// Start 启动每日清理循环：立即执行一次，之后每 24h 触发。
// DB 未就绪时静默跳过（下一次 ticker 自愈）。供 main/router 显式接线；
// 同时由包级 init 自动启动（沿用 SessionTTLCron 惯例，main.go 零改动）。
func (t *MessageTraceCleanupTask) Start(ctx context.Context) {
	if t == nil || t.db == nil {
		return
	}
	t.wg.Add(1)
	go t.run(ctx)
}

func (t *MessageTraceCleanupTask) run(ctx context.Context) {
	defer t.wg.Done()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.trigger(ctx)
		}
	}
}

func (t *MessageTraceCleanupTask) trigger(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[trace_cleanup_cron] panic: %v", r)
		}
	}()
	masked, nulled, err := t.RunOnce(ctx)
	if err != nil {
		logger.Errorf("[trace_cleanup_cron] run failed: masked=%d nulled=%d err=%v", masked, nulled, err)
		return
	}
	if masked > 0 || nulled > 0 {
		logger.Infof("[trace_cleanup_cron] done: pii_masked=%d ttl_nulled=%d", masked, nulled)
	}
}

// Stop 优雅停止（幂等）。
func (t *MessageTraceCleanupTask) Stop(ctx context.Context) {
	if t == nil {
		return
	}
	t.stopOnce.Do(func() { close(t.stopCh) })
	done := make(chan struct{})
	go func() { t.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// traceCleanupCron 全局单例（init 自启动 + Stop 优雅退出，模式同 SessionTTLCron）
var traceCleanupCron *MessageTraceCleanupTask

func init() {
	traceCleanupCron = NewMessageTraceCleanupTask(nil)
	traceCleanupCron.Start(context.Background())
}

// StopMessageTraceCleanupCron 进程退出时可调用（可选接线）。
func StopMessageTraceCleanupCron(ctx context.Context) {
	if traceCleanupCron != nil {
		traceCleanupCron.Stop(ctx)
	}
}
