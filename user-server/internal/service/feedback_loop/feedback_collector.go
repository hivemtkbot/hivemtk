package feedbackloop

// feedback_collector.go 反馈信号采集器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十七章 §17.4.1
//
// 职责：统一接收三类反馈信号 → 计算奖励 → 写入 feedback_events → 异步聚合到 feedback_signals
//
// 架构：
//   主链路 → Collect() (异步队列, <1ms 返回)
//                  ↓
//              [worker goroutine]
//                  ↓
//              批量刷盘（每 BatchSize 条或 FlushInterval 间隔）
//                  ↓
//              persist() (事务：写 feedback_events + upsert feedback_signals)
//
// 关键设计：
//   1. 异步队列防止阻塞主链路（SalesEngine / API handler）
//   2. 批量刷盘降低 DB 压力
//   3. upsert feedback_signals 保证 session 级聚合原子性
//   4. CollectSync 提供同步持久化路径（测试 / 关键场景）
//   5. 优雅关闭：Stop() 触发 stopCh，worker 刷盘剩余 batch 后退出

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// FeedbackCollector 反馈信号采集器
type FeedbackCollector struct {
	repo   *repository.FeedbackLoopRepository
	config FeedbackCollectorConfig
	queue  chan *dto.CollectRequest // 异步队列
	stopCh chan struct{}            // 通知 worker 优雅关闭
	done   chan struct{}            // worker 退出后关闭
}

// NewFeedbackCollector 创建采集器（启动后台 worker）
//
// 参数：
//
//	db     - GORM DB（写入 feedback_events / feedback_signals，由 repository 层使用）
//	cfg    - 配置（零值字段会用默认值填充）
func NewFeedbackCollector(db *gorm.DB, cfg FeedbackCollectorConfig) *FeedbackCollector {
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 1000
	}
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 50
	}
	if cfg.Weights == nil {
		cfg.Weights = DefaultSignalWeights
	}
	c := &FeedbackCollector{
		repo:   repository.NewFeedbackLoopRepositoryWithDB(db),
		config: cfg,
		queue:  make(chan *dto.CollectRequest, cfg.QueueSize),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	go c.worker()
	return c
}

// Collect 异步采集（主链路调用，立即返回）
//
// 行为：
//   - 请求入队（队列满则返回 ErrQueueFull，事件丢弃但主链路不阻塞）
//   - worker 后台批量持久化 + 聚合
func (c *FeedbackCollector) Collect(ctx context.Context, req *dto.CollectRequest) error {
	if req == nil {
		return nil
	}
	if err := ValidateCollectRequest(req); err != nil {
		return err
	}
	select {
	case c.queue <- req:
		return nil
	default:
		return fmt.Errorf("%w: session=%s signal=%s", ErrQueueFull, req.SessionID, req.SignalKey)
	}
}

// CollectSync 同步采集（测试 / 关键场景使用）
//
// 立即持久化 + 聚合，不经过队列
func (c *FeedbackCollector) CollectSync(ctx context.Context, req *dto.CollectRequest) error {
	if req == nil {
		return nil
	}
	if err := ValidateCollectRequest(req); err != nil {
		return err
	}
	return c.persist(ctx, req)
}

// Stop 优雅关闭（等待 worker 刷盘剩余 batch 后退出）
func (c *FeedbackCollector) Stop() {
	close(c.stopCh)
	<-c.done
}

// ----------------------------------------------------------------------------
// worker 后台批量刷盘
// ----------------------------------------------------------------------------

// worker 后台 worker（批量写入 + 信号聚合）
func (c *FeedbackCollector) worker() {
	defer close(c.done)

	batch := make([]*dto.CollectRequest, 0, c.config.BatchSize)
	ticker := time.NewTicker(c.config.FlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		c.flushBatch(ctx, batch)
		cancel()
		batch = batch[:0]
	}
	// 修复：flush（flushBatch→persist）panic 不得杀死 worker（否则 done 永不关闭、
	// Stop() 关机会死锁且反馈采集静默停止）。recover 后仅记日志，循环继续。
	safeFlush := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[feedback_collector] flush panic recovered: %v", r)
			}
		}()
		flush()
	}

	for {
		select {
		case req := <-c.queue:
			batch = append(batch, req)
			if len(batch) >= c.config.BatchSize {
				safeFlush()
			}
		case <-ticker.C:
			safeFlush()
		case <-c.stopCh:
			// 优雅关闭：先排空队列（非阻塞），再刷盘剩余 batch
			for {
				select {
				case req := <-c.queue:
					batch = append(batch, req)
				default:
					safeFlush()
					return
				}
			}
		}
	}
}

// flushBatch 批量持久化 + 聚合
func (c *FeedbackCollector) flushBatch(ctx context.Context, batch []*dto.CollectRequest) {
	for _, req := range batch {
		if err := c.persist(ctx, req); err != nil {
			// 单条失败不阻断 batch 其他记录
			continue
		}
	}
}

