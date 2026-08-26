package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/model"
)

// mockScriptLibraryRepo 可注入 mock（T-2 归因闭环测试用）
type mockScriptLibraryRepo struct {
	mu         sync.Mutex
	templates  []model.ScriptLibrary
	usageCalls []struct {
		templateID uint
		success    bool
	}
}

func (m *mockScriptLibraryRepo) ListObjectionTemplates(ctx context.Context, objectionCategory string, limit int) ([]model.ScriptLibrary, error) {
	return m.templates, nil
}

func (m *mockScriptLibraryRepo) IncrementUsageStats(ctx context.Context, templateID uint, success bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usageCalls = append(m.usageCalls, struct {
		templateID uint
		success    bool
	}{templateID, success})
	return nil
}

func (m *mockScriptLibraryRepo) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.usageCalls)
}

func (m *mockScriptLibraryRepo) lastCall() (uint, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.usageCalls) == 0 {
		return 0, false
	}
	c := m.usageCalls[len(m.usageCalls)-1]
	return c.templateID, c.success
}

// TestObjection_Handle_AutoRecordUsage Handle 推荐模板后应自动异步记录 usage（T-2 归因闭环）
func TestObjection_Handle_AutoRecordUsage(t *testing.T) {
	repo := &mockScriptLibraryRepo{
		templates: []model.ScriptLibrary{
			{ID: 42, Category: "objection", Subcategory: "price", Title: "价格异议话术", Content: "先讲价值再谈价格"},
		},
	}
	s := &ObjectionHandlerService{scriptRepo: repo}

	resp, err := s.Handle(context.Background(), HandleRequest{Text: "太贵了，能便宜点吗"})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if resp.Template == nil || resp.Template.ID != 42 {
		t.Fatalf("resp.Template.ID = %+v, want 42", resp.Template)
	}

	// 异步记录：轮询等待 goroutine 完成
	deadline := time.Now().Add(2 * time.Second)
	for repo.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if repo.callCount() == 0 {
		t.Fatal("Handle 推荐模板后应自动调用 IncrementUsageStats")
	}
	id, success := repo.lastCall()
	if id != 42 {
		t.Errorf("recorded template_id = %d, want 42", id)
	}
	if success {
		t.Error("自动记录默认 success=false（仅计 usage_count）")
	}

	// 无模板推荐时不应记录
	repo2 := &mockScriptLibraryRepo{}
	s2 := &ObjectionHandlerService{scriptRepo: repo2}
	if _, err := s2.Handle(context.Background(), HandleRequest{Text: "太贵了"}); err != nil {
		t.Fatalf("handle(2): %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if repo2.callCount() != 0 {
		t.Errorf("未推荐模板时不应记录 usage，实际 %d 次", repo2.callCount())
	}
}

// TestObjection_RecordUsage_Manual 手动 RecordUsage 端点链路仍走仓储
func TestObjection_RecordUsage_Manual(t *testing.T) {
	repo := &mockScriptLibraryRepo{}
	s := &ObjectionHandlerService{scriptRepo: repo}
	if err := s.RecordUsage(context.Background(), 7, true); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	id, success := repo.lastCall()
	if id != 7 || !success {
		t.Errorf("last call = (%d, %v), want (7, true)", id, success)
	}
}
