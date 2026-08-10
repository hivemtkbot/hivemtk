package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

// reach_ops_tools_test.go 触达运维类工具补充测试
//
// P2-3：从 tooluse 包迁入 app 包——本测试依赖真实 service.ReachPipelineService
// 装配（经 reachBatchPipelineAdapter 注入端口），属于装配层职责。
//
// 覆盖 reach_tools.go 中此前未被 reach_e2e_test / reach_channels_test 覆盖的工具：
//   - reach.batch     （批量触达：依赖 Pipeline.EnqueueJob，并发 + 失败收集）
//   - reach.schedule  （定时触达：依赖 Pipeline.EnqueueJob + RunAt 校验）
//   - reach.recall    （撤回：依赖 Adapter.Recall）
//   - reach.health    （账号健康度：依赖 Adapter.AccountHealth）
//   - reach.history   （触达历史：依赖 Pipeline.ListJobs）

// 批量/定时/历史测试统一经 reachBatchPipelineAdapter（与生产装配同路径）注入端口；
// Pipeline 缺失分支直接传 nil 端口验证。

// ===== reach.batch =====

func TestReachBatchTool_MissingPipelineDep(t *testing.T) {
	tool := tooluse.NewReachBatchTool(tooluse.ReachToolDeps{Adapter: &e2eMockReachAdapter{}})
	_, err := tool.Execute(context.Background(), map[string]any{
		"pipeline_id": "1",
		"items":       []any{map[string]any{"customer_id": "c1"}},
	})
	if err == nil {
		t.Fatal("expected error when Pipeline dependency is nil")
	}
}

func TestReachBatchTool_MissingRequired(t *testing.T) {
	p := &reachBatchPipelineAdapter{}
	tool := tooluse.NewReachBatchTool(tooluse.ReachToolDeps{Pipeline: p, Adapter: &e2eMockReachAdapter{}})
	// 缺 items
	if _, err := tool.Execute(context.Background(), map[string]any{"pipeline_id": "1"}); err == nil {
		t.Fatal("expected error when items missing")
	}
}

func TestReachBatchTool_EmptyItems(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ReachPipeline{}, &model.ReachJob{})
	svc := &reachBatchPipelineAdapter{svc: service.NewReachPipelineService(db)}
	tool := tooluse.NewReachBatchTool(tooluse.ReachToolDeps{Pipeline: svc, Adapter: &e2eMockReachAdapter{}})
	_, err := tool.Execute(context.Background(), map[string]any{
		"pipeline_id": "1",
		"items":       []any{},
	})
	if err == nil {
		t.Fatal("expected error when items is empty")
	}
}

// seedPipeline 在测试库创建一条真实 pipeline 并返回其 ID。
func seedPipeline(t *testing.T, svc *service.ReachPipelineService) uint {
	t.Helper()
	p, err := svc.CreatePipeline(context.Background(), &service.CreatePipelineRequest{
		Name:    "test-pipeline",
		Channel: "sms",
	})
	if err != nil {
		t.Fatalf("seed pipeline failed: %v", err)
	}
	return p.ID
}

func TestReachBatchTool_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ReachPipeline{}, &model.ReachJob{})
	rawSvc := service.NewReachPipelineService(db)
	pid := seedPipeline(t, rawSvc)
	svc := &reachBatchPipelineAdapter{svc: rawSvc}
	tool := tooluse.NewReachBatchTool(tooluse.ReachToolDeps{Pipeline: svc, Adapter: &e2eMockReachAdapter{}})
	res, err := tool.Execute(context.Background(), map[string]any{
		"pipeline_id": fmt.Sprintf("%d", pid),
		"channel":     "sms",
		"items": []any{
			map[string]any{"customer_id": "c1", "payload": map[string]any{"content": "hi"}},
			map[string]any{"customer_id": "c2", "payload": map[string]any{"content": "yo"}},
			map[string]any{"account_id": "a1"}, // 缺 customer_id -> 进 failed
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result data type: %T", res.Data)
	}
	if data["total"].(int) != 3 {
		t.Errorf("expected total=3, got %v", data["total"])
	}
	if data["success_count"].(int) != 2 {
		t.Errorf("expected success_count=2, got %v", data["success_count"])
	}
	if data["failed_count"].(int) != 1 {
		t.Errorf("expected failed_count=1, got %v", data["failed_count"])
	}
}

// ===== reach.schedule =====

func TestReachScheduleTool_MissingPipelineDep(t *testing.T) {
	tool := tooluse.NewReachScheduleTool(tooluse.ReachToolDeps{Adapter: &e2eMockReachAdapter{}})
	_, err := tool.Execute(context.Background(), map[string]any{
		"pipeline_id": "1", "customer_id": "c1",
		"run_at":  time.Now().Add(time.Hour).Format(time.RFC3339),
		"payload": map[string]any{"content": "x"},
	})
	if err == nil {
		t.Fatal("expected error when Pipeline dependency is nil")
	}
}

func TestReachScheduleTool_PastRunAt(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ReachPipeline{}, &model.ReachJob{})
	svc := &reachBatchPipelineAdapter{svc: service.NewReachPipelineService(db)}
	tool := tooluse.NewReachScheduleTool(tooluse.ReachToolDeps{Pipeline: svc, Adapter: &e2eMockReachAdapter{}})
	_, err := tool.Execute(context.Background(), map[string]any{
		"pipeline_id": "1", "customer_id": "c1",
		"run_at":  time.Now().Add(-time.Hour).Format(time.RFC3339),
		"payload": map[string]any{"content": "x"},
	})
	if err == nil {
		t.Fatal("expected error when run_at is in the past")
	}
}

