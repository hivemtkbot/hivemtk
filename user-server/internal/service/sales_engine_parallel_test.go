package service

// sales_engine_parallel_test.go SalesEngine.HandleParallel 单元测试
//
// 设计依据: AI 智能体性能优化
//
// 测试目标:
//   - TestSalesEngine_HandleParallel_Phase0: 基本 smoke test, 验证 Phase 0 并行执行不 panic
//   - 不依赖真实 DB / LLM dispatcher
//   - 通过 nil 注入短路, 仅测试控制流

import (
	"context"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/featureflag"
)

// TestSalesEngine_HandleParallel_NilRequest 测试 nil req 应返回 error
func TestSalesEngine_HandleParallel_NilRequest(t *testing.T) {
	e := &SalesEngine{}
	_, err := e.HandleParallel(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil request")
	}
}

// TestSalesEngine_HandleParallel_EmptyMessage 测试空 UserMessage 应返回 error
func TestSalesEngine_HandleParallel_EmptyMessage(t *testing.T) {
	e := &SalesEngine{}
	req := &dto.SalesRequest{
		SessionID:   "s1",
		CustomerID:  "c1",
		UserMessage: "",
	}
	_, err := e.HandleParallel(context.Background(), req)
	if err == nil {
		t.Error("expected error for empty user message")
	}
}

// TestSalesEngine_HandleParallel_Phase0 测试 Phase 0 并行不 panic
func TestSalesEngine_HandleParallel_Phase0(t *testing.T) {
	// 全部依赖为 nil, 测试控制流不 panic
	// 注意: dispatcher 为 nil 时, Phase 1 generateCandidate 会失败,
	//       但 resp.Reply 会被设为兜底文案, 不会 panic
	e := &SalesEngine{}

	req := &dto.SalesRequest{
		SessionID:   "s1",
		CustomerID:  "c1",
		UserMessage: "你好",
		Platform:    "wechat",
	}
	resp, err := e.HandleParallel(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Steps) == 0 {
		t.Error("expected at least 1 step logged")
	}
	// Phase 0 必须出现在 steps 中
	found := false
	for _, s := range resp.Steps {
		if s.Step == PhaseParallel {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected phase0 step in resp.Steps, got %+v", resp.Steps)
	}
	// LatencyMs 必填
	if resp.LatencyMs < 0 {
		t.Errorf("expected latency >= 0, got %d", resp.LatencyMs)
	}
}

// TestSalesEngine_HandleParallel_DefaultConfig 测试 Config=nil 时自动填充默认配置
func TestSalesEngine_HandleParallel_DefaultConfig(t *testing.T) {
	e := &SalesEngine{}
	req := &dto.SalesRequest{
		SessionID:   "s1",
		CustomerID:  "c1",
		UserMessage: "你好",
		Config:      nil, // 显式 nil, 应被自动填充
	}
	resp, err := e.HandleParallel(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// TestSalesEngine_ShouldUseParallel 测试 FeatureFlag 控制
func TestSalesEngine_ShouldUseParallel(t *testing.T) {
	// 通过临时环境变量控制
	// 兼容 FeatureFlag 热加载: t.Setenv 只改 env 不刷缓存,
	// 需要 ReloadAll() 才能让新的 env 值生效。
	t.Run("FF_PARALLEL=1", func(t *testing.T) {
		t.Setenv("FF_PARALLEL", "1")
		featureflag.DefaultManager().ReloadAll()
		e := &SalesEngine{}
		if !e.shouldUseParallel() {
			t.Error("expected shouldUseParallel=true when FF_PARALLEL=1")
		}
	})

	t.Run("FF_PARALLEL=0", func(t *testing.T) {
		t.Setenv("FF_PARALLEL", "0")
		featureflag.DefaultManager().ReloadAll()
		e := &SalesEngine{}
		if e.shouldUseParallel() {
			t.Error("expected shouldUseParallel=false when FF_PARALLEL=0")
		}
	})

	t.Run("FF_PARALLEL empty", func(t *testing.T) {
		// 不设置
		t.Setenv("FF_PARALLEL", "")
		featureflag.DefaultManager().ReloadAll()
		e := &SalesEngine{}
		if e.shouldUseParallel() {
			t.Error("expected shouldUseParallel=false when FF_PARALLEL unset")
		}
	})
}

// TestSalesEngine_HandleParallel_PhaseConstants 验证阶段常量未变更
func TestSalesEngine_HandleParallel_PhaseConstants(t *testing.T) {
	if PhaseParallel != "0_phase_parallel" {
		t.Errorf("PhaseParallel constant changed: %s", PhaseParallel)
	}
	if PhaseSerial != "1_phase_serial" {
		t.Errorf("PhaseSerial constant changed: %s", PhaseSerial)
	}
	if PhaseAsync != "2_phase_async" {
		t.Errorf("PhaseAsync constant changed: %s", PhaseAsync)
	}
}
