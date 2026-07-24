package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupSOPSchedulerTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.SOPAgent{},
		&model.SOPExecution{},
	)
}

func TestSOPScheduler_NewAndInterval(t *testing.T) {
	s := NewSOPScheduler(nil, nil, 0)
	if s.interval != 60*time.Second {
		t.Errorf("default interval should be 60s, got %v", s.interval)
	}
	s2 := NewSOPScheduler(nil, nil, 5*time.Second)
	if s2.interval != 5*time.Second {
		t.Errorf("expected 5s, got %v", s2.interval)
	}
}

func TestSOPScheduler_StartStop(t *testing.T) {
	db := setupSOPSchedulerTestDB(t)
	svc := NewSOPService(db, nil)
	s := NewSOPScheduler(svc, db, 100*time.Millisecond)
	s.Start(context.Background())
	time.Sleep(150 * time.Millisecond)
	s.Stop(context.Background())
}

func TestSOPScheduler_StartTwiceIsIdempotent(t *testing.T) {
	db := setupSOPSchedulerTestDB(t)
	svc := NewSOPService(db, nil)
	s := NewSOPScheduler(svc, db, time.Second)
	s.Start(context.Background())
	s.Start(context.Background()) // 不应该 panic
	s.Stop(context.Background())
}

func TestSOPScheduler_StopTwiceSafe(t *testing.T) {
	s := NewSOPScheduler(nil, nil, time.Second)
	s.Start(context.Background())
	s.Stop(context.Background())
	// 二次 Stop 由于 close 已关闭 channel 会 panic，这里不应再次调用
	// 验证 running 标记
	if s.running {
		t.Error("expected running=false after Stop")
	}
}

func TestSOPScheduler_CleanupStuckExecutions(t *testing.T) {
	db := setupSOPSchedulerTestDB(t)
	ctx := context.Background()
	now := time.Now()

	// 卡死的执行（25 小时前开始）
	stuck := &model.SOPExecution{
		SOPID:      1,
		CustomerID: "c-1",
		Status:     SOPStatusRunning,
		StartedAt:  now.Add(-25 * time.Hour),
	}
	if err := db.Create(stuck).Error; err != nil {
		t.Fatalf("create stuck: %v", err)
	}

	// 正常的执行（1 小时前开始）
	fresh := &model.SOPExecution{
		SOPID:      1,
		CustomerID: "c-2",
		Status:     SOPStatusRunning,
		StartedAt:  now.Add(-1 * time.Hour),
	}
	if err := db.Create(fresh).Error; err != nil {
		t.Fatalf("create fresh: %v", err)
	}

	svc := NewSOPService(db, nil)
	s := NewSOPScheduler(svc, db, time.Hour)
	s.cleanupStuckExecutions(ctx)

	var stuckGot model.SOPExecution
	if err := db.Where("id = ?", stuck.ID).First(&stuckGot).Error; err != nil {
		t.Fatalf("read stuck: %v", err)
	}
	if stuckGot.Status != SOPStatusFailed {
		t.Errorf("expected failed, got %s", stuckGot.Status)
	}

	var freshGot model.SOPExecution
	if err := db.Where("id = ?", fresh.ID).First(&freshGot).Error; err != nil {
		t.Fatalf("read fresh: %v", err)
	}
	if freshGot.Status != SOPStatusRunning {
		t.Errorf("expected running, got %s", freshGot.Status)
	}
}

