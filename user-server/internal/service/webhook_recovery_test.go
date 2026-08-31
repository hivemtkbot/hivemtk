package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

// newRecoveryFixture 构造带测试库的扫描器；无 PG 时跳过。
func newRecoveryFixture(t *testing.T) (*webhookRecoveryScanner, *repository.WebhookEventRepository) {
	t.Helper()
	db := testutil.NewTestDB(t, &model.WebhookEvent{})
	if db == nil {
		t.Skip("test db unavailable")
	}
	repo := repository.NewWebhookEventRepository()
	repository.SetWebhookEventRepoDB(repo, db)
	svc := &WebhookService{db: db}
	sc := newWebhookRecoveryScanner(svc)
	if sc == nil {
		t.Fatal("scanner should not be nil with db")
	}
	return sc, repo
}

// backdateEvent 把事件的 created_at 回拨到冷却期之外。
func backdateEvent(t *testing.T, repo *repository.WebhookEventRepository, evt *model.WebhookEvent, d time.Duration) {
	t.Helper()
	if err := repo.GetDB().Model(&model.WebhookEvent{}).Where("id = ?", evt.ID).
		Update("created_at", time.Now().Add(-d)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

// T1 验收①：冷却期外 processed=false 事件被列出；冷却期内与已处理事件不列出。
func TestRecoveryListStaleUnprocessed(t *testing.T) {
	sc, repo := newRecoveryFixture(t)
	ctx := context.Background()

	stale := &model.WebhookEvent{Platform: "whatsapp", EventID: "evt_stale_1", AccountID: "1", RawData: "{}", Processed: false}
	fresh := &model.WebhookEvent{Platform: "whatsapp", EventID: "evt_fresh_1", AccountID: "1", RawData: "{}", Processed: false}
	done := &model.WebhookEvent{Platform: "whatsapp", EventID: "evt_done_1", AccountID: "1", RawData: "{}", Processed: true}
	for _, e := range []*model.WebhookEvent{stale, fresh, done} {
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("create %s: %v", e.EventID, err)
		}
	}
	backdateEvent(t, repo, stale, sc.cooldown+time.Minute)
	backdateEvent(t, repo, done, sc.cooldown+time.Minute)

	events, err := repo.ListStaleUnprocessed(ctx, time.Now().Add(-sc.cooldown), 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	ids := map[string]bool{}
	for _, e := range events {
		ids[e.EventID] = true
	}
	if !ids["evt_stale_1"] {
		t.Fatalf("stale unprocessed event should be listed")
	}
	if ids["evt_fresh_1"] {
		t.Fatalf("fresh event within cooldown should not be listed")
	}
	if ids["evt_done_1"] {
		t.Fatalf("processed event should not be listed")
	}
}

// T1 验收②：存量无 account_id 的行不可重放且被终止（防无限重放）。
func TestReplayLegacyRowWithoutAccountTerminates(t *testing.T) {
	sc, repo := newRecoveryFixture(t)
	ctx := context.Background()
	evt := &model.WebhookEvent{Platform: "whatsapp", EventID: "evt_legacy_1", AccountID: "", RawData: "{}", Processed: false}
	if err := repo.Create(ctx, evt); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := sc.replay(ctx, evt); got {
		t.Fatalf("legacy row should not be replayable")
	}
	fresh, err := repo.GetByID(ctx, evt.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !fresh.Processed {
		t.Fatalf("legacy row should be marked processed to stop infinite replay")
	}
}

// T1 验收③：截断载荷（TruncateForStore 标记）不可重放且被终止。
func TestReplayTruncatedPayloadTerminates(t *testing.T) {
	sc, repo := newRecoveryFixture(t)
	ctx := context.Background()
	evt := &model.WebhookEvent{Platform: "whatsapp", EventID: "evt_trunc_1", AccountID: "1", RawData: `{"a":1` + webhookRawTruncatedSuffix, Processed: false}
	if err := repo.Create(ctx, evt); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := sc.replay(ctx, evt); got {
		t.Fatalf("truncated payload should not be replayable")
	}
	fresh, _ := repo.GetByID(ctx, evt.ID)
	if !fresh.Processed {
		t.Fatalf("truncated row should be marked processed")
	}
}

// T1 验收④：毒丸事件达到重试上限后被跳过并标记 processed。
// （GetGlobalCache 未初始化时回落内存缓存，进程内计数即可验证。）
func TestReplayPoisonEventGivesUp(t *testing.T) {
	sc, repo := newRecoveryFixture(t)
	ctx := context.Background()
	evt := &model.WebhookEvent{Platform: "custom", EventID: "evt_poison_" + time.Now().Format("150405.000000000"), AccountID: "1", RawData: "not-json", Processed: false}
	if err := repo.Create(ctx, evt); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < webhookRecoveryMaxRetry; i++ {
		sc.incrRetry(ctx, evt.EventID)
	}
	if got := sc.replay(ctx, evt); got {
		t.Fatalf("poison event should give up")
	}
	fresh, _ := repo.GetByID(ctx, evt.ID)
	if !fresh.Processed {
		t.Fatalf("poison event should be marked processed")
	}
}

// T1 验收⑤：disabled 时 startRecoveryScanner 静默跳过（不 panic、不加 wg）。
// 注意：零值服务的 stopCh 为 nil（Stop 仅供完整构造的服务调用），此处不调 Stop。
func TestRecoveryScannerDisabled(t *testing.T) {
	db := testutil.NewTestDB(t, &model.WebhookEvent{})
	if db == nil {
		t.Skip("test db unavailable")
	}
	t.Setenv("WEBHOOK_RECOVERY_ENABLED", "false")
	svc := &WebhookService{db: db}
	svc.startRecoveryScanner() // disabled 分支应无副作用
}

// T1 验收⑥：重放成功路径 —— handleFn 执行后事件被 markProcessed；
// handleFn 返回即视为本链路已收敛（成功或由 handleJob 内部标记）。
func TestReplayMarksProcessedOnUnknownPlatform(t *testing.T) {
	sc, repo := newRecoveryFixture(t)
	ctx := context.Background()
	called := 0
	sc.handleFn = func(ctx context.Context, job *webhookJob) {
		called++
		job.event.Processed = true // 模拟 handleJob 正常走完并标记
		_ = repo.Update(ctx, job.event)
	}
	raw := `{"event_id":"evt_replay_ok","event_type":"message","content":"hi","from":"u1"}`
	evt := &model.WebhookEvent{Platform: "custom", EventID: "evt_replay_ok", AccountID: "1", RawData: raw, Processed: false}
	if err := repo.Create(ctx, evt); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := sc.replay(ctx, evt); !got {
		t.Fatalf("replay should succeed")
	}
	if called != 1 {
		t.Fatalf("handleFn should be called once, got %d", called)
	}
	fresh, _ := repo.GetByID(ctx, evt.ID)
	if !fresh.Processed {
		t.Fatalf("event should be marked processed after replay")
	}
}

// T1 验收⑦：scanOnce 端到端 —— 冷却期外的积压事件被批量重放。
func TestScanOnceReplaysStaleBatch(t *testing.T) {
	sc, repo := newRecoveryFixture(t)
	ctx := context.Background()
	sc.handleFn = func(ctx context.Context, job *webhookJob) {
		job.event.Processed = true
		_ = repo.Update(ctx, job.event)
	}
	for i := 0; i < 3; i++ {
		evt := &model.WebhookEvent{Platform: "custom", EventID: "evt_batch_" + time.Now().Format("150405.000000000"), AccountID: "1", RawData: "{}", Processed: false}
		if err := repo.Create(ctx, evt); err != nil {
			t.Fatalf("create: %v", err)
		}
		backdateEvent(t, repo, evt, sc.cooldown+time.Minute)
		time.Sleep(2 * time.Millisecond) // 保证 EventID 时间戳唯一
	}
	n := sc.scanOnce(ctx)
	if n != 3 {
		t.Fatalf("expected 3 replayed, got %d", n)
	}
}
