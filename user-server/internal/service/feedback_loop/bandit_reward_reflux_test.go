package feedbackloop

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

type mockBanditUpdater struct {
	calls []mockRewardCall
}

type mockRewardCall struct {
	ExperimentID string
	ArmKey       string
	Success      bool
	Reward       float64
}

func (m *mockBanditUpdater) UpdateReward(_ context.Context, experimentID, armKey string, success bool, reward float64) error {
	m.calls = append(m.calls, mockRewardCall{experimentID, armKey, success, reward})
	return nil
}

// D04: 转化事件正确映射到 running 实验臂并回流；台账防重复
func TestD04_RefluxOnceEndToEnd(t *testing.T) {

	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)

	exp := &model.PromptABTest{
		ExperimentID:   "exp-d04-1",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          42,
		ArmKeys:        model.JSONArray{"A", "B"},
		Status:         "running",
	}
	if err := db.Create(exp).Error; err != nil {
		t.Fatal(err)
	}
	arm := &model.BanditArm{
		ExperimentID:   "exp-d04-1",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		ArmKey:         "A",
		SOPID:          42,
		Variant:        "A",
	}
	if err := db.Create(arm).Error; err != nil {
		t.Fatal(err)
	}

	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)

	ev := &model.FeedbackEvent{
		EventID:    "evt-d04-1",
		SessionID:  "sess-1",
		CustomerID: "c1",
		SOPID:      42,
		SignalKey:  "conversion",
		Reward:     2.0,
		Variant:    "A",
		CreatedAt:  time.Now(),
	}
	if err := db.Create(ev).Error; err != nil {
		t.Fatal(err)
	}

	stats, err := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 1 || stats.Refluxed != 1 {
		t.Fatalf("scanned=%d refluxed=%d, want 1/1 (skipped=%d duped=%d failed=%d)", stats.Scanned, stats.Refluxed, stats.Skipped, stats.Duped, stats.Failed)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("UpdateReward 应被调用 1 次, got %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.ExperimentID != "exp-d04-1" || call.ArmKey != "A" || !call.Success || call.Reward != 2.0 {
		t.Errorf("回流参数不符: %+v", call)
	}

	stats2, _ := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if stats2.Refluxed != 0 || stats2.Duped != 1 {
		t.Errorf("重扫应 duped=1 refluxed=0, got %+v", stats2)
	}
	if len(mock.calls) != 1 {
		t.Errorf("重扫不应再调 UpdateReward, got %d 次", len(mock.calls))
	}
}

// D04: 白名单外信号不入回流（tool_call 高频低值）
func TestD04_SignalWhitelist(t *testing.T) {

	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)

	ev := &model.FeedbackEvent{
		EventID:    "evt-d04-tool",
		SessionID:  "sess-w",
		CustomerID: "c1",
		SignalKey:  "tool_call",
		Reward:     0.3,
		CreatedAt:  time.Now(),
	}
	if err := db.Create(ev).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 0 || stats.Refluxed != 0 {
		t.Errorf("tool_call 应被白名单过滤, got scanned=%d refluxed=%d", stats.Scanned, stats.Refluxed)
	}
}

// D04: 负信号（complaint -2.0）→ success=false 且 reward 取绝对值
func TestD04_NegativeSignal(t *testing.T) {

	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)

	exp := &model.PromptABTest{
		ExperimentID:   "exp-d04-2",
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          7,
		ArmKeys:        model.JSONArray{"A"},
		Status:         "running",
	}
	db.Create(exp)
	db.Create(&model.BanditArm{ExperimentID: "exp-d04-2", ExperimentType: model.BanditExperimentTypeSOPVariant, ArmKey: "A", SOPID: 7})

	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)
	db.Create(&model.FeedbackEvent{
		EventID:    "evt-d04-neg",
		SessionID:  "sess-n",
		CustomerID: "c1",
		SOPID:      7,
		SignalKey:  "complaint",
		Reward:     -2.0,
		CreatedAt:  time.Now(),
	})
	stats, err := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Refluxed != 1 || len(mock.calls) != 1 {
		t.Fatalf("负信号应回流 1 次, got %+v", stats)
	}
	if mock.calls[0].Success || mock.calls[0].Reward != 2.0 {
		t.Errorf("负信号应 success=false reward=|−2.0|, got %+v", mock.calls[0])
	}
}

// D04: 无运行中实验 → skip
func TestD04_NoRunningExperimentSkips(t *testing.T) {

	db := testutil.NewTestDB(t,
		&model.FeedbackEvent{},
		&model.PromptABTest{},
		&model.BanditArm{},
		&model.BanditRefluxLog{},
	)
	mock := &mockBanditUpdater{}
	r := NewBanditRewardReflux(db, mock)
	db.Create(&model.FeedbackEvent{
		EventID:    "evt-d04-orphan",
		SessionID:  "sess-x",
		CustomerID: "c1",
		SOPID:      999,
		SignalKey:  "conversion",
		Reward:     2.0,
		CreatedAt:  time.Now(),
	})
	stats, _ := r.RefluxOnce(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if stats.Skipped != 1 {
		t.Errorf("无实验应 skip, got %+v", stats)
	}
}