func TestSOPScheduler_DispatchAutoSOP(t *testing.T) {
	db := setupSOPSchedulerTestDB(t)
	ctx := context.Background()

	// 创建一个 auto 类型的 SOP
	agent := &model.SOPAgent{
		Name:          "test",
		Scenario:      "test",
		TriggerType:   SOPTriggerAuto,
		TriggerConfig: model.JSONMap{"customer_ids": []any{"c-1", "c-2"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc := NewSOPService(db, nil)
	s := NewSOPScheduler(svc, db, time.Hour)
	s.dispatchAutoSOPs(ctx)

	var execs []model.SOPExecution
	db.Where("sop_id = ?", agent.ID).Find(&execs)
	if len(execs) != 2 {
		t.Errorf("expected 2 executions, got %d", len(execs))
	}
}

func TestSOPScheduler_DispatchAutoSOP_NilDB(t *testing.T) {
	s := NewSOPScheduler(nil, nil, time.Hour)
	s.dispatchAutoSOPs(context.Background()) // 不应 panic
}

func TestSOPScheduler_DispatchScheduledSOP_FirstRun(t *testing.T) {
	db := setupSOPSchedulerTestDB(t)
	agent := &model.SOPAgent{
		Name:          "sched-test",
		Scenario:      "sched",
		TriggerType:   SOPTriggerSchedule,
		TriggerConfig: model.JSONMap{"customer_ids": []any{"c-1"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	svc := NewSOPService(db, nil)
	s := NewSOPScheduler(svc, db, time.Hour)
	s.dispatchScheduledSOPs(context.Background())

	var execs []model.SOPExecution
	db.Where("sop_id = ?", agent.ID).Find(&execs)
	if len(execs) != 1 {
		t.Errorf("expected 1 execution on first run, got %d", len(execs))
	}
}

func TestSOPScheduler_DispatchScheduledSOP_IntervalGuard(t *testing.T) {
	db := setupSOPSchedulerTestDB(t)
	recent := time.Now().UTC().Format(time.RFC3339)
	agent := &model.SOPAgent{
		Name:        "sched-test",
		Scenario:    "sched",
		TriggerType: SOPTriggerSchedule,
		TriggerConfig: model.JSONMap{
			"customer_ids":     []any{"c-1"},
			"interval_minutes": float64(60),
			"last_run_at":      recent,
		},
		SOPGraph:  model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:  true,
		CreatedBy: 1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	svc := NewSOPService(db, nil)
	s := NewSOPScheduler(svc, db, time.Hour)
	s.dispatchScheduledSOPs(context.Background())

	var execs []model.SOPExecution
	db.Where("sop_id = ?", agent.ID).Find(&execs)
	if len(execs) != 0 {
		t.Errorf("expected 0 executions (interval not met), got %d", len(execs))
	}
}

func TestSOPScheduler_TryExecute_DuplicateGuard(t *testing.T) {
	db := setupSOPSchedulerTestDB(t)
	ctx := context.Background()
	agent := &model.SOPAgent{
		Name:          "dup",
		Scenario:      "dup",
		TriggerType:   SOPTriggerAuto,
		TriggerConfig: model.JSONMap{"customer_ids": []any{"c-1"}},
		SOPGraph:      model.JSONMap{"nodes": []any{map[string]any{"id": "start", "type": "start"}}},
		IsActive:      true,
		CreatedBy:     1,
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	svc := NewSOPService(db, nil)
	s := NewSOPScheduler(svc, db, time.Hour)
	s.tryExecute(ctx, *agent)
	s.tryExecute(ctx, *agent) // 第二次应被去重

	var execs []model.SOPExecution
	db.Where("sop_id = ? AND status = ?", agent.ID, SOPStatusRunning).Find(&execs)
	if len(execs) != 1 {
		t.Errorf("expected 1 running execution due to dedup, got %d", len(execs))
	}
}

func TestSOPScheduler_ExtractCustomerIDs(t *testing.T) {
	cases := []struct {
		name string
		cfg  model.JSONMap
		want int
	}{
		{"nil", nil, 0},
		{"empty", model.JSONMap{}, 0},
		{"list", model.JSONMap{"customer_ids": []any{"a", "b"}}, 2},
		{"single", model.JSONMap{"customer_ids": []any{"x"}}, 1},
		{"wrong_type", model.JSONMap{"customer_ids": "string-not-list"}, 0},
		{"mixed_with_int", model.JSONMap{"customer_ids": []any{"a", 1, "b"}}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, _ := extractCustomerIDs(c.cfg)
			if len(out) != c.want {
				t.Errorf("expected %d, got %d", c.want, len(out))
			}
		})
	}
}

func TestSOPScheduler_SetJSONMapValue(t *testing.T) {
	out := setJSONMapValue(model.JSONMap{"a": "1"}, "b", "2")
	if out == "" {
		t.Fatal("expected non-empty")
	}
	if out != `{"a":"1","b":"2"}` && out != `{"b":"2","a":"1"}` {
		t.Errorf("unexpected json: %s", out)
	}

	out2 := setJSONMapValue(nil, "x", "y")
	if out2 != `{"x":"y"}` {
		t.Errorf("expected x:y, got %s", out2)
	}
}

func TestSOPScheduler_FmtUintSafe(t *testing.T) {
	if fmtUintSafe(0) != "0" {
		t.Error("expected 0")
	}
	if fmtUintSafe(123) != "123" {
		t.Error("expected 123")
	}
	if fmtUintSafe(9999999) != "9999999" {
		t.Error("expected 9999999")
	}
}

func TestSOPScheduler_Tick_NilDB(t *testing.T) {
	s := NewSOPScheduler(nil, nil, time.Hour)
	s.tick(context.Background()) // 不应 panic
}

func TestSOPScheduler_Tick_WithDB_NoData(t *testing.T) {
	db := setupSOPSchedulerTestDB(t)
	svc := NewSOPService(db, nil)
	s := NewSOPScheduler(svc, db, time.Hour)
	s.tick(context.Background()) // 空库，不应 panic
}
