// Package integration 知识库中间表 CRUD 集成测试 (Task 36)
//
// 目标: 验证 knowledge_bases + agent_kb_bindings 中间表的全生命周期:
//   1. 知识库 CRUD (Create/Get/Update/Delete/List/Count)
//   2. 中间表 CRUD (Bind/Unbind/ListByAgent/ListByKB/BatchBind)
//   3. 业务规则: kb_code 唯一 / (agent, kb) 唯一 / 重复 bind 覆盖 / 删除级联
//   4. UpdateKB 业务校验: type/owner_type 合法性
//
// 依赖: 真实 PostgreSQL (testutil.NewTestDB), 跨包依赖 service + repository + model.
package integration

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
)

// ----------------------------------------------------------------------------
// knowledge_bases CRUD 集成测试
// ----------------------------------------------------------------------------

// TestKBCRUD_FullLifecycle 测试知识库完整生命周期
func TestKBCRUD_FullLifecycle(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	// 1. Create
	agent1 := uint(50)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-FAQ-LIFECYCLE",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "lifecycle test",
		Description:  "原始描述",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatalf("create: %v", err)
	}
	if kb.ID == 0 {
		t.Fatal("expected auto-increment ID")
	}

	// 2. Get
	got, err := kbSvc.GetKB(ctx, kb.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.KBCode != "KB-FAQ-LIFECYCLE" {
		t.Fatalf("get mismatch: %+v", got)
	}

	// 3. Update
	kb.Name = "updated name"
	kb.Description = "更新后描述"
	if err := kbSvc.UpdateKB(ctx, kb.ID, kb); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := kbSvc.GetKB(ctx, kb.ID)
	if got2.Name != "updated name" {
		t.Errorf("update name failed: %q", got2.Name)
	}
	if got2.Description != "更新后描述" {
		t.Errorf("update description failed: %q", got2.Description)
	}

	// 4. List
	listed, total, err := kbSvc.ListKBs(ctx, model.KnowledgeBaseTypeFAQ, "", agent1, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 1 {
		t.Errorf("expected total >= 1, got %d", total)
	}
	found := false
	for _, k := range listed {
		if k.ID == kb.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created KB not found in list")
	}

	// 5. Delete
	if err := kbSvc.DeleteKB(ctx, kb.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got3, _ := kbSvc.GetKB(ctx, kb.ID)
	if got3 != nil {
		t.Error("expected nil after delete")
	}
}

// TestKBCRUD_DuplicateCode 验证 kb_code 唯一约束
func TestKBCRUD_DuplicateCode(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb1 := &model.KnowledgeBase{
		KBCode:       "KB-UNIQUE-001",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "first",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb1); err != nil {
		t.Fatalf("first create: %v", err)
	}

	// 重复 kb_code 应失败
	agent2 := uint(2)
	kb2 := &model.KnowledgeBase{
		KBCode:       "KB-UNIQUE-001", // 同样的 code
		Type:         model.KnowledgeBaseTypeRAG,
		Name:         "second",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent2,
		Enabled:      boolPtr(true),
	}
	err := kbSvc.CreateKB(ctx, kb2)
	if err == nil {
		t.Error("expected duplicate kb_code error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
		!strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Errorf("expected unique violation, got: %v", err)
	}
}

// TestKBCRUD_PrivateRequiresOwner 验证 private 必填 owner_agent_id
func TestKBCRUD_PrivateRequiresOwner(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	// private 但 owner_agent_id 为 nil
	kb1 := &model.KnowledgeBase{
		KBCode:    "KB-PRI-NO-OWNER",
		Type:      model.KnowledgeBaseTypeFAQ,
		Name:      "no owner",
		OwnerType: model.KnowledgeBaseOwnerPrivate,
		// OwnerAgentID: nil
		Enabled: boolPtr(true),
	}
	err := kbSvc.CreateKB(ctx, kb1)
	if err == nil {
		t.Error("expected error for private KB without owner_agent_id")
	}

	// private 但 owner_agent_id = 0
	zero := uint(0)
	kb2 := &model.KnowledgeBase{
		KBCode:       "KB-PRI-OWNER-ZERO",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "owner zero",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &zero,
		Enabled:      boolPtr(true),
	}
	err = kbSvc.CreateKB(ctx, kb2)
	if err == nil {
		t.Error("expected error for private KB with owner_agent_id=0")
	}
}

// TestKBCRUD_SharedRequiresNoOwner 验证 shared 必不能有 owner_agent_id
func TestKBCRUD_SharedRequiresNoOwner(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-SHARED-WITH-OWNER",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "shared with owner",
		OwnerType:    model.KnowledgeBaseOwnerShared,
		OwnerAgentID: &agent1, // 违反 shared 规则
		Enabled:      boolPtr(true),
	}
	err := kbSvc.CreateKB(ctx, kb)
	if err == nil {
		t.Error("expected error for shared KB with owner_agent_id")
	}
}

// TestKBCRUD_InvalidType 验证非法 type 校验
func TestKBCRUD_InvalidType(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-INVALID-TYPE",
		Type:         "INVALID",
		Name:         "invalid",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	err := kbSvc.CreateKB(ctx, kb)
	if err == nil {
		t.Error("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), "type 非法") {
		t.Errorf("expected 'type 非法' error, got: %v", err)
	}
}

// TestKBCRUD_InvalidOwnerType 验证非法 owner_type 校验
func TestKBCRUD_InvalidOwnerType(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-INVALID-OWNER",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "invalid owner",
		OwnerType:    "PUBLIC", // 非法
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	err := kbSvc.CreateKB(ctx, kb)
	if err == nil {
		t.Error("expected error for invalid owner_type")
	}
}

// TestKBCRUD_EmptyName 验证 name 必填
func TestKBCRUD_EmptyName(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-NO-NAME",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	err := kbSvc.CreateKB(ctx, kb)
	if err == nil {
		t.Error("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected 'name' error, got: %v", err)
	}
}

// TestKBCRUD_DefaultEnabled 验证 enabled 默认值
func TestKBCRUD_DefaultEnabled(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-DEFAULT-ENABLED",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "default enabled",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		// Enabled: nil  // 不传, 期望默认 true
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, _ := kbSvc.GetKB(ctx, kb.ID)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Enabled == nil || !*got.Enabled {
		t.Errorf("expected enabled=true by default, got %v", got.Enabled)
	}
}

// TestKBCRUD_CountByAgent 验证 CountByAgent
func TestKBCRUD_CountByAgent(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbRepo := repository.NewKnowledgeBaseRepository(db)

	agent1 := uint(11)
	agent2 := uint(22)

	// agent1 拥有 3 个 private KB
	for i, code := range []string{"K1", "K2", "K3"} {
		kb := &model.KnowledgeBase{
			KBCode:       code,
			Type:         model.KnowledgeBaseTypeFAQ,
			Name:         "kb-" + code,
			OwnerType:    model.KnowledgeBaseOwnerPrivate,
			OwnerAgentID: &agent1,
			Enabled:      boolPtr(true),
		}
		if err := kbRepo.Create(ctx, kb); err != nil {
			t.Fatalf("create %s: %v", code, err)
		}
		_ = i
	}
	// agent2 拥有 1 个
	kb := &model.KnowledgeBase{
		KBCode:       "K4",
		Type:         model.KnowledgeBaseTypeRAG,
		Name:         "agent2 kb",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent2,
		Enabled:      boolPtr(true),
	}
	if err := kbRepo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}

	c1, _ := kbRepo.CountByAgent(ctx, agent1)
	if c1 != 3 {
		t.Errorf("expected agent1 count=3, got %d", c1)
	}
	c2, _ := kbRepo.CountByAgent(ctx, agent2)
	if c2 != 1 {
		t.Errorf("expected agent2 count=1, got %d", c2)
	}
}

// ----------------------------------------------------------------------------
// agent_kb_bindings CRUD 集成测试
// ----------------------------------------------------------------------------

// TestBindingCRUD_BindUnbind 验证 Bind/Unbind 生命周期
func TestBindingCRUD_BindUnbind(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)
	kbRepo := repository.NewKnowledgeBaseRepository(db)

	// 创建 KB
	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-BIND-001",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "bind test",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbRepo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// 1. Bind
	if err := bindSvc.Bind(ctx, agent1, kb.ID, 5); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// 2. List
	bindings, err := bindSvc.ListByAgent(ctx, agent1)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 {
		t.Errorf("expected 1 binding, got %d", len(bindings))
	}
	if len(bindings) > 0 && bindings[0].Priority != 5 {
		t.Errorf("expected priority=5, got %d", bindings[0].Priority)
	}

	// 3. Unbind
	if err := bindSvc.Unbind(ctx, agent1, kb.ID); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	bindings2, _ := bindSvc.ListByAgent(ctx, agent1)
	if len(bindings2) != 0 {
		t.Errorf("expected 0 bindings after unbind, got %d", len(bindings2))
	}
}

// TestBindingCRUD_BatchBind 验证批量绑定 (事务回滚)
func TestBindingCRUD_BatchBind(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)
	kbRepo := repository.NewKnowledgeBaseRepository(db)

	// 创建 3 个 shared KB
	kbIDs := make([]uint, 3)
	for i := 0; i < 3; i++ {
		kb := &model.KnowledgeBase{
			KBCode:    "KB-SHARED-BATCH-" + string(rune('A'+i)),
			Type:      model.KnowledgeBaseTypeFAQ,
			Name:      "batch",
			OwnerType: model.KnowledgeBaseOwnerShared,
			Enabled:   boolPtr(true),
		}
		if err := kbRepo.Create(ctx, kb); err != nil {
			t.Fatal(err)
		}
		kbIDs[i] = kb.ID
	}

	// 1. 正常批量绑定
	items := []service.BatchBindItem{
		{AgentID: 100, KBID: kbIDs[0], Priority: 1},
		{AgentID: 100, KBID: kbIDs[1], Priority: 2},
		{AgentID: 200, KBID: kbIDs[0], Priority: 1},
	}
	if err := bindSvc.BatchBind(ctx, items); err != nil {
		t.Fatalf("batch bind: %v", err)
	}

	// 2. 验证
	got100, _ := bindSvc.ListByAgent(ctx, 100)
	if len(got100) != 2 {
		t.Errorf("expected agent100 has 2 bindings, got %d", len(got100))
	}
	got200, _ := bindSvc.ListByAgent(ctx, 200)
	if len(got200) != 1 {
		t.Errorf("expected agent200 has 1 binding, got %d", len(got200))
	}

	// 3. 批量绑定包含不存在的 KB, 全部回滚
	itemsBad := []service.BatchBindItem{
		{AgentID: 300, KBID: kbIDs[0], Priority: 1},
		{AgentID: 300, KBID: 99999, Priority: 1}, // 不存在
	}
	err := bindSvc.BatchBind(ctx, itemsBad)
	if err == nil {
		t.Error("expected error for non-existent KB")
	}
	// 回滚验证
	got300, _ := bindSvc.ListByAgent(ctx, 300)
	if len(got300) != 0 {
		t.Errorf("expected 0 bindings for agent300 (rollback), got %d", len(got300))
	}
}

