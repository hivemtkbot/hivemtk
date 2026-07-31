package repository

import (
	"context"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupAgentKBBindingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.AgentKBBinding{}, &model.KnowledgeBase{})
	if err := db.Exec("TRUNCATE TABLE agent_kb_bindings RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate agent_kb_bindings: %v", err)
	}
	return db
}

func setupAgentKBBindingRepoWithTX(t *testing.T) (repo *AgentKBBindingRepository, tx *gorm.DB, cleanup func()) {
	t.Helper()
	db := setupAgentKBBindingTestDB(t)
	tx = db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	repo = NewAgentKBBindingRepository(tx)
	cleanup = func() { tx.Rollback() }
	return
}

func newTestBinding(agentID, kbID uint, kbType, role string) *model.AgentKBBinding {
	return &model.AgentKBBinding{
		AgentID:  agentID,
		KBID:     kbID,
		KBType:   kbType,
		Role:     role,
		Priority: 0,
		Enabled:  boolPtr(true),
	}
}

func TestAgentKBBindingRepository_CreateAndCheckExists(t *testing.T) {
	repo, _, done := setupAgentKBBindingRepoWithTX(t)
	defer done()
	ctx := context.Background()

	b := newTestBinding(1, 100, model.KnowledgeBaseTypeFAQ, model.AgentKBBindingRolePrimary)
	if err := repo.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if b.ID == 0 {
		t.Error("expected auto-increment ID")
	}
	exists, err := repo.CheckExists(ctx, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected exists=true")
	}
	// 不存在的组合
	exists, _ = repo.CheckExists(ctx, 1, 999)
	if exists {
		t.Error("expected exists=false for non-existing")
	}
}

func TestAgentKBBindingRepository_DuplicateConflict(t *testing.T) {
	repo, _, done := setupAgentKBBindingRepoWithTX(t)
	defer done()
	ctx := context.Background()

	if err := repo.Create(ctx, newTestBinding(1, 100, "faq", "primary")); err != nil {
		t.Fatal(err)
	}
	// 重复 (agent, kb) 应失败
	err := repo.Create(ctx, newTestBinding(1, 100, "faq", "primary"))
	if err == nil {
		t.Error("expected duplicate error")
	}
}

func TestAgentKBBindingRepository_ListByAgent(t *testing.T) {
	repo, _, done := setupAgentKBBindingRepoWithTX(t)
	defer done()
	ctx := context.Background()

	if err := repo.Create(ctx, newTestBinding(1, 100, "faq", "primary")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestBinding(1, 101, "rag", "primary")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestBinding(1, 102, "sop", "reference")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestBinding(2, 200, "faq", "primary")); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListByAgent(ctx, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 bindings for agent=1, got %d", len(got))
	}

	// 按类型过滤
	faqs, _ := repo.ListByAgent(ctx, 1, "faq")
	if len(faqs) != 1 {
		t.Errorf("expected 1 FAQ binding, got %d", len(faqs))
	}
}

func TestAgentKBBindingRepository_ListByKB(t *testing.T) {
	repo, _, done := setupAgentKBBindingRepoWithTX(t)
	defer done()
	ctx := context.Background()

	if err := repo.Create(ctx, newTestBinding(1, 100, "faq", "primary")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestBinding(2, 100, "faq", "primary")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestBinding(3, 200, "faq", "primary")); err != nil {
		t.Fatal(err)
	}

	got, _ := repo.ListByKB(ctx, 100)
	if len(got) != 2 {
		t.Errorf("expected 2 agents bound to KB=100, got %d", len(got))
	}
}

func TestAgentKBBindingRepository_DeleteByAgentKB(t *testing.T) {
	repo, _, done := setupAgentKBBindingRepoWithTX(t)
	defer done()
	ctx := context.Background()

	if err := repo.Create(ctx, newTestBinding(1, 100, "faq", "primary")); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteByAgentKB(ctx, 1, 100); err != nil {
		t.Fatal(err)
	}
	exists, _ := repo.CheckExists(ctx, 1, 100)
	if exists {
		t.Error("expected deleted")
	}
}

func TestAgentKBBindingRepository_Update(t *testing.T) {
	repo, _, done := setupAgentKBBindingRepoWithTX(t)
	defer done()
	ctx := context.Background()

	b := newTestBinding(1, 100, "faq", "primary")
	if err := repo.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	b.Role = model.AgentKBBindingRoleReference
	b.Priority = 10
	if err := repo.Update(ctx, b.ID, b); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByAgentKB(ctx, 1, 100)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Role != model.AgentKBBindingRoleReference {
		t.Errorf("Role not updated: got %q", got.Role)
	}
	if got.Priority != 10 {
		t.Errorf("Priority not updated: got %d", got.Priority)
	}
}

func TestAgentKBBindingRepository_GetByAgentKB_NotFound(t *testing.T) {
	repo, _, done := setupAgentKBBindingRepoWithTX(t)
	defer done()
	ctx := context.Background()

	got, err := repo.GetByAgentKB(ctx, 999, 999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for not-found")
	}
}
