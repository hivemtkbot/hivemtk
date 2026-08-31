package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"

	"github.com/stretchr/testify/assert"
)

// TestReachAlertHook_InvokedOnFire 验证 P1-8 告警钩子机制：
// SetAlertHook 注入后，fireAlert 应同步调用该回调并透传终态与原因。
func TestReachAlertHook_InvokedOnFire(t *testing.T) {
	svc := &ReachPipelineService{}
	called := false
	var gotState, gotReason string
	svc.SetAlertHook(func(ctx context.Context, job *model.ReachJob, finalState, reason string) {
		called = true
		gotState = finalState
		gotReason = reason
	})

	svc.fireAlert(context.Background(), &model.ReachJob{
		ID:         1,
		Channel:    "telegram",
		AccountID:  "acc1",
		CustomerID: "user1",
	}, "failed", "boom")

	assert.True(t, called, "告警钩子应在 fireAlert 时被调用")
	assert.Equal(t, "failed", gotState)
	assert.Equal(t, "boom", gotReason)
}

// TestReachAlertHook_NilNoPanic 验证未注入告警钩子时 fireAlert 不 panic（向后兼容）。
func TestReachAlertHook_NilNoPanic(t *testing.T) {
	svc := &ReachPipelineService{}
	assert.NotPanics(t, func() {
		svc.fireAlert(context.Background(), &model.ReachJob{ID: 1}, "failed", "x")
	})
}