// TestBindingCRUD_DuplicateBinding 验证 (agent, kb) 唯一
func TestBindingCRUD_DuplicateBinding(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)
	kbRepo := repository.NewKnowledgeBaseRepository(db)
	bindRepo := repository.NewAgentKBBindingRepository(db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-DUP-BIND",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "dup",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbRepo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// 第一次 Bind OK
	if err := bindSvc.Bind(ctx, agent1, kb.ID, 0); err != nil {
		t.Fatal(err)
	}
	// Service 层 Bind 走先删后建, 不应报错
	if err := bindSvc.Bind(ctx, agent1, kb.ID, 99); err != nil {
		t.Errorf("service-level repeat bind should not error, got: %v", err)
	}
	// 但 Repository 层 Create 应报重复错误 (绕过 service 直接调)
	err := bindRepo.Create(ctx, &model.AgentKBBinding{
		AgentID:  agent1,
		KBID:     kb.ID,
		KBType:   model.KnowledgeBaseTypeFAQ,
		Role:     model.AgentKBBindingRolePrimary,
		Enabled:  boolPtr(true),
	})
	if err == nil {
		t.Error("expected unique violation at repository layer")
	}
}

// TestBindingCRUD_CascadeDeleteOnKBDelete 验证删 KB 级联删 binding
func TestBindingCRUD_CascadeDeleteOnKBDelete(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, bindSvc := newIsolationSetup(t, db)

	// 1. shared KB + 3 个 binding
	kb := &model.KnowledgeBase{
		KBCode:    "KB-CASCADE-TEST",
		Type:      model.KnowledgeBaseTypeSOP,
		Name:      "cascade",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}
	for i := uint(1); i <= 3; i++ {
		if err := bindSvc.Bind(ctx, i, kb.ID, 0); err != nil {
			t.Fatal(err)
		}
	}

	bindRepo := repository.NewAgentKBBindingRepository(db)
	before, _ := bindRepo.ListByKB(ctx, kb.ID)
	if len(before) != 3 {
		t.Fatalf("expected 3 bindings before delete, got %d", len(before))
	}

	// 2. 删 KB
	if err := kbSvc.DeleteKB(ctx, kb.ID); err != nil {
		t.Fatal(err)
	}

	// 3. bindings 应级联删除
	after, _ := bindRepo.ListByKB(ctx, kb.ID)
	if len(after) != 0 {
		t.Errorf("expected 0 bindings after KB delete (cascade), got %d", len(after))
	}
}

