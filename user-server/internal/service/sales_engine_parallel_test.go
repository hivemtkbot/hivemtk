package service


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
		Config:      nil, 
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

