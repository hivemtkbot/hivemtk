package service

// reach_send_pipeline_stats.go 带统计的 Pipeline 装饰器：包装任意 SendPipeline，
// 汇总发送/成功/失败/限流/退订跳过/静默延迟/回退/重试次数（用于运维监控）。

import (
	"context"
	"sync"
)

// SendPipelineStats Pipeline 统计（用于运维监控）
type SendPipelineStats struct {
	TotalSends   int64
	SuccessSends int64
	FailedSends  int64
	RateLimited  int64
	// DoNotContactSkipped R-3a：因命中全局退订标志位被跳过的次数
	DoNotContactSkipped int64
	// QuietHoursDeferred R-4：命中全渠道 quiet hours 进入延迟队列的次数
	QuietHoursDeferred int64
	FallbackUsed       int64
	TotalRetries       int64
	TotalCost          float64
}

type countedSendPipeline struct {
	inner SendPipeline
	stats SendPipelineStats
	mu    sync.RWMutex
}

// NewCountedSendPipeline 创建带统计的 Pipeline
func NewCountedSendPipeline(inner SendPipeline) SendPipeline {
	return &countedSendPipeline{inner: inner}
}

func (p *countedSendPipeline) Send(ctx context.Context, req *ReachSendRequest) *SendResponse {
	resp := p.inner.Send(ctx, req)
	p.mu.Lock()
	p.stats.TotalSends++
	if resp.Success {
		p.stats.SuccessSends++
	} else {
		p.stats.FailedSends++
	}
	if resp.FallbackUsed {
		p.stats.FallbackUsed++
	}
	if resp.Deferred {
		p.stats.QuietHoursDeferred++
	}
	p.stats.TotalRetries += int64(resp.RetryCount)
	for _, step := range resp.StepResults {
		if !step.Success {
			switch step.Step {
			case SendStepRateLimit:
				p.stats.RateLimited++
			case SendStepPermission:

				if step.Error == ErrSendDoNotContact.Error() {
					p.stats.DoNotContactSkipped++
				}
			}
		}
	}
	p.mu.Unlock()
	return resp
}

func (p *countedSendPipeline) Stats(ctx context.Context) SendPipelineStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}
