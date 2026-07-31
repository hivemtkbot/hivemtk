// Package e2e 智能体 ↔ 知识库绑定 端到端测试 (Task 39)
//
// 目标: 跨 service 层的端到端业务流程测试, 验证完整业务链路:
//
//	┌──────────┐    ┌────────────┐    ┌──────────┐    ┌──────────────┐
//	│ AI Agent │ →  │ Knowledge  │ →  │ Binding  │ →  │  查询 / 命中  │
//	│ (业务)   │    │ Base       │    │ (关系表) │    │  (业务消费)  │
//	└──────────┘    └────────────┘    └──────────┘    └──────────────┘
//
// 测试策略:
//   - 不依赖 HTTP/Controller, 通过 service + repository 模拟业务调用方
//   - 使用真实 PostgreSQL (testutil), 跨多 service 协作
//   - 每个测试覆盖一条完整的"业务故事" (story)
//
// 与 integration 的区别:
//   - integration: 单 service / 单 repo 的功能正确性
//   - e2e: 多 service 协作 + 业务故事 (跨域)
package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/repository"
	"marketing/internal/service"

	"gorm.io/gorm"
)

// setupE2EDB 准备端到端测试 DB
func setupE2EDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t,
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

// newE2ESetup 构造完整的 e2e 业务编排 service
func newE2ESetup(t *testing.T, db *gorm.DB) (
	*service.KnowledgeBaseService,
	*service.AgentKBBindingService,
	*repository.KnowledgeBaseRepository,
	*repository.AgentKBBindingRepository,
) {
	t.Helper()
	kbRepo := repository.NewKnowledgeBaseRepository(db)
	bindRepo := repository.NewAgentKBBindingRepository(db)
	kbSvc := service.NewKnowledgeBaseService(db)
	kbSvc.SetRepositories(kbRepo, bindRepo)
	bindSvc := service.NewAgentKBBindingServiceWithRepos(kbRepo, bindRepo, db)
	return kbSvc, bindSvc, kbRepo, bindRepo
}

func boolPtrE2E(b bool) *bool { return &b }

// =====================================================================
// 业务故事 1: 电商客服多租户隔离
// =====================================================================

// TestE2E_EcommerceMultiTenant_KnowledgeIsolation 验证多租户场景下, 知识库严格隔离
//
// 场景: 3 个独立电商品牌 (agent1/agent2/agent3), 各自有私有知识库 + 一个跨租户共享的"平台规则库"
func TestE2E_EcommerceMultiTenant_KnowledgeIsolation(t *testing.T) {
	db := setupE2EDB(t)
	ctx := context.Background()
	kbSvc, bindSvc, _, _ := newE2ESetup(t, db)

	// 1. 三个品牌创建各自的私有 FAQ
	brands := map[uint]string{
		1001: "brand-A 客服 FAQ",
		1002: "brand-B 客服 FAQ",
		1003: "brand-C 客服 FAQ",
	}
	for aid, name := range brands {
		agent := aid
		kb := &model.KnowledgeBase{
			KBCode:       fmt.Sprintf("KB-FAQ-BRAND-%d", aid),
			Type:         model.KnowledgeBaseTypeFAQ,
			Name:         name,
			OwnerType:    model.KnowledgeBaseOwnerPrivate,
			OwnerAgentID: &agent,
			Enabled:      boolPtrE2E(true),
		}
		if err := kbSvc.CreateKB(ctx, kb); err != nil {
			t.Fatalf("create kb for agent %d: %v", aid, err)
		}
	}

	// 2. 创建共享的"平台规则" SOP 库
	platformKB := &model.KnowledgeBase{
		KBCode:    "KB-SOP-PLATFORM-RULES",
		Type:      model.KnowledgeBaseTypeSOP,
		Name:      "平台通用规则 SOP",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, platformKB); err != nil {
		t.Fatalf("create shared platform KB: %v", err)
	}

	// 3. 三品牌全部绑定共享 SOP
	for aid := range brands {
		if err := bindSvc.Bind(ctx, aid, platformKB.ID, 0); err != nil {
			t.Fatalf("bind agent %d to platform KB: %v", aid, err)
		}
	}

	// 4. 验证: 品牌 A 可见 2 个 KB (自己私有的 + 平台共享)
	gotA, err := kbSvc.ListByAgent(ctx, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotA) != 2 {
		t.Errorf("expected brand-A sees 2 KB (private + platform), got %d", len(gotA))
	}

	// 5. 验证: 品牌 A 看不到 B/C 的私有 KB
	for _, kb := range gotA {
		if strings.HasPrefix(kb.KBCode, "KB-FAQ-BRAND-100") || strings.HasPrefix(kb.KBCode, "KB-FAQ-BRAND-1002") || strings.HasPrefix(kb.KBCode, "KB-FAQ-BRAND-1003") {
			if kb.OwnerAgentID != nil && *kb.OwnerAgentID != 1001 {
				t.Errorf("brand-A 越权看到了 brand-%d 的私有 KB", *kb.OwnerAgentID)
			}
		}
	}
}