func TestReachScheduleTool_InvalidRunAt(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ReachPipeline{}, &model.ReachJob{})
	svc := &reachBatchPipelineAdapter{svc: service.NewReachPipelineService(db)}
	tool := tooluse.NewReachScheduleTool(tooluse.ReachToolDeps{Pipeline: svc, Adapter: &e2eMockReachAdapter{}})
	if _, err := tool.Execute(context.Background(), map[string]any{
		"pipeline_id": "1", "customer_id": "c1",
		"run_at": "not-a-time", "payload": map[string]any{"content": "x"},
	}); err == nil {
		t.Fatal("expected error on invalid run_at format")
	}
}

func TestReachScheduleTool_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ReachPipeline{}, &model.ReachJob{})
	rawSvc := service.NewReachPipelineService(db)
	pid := seedPipeline(t, rawSvc)
	svc := &reachBatchPipelineAdapter{svc: rawSvc}
	tool := tooluse.NewReachScheduleTool(tooluse.ReachToolDeps{Pipeline: svc, Adapter: &e2eMockReachAdapter{}})
	runAt := time.Now().Add(2 * time.Hour)
	res, err := tool.Execute(context.Background(), map[string]any{
		"pipeline_id": fmt.Sprintf("%d", pid), "customer_id": "c9", "channel": "email",
		"run_at":  runAt.Format(time.RFC3339),
		"payload": map[string]any{"content": "promo"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := res.Data.(map[string]any)
	if data["state"] != "pending" {
		t.Errorf("expected state=pending, got %v", data["state"])
	}
}

// ===== reach.recall =====

func TestReachRecallTool_HappyPath(t *testing.T) {
	adapter := &e2eMockReachAdapter{}
	tool := tooluse.NewReachRecallTool(tooluse.ReachToolDeps{Adapter: adapter})
	res, err := tool.Execute(context.Background(), map[string]any{
		"channel": "sms", "message_id": "msg-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := res.Data.(map[string]any)
	if data["recalled"] != true {
		t.Errorf("expected recalled=true, got %v", data["recalled"])
	}
	if data["message_id"] != "msg-123" {
		t.Errorf("expected message_id=msg-123, got %v", data["message_id"])
	}
}

func TestReachRecallTool_AdapterError(t *testing.T) {
	adapter := &errRecallAdapter{err: errors.New("channel not supported")}
	tool := tooluse.NewReachRecallTool(tooluse.ReachToolDeps{Adapter: adapter})
	_, err := tool.Execute(context.Background(), map[string]any{
		"channel": "sms", "message_id": "msg-1",
	})
	if err == nil {
		t.Fatal("expected error when adapter.Recall fails")
	}
}

// errRecallAdapter Recall 返回错误的 mock adapter（其余方法 NoOp）
type errRecallAdapter struct {
	e2eMockReachAdapter
	err error
}

func (e *errRecallAdapter) Recall(_ context.Context, _, _ string) error { return e.err }

// ===== reach.health =====

func TestReachHealthTool_HappyPath(t *testing.T) {
	adapter := &e2eMockReachAdapter{}
	tool := tooluse.NewReachHealthTool(tooluse.ReachToolDeps{Adapter: adapter})
	res, err := tool.Execute(context.Background(), map[string]any{
		"channel": "sms", "account_id": "acc-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Data == nil {
		t.Fatal("expected health info, got nil")
	}
}

// ===== reach.history =====

func TestReachHistoryTool_MissingPipelineDep(t *testing.T) {
	tool := tooluse.NewReachHistoryTool(tooluse.ReachToolDeps{Adapter: &e2eMockReachAdapter{}})
	_, err := tool.Execute(context.Background(), map[string]any{"channel": "sms"})
	if err == nil {
		t.Fatal("expected error when Pipeline dependency is nil")
	}
}

func TestReachHistoryTool_PageSizeCap(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ReachPipeline{}, &model.ReachJob{})
	svc := &reachBatchPipelineAdapter{svc: service.NewReachPipelineService(db)}
	tool := tooluse.NewReachHistoryTool(tooluse.ReachToolDeps{Pipeline: svc, Adapter: &e2eMockReachAdapter{}})
	res, err := tool.Execute(context.Background(), map[string]any{
		"channel": "sms", "state": "success", "page": 1, "page_size": 9999,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := res.Data.(map[string]any)
	// 注意：reach.history 工具自实现分页上限为 200（reach_tools.go:1311），
	// 而 ReachPipelineService.ListJobs 的上限为 20（reach_pipeline.go:476），
	// 两者不一致（产品级审查发现点）。此处断言以工具实际行为(200)为准。
	if data["page_size"].(int) != 200 {
		t.Errorf("expected page_size capped to 200 by tool, got %v", data["page_size"])
	}
}