// TestBindingCRUD_PrioritySort 验证 priority 排序
func TestBindingCRUD_PrioritySort(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)
	kbRepo := repository.NewKnowledgeBaseRepository(db)
	bindRepo := repository.NewAgentKBBindingRepository(db)

	agent1 := uint(1)
	// 5 个 KB, priority 3/1/5/2/4
	for i, pri := range []int{3, 1, 5, 2, 4} {
		kb := &model.KnowledgeBase{
			KBCode:       "KB-PRI-" + string(rune('A'+i)),
			Type:         model.KnowledgeBaseTypeFAQ,
			Name:         "pri test",
			OwnerType:    model.KnowledgeBaseOwnerPrivate,
			OwnerAgentID: &agent1,
			Enabled:      boolPtr(true),
		}
		if err := kbRepo.Create(ctx, kb); err != nil {
			t.Fatal(err)
		}
		if err := bindSvc.Bind(ctx, agent1, kb.ID, pri); err != nil {
			t.Fatal(err)
		}
	}

	// 验证按 priority DESC 排序
	bindings, _ := bindRepo.ListByAgent(ctx, agent1, "")
	if len(bindings) != 5 {
		t.Fatalf("expected 5, got %d", len(bindings))
	}
	prev := 999
	for _, b := range bindings {
		if b.Priority > prev {
			t.Errorf("sort order broken: %v", bindings)
			break
		}
		prev = b.Priority
	}
	// 第一个应是 priority=5
	if bindings[0].Priority != 5 {
		t.Errorf("expected top priority=5, got %d", bindings[0].Priority)
	}
}

