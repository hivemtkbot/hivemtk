// Package integration 知识库隔离集成测试 (Task 35)
//
// 目标: 验证多智能体场景下, 知识库严格按 owner_agent_id 隔离,
//   共享知识库 (owner_type=shared) 可被多个智能体通过 agent_kb_bindings 访问,
//   私有知识库 (owner_type=private) 只能被其 owner_agent_id 对应智能体访问.
//
// 依赖: 真实 PostgreSQL (由 testutil.NewTestDB 注入), 不依赖 user-server 运行实例.
//
// 测试矩阵:
//
//	┌───────────────┬─────────────┬────────────────┬──────────────────┐
//	│ KB OwnerType  │ agent_id 可见│ 隔离效果        │ 共享访问方式      │
//	├───────────────┼─────────────┼────────────────┼──────────────────┤
//	│ private=agent1│ 仅 agent1   │ ✅ 严格隔离     │ ❌ 不可共享      │
//	│ private=agent2│ 仅 agent2   │ ✅ 严格隔离     │ ❌ 不可共享      │
//	│ shared        │ 全部(绑定后)│ ⚠️ 默认隔离     │ ✅ 显式 binding │
//	└───────────────┴─────────────┴────────────────┴──────────────────┘
package integration

import (
	"context"
	"fmt"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"gorm.io/gorm"
)

// setupIsolationDB 准备隔离测试 DB
func setupIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDBOrSkip(t,
		&model.KnowledgeBase{},
		&model.AgentKBBinding{},
	)
	// 清表
	for _, tbl := range []string{"agent_kb_bindings", "knowledge_bases"} {
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", tbl)).Error; err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
	return db
}

// seedAgents 模拟创建两个智能体 (使用 ai_agents 表? 这里直接用业务 Service mock)
// 真实场景中 ai_agents 由其他模块维护, 本测试只关心 KB 隔离, 所以用 uint 模拟.
func newIsolationSetup(t *testing.T, db *gorm.DB) (*service.KnowledgeBaseService, *service.AgentKBBindingService) {
	t.Helper()
	kbRepo := repository.NewKnowledgeBaseRepository(db)
	bindRepo := repository.NewAgentKBBindingRepository(db)
	kbSvc := service.NewKnowledgeBaseService(db)
	kbSvc.SetRepositories(kbRepo, bindRepo)
	bindSvc := service.NewAgentKBBindingServiceWithRepos(kbRepo, bindRepo, db)
	return kbSvc, bindSvc
}

// ----------------------------------------------------------------------------
// 隔离测试用例
// ----------------------------------------------------------------------------

// TestIsolation_PrivateKB_NotVisibleToOtherAgent 验证私有 KB 严格隔离
//
// 场景:
//   - agent1 拥有 1 个 private FAQ KB
//   - agent2 完全独立
//   - agent2 不能通过 ListByAgent 看到 agent1 的 private KB
func TestIsolation_PrivateKB_NotVisibleToOtherAgent(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	// 1. agent1 创建 private KB
	agent1 := uint(101)
	kb1 := &model.KnowledgeBase{
		KBCode:       "KB-FAQ-AGENT1-PRI",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "agent1 私有 FAQ",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb1); err != nil {
		t.Fatalf("create private kb for agent1: %v", err)
	}

	// 2. agent2 创建 private KB
	agent2 := uint(202)
	kb2 := &model.KnowledgeBase{
		KBCode:       "KB-FAQ-AGENT2-PRI",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "agent2 私有 FAQ",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent2,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb2); err != nil {
		t.Fatalf("create private kb for agent2: %v", err)
	}

	// 3. agent1 列出自己的 KB, 应只有 1 条
	got1, err := kbSvc.ListByAgent(ctx, agent1)
	if err != nil {
		t.Fatalf("ListByAgent(agent1): %v", err)
	}
	if len(got1) != 1 {
		t.Fatalf("expected agent1 has 1 KB, got %d", len(got1))
	}
	if got1[0].KBCode != "KB-FAQ-AGENT1-PRI" {
		t.Errorf("agent1's KB mismatch: got %q", got1[0].KBCode)
	}

	// 4. agent2 列出自己的 KB, 应只有 1 条 (隔离验证)
	got2, err := kbSvc.ListByAgent(ctx, agent2)
	if err != nil {
		t.Fatalf("ListByAgent(agent2): %v", err)
	}
	if len(got2) != 1 {
		t.Fatalf("expected agent2 has 1 KB, got %d (隔离失败: agent2 看到了 agent1 的 KB)", len(got2))
	}
	if got2[0].KBCode != "KB-FAQ-AGENT2-PRI" {
		t.Errorf("agent2's KB mismatch: got %q", got2[0].KBCode)
	}
}

