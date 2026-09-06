package websocket

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/pkg/utils/logger"
)

var (
	seqEpoch = time.Now().UnixNano()

	epochStr = strconv.FormatInt(seqEpoch, 36)

	seqCache cache.Cache

	seqIsRedis func() bool = cache.GlobalIsRedis

	localSeqFallback uint64

	redisDegraded atomic.Bool

	localSeqPeak uint64

	seqWarnOnce sync.Once
)

func seqBackend() cache.Cache {
	if seqCache != nil {
		return seqCache
	}
	if cache.GlobalIsRedis() {
		return cache.GetGlobalCache()
	}
	return nil
}

// CurrentEpoch 当前进程纪元串（握手/resume 校验用）
func CurrentEpoch() string { return epochStr }

// NextSeq 获取下一个消息序号。
// Redis 可用：INCR（多副本全局唯一）；不可用/降级：进程内 atomic。
func NextSeq() uint64 {

	if redisDegraded.Load() {
		if c := seqBackend(); c != nil && seqIsRedis() {
			key := seqKey()
			n, err := c.Incr(context.Background(), key, 0)
			if err == nil && uint64(n) <= localSeqPeak {
				_, _ = c.Incr(context.Background(), key, 0)
				local := atomic.LoadUint64(&localSeqPeak)
				for uint64(n) <= local {
					n, err = c.Incr(context.Background(), key, 0)
					if err != nil {
						break
					}
				}
			}
			if err == nil {
				redisDegraded.Store(false)
				return uint64(n)
			}
		}
		return atomic.AddUint64(&localSeqFallback, 1)
	}

	if c := seqBackend(); c != nil && seqIsRedis() {
		n, err := c.Incr(context.Background(), seqKey(), 0)
		if err == nil && n > 0 {
			local := atomic.LoadUint64(&localSeqPeak)
			if uint64(n) > local {
				atomic.StoreUint64(&localSeqPeak, uint64(n))
			}
			return uint64(n)
		}
		seqWarnOnce.Do(func() {
			logger.GetLogger().Warn().Err(err).Msg("[WS] Redis Incr failed, seq degraded to in-process atomic (multi-replica unsafe until restart)")
		})
		redisDegraded.Store(true)
	}
	return atomic.AddUint64(&localSeqFallback, 1)
}

// PeekSeq 诊断用：当前序号水位（Redis 模式返回远端值，降级返回本地值）
func PeekSeq() uint64 {
	if !redisDegraded.Load() {
		if c := seqBackend(); c != nil && seqIsRedis() {
			var v uint64
			if err := c.GetJSON(context.Background(), seqKey(), &v); err == nil {
				return v
			}
		}
	}
	return atomic.LoadUint64(&localSeqFallback)
}

func seqKey() string { return fmt.Sprintf("mtk:ws:seq:%s", epochStr) }
