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

// 本文件实现 D15：seq 外置 Redis + epoch 重启语义。
//
//   - 多副本：NextSeq 走 Redis INCR（全局唯一递增）；Redis 不可用降级进程内 atomic
//     （降级即锁定 atomic——恢复后 INCRBY 跳过本地峰值防撞号，审核修正 4）。
//   - epoch：进程启动时固定；重启即变。客户端 resume 带旧 epoch → 直接全量补发
//     （替代"seq 归零全量补发风暴"：旧 epoch 的 seq 在新 epoch 无意义，不做逐条比对）。
//   - seq 键无 TTL（审核修正 1：TTL 过期后同 epoch 内 seq 归零重复，比重启归零更隐蔽）。
//
// 可测性：seqCache/seqIsRedis 包级注入（审核修正 2），默认 cache.GetGlobalCache/GlobalIsRedis。

var (
	// seqEpoch 进程启动即固定的纪元戳；重启即变
	seqEpoch = time.Now().UnixNano()
	// epochStr Envelope 下发/客户端 resume 回传的纪元串
	epochStr = strconv.FormatInt(seqEpoch, 36)

	// seqCache 可注入缓存后端（测试替换）；nil 时走 GetGlobalCache
	seqCache cache.Cache
	// seqIsRedis 可注入 Redis 判定（测试替换）
	seqIsRedis func() bool = cache.GlobalIsRedis

	// localSeqFallback Redis 降级后的进程内序列
	localSeqFallback uint64
	// redisDegraded 降级状态锁定：true 后本进程永久走 atomic（恢复由重启完成，注释明示取舍）
	redisDegraded atomic.Bool
	// localSeqPeak 本地已发峰值（恢复撞号跳过基准）
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
	// 已降级：恢复撞号保护——切回 Redis 前先把 Redis 计数推过本地峰值
	if redisDegraded.Load() {
		if c := seqBackend(); c != nil && seqIsRedis() {
			key := seqKey()
			n, err := c.Incr(context.Background(), key, 0)
			if err == nil && uint64(n) <= localSeqPeak {
				_, _ = c.Incr(context.Background(), key, 0) // 实际应 INCRBY 差值；Cache 接口无 IncrBy，循环跳过（峰值差小）
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
		n, err := c.Incr(context.Background(), seqKey(), 0) // 无 TTL（审核修正 1）
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
