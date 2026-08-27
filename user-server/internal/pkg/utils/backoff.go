package utils

import (
	"math/rand"
	"sync"
	"time"
)

// BackOff 退避策略接口（与 github.com/cenkalti/backoff/v5 的 BackOff 行为对齐）。
//
// NextBackOff 返回：
//   - (duration, false) → 表示本次应等待 duration 后再次调用 fn（duration <= 0 表示立即重试）
//   - (0, true)          → 表示停止重试，调用方应放弃
//
// 实现：
//   - NewExponentialBackOff 指数退避（默认实现）
//   - NewConstantBackOff    固定间隔退避
type BackOff interface {
	NextBackOff() (time.Duration, bool)
}

// ExponentialBackOff 指数退避策略。
//
// 字段语义（与 cenkalti/backoff 对齐）：
//   - InitialInterval 起始间隔，默认 500ms
//   - RandomizationFactor 抖动因子 [0, 1]，默认 0.5（±50% 抖动）
//   - Multiplier 倍增系数，默认 1.5
//   - MaxInterval 单次最大间隔，默认 60s
//   - MaxElapsedTime 总耗时上限，0 表示永不过期（依赖 Stop 标志）
type ExponentialBackOff struct {
	InitialInterval     time.Duration
	RandomizationFactor float64
	Multiplier          float64
	MaxInterval         time.Duration
	MaxElapsedTime      time.Duration

	mu             sync.Mutex
	currentInterval time.Duration
	startTime      time.Time
}

// NewExponentialBackOff 返回带默认值的 ExponentialBackOff。
//
// 默认：500ms 起步，1.5x 增长，上限 60s，±50% 抖动，永不过期。
func NewExponentialBackOff() *ExponentialBackOff {
	return &ExponentialBackOff{
		InitialInterval:     500 * time.Millisecond,
		RandomizationFactor: 0.5,
		Multiplier:          1.5,
		MaxInterval:         60 * time.Second,
		MaxElapsedTime:      0,
	}
}

// NextBackOff 计算下一次退避时长。
//
// 行为：
//   - 第一次调用返回 InitialInterval（带抖动）
//   - 后续每次按 Multiplier 倍增，直到 MaxInterval
//   - 若设置了 MaxElapsedTime 且已超时 → 返回 (0, true) 表示停止
//   - 否则永远返回 (duration, false)
func (b *ExponentialBackOff) NextBackOff() (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 首次调用：记录起始时间
	if b.startTime.IsZero() {
		b.startTime = time.Now()
		b.currentInterval = b.InitialInterval
	}

	// 检查总耗时上限
	if b.MaxElapsedTime > 0 && time.Since(b.startTime) > b.MaxElapsedTime {
		return 0, true
	}

	// 抖动
	interval := jitter(b.currentInterval, b.RandomizationFactor)

	// 计算下一次间隔
	next := time.Duration(float64(b.currentInterval) * b.Multiplier)
	if next > b.MaxInterval {
		next = b.MaxInterval
	}
	b.currentInterval = next

	return interval, false
}

// Reset 重置退避状态，允许重新从 InitialInterval 开始计数。
func (b *ExponentialBackOff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startTime = time.Time{}
	b.currentInterval = 0
}

// ConstantBackOff 固定间隔退避策略。
type ConstantBackOff struct {
	Interval time.Duration
}

// NextBackOff 永远返回 (Interval, false)。
func (c *ConstantBackOff) NextBackOff() (time.Duration, bool) {
	if c.Interval <= 0 {
		return 0, false
	}
	return c.Interval, false
}

// StopBackOff 立即停止的退避策略：调用方下一次 NextBackOff 后立即退出。
type StopBackOff struct {
	called bool
}

// NextBackOff 第一次返回 (0, true)，之后每次返回 (0, false)。
//
// 用法：传给 SafeGoWithRetry 让其只执行一次 fn 就结束（即便失败也不重试）。
func (s *StopBackOff) NextBackOff() (time.Duration, bool) {
	if s.called {
		return 0, false
	}
	s.called = true
	return 0, true
}

// jitter 对 interval 应用 [1-factor, 1+factor] 区间内的随机抖动，避免雷鸣群。
func jitter(interval time.Duration, factor float64) time.Duration {
	if factor <= 0 || interval <= 0 {
		return interval
	}
	if factor > 1 {
		factor = 1
	}
	delta := float64(interval) * factor
	minD := float64(interval) - delta
	maxD := float64(interval) + delta
	return time.Duration(minD + rand.Float64()*(maxD-minD))
}