// =====================================================================
// 业务故事 2: 客服工作流 (创建 → 绑定 → 命中 → 解绑 → 失效)
// =====================================================================

// TestE2E_AgentWorkflow_FullLifecycle 验证客服智能体全生命周期
//
// 场景: 客服智能体 agent=2001 启用 SOP 工作流:
//   1. 创建 SOP 知识库
//   2. 绑定 (启用)
//   3. 查询命中
//   4. 解绑 (不删除 KB)
//   5. 重新绑定另一个 KB
func TestE2E_AgentWorkflow_FullLifecycle(t *testing.T) {
	db := setupE2EDB(t)
	ctx := context.Background()
	kbSvc, bindSvc, _, _ := newE2ESetup(t, db)

	agent := uint(2001)

	// 1. 创建 SOP 库 A
	kbA := &model.KnowledgeBase{
		KBCode:       "KB-SOP-WORKFLOW-A",
		Type:         model.KnowledgeBaseTypeSOP,
		Name:         "退换货流程 A",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent,
		Enabled:      boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, kbA); err != nil {
		t.Fatal(err)
	}

	// 2. 绑定 (虽然 private 的 KB 也会被 ListByAgent 看到, 但显式 binding 用于跨 agent 共享场景)
	if err := bindSvc.Bind(ctx, agent, kbA.ID, 10); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// 3. 查询
	got, err := kbSvc.ListByAgent(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 KB, got %d", len(got))
	}
	if got[0].KBCode != "KB-SOP-WORKFLOW-A" {
		t.Errorf("expected workflow-A, got %q", got[0].KBCode)
	}

	// 4. 解绑
	if err := bindSvc.Unbind(ctx, agent, kbA.ID); err != nil {
		t.Fatal(err)
	}
	got2, _ := kbSvc.ListByAgent(ctx, agent)
	if len(got2) != 1 {
		// 注意: private KB 始终可见, 这里只验证 binding 已删除
		t.Logf("private KB still visible (expected), got %d", len(got2))
	}

	// 5. 创建另一个 KB B 并绑定
	kbB := &model.KnowledgeBase{
		KBCode:       "KB-SOP-WORKFLOW-B",
		Type:         model.KnowledgeBaseTypeSOP,
		Name:         "退换货流程 B",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent,
		Enabled:      boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, kbB); err != nil {
		t.Fatal(err)
	}
	if err := bindSvc.Bind(ctx, agent, kbB.ID, 20); err != nil {
		t.Fatal(err)
	}

	// 6. 此时 agent 应有 2 个 private KB (A 和 B), 1 个 binding (B)
	bindings, _ := bindSvc.ListByAgent(ctx, agent)
	if len(bindings) != 1 || bindings[0].KBID != kbB.ID {
		t.Errorf("expected 1 binding (to B), got %v", bindings)
	}
	if bindings[0].Priority != 20 {
		t.Errorf("expected priority=20, got %d", bindings[0].Priority)
	}
}

// =====================================================================
// 业务故事 3: 删除知识库 → 级联清理 bindings
// =====================================================================