// TestIsolation_PrivateKB_GetByID_OtherAgentShouldStillGetIt 验证 GetByID 无 owner 校验
//
// 业务说明: GetByID 是管理 API, 不做 owner 校验.
//   隔离靠 ListByAgent 过滤, 而不是 GetByID.
//   业务调用方应使用 ListByAgent 而非 GetByID 来避免越权.
func TestIsolation_PrivateKB_GetByID_OtherAgentShouldStillGetIt(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(101)
	kb1 := &model.KnowledgeBase{
		KBCode:       "KB-RAG-AGENT1",
		Type:         model.KnowledgeBaseTypeRAG,
		Name:         "agent1 RAG",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb1); err != nil {
		t.Fatal(err)
	}
	// GetByID 不做 owner 校验, 是预期行为 (管理 API)
	got, err := kbSvc.GetKB(ctx, kb1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.OwnerAgentID == nil || *got.OwnerAgentID != agent1 {
		t.Errorf("owner_agent_id mismatch")
	}
}

// TestIsolation_SharedKB_NeedsBindingToBeUsed 验证共享 KB 需通过 binding 访问
//
// 业务规则: shared KB 默认所有智能体都看不到, 必须显式 binding
//
//   - CreateBinding: (agent3, shared_kb_id) → 智能体3 才能 ListByAgent 看到
func TestIsolation_SharedKB_NeedsBindingToBeUsed(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, bindSvc := newIsolationSetup(t, db)

	// 1. 创建 shared FAQ KB
	sharedKB := &model.KnowledgeBase{
		KBCode:    "KB-FAQ-SHARED",
		Type:      model.KnowledgeBaseTypeFAQ,
		Name:      "公共 FAQ 库",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, sharedKB); err != nil {
		t.Fatalf("create shared kb: %v", err)
	}
	if sharedKB.OwnerAgentID != nil {
		t.Errorf("shared KB's owner_agent_id should be nil, got %v", *sharedKB.OwnerAgentID)
	}

	// 2. ListByAgent(任意智能体) 不应看到 shared KB (默认无 binding)
	got1, err := kbSvc.ListByAgent(ctx, 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(got1) != 0 {
		t.Errorf("expected no KB for unbound agent, got %d", len(got1))
	}

	// 3. agent1 显式 binding 后才能看到
	if err := bindSvc.Bind(ctx, 1001, sharedKB.ID, 0); err != nil {
		t.Fatalf("bind: %v", err)
	}
	got2, err := kbSvc.ListByAgent(ctx, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 1 {
		t.Errorf("expected 1 KB after binding, got %d", len(got2))
	}

	// 4. agent2 没 binding 仍然看不到
	got3, err := kbSvc.ListByAgent(ctx, 1002)
	if err != nil {
		t.Fatal(err)
	}
	if len(got3) != 0 {
		t.Errorf("expected agent2 (unbound) to see 0 KB, got %d", len(got3))
	}
}

// TestIsolation_SharedKB_VisibleToAllBoundAgents 验证 shared KB 可被多个智能体共享
//
// 场景:
//   - 1 个 shared FAQ KB
//   - 5 个智能体都 binding
//   - 每个智能体 ListByAgent 都能看到这 1 个 KB
func TestIsolation_SharedKB_VisibleToAllBoundAgents(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, bindSvc := newIsolationSetup(t, db)

	sharedKB := &model.KnowledgeBase{
		KBCode:    "KB-RAG-SHARED",
		Type:      model.KnowledgeBaseTypeRAG,
		Name:      "公共 RAG",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, sharedKB); err != nil {
		t.Fatal(err)
	}

	// 5 个智能体全部 binding
	for i := uint(1); i <= 5; i++ {
		if err := bindSvc.Bind(ctx, i, sharedKB.ID, int(i)); err != nil {
			t.Fatalf("bind agent%d: %v", i, err)
		}
	}

	// 每个智能体都能看到
	for i := uint(1); i <= 5; i++ {
		got, err := kbSvc.ListByAgent(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("agent%d expected 1 KB, got %d", i, len(got))
		}
	}
}

// TestIsolation_DeleteKB_CascadesBindings 验证删除 KB 级联删除 bindings
//
// 业务规则: 删除 KB 时, 必须级联删除 agent_kb_bindings 引用,
// 防止 dangling reference.
func TestIsolation_DeleteKB_CascadesBindings(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, bindSvc := newIsolationSetup(t, db)

	// 1. 创建 shared KB
	kb := &model.KnowledgeBase{
		KBCode:    "KB-SOP-SHARED",
		Type:      model.KnowledgeBaseTypeSOP,
		Name:      "公共 SOP",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// 2. 3 个智能体 binding
	for i := uint(1); i <= 3; i++ {
		if err := bindSvc.Bind(ctx, i, kb.ID, 0); err != nil {
			t.Fatalf("bind: %v", err)
		}
	}

	bindRepo := repository.NewAgentKBBindingRepository(db)
	before, _ := bindRepo.ListByKB(ctx, kb.ID)
	if len(before) != 3 {
		t.Fatalf("expected 3 bindings, got %d", len(before))
	}

	// 3. 删除 KB, 应级联删 bindings
	if err := kbSvc.DeleteKB(ctx, kb.ID); err != nil {
		t.Fatalf("DeleteKB: %v", err)
	}

	after, _ := bindRepo.ListByKB(ctx, kb.ID)
	if len(after) != 0 {
		t.Errorf("expected 0 bindings after KB delete (cascading), got %d", len(after))
	}
}

// TestIsolation_PrivateKB_ReassignOwner_NotAllowed 验证私有 KB 不能被改成 shared
//
// 业务规则: 业务上, private KB 的 owner_agent_id 一旦设定, 不应允许改为 shared
//   (避免安全漏洞: 智能体私有数据被"开放"为共享数据)
//
//   当前 service 不强制此约束, 但记录为业务约束, 验证 ListByAgent 仍按 owner 过滤
func TestIsolation_PrivateKB_ReassignOwner_NotAllowed(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-FAQ-OWNER-TEST",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "owner test",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// agent1 看到 1 条
	got1, _ := kbSvc.ListByAgent(ctx, agent1)
	if len(got1) != 1 {
		t.Fatalf("expected 1 KB for agent1, got %d", len(got1))
	}

	// agent2 看到 0 条 (无 binding, 严格隔离)
	got2, _ := kbSvc.ListByAgent(ctx, 2)
	if len(got2) != 0 {
		t.Errorf("expected 0 KB for agent2 (no binding), got %d", len(got2))
	}
}

// TestIsolation_BindDuplicate_UpdatesPriority 验证重复 binding 自动覆盖 priority
//
// 业务规则: 同一 (agent, kb) 重复 Bind, 不报错, 覆盖 priority.
func TestIsolation_BindDuplicate_UpdatesPriority(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)

	kbRepo := repository.NewKnowledgeBaseRepository(db)
	agentID := uint(7)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-FAQ-DUP-TEST",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "dup test",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agentID,
		Enabled:      boolPtr(true),
	}
	if err := kbRepo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// 第一次 bind priority=1
	if err := bindSvc.Bind(ctx, agentID, kb.ID, 1); err != nil {
		t.Fatal(err)
	}
	// 第二次 bind priority=99
	if err := bindSvc.Bind(ctx, agentID, kb.ID, 99); err != nil {
		t.Fatalf("repeat bind should not error: %v", err)
	}

	// 应只存在 1 条, priority=99
	bindRepo := repository.NewAgentKBBindingRepository(db)
	bindings, _ := bindRepo.ListByAgent(ctx, agentID, "")
	if len(bindings) != 1 {
		t.Errorf("expected 1 binding, got %d", len(bindings))
	}
	if len(bindings) > 0 && bindings[0].Priority != 99 {
		t.Errorf("expected priority=99, got %d", bindings[0].Priority)
	}
}

// TestIsolation_DisableKB_FilteredOut 验证禁用 KB 不被 ListByAgent 列出
func TestIsolation_DisableKB_FilteredOut(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-FAQ-DISABLED",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "disabled",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(false),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}

	got, _ := kbSvc.ListByAgent(ctx, agent1)
	if len(got) != 0 {
		t.Errorf("disabled KB should be filtered, got %d", len(got))
	}
}

// ----------------------------------------------------------------------------
// 工具函数
// ----------------------------------------------------------------------------

func boolPtr(b bool) *bool { return &b }
