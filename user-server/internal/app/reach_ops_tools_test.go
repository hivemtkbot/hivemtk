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
			map[string]any{"account_id": "a1"}, 
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
	if data["page_size"].(int) != 200 {
		t.Errorf("expected page_size capped to 200 by tool, got %v", data["page_size"])
	}
}