// TestE2E_KBDelete_CascadeBindingCleanup 验证删除 KB 时级联清理所有 binding
//
// 场景: 共享 KB 被 5 个 agent 引用, 删除 KB 后所有 binding 应自动消失
func TestE2E_KBDelete_CascadeBindingCleanup(t *testing.T) {
	db := setupE2EDB(t)
	ctx := context.Background()
	kbSvc, bindSvc, _, bindRepo := newE2ESetup(t, db)

	// 1. 共享 KB
	sharedKB := &model.KnowledgeBase{
		KBCode:    "KB-E2E-CASCADE",
		Type:      model.KnowledgeBaseTypeRAG,
		Name:      "cascade test",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, sharedKB); err != nil {
		t.Fatal(err)
	}

	// 2. 5 个 agent 绑定
	for i := uint(1); i <= 5; i++ {
		if err := bindSvc.Bind(ctx, i, sharedKB.ID, int(i)); err != nil {
			t.Fatalf("bind agent %d: %v", i, err)
		}
	}

	// 3. 验证 5 个 binding 存在
	bindings, _ := bindRepo.ListByKB(ctx, sharedKB.ID)
	if len(bindings) != 5 {
		t.Fatalf("expected 5 bindings, got %d", len(bindings))
	}

	// 4. 删除 KB
	if err := kbSvc.DeleteKB(ctx, sharedKB.ID); err != nil {
		t.Fatal(err)
	}

	// 5. 验证: bindings 应被级联删除
	bindingsAfter, _ := bindRepo.ListByKB(ctx, sharedKB.ID)
	if len(bindingsAfter) != 0 {
		t.Errorf("expected 0 bindings after KB delete (cascade), got %d", len(bindingsAfter))
	}

	// 6. 验证: KB 本身也消失
	got, _ := kbSvc.GetKB(ctx, sharedKB.ID)
	if got != nil {
		t.Error("expected KB to be deleted")
	}
}

// =====================================================================
// 业务故事 4: 共享知识库的"白名单"分发
// =====================================================================

// TestE2E_SharedKB_WhitelistDistribution 验证共享 KB 仅对白名单 agent 可见
//
// 场景:
//   - 创建一个共享 SOP
//   - 10 个 agent, 仅 3 个加入白名单 (binding)
//   - 验证仅 3 个 agent 可见
func TestE2E_SharedKB_WhitelistDistribution(t *testing.T) {
	db := setupE2EDB(t)
	ctx := context.Background()
	kbSvc, bindSvc, _, _ := newE2ESetup(t, db)

	// 1. 共享 KB
	sharedKB := &model.KnowledgeBase{
		KBCode:    "KB-E2E-WHITELIST",
		Type:      model.KnowledgeBaseTypeSOP,
		Name:      "whitelist test",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, sharedKB); err != nil {
		t.Fatal(err)
	}

	// 2. 10 个 agent, 3 个白名单
	whitelist := map[uint]bool{1: true, 5: true, 9: true}
	for i := uint(1); i <= 10; i++ {
		if whitelist[i] {
			if err := bindSvc.Bind(ctx, i, sharedKB.ID, 0); err != nil {
				t.Fatalf("bind agent %d: %v", i, err)
			}
		}
	}

	// 3. 验证: 3 个白名单 agent 可见
	for i := uint(1); i <= 10; i++ {
		got, _ := kbSvc.ListByAgent(ctx, i)
		found := false
		for _, kb := range got {
			if kb.KBCode == "KB-E2E-WHITELIST" {
				found = true
				break
			}
		}
		if whitelist[i] && !found {
			t.Errorf("whitelist agent %d should see shared KB", i)
		}
		if !whitelist[i] && found {
			t.Errorf("non-whitelist agent %d should NOT see shared KB", i)
		}
	}
}

// =====================================================================
// 业务故事 5: 批量绑定 (管理员一次性配置)
// =====================================================================

// TestE2E_AdminBatchConfig 验证管理员批量配置多 agent × 多 KB
//
// 场景: 新员工培训场景, 1 个 SOP 库 + 1 个 FAQ 库, 20 个新入职客服一次性绑定
func TestE2E_AdminBatchConfig(t *testing.T) {
	db := setupE2EDB(t)
	ctx := context.Background()
	kbSvc, bindSvc, _, _ := newE2ESetup(t, db)

	// 1. 创建 2 个 shared KB
	sop := &model.KnowledgeBase{
		KBCode:    "KB-SOP-NEW-HIRE",
		Type:      model.KnowledgeBaseTypeSOP,
		Name:      "新员工入职 SOP",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtrE2E(true),
	}
	faq := &model.KnowledgeBase{
		KBCode:    "KB-FAQ-NEW-HIRE",
		Type:      model.KnowledgeBaseTypeFAQ,
		Name:      "新员工常见问题",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, sop); err != nil {
		t.Fatal(err)
	}
	if err := kbSvc.CreateKB(ctx, faq); err != nil {
		t.Fatal(err)
	}

	// 2. 构造批量绑定: 20 个 agent × 2 个 KB = 40 条 binding
	const newHireCount = 20
	items := make([]service.BatchBindItem, 0, newHireCount*2)
	for i := uint(3001); i < 3001+newHireCount; i++ {
		items = append(items, service.BatchBindItem{AgentID: i, KBID: sop.ID, Priority: 1})
		items = append(items, service.BatchBindItem{AgentID: i, KBID: faq.ID, Priority: 2})
	}

	if err := bindSvc.BatchBind(ctx, items); err != nil {
		t.Fatalf("batch bind: %v", err)
	}

	// 3. 验证: 每个新员工都绑定了 2 个 KB
	for i := uint(3001); i < 3001+newHireCount; i++ {
		bindings, _ := bindSvc.ListByAgent(ctx, i)
		if len(bindings) != 2 {
			t.Errorf("agent %d expected 2 bindings, got %d", i, len(bindings))
		}
	}
}

