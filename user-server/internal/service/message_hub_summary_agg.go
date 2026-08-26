package service

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// MessageHubSummaryAggregationService message_hub 小时级增量汇总服务（M18 表 D-3）。
//
// 决策源：docs/architecture/MASTER_COMPETITIVE_DECISIONS.md M18 表 D-3 / X-8。
//
// 流水线：
//
//	message_hub(raw, 按 id 序)
//	  → watermark(aggregation_watermarks.last_event_id)
//	  → 内存按 (hour_bucket, merchant_id, platform) 聚合
//	  → 批量 INSERT ... ON CONFLICT DO UPDATE（计数累加）
//	  → 水位线与数据同事务推进
//
// 正确性：
//   - 增量正确性：仅消费 id > watermark 的行；
//   - 幂等：水位线在 upsert 同事务推进，重跑不重复累加；
//   - 迟到事件：迟到消息的 id 必然大于水位线，下轮被增量累加进历史 bucket，天然修正。
type MessageHubSummaryAggregationService struct {
	repo repository.MessageHubSummaryRepository
	db   *gorm.DB

	// batchSize 单批扫描源表行数上限（D-3 落地要点：LIMIT 50K）
	batchSize int
}

func NewMessageHubSummaryAggregationService(database *gorm.DB) *MessageHubSummaryAggregationService {
	if database == nil {
		database = db.GetDB()
	}
	return &MessageHubSummaryAggregationService{
		repo:      repository.NewMessageHubSummaryRepository(database),
		db:        database,
		batchSize: 50000,
	}
}

// RunOnce 消费自上次水位线以来的全部新消息并累加进 summary 表。
// 返回本轮消费的消息行数。DB 未就绪返回 (0, nil) 静默跳过。
func (s *MessageHubSummaryAggregationService) RunOnce(ctx context.Context) (int64, error) {
	if s.db == nil || s.repo == nil {
		return 0, nil
	}
	wm, err := s.repo.LoadWatermark(ctx, model.SummarySourceMessageHub)
	if err != nil {
		return 0, err
	}
	var consumed int64
	for {
		rows := make([]model.MessageHub, 0, s.batchSize)
		if err := s.db.WithContext(ctx).
			Model(&model.MessageHub{}).
			Where("id > ?", wm).
			Order("id ASC").
			Limit(s.batchSize).
			Find(&rows).Error; err != nil {
			return consumed, err
		}
		if len(rows) == 0 {
			return consumed, nil
		}
		deltas, maxID := aggregateHubRows(rows)
		if err := s.repo.UpsertIncrementBatch(ctx, model.SummarySourceMessageHub, maxID, deltas); err != nil {
			return consumed, err
		}
		wm = maxID
		consumed += int64(len(rows))
		if len(rows) < s.batchSize {
			return consumed, nil
		}
	}
}

// summaryKey summary 维度键
type summaryKey struct {
	bucket     time.Time
	merchantID uint
	platform   string
}

// aggregateHubRows 将原始消息行内存聚合为维度增量；同时返回最大 id（新水位线）。
func aggregateHubRows(rows []model.MessageHub) ([]repository.MsgHourlyDelta, int64) {
	type acc struct {
		delta repository.MsgHourlyDelta
		sess  map[string]struct{}
	}
	accs := make(map[summaryKey]*acc)
	var maxID int64
	for _, h := range rows {
		if int64(h.ID) > maxID {
			maxID = int64(h.ID)
		}
		k := summaryKey{
			bucket:     h.CreatedAt.Truncate(time.Hour),
			merchantID: 0, // 私域单商户，预留维度恒 0
			platform:   h.Platform,
		}
		a := accs[k]
		if a == nil {
			a = &acc{delta: repository.MsgHourlyDelta{
				HourBucket: k.bucket,
				MerchantID: k.merchantID,
				Platform:   k.platform,
			}, sess: make(map[string]struct{})}
			accs[k] = a
		}
		a.delta.MessageCount++
		if h.IsAIReply {
			a.delta.AICount++
		} else if h.Direction == "outbound" {
			a.delta.HumanCount++
		}
		if h.ConversationID != "" {
			a.sess[h.ConversationID] = struct{}{}
		}
	}
	deltas := make([]repository.MsgHourlyDelta, 0, len(accs))
	for _, a := range accs {
		a.delta.SessionCount = int64(len(a.sess))
		deltas = append(deltas, a.delta)
	}
	return deltas, maxID
}

// ===== 定时驱动（沿用 SessionTTLCron 惯例） =====

type hubSummaryAggCron struct {
	svc      *MessageHubSummaryAggregationService
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

var hubSummaryAggCronInst *hubSummaryAggCron

func init() {
	hubSummaryAggCronInst = startHubSummaryAggCron(NewMessageHubSummaryAggregationService(nil))
}

func startHubSummaryAggCron(svc *MessageHubSummaryAggregationService) *hubSummaryAggCron {
	if svc == nil || svc.db == nil {
		return nil
	}
	c := &hubSummaryAggCron{svc: svc, stopCh: make(chan struct{})}
	c.wg.Add(1)
	go c.run(context.Background())
	return c
}

func (c *hubSummaryAggCron) run(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.trigger(ctx)
		}
	}
}

func (c *hubSummaryAggCron) trigger(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[hub_summary_agg_cron] panic: %v", r)
		}
	}()
	n, err := c.svc.RunOnce(ctx)
	if err != nil {
		logger.Errorf("[hub_summary_agg_cron] run failed: %v", err)
		return
	}
	if n > 0 {
		logger.Infof("[hub_summary_agg_cron] aggregated %d message_hub rows", n)
	}
}

// StopMessageHubSummaryAggCron 进程退出时可调用（可选接线）。
func StopMessageHubSummaryAggCron(ctx context.Context) {
	if hubSummaryAggCronInst == nil {
		return
	}
	hubSummaryAggCronInst.stopOnce.Do(func() { close(hubSummaryAggCronInst.stopCh) })
	done := make(chan struct{})
	go func() { hubSummaryAggCronInst.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
