package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

type mockFlagRepo struct {
	flags map[string]*model.FeatureFlag
	logs  []model.FeatureFlagEvalLog
}

func newMockFlagRepo() *mockFlagRepo {
	return &mockFlagRepo{flags: map[string]*model.FeatureFlag{}}
}

func (m *mockFlagRepo) Create(ctx context.Context, f *model.FeatureFlag) error {
	f.ID = uint(len(m.flags) + 1)
	m.flags[f.Key] = f
	return nil
}
func (m *mockFlagRepo) Update(ctx context.Context, f *model.FeatureFlag) error {
	m.flags[f.Key] = f
	return nil
}
func (m *mockFlagRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockFlagRepo) GetByID(ctx context.Context, id uint) (*model.FeatureFlag, error) {
	for _, f := range m.flags {
		if f.ID == id {
			return f, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockFlagRepo) GetByKey(ctx context.Context, key string) (*model.FeatureFlag, error) {
	if f, ok := m.flags[key]; ok {
		return f, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockFlagRepo) List(ctx context.Context, page, pageSize int) ([]model.FeatureFlag, int64, error) {
	return nil, 0, nil
}
func (m *mockFlagRepo) ListStale(ctx context.Context, olderThan time.Time) ([]model.FeatureFlag, error) {
	return nil, nil
}
func (m *mockFlagRepo) CreateAudit(ctx context.Context, a *model.FeatureFlagAuditLog) error {
	return nil
}
func (m *mockFlagRepo) ListAudit(ctx context.Context, flagID uint, limit int) ([]model.FeatureFlagAuditLog, error) {
	return nil, nil
}
func (m *mockFlagRepo) CreateEvalLog(ctx context.Context, e *model.FeatureFlagEvalLog) error {
	m.logs = append(m.logs, *e)
	return nil
}
func (m *mockFlagRepo) ListEvalLogs(ctx context.Context, flagKey string, limit int) ([]model.FeatureFlagEvalLog, error) {
	return m.logs, nil
}
func (m *mockFlagRepo) TouchEvaluated(ctx context.Context, id uint) error { return nil }

// K2 评估语义：kill switch > rollout 分桶 > not_found
func TestFeatureFlag_Evaluate_Semantics(t *testing.T) {
	repo := newMockFlagRepo()
	svc := NewFeatureFlagService(repo)
	ctx := context.Background()

	rollout := 50
	_, err := svc.Create(ctx, &FlagCreateRequest{Key: "ff_on", Enabled: true, RolloutPercentage: &rollout}, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Create(ctx, &FlagCreateRequest{Key: "ff_off", Enabled: false}, 1)
	_, _ = svc.Create(ctx, &FlagCreateRequest{Key: "ff_full", Enabled: true}, 1)

	if r := svc.Evaluate(ctx, "ff_off", map[string]any{"user_id": "u1"}); r.Enabled || r.Reason != "disabled" {
		t.Fatalf("禁用 flag 应 disabled, got %+v", r)
	}

	if r := svc.Evaluate(ctx, "ff_full", nil); !r.Enabled || r.Reason != "rollout" {
		t.Fatalf("100%% flag 应开启, got %+v", r)
	}

	zero := 0
	_, _ = svc.Create(ctx, &FlagCreateRequest{Key: "ff_zero", Enabled: true, RolloutPercentage: &zero}, 1)
	if r := svc.Evaluate(ctx, "ff_zero", nil); r.Enabled || r.Reason != "rollout_excluded" {
		t.Fatalf("0%% flag 应排除, got %+v", r)
	}

	if r := svc.Evaluate(ctx, "ff_missing", nil); r.Enabled || r.Reason != "not_found" {
		t.Fatalf("未知 flag 应 not_found, got %+v", r)
	}

	r1 := svc.Evaluate(ctx, "ff_on", map[string]any{"user_id": "sticky-user"})
	for i := 0; i < 10; i++ {
		if r2 := svc.Evaluate(ctx, "ff_on", map[string]any{"user_id": "sticky-user"}); r2.Enabled != r1.Enabled {
			t.Fatalf("分桶不粘性: %v vs %v", r1, r2)
		}
	}

	_, _ = svc.Create(ctx, &FlagCreateRequest{Key: "ff_payload", Enabled: true, Payload: `{"theme":"dark"}`}, 1)
	if r := svc.Evaluate(ctx, "ff_payload", nil); !r.Enabled {
		t.Fatalf("payload flag 应开启")
	} else if m, ok := r.Value.(map[string]any); !ok || m["theme"] != "dark" {
		t.Fatalf("payload 应解析为 map, got %#v", r.Value)
	}
}

// K2 批量评估 + 评估日志
func TestFeatureFlag_EvaluateBatch_AndLogs(t *testing.T) {
	repo := newMockFlagRepo()
	svc := NewFeatureFlagService(repo)
	ctx := context.Background()
	_, _ = svc.Create(ctx, &FlagCreateRequest{Key: "a", Enabled: true}, 1)
	_, _ = svc.Create(ctx, &FlagCreateRequest{Key: "b", Enabled: false}, 1)

	results := svc.EvaluateBatch(ctx, []string{"a", "b", "c"}, map[string]any{"user_id": "u9"})
	if len(results) != 3 {
		t.Fatalf("应返回 3 个结果, got %d", len(results))
	}
	if !results[0].Enabled || results[1].Enabled || results[2].Enabled {
		t.Fatalf("批量结果不符: %+v", results)
	}
}

// K2 rollout 边界
func TestFeatureFlag_RolloutBounds(t *testing.T) {
	repo := newMockFlagRepo()
	svc := NewFeatureFlagService(repo)
	ctx := context.Background()
	f, _ := svc.Create(ctx, &FlagCreateRequest{Key: "r", Enabled: true}, 1)

	if _, err := svc.SetRollout(ctx, f.ID, 101, 1); err == nil {
		t.Fatal("rollout=101 应拒绝")
	}
	if _, err := svc.SetRollout(ctx, f.ID, -1, 1); err == nil {
		t.Fatal("rollout=-1 应拒绝")
	}
	updated, err := svc.SetRollout(ctx, f.ID, 30, 1)
	if err != nil || updated.RolloutPercentage != 30 {
		t.Fatalf("rollout=30 应生效: %+v err=%v", updated, err)
	}
}