// ----------------------------------------------------------------------------
// 持久化 + 聚合
// ----------------------------------------------------------------------------

// persist 持久化单条事件 + upsert feedback_signals
//
// 事务（由 repository 封装）：
//  1. 写 feedback_events（事件流水）
//  2. upsert feedback_signals（按 session_id 聚合）
func (c *FeedbackCollector) persist(ctx context.Context, req *dto.CollectRequest) error {
	if c.repo == nil {
		return fmt.Errorf("repo is nil")
	}
	weight := c.lookupWeight(req.SignalKey)
	reward := c.computeReward(req.SignalKey, req.SignalValue, weight)

	eventID := c.genEventID(req)
	signalValueJSON, err := json.Marshal(req.SignalValue)
	if err != nil {
		signalValueJSON = []byte("null")
	}

	event := &model.FeedbackEvent{
		EventID:           eventID,
		SessionID:         req.SessionID,
		CustomerID:        req.CustomerID,
		SOPID:             req.SOPID,
		ExecutionID:       req.ExecutionID,
		Variant:           req.Variant,
		PromptCandidateID: req.PromptCandidateID,
		EventType:         string(req.EventType),
		SignalKey:         string(req.SignalKey),
		SignalValue:       model.JSONMap{"v": json.RawMessage(signalValueJSON)},
		Weight:            weight,
		Reward:            reward,
		AIReply:           req.AIReply,
		CustomerMsg:       req.CustomerMsg,
		Metadata:          model.JSONMap(req.Metadata),
		CreatedBy:         req.CreatedBy,
	}

	breakdownJSON := fmt.Sprintf(`{"%s":1}`, string(req.SignalKey))
	sig := repository.FeedbackSignalUpsert{
		SessionID:         req.SessionID,
		CustomerID:        req.CustomerID,
		SOPID:             req.SOPID,
		Variant:           req.Variant,
		PromptCandidateID: req.PromptCandidateID,
		Reward:            reward,
		BreakdownJSON:     breakdownJSON,
	}
	return c.repo.PersistFeedback(ctx, event, sig)
}

// ----------------------------------------------------------------------------
// 奖励计算
// ----------------------------------------------------------------------------

// lookupWeight 查找信号权重（未配置则返回 0）
func (c *FeedbackCollector) lookupWeight(key dto.FeedbackSignalKey) float64 {
	if w, ok := c.config.Weights[key]; ok {
		return w
	}
	return 0
}

// computeReward 计算奖励值
//
// 信号类型归一化规则：
//   - rating（评分 1-5）：reward = weight * (v / 5.0)
//   - reply_rate（回复率 0-1）：reward = weight * v
//   - duration（会话时长秒）：reward = weight * min(v/300, 1)
//   - 其他（bool/string）：reward = weight * 1.0
func (c *FeedbackCollector) computeReward(key dto.FeedbackSignalKey, value any, weight float64) float64 {
	switch key {
	case dto.FBSignalRating:
		if v, ok := toFloat64(value); ok {
			return weight * (v / 5.0)
		}
	case dto.FBSignalReplyRate:
		if v, ok := toFloat64(value); ok {
			return weight * v
		}
	case dto.FBSignalDuration:
		if v, ok := toFloat64(value); ok {
			normalized := v / 300.0
			if normalized > 1 {
				normalized = 1
			}
			return weight * normalized
		}
	}
	// 布尔型 / 字符型：weight * 1.0
	return weight
}

// toFloat64 安全转换为 float64（支持 int/int64/float32/float64）
func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case bool:
		if x {
			return 1.0, true
		}
		return 0.0, true
	}
	return 0, false
}

// genEventID 生成事件唯一 ID（防重复）
//
// 组成：session_id|signal_key|customer_msg|unix_nano|random_nonce 的 sha256 前 32 字符
//
// 注意：仅依赖 time.Now().UnixNano() 无法保证唯一性——
// 现代系统时钟分辨率可能仅到微秒，紧凑循环中多次调用会得到相同纳秒值。
// 因此追加 8 字节 crypto/rand 随机 nonce，确保跨调用唯一。
func (c *FeedbackCollector) genEventID(req *dto.CollectRequest) string {
	now := time.Now().UnixNano()
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%d|", req.SessionID, req.SignalKey, req.CustomerMsg, now)
	// 追加 8 字节随机 nonce（rand.Read 失败时退化为时间戳字节，仍保证大部分场景唯一）
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		// 极罕见情况（OS 熵池异常）；用 now 的字节模式填充
		for i := 0; i < 8; i++ {
			nonce[i] = byte(now >> (i * 8))
		}
	}
	h.Write(nonce)
	return hex.EncodeToString(h.Sum(nil))[:32]
}