// TestBindingCRUD_DisabledBinding_FilteredOut 验证禁用 binding 不被列出
func TestBindingCRUD_DisabledBinding_FilteredOut(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)
	kbRepo := repository.NewKnowledgeBaseRepository(db)
	bindRepo := repository.NewAgentKBBindingRepository(db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-DIS-BIND",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "disabled bind",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbRepo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}
	if err := bindSvc.Bind(ctx, agent1, kb.ID, 0); err != nil {
		t.Fatal(err)
	}

	// 禁用 binding
	bindings, _ := bindRepo.ListByAgent(ctx, agent1, "")
	if len(bindings) != 1 {
		t.Fatal("setup failed")
	}
	bindings[0].Enabled = boolPtr(false)
	if err := bindRepo.Update(ctx, bindings[0].ID, &bindings[0]); err != nil {
		t.Fatal(err)
	}

	// 重新 List, 应为 0 (默认过滤 enabled=true)
	got, _ := bindRepo.ListByAgent(ctx, agent1, "")
	if len(got) != 0 {
		t.Errorf("expected 0 enabled bindings, got %d", len(got))
	}

	// ListByAgentAll 应能列出 (不过滤 enabled)
	allGot, _ := bindRepo.ListByAgentAll(ctx, agent1)
	if len(allGot) != 1 {
		t.Errorf("expected 1 (including disabled) in ListByAgentAll, got %d", len(allGot))
	}
}