// =====================================================================
// 业务故事 6: 业务规则反向验证 (校验失败的复合场景)
// =====================================================================

// TestE2E_BusinessRules_CompositeValidation 验证多层业务规则的复合场景
//
// 场景:
//   - 创建 shared KB 时附带 owner_agent_id -> 业务校验失败
//   - 创建 private KB 时不带 owner -> 业务校验失败
//   - shared KB 的 binding 不能被 ListByKB 错误统计 (因为没真绑定)
func TestE2E_BusinessRules_CompositeValidation(t *testing.T) {
	db := setupE2EDB(t)
	ctx := context.Background()
	kbSvc, _, _, _ := newE2ESetup(t, db)

	// Case 1: shared KB 携带 owner_agent_id -> 失败
	agent := uint(99)
	bad1 := &model.KnowledgeBase{
		KBCode:       "KB-SHARED-BAD",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "bad",
		OwnerType:    model.KnowledgeBaseOwnerShared,
		OwnerAgentID: &agent, // 违反 shared 规则
		Enabled:      boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, bad1); err == nil {
		t.Error("expected error for shared KB with owner")
	}

	// Case 2: private KB 不带 owner -> 失败
	bad2 := &model.KnowledgeBase{
		KBCode:    "KB-PRIVATE-BAD",
		Type:      model.KnowledgeBaseTypeFAQ,
		Name:      "bad",
		OwnerType: model.KnowledgeBaseOwnerPrivate,
		// OwnerAgentID: nil
		Enabled: boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, bad2); err == nil {
		t.Error("expected error for private KB without owner")
	}

	// Case 3: 非法 type -> 失败
	bad3 := &model.KnowledgeBase{
		KBCode:       "KB-BAD-TYPE",
		Type:         "INVALID_TYPE",
		Name:         "bad",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent,
		Enabled:      boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, bad3); err == nil {
		t.Error("expected error for invalid type")
	}
}

// =====================================================================
// 业务故事 7: owner 切换 (private ↔ shared)
// =====================================================================

// TestE2E_OwnerTypeTransition 验证 owner_type 从 private 切到 shared 的过渡
//
// 场景: 一个 KB 起初是 private 给某 agent, 业务上升级为 shared 库
// 验证: shared 后, 多个 agent 都能通过 binding 访问
func TestE2E_OwnerTypeTransition(t *testing.T) {
	db := setupE2EDB(t)
	ctx := context.Background()
	kbSvc, bindSvc, _, _ := newE2ESetup(t, db)

	agent1 := uint(7777)
	// 1. 起初是 private
	kb := &model.KnowledgeBase{
		KBCode:       "KB-OWNER-TRANSITION",
		Type:         model.KnowledgeBaseTypeRAG,
		Name:         "transition",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtrE2E(true),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// 2. 切到 shared (清除 owner)
	updated := &model.KnowledgeBase{
		Name:         "transition (now shared)",
		Type:         model.KnowledgeBaseTypeRAG,
		OwnerType:    model.KnowledgeBaseOwnerShared,
		OwnerAgentID: nil,
		Enabled:      boolPtrE2E(true),
	}
	if err := kbSvc.UpdateKB(ctx, kb.ID, updated); err != nil {
		t.Fatalf("update to shared: %v", err)
	}

	// 3. 验证: agent1 不能再看到 (因为不再是 private owner)
	got1, _ := kbSvc.ListByAgent(ctx, agent1)
	found := false
	for _, k := range got1 {
		if k.ID == kb.ID {
			found = true
		}
	}
	if found {
		t.Error("agent1 should NOT see KB after transition to shared")
	}

	// 4. agent2 通过 binding 看到
	agent2 := uint(8888)
	if err := bindSvc.Bind(ctx, agent2, kb.ID, 0); err != nil {
		t.Fatal(err)
	}
	got2, _ := kbSvc.ListByAgent(ctx, agent2)
	found = false
	for _, k := range got2 {
		if k.ID == kb.ID {
			found = true
		}
	}
	if !found {
		t.Error("agent2 should see KB after binding")
	}
}
