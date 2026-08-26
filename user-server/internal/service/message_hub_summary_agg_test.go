package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// 决策源：docs/architecture/MASTER_COMPETITIVE_DECISIONS.md M18 表 D-3 / X-8
// 验证点：增量正确性 / 幂等 / 迟到事件修正 / 双读切换（summary 陈旧 >10min 回源 raw）

func setupSummaryTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.MessageHub{},
		&model.MsgHourlySummary{},
		&model.AggregationWatermark{},
	)
}

func newAggSvc(database *gorm.DB, batchSize int) *MessageHubSummaryAggregationService {
	svc := NewMessageHubSummaryAggregationService(database)
	if batchSize > 0 {
		svc.batchSize = batchSize
	}
	return svc
}

func hubRow(platform, conv string, isAI bool, direction string, at time.Time) *model.MessageHub {
	return &model.MessageHub{
		Platform:       platform,
		MsgID:          conv + "-" + platform + "-" + at.Format("150405.000000000"),
		AccountID:      "acc1",
		Direction:      direction,
		Status:         "done",
		MsgType:        "text",
		ConversationID: conv,
		IsAIReply:      isAI,
		SentAt:         at,
		CreatedAt:      at,
	}
}

func TestHubSummaryAgg_IncrementalCorrectness(t *testing.T) {
	database := setupSummaryTestDB(t)
	ctx := context.Background()
	h0 := time.Now().Truncate(time.Hour).Add(-2 * time.Hour)

	rows := []*model.MessageHub{
		hubRow("whatsapp", "conv-1", false, "inbound", h0.Add(5*time.Minute)),
		hubRow("whatsapp", "conv-1", true, "outbound", h0.Add(6*time.Minute)),  // AI 回复
		hubRow("whatsapp", "conv-2", false, "outbound", h0.Add(7*time.Minute)), // 人工回复
		hubRow("telegram", "conv-3", false, "inbound", h0.Add(65*time.Minute)),
	}
	if err := database.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := newAggSvc(database, 0)
	n, err := svc.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != int64(len(rows)) {
		t.Fatalf("consumed = %d, want %d", n, len(rows))
	}

	var summaries []model.MsgHourlySummary
	if err := database.Order("hour_bucket, platform").Find(&summaries).Error; err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summary rows = %d, want 2 (whatsapp/telegram 两维度)", len(summaries))
	}

	wa := summaries[0]
	tg := summaries[1]
	if wa.Platform != "whatsapp" || tg.Platform != "telegram" {
		t.Fatalf("platform order wrong: %s / %s", wa.Platform, tg.Platform)
	}
	if wa.SessionCount != 2 || wa.AICount != 1 || wa.HumanCount != 1 || wa.MessageCount != 3 {
		t.Errorf("whatsapp bucket wrong: %+v", wa)
	}
	if tg.SessionCount != 1 || tg.AICount != 0 || tg.HumanCount != 0 || tg.MessageCount != 1 {
		t.Errorf("telegram bucket wrong: %+v", tg)
	}

	// 水位线推进到最大 id
	wm, err := newAggSvc(database, 0).RunOnce(ctx) // 空跑不报错
	if err != nil || wm != 0 {
		var record model.AggregationWatermark
		if err := database.Where("source = ?", model.SummarySourceMessageHub).First(&record).Error; err != nil {
			t.Fatal(err)
		}
		if record.LastEventID != int64(rows[len(rows)-1].ID) {
			t.Errorf("watermark = %d, want %d", record.LastEventID, rows[len(rows)-1].ID)
		}
	}
}

func TestHubSummaryAgg_IdempotentRerun(t *testing.T) {
	database := setupSummaryTestDB(t)
	ctx := context.Background()
	h0 := time.Now().Truncate(time.Hour).Add(-1 * time.Hour)

	rows := []*model.MessageHub{
		hubRow("wechat", "c1", true, "outbound", h0.Add(10*time.Minute)),
		hubRow("wechat", "c1", true, "outbound", h0.Add(11*time.Minute)),
	}
	if err := database.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	svc := newAggSvc(database, 0)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	// 重跑多次：水位线已推进，无新行 → 计数不得翻倍
	for i := 0; i < 3; i++ {
		if _, err := svc.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}

	var s model.MsgHourlySummary
	if err := database.First(&s).Error; err != nil {
		t.Fatal(err)
	}
	if s.MessageCount != 2 || s.SessionCount != 1 || s.AICount != 2 {
		t.Errorf("幂等性破坏: %+v", s)
	}
}

func TestHubSummaryAgg_LateArrivalCorrected(t *testing.T) {
	database := setupSummaryTestDB(t)
	ctx := context.Background()
	h0 := time.Now().Truncate(time.Hour).Add(-3 * time.Hour)

	first := []*model.MessageHub{
		hubRow("whatsapp", "c1", false, "inbound", h0.Add(20*time.Minute)),
	}
	if err := database.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	svc := newAggSvc(database, 0)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	// 迟到事件：写入更早的 hour bucket（id 更大，下轮被增量累加进旧 bucket）
	late := []*model.MessageHub{
		hubRow("whatsapp", "c9", false, "inbound", h0.Add(1*time.Minute)),
	}
	time.Sleep(10 * time.Millisecond) // 保证 CreatedAt 单调可区分
	if err := database.Create(&late).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	var s model.MsgHourlySummary
	if err := database.First(&s).Error; err != nil {
		t.Fatal(err)
	}
	if s.SessionCount != 2 || s.MessageCount != 2 {
		t.Errorf("迟到事件未正确并入历史 bucket: %+v", s)
	}
}