// TestKBCRUD_UpdateKB_TypeChangeValidation 验证 UpdateKB 类型校验
func TestKBCRUD_UpdateKB_TypeChangeValidation(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-UPDATE-TYPE",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "type change",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// 尝试更新 type 为非法值
	update := &model.KnowledgeBase{
		KBCode:       "KB-UPDATE-TYPE",
		Type:         "BOGUS",
		Name:         "type change",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	err := kbSvc.UpdateKB(ctx, kb.ID, update)
	if err == nil {
		t.Error("expected error for invalid type update")
	}
}

// TestKBCRUD_UpdateKB_OwnerTypeChangeToShared 验证 private -> shared
func TestKBCRUD_UpdateKB_OwnerTypeChangeToShared(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-PRI-TO-SHARED",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "promote",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// private -> shared (调用方应把 owner_agent_id 设为 nil, 然后由 service 确认 owner_type=shared 时无 owner)
	update := &model.KnowledgeBase{
		KBCode:       kb.KBCode,
		Name:         "promoted",
		Type:         model.KnowledgeBaseTypeFAQ,
		OwnerType:    model.KnowledgeBaseOwnerShared,
		OwnerAgentID: nil, // 升级到 shared 时必须显式清空
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.UpdateKB(ctx, kb.ID, update); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := kbSvc.GetKB(ctx, kb.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("got is nil after update")
	}
	if got.OwnerType != model.KnowledgeBaseOwnerShared {
		t.Errorf("expected shared, got %q", got.OwnerType)
	}
	if got.OwnerAgentID != nil {
		t.Errorf("expected owner_agent_id=nil after shared update, got %v", *got.OwnerAgentID)
	}
}

// TestKBCRUD_UpdateKB_SharedToPrivateWithOwner 验证 shared -> private 转换
func TestKBCRUD_UpdateKB_SharedToPrivateWithOwner(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	// 1. 创建 shared KB
	kb := &model.KnowledgeBase{
		KBCode:    "KB-SHARED-TO-PRIV",
		Type:      model.KnowledgeBaseTypeFAQ,
		Name:      "demote",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtr(true),
	}
	if err := kbSvc.CreateKB(ctx, kb); err != nil {
		t.Fatal(err)
	}

	// 2. shared -> private (需 owner_agent_id)
	agent1 := uint(99)
	update := &model.KnowledgeBase{
		KBCode:       kb.KBCode, // 必填, 保留原值
		Name:         "demoted",
		Type:         model.KnowledgeBaseTypeFAQ,
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbSvc.UpdateKB(ctx, kb.ID, update); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := kbSvc.GetKB(ctx, kb.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("got is nil after update")
	}
	if got.OwnerType != model.KnowledgeBaseOwnerPrivate {
		t.Errorf("expected private, got %q", got.OwnerType)
	}
	if got.OwnerAgentID == nil || *got.OwnerAgentID != agent1 {
		t.Errorf("expected owner_agent_id=%d, got %v", agent1, got.OwnerAgentID)
	}
}

// TestKBCRUD_ListByType_All 验证 ListByType 列出全部
func TestKBCRUD_ListByType_All(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	kbSvc, _ := newIsolationSetup(t, db)

	// 2 个 FAQ, 1 个 RAG, 1 个 SOP
	for i, t1 := range []string{
		model.KnowledgeBaseTypeFAQ, model.KnowledgeBaseTypeFAQ,
		model.KnowledgeBaseTypeRAG, model.KnowledgeBaseTypeSOP,
	} {
		agent1 := uint(1)
		_ = i
		kb := &model.KnowledgeBase{
			KBCode:       "KB-TYPE-" + t1 + string(rune('A'+i)),
			Type:         t1,
			Name:         t1,
			OwnerType:    model.KnowledgeBaseOwnerShared,
			Enabled:      boolPtr(true),
			OwnerAgentID: nil, // shared
		}
		_ = agent1
		if err := kbSvc.CreateKB(ctx, kb); err != nil {
			t.Fatal(err)
		}
	}

	faqs, _ := kbSvc.ListByType(ctx, model.KnowledgeBaseTypeFAQ)
	if len(faqs) != 2 {
		t.Errorf("expected 2 FAQ, got %d", len(faqs))
	}
	rags, _ := kbSvc.ListByType(ctx, model.KnowledgeBaseTypeRAG)
	if len(rags) != 1 {
		t.Errorf("expected 1 RAG, got %d", len(rags))
	}
}

// TestBindingCRUD_BindNonExistentKB 验证绑定不存在的 KB 应失败
func TestBindingCRUD_BindNonExistentKB(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)

	err := bindSvc.Bind(ctx, 1, 99999, 0)
	if err == nil {
		t.Error("expected error for non-existent KB")
	}
	if !strings.Contains(err.Error(), "知识库不存在") {
		t.Errorf("expected '知识库不存在' error, got: %v", err)
	}
}

// TestBindingCRUD_EmptyAgentID 验证 agent_id=0 应失败
func TestBindingCRUD_EmptyAgentID(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)
	kbRepo := repository.NewKnowledgeBaseRepository(db)

	agent1 := uint(1)
	kb := &model.KnowledgeBase{
		KBCode:       "KB-EMPTY-AGENT",
		Type:         model.KnowledgeBaseTypeFAQ,
		Name:         "empty agent",
		OwnerType:    model.KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &agent1,
		Enabled:      boolPtr(true),
	}
	if err := kbRepo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}

	err := bindSvc.Bind(ctx, 0, kb.ID, 0)
	if err == nil {
		t.Error("expected error for agent_id=0")
	}
}