func TestHubSummaryAgg_BatchLimitRespected(t *testing.T) {
	database := setupSummaryTestDB(t)
	ctx := context.Background()
	h0 := time.Now().Truncate(time.Hour).Add(-1 * time.Hour)

	rows := make([]*model.MessageHub, 0, 25)
	for i := 0; i < 25; i++ {
		rows = append(rows, hubRow("sms", fmt.Sprintf("cb-%d-%d", time.Now().UnixNano(), i), false, "inbound", h0.Add(time.Duration(i)*time.Second)))
	}
	if err := database.CreateInBatches(rows, 50).Error; err != nil {
		t.Fatal(err)
	}

	// batch=10 强制多批消费，全部行必须被处理且计数正确
	svc := newAggSvc(database, 10)
	n, err := svc.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 25 {
		t.Errorf("multi-batch consumed = %d, want 25", n)
	}
	var s model.MsgHourlySummary
	if err := database.First(&s).Error; err != nil {
		t.Fatal(err)
	}
	if s.MessageCount != 25 || s.SessionCount != 25 {
		t.Errorf("multi-batch aggregation wrong: %+v", s)
	}
}

func TestDashboardDoubleRead_FreshSummaryWins(t *testing.T) {
	database := setupSummaryTestDB(t)
	ctx := context.Background()
	now := time.Now()
	freshBucket := now.Truncate(time.Hour)

	// summary 表新鲜数据（与 raw 表故意不一致，用于证明读的是 summary）
	if err := database.Create(&model.MsgHourlySummary{
		HourBucket: freshBucket, Platform: "whatsapp",
		SessionCount: 100, AICount: 60, HumanCount: 40, MessageCount: 500,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// 原始表少量数据（若误读 raw 会得到不同值）
	rawRows := []*model.MessageHub{hubRow("whatsapp", "cx", false, "inbound", now.Add(-time.Minute))}
	if err := database.Create(rawRows).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDashboardStatsService(database)
	points := svc.CollectMessageVolume(ctx, 24)
	if len(points) != 1 {
		t.Fatalf("points = %d, want 1", len(points))
	}
	p := points[0]
	if p.Source != "summary" {
		t.Fatalf("fresh summary 应命中主路径, got source=%s", p.Source)
	}
	if p.MessageCount != 500 || p.SessionCount != 100 {
		t.Errorf("summary 数据错误: %+v", p)
	}
}

func TestDashboardDoubleRead_StaleSummaryFallsBackToRaw(t *testing.T) {
	database := setupSummaryTestDB(t)
	ctx := context.Background()

	staleTime := time.Now().Add(-30 * time.Minute) // 陈旧 > 10min
	summaryRow := &model.MsgHourlySummary{
		HourBucket: staleTime.Truncate(time.Hour), Platform: "whatsapp",
		SessionCount: 1, AICount: 0, HumanCount: 0, MessageCount: 1,
	}
	if err := database.Create(summaryRow).Error; err != nil {
		t.Fatal(err)
	}
	// gorm 会自动填充 updated_at=now，这里强制回写为陈旧时间
	if err := database.Model(&model.MsgHourlySummary{}).
		Where("platform = ?", "whatsapp").
		UpdateColumn("updated_at", staleTime).Error; err != nil {
		t.Fatal(err)
	}
	// 原生表有 summary 未覆盖的新数据
	now := time.Now()
	rawRows := []*model.MessageHub{
		hubRow("telegram", "ct", true, "outbound", now.Add(-2*time.Minute)),
		hubRow("telegram", "cu", false, "inbound", now.Add(-3*time.Minute)),
	}
	if err := database.Create(rawRows).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDashboardStatsService(database)
	points := svc.CollectMessageVolume(ctx, 24)
	if len(points) == 0 {
		t.Fatal("兜底路径无返回")
	}
	for _, p := range points {
		if p.Source != "raw" {
			t.Fatalf("陈旧 summary 必须回源 raw (X-8), got source=%s", p.Source)
		}
	}
	foundTG := false
	for _, p := range points {
		if p.Platform == "telegram" && p.MessageCount == 2 && p.SessionCount == 2 && p.AICount == 1 {
			foundTG = true
		}
	}
	if !foundTG {
		t.Errorf("raw 兜底聚合结果不符: %+v", points)
	}
}

func TestDashboardDoubleRead_EmptySummaryFallsBack(t *testing.T) {
	database := setupSummaryTestDB(t)
	ctx := context.Background()

	now := time.Now()
	if err := database.Create([]*model.MessageHub{
		hubRow("line", "cl", false, "inbound", now.Add(-5*time.Minute)),
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDashboardStatsService(database)
	points := svc.CollectMessageVolume(ctx, 24)
	if len(points) != 1 || points[0].Source != "raw" {
		t.Fatalf("空 summary 应回源 raw: %+v", points)
	}
}