// TestBindingCRUD_BatchBind_EmptyItems 验证空 items 不报错
func TestBindingCRUD_BatchBind_EmptyItems(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)

	if err := bindSvc.BatchBind(ctx, nil); err != nil {
		t.Errorf("expected nil error for empty items, got: %v", err)
	}
	if err := bindSvc.BatchBind(ctx, []service.BatchBindItem{}); err != nil {
		t.Errorf("expected nil error for empty items, got: %v", err)
	}
}

// TestBindingCRUD_ListByKB 验证 ListByKB
func TestBindingCRUD_ListByKB(t *testing.T) {
	db := setupIsolationDB(t)
	ctx := context.Background()
	_, bindSvc := newIsolationSetup(t, db)
	kbRepo := repository.NewKnowledgeBaseRepository(db)

	// 1 个 shared KB, 4 个智能体 binding
	kb := &model.KnowledgeBase{
		KBCode:    "KB-LIST-BY-KB",
		Type:      model.KnowledgeBaseTypeFAQ,
		Name:      "list by kb",
		OwnerType: model.KnowledgeBaseOwnerShared,
		Enabled:   boolPtr(true),
	}
	if err := kbRepo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}
	for i := uint(1); i <= 4; i++ {
		if err := bindSvc.Bind(ctx, i, kb.ID, int(i)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := bindSvc.ListByKB(ctx, kb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Errorf("expected 4 bindings, got %d", len(got))
	}
	// priority 排序: 4 优先
	if got[0].Priority != 4 {
		t.Errorf("expected top priority=4, got %d", got[0].Priority)
	}
}